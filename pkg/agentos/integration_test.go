package agentos

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/api"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/infra/db"
	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/workflow"
	_ "modernc.org/sqlite"
)

func loadManifest(t *testing.T, name string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.ParseFile("../../examples/manifests/" + name)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", name, err)
	}
	return m
}

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	dbConn, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	dbConn.SetMaxOpenConns(1)
	dbConn.SetMaxIdleConns(1)

	if err := dbConn.Ping(); err != nil {
		dbConn.Close()
		t.Fatalf("Ping: %v", err)
	}

	cleanup := func() {
		dbConn.SetMaxOpenConns(0)
		dbConn.SetMaxIdleConns(0)
		dbConn.Close()
		os.Remove(dbPath)
	}

	return dbConn, cleanup
}

func TestIntegration_CafeteriaLoyalty_FullPipeline(t *testing.T) {
	m := loadManifest(t, "cafeteria-loyalty.yaml")

	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(1000)
	notifyBus := NewNotificationBus(500)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	parser := &manifest.Parser{}
	if err := parser.Validate(m); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	engine, err := policy.NewEngine(m)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	fsmCfg := workflow.FSMConfig{
		EntityName:   "Transaction",
		InitialState: workflow.State("open"),
		States: map[workflow.State]workflow.StateConfig{
			"open": {
				Transitions: []workflow.Transition{
					{To: "completed", Action: "complete", AllowedRoles: []string{"staff", "admin"}},
					{To: "cancelled", Action: "cancel", AllowedRoles: []string{"admin"}},
				},
			},
			"completed": {
				Transitions: []workflow.Transition{
					{To: "open", Action: "reopen", AllowedRoles: []string{"admin"}},
				},
			},
			"cancelled": {},
		},
	}
	if err := hook.RegisterFSM("Transaction", fsmCfg); err != nil {
		t.Fatalf("RegisterFSM: %v", err)
	}

	{
		ctx := &policy.Context{
			ActorID:    "customer",
			Resource:   "loyalty_account",
			Action:     "read",
			Attributes: map[string]interface{}{"owner": "customer"},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if !result.Allowed {
			t.Error("customer should read own loyalty_account")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "barista",
			Resource:   "loyalty_account",
			Action:     "add_points",
			Attributes: map[string]interface{}{"amount": float64(50)},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if !result.Allowed {
			t.Error("barista should add_points <= 100")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "barista",
			Resource:   "loyalty_account",
			Action:     "add_points",
			Attributes: map[string]interface{}{"amount": float64(500)},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if result.Allowed {
			t.Error("barista should NOT add_points > 100")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "admin",
			Resource:   "*",
			Action:     "delete",
			Attributes: map[string]interface{}{},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if !result.Allowed {
			t.Error("admin should have wildcard permissions")
		}
	}

	{
		req := &agent.LLMHookRequest{
			Context: &agent.TurnContext{
				Inbound: &bus.InboundContext{SenderID: "manager"},
			},
			Messages: []providers.Message{{Role: "user", Content: "Generate my report"}},
		}
		_, decision, err := hook.BeforeLLM(context.Background(), req)
		if err != nil {
			t.Fatalf("BeforeLLM: %v", err)
		}
		if decision.Action != agent.HookActionContinue {
			t.Errorf("BeforeLLM decision: want continue, got %s", decision.Action)
		}
		if len(req.Messages) < 2 {
			t.Error("BeforeLLM should prepend context block")
		}
		if !strings.Contains(req.Messages[0].Content, "manager") {
			t.Error("Context block should mention actor 'manager'")
		}
		if !strings.Contains(req.Messages[0].Content, "Customer") {
			t.Error("Context block should list available entities")
		}
	}

	{
		call := &agent.ToolCallHookRequest{
			Tool:      "write",
			Arguments: map[string]any{"target": "/reports/sales"},
			Context:   &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "admin"}},
		}
		_, decision, err := hook.BeforeTool(context.Background(), call)
		if err != nil {
			t.Fatalf("BeforeTool(write for admin): %v", err)
		}
		if decision.Action != agent.HookActionContinue {
			t.Errorf("BeforeTool(write for admin): want continue, got %s", decision.Action)
		}
	}

	{
		call := &agent.ToolCallHookRequest{
			Tool:      "bash",
			Arguments: map[string]any{"command": "rm -rf /"},
			Context:   &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "customer"}},
		}
		_, decision, err := hook.BeforeTool(context.Background(), call)
		if err != nil {
			t.Fatalf("BeforeTool(bash for customer): %v", err)
		}
		if decision.Action != agent.HookActionDenyTool {
			t.Error("customer should be denied bash tool")
		}
	}

	{
		call := &agent.ToolCallHookRequest{
			Tool:      "bash",
			Arguments: map[string]any{"command": "ls -la"},
			Context:   &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "admin"}},
		}
		_, decision, err := hook.BeforeTool(context.Background(), call)
		if err != nil {
			t.Fatalf("BeforeTool(bash for admin): %v", err)
		}
		if decision.Action != agent.HookActionContinue {
			t.Errorf("BeforeTool(bash for admin): want continue, got %s", decision.Action)
		}
	}

	{
		call := &agent.ToolCallHookRequest{
			Tool:      "write",
			Arguments: map[string]any{"target": "/data/export"},
			Context:   &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "manager"}},
		}
		_, decision, err := hook.BeforeTool(context.Background(), call)
		if err != nil {
			t.Fatalf("BeforeTool(write for manager): %v", err)
		}
		if decision.Action != agent.HookActionDenyTool {
			t.Errorf("manager without tool permission should be denied, got %s", decision.Action)
		}
	}

	{
		call := &agent.ToolCallHookRequest{
			Tool:      "task_update",
			Arguments: map[string]any{"entity_id": "tx-001", "entity_type": "Transaction"},
			Context:   &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "admin"}},
		}
		_, _, err := hook.BeforeTool(context.Background(), call)
		if err != nil {
			t.Fatalf("BeforeTool(task_update): %v", err)
		}
	}

	{
		payload := TurnEndPayload{
			Status:      agent.TurnEndStatusCompleted,
			UserMessage: "points added successfully",
			ToolCount:   3,
			Iteration:   1,
			Context:     &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "barista"}},
		}
		err := hook.AfterTurn(context.Background(), payload)
		if err != nil {
			t.Fatalf("AfterTurn: %v", err)
		}
	}

	entries, err := auditLog.Query(AuditFilter{Limit: 100})
	if err != nil {
		t.Fatalf("auditLog.Query: %v", err)
	}
	if len(entries) == 0 {
		t.Error("auditLog should have entries after tool calls and turns")
	}

	{
		notifyBus.Notify(Notification{
			FromActor: "admin",
			ToActor:   "manager",
			Type:      "test",
			Title:     "Test Notification",
			Body:      "Integration test",
		})
		notifs := hook.GetNotifications("manager")
		if len(notifs) != 1 {
			t.Errorf("manager should have 1 notification, got %d", len(notifs))
		}
	}

	{
		openAPISpec := &api.OpenAPISpec{}
		gen := &api.Generator{}
		_ = gen

		apiGen, err := api.NewGenerator(m)
		if err != nil {
			t.Fatalf("NewGenerator: %v", err)
		}

		spec := apiGen.GenerateOpenAPI()
		if spec.OpenAPI != "3.0.3" {
			t.Errorf("OpenAPI version: want 3.0.3, got %s", spec.OpenAPI)
		}
		if len(spec.Paths) == 0 {
			t.Error("OpenAPI spec should have paths")
		}
		if len(spec.Components.Schemas) == 0 {
			t.Error("OpenAPI spec should have schemas")
		}
		_ = openAPISpec
	}

	{
		state, err := hook.GetWorkflowState("Transaction", "tx-001")
		if err != nil {
			t.Fatalf("GetWorkflowState: %v", err)
		}
		if state == nil {
			t.Log("workflow state may be nil for new entity (expected)")
		}
	}

	data, err := hook.ExportContextJSON()
	if err != nil {
		t.Fatalf("ExportContextJSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("exported context should not be empty")
	}
}

func TestIntegration_ParkingTicket_MigrationAndCRUD(t *testing.T) {
	m := loadManifest(t, "parking-ticket.yaml")

	dbConn, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := db.NewMigrator(db.NewSQLDB(dbConn), m)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	parser := &manifest.Parser{}
	if err := parser.Validate(m); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	engine, err := policy.NewEngine(m)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	{
		ctx := &policy.Context{
			ActorID:    "driver",
			Resource:   "own_tickets",
			Action:     "read",
			Attributes: map[string]interface{}{"owner": "driver"},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if !result.Allowed {
			t.Error("driver should read own_tickets")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "driver",
			Resource:   "parking_rates",
			Action:     "read",
			Attributes: map[string]interface{}{},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if !result.Allowed {
			t.Error("driver should read parking_rates")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "driver",
			Resource:   "tickets",
			Action:     "create",
			Attributes: map[string]interface{}{"shift_active": true},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if result.Allowed {
			t.Error("driver should NOT create tickets (role limitation)")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "attendant",
			Resource:   "tickets",
			Action:     "create",
			Attributes: map[string]interface{}{"shift_active": true},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if !result.Allowed {
			t.Error("attendant with active shift should create tickets")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "attendant",
			Resource:   "tickets",
			Action:     "create",
			Attributes: map[string]interface{}{"shift_active": false},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if result.Allowed {
			t.Error("attendant WITHOUT active shift should NOT create tickets")
		}
	}

	apiGen, err := api.NewGeneratorWithDB(m, dbConn)
	if err != nil {
		t.Fatalf("NewGeneratorWithDB: %v", err)
	}

	mux, err := apiGen.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	srv := httptest.NewServer(mux)
	defer srv.Close()

	{
		resp, err := http.Get(srv.URL + "/_health")
		if err != nil {
			t.Fatalf("GET /_health: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("/_health: want 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	{
		resp, err := http.Get(srv.URL + "/_manifest")
		if err != nil {
			t.Fatalf("GET /_manifest: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("/_manifest: want 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	{
		resp, err := http.Get(srv.URL + "/_openapi.json")
		if err != nil {
			t.Fatalf("GET /_openapi.json: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("/_openapi.json: want 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	{
		resp, err := http.Get(srv.URL + "/_docs")
		if err != nil {
			t.Fatalf("GET /_docs: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("/_docs: want 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	{
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/parking/tickets", strings.NewReader(`{"vehicle_id":"ABC-1234","entry_time":"2025-01-15T10:00:00Z"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Actor-ID", "attendant")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /tickets: %v", err)
		}
		if resp.StatusCode == http.StatusMethodNotAllowed {
			t.Error("endpoint pattern not registered (check BuildMux routing)")
		}
		resp.Body.Close()
	}

	{
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/parking/tickets", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Actor-ID", "driver")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /tickets (driver): %v", err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("driver should be forbidden to create tickets, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	{
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/parking/rates", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /rates: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusMethodNotAllowed {
			t.Error("parking rates endpoint not registered in mux")
		} else {
			t.Logf("GET /rates: status %d (endpoint registered)", resp.StatusCode)
		}
	}
}

func TestIntegration_TaskManagement_RBACAndWorkflow(t *testing.T) {
	m := loadManifest(t, "task-management.yaml")

	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(500)
	notifyBus := NewNotificationBus(200)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	parser := &manifest.Parser{}
	if err := parser.Validate(m); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	fsmCfg := workflow.FSMConfig{
		EntityName:   "Task",
		InitialState: workflow.State("todo"),
		States: map[workflow.State]workflow.StateConfig{
			"todo": {
				Transitions: []workflow.Transition{
					{To: "in_progress", Action: "start", AllowedRoles: []string{"member", "manager", "admin"}},
					{To: "done", Action: "complete", AllowedRoles: []string{"member", "manager", "admin"}},
				},
			},
			"in_progress": {
				Transitions: []workflow.Transition{
					{To: "done", Action: "complete", AllowedRoles: []string{"member", "manager", "admin"}},
					{To: "todo", Action: "pause", AllowedRoles: []string{"manager", "admin"}},
				},
			},
			"done": {
				Transitions: []workflow.Transition{
					{To: "todo", Action: "reopen", AllowedRoles: []string{"manager", "admin"}},
				},
			},
		},
	}
	if err := hook.RegisterFSM("Task", fsmCfg); err != nil {
		t.Fatalf("RegisterFSM: %v", err)
	}

	engine, err := policy.NewEngine(m)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	{
		ctx := &policy.Context{
			ActorID:    "member",
			Resource:   "tasks",
			Action:     "write",
			Attributes: map[string]interface{}{"owner": "member"},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if !result.Allowed {
			t.Error("member should write own tasks")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "member",
			Resource:   "tasks",
			Action:     "write",
			Attributes: map[string]interface{}{"owner": "other"},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if result.Allowed {
			t.Error("member should NOT write tasks owned by others")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "viewer",
			Resource:   "tasks",
			Action:     "write",
			Attributes: map[string]interface{}{},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if result.Allowed {
			t.Error("viewer should NOT write tasks")
		}
	}

	{
		ctx := &policy.Context{
			ActorID:    "admin",
			Resource:   "*",
			Action:     "*",
			Attributes: map[string]interface{}{},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(ctx)
		if !result.Allowed {
			t.Error("admin should have wildcard permissions")
		}
	}

	{
		roles := engine.GetAllRoles("member")
		if len(roles) == 0 || !containsStr(roles, "member") {
			t.Errorf("member should have 'member' role, got %v", roles)
		}
	}

	{
		roles := engine.GetAllRoles("admin")
		hasAdmin := false
		for _, r := range roles {
			if r == "admin" {
				hasAdmin = true
				break
			}
		}
		if !hasAdmin {
			t.Errorf("admin should include admin role (via inheritance), got %v", roles)
		}
	}

	{
		fsm, err := workflow.NewFSM(fsmCfg)
		if err != nil {
			t.Fatalf("NewFSM: %v", err)
		}

		allowed, _, err := fsm.CanTransition(workflow.State("todo"), workflow.Action("start"), []string{"member"})
		if err != nil {
			t.Fatalf("CanTransition: %v", err)
		}
		if !allowed {
			t.Error("member should be able to start a todo task")
		}

		fsm.Transition([]string{"member"}, workflow.Action("start"))

		allowed, _, err = fsm.CanTransition(workflow.State("in_progress"), workflow.Action("complete"), []string{"member"})
		if err != nil {
			t.Fatalf("CanTransition: %v", err)
		}
		if !allowed {
			t.Error("member should be able to complete an in_progress task")
		}
	}

	{
		fsm, err := workflow.NewFSM(fsmCfg)
		if err != nil {
			t.Fatalf("NewFSM: %v", err)
		}

		allowed, _, err := fsm.CanTransition(workflow.State("todo"), workflow.Action("start"), []string{"viewer"})
		if err != nil {
			if allowed {
				t.Error("viewer should NOT be able to start tasks (got error but was allowed)")
			} else {
				t.Logf("viewer correctly denied (err=%v)", err)
			}
		} else if allowed {
			t.Error("viewer should NOT be able to start tasks")
		} else {
			t.Log("viewer correctly denied without error")
		}
	}

	{
		fsm, err := workflow.NewFSM(fsmCfg)
		if err != nil {
			t.Fatalf("NewFSM: %v", err)
		}

		allowed, _, err := fsm.CanTransition(workflow.State("todo"), workflow.Action("complete"), []string{"member"})
		if err != nil {
			t.Fatalf("CanTransition: %v", err)
		}
		if !allowed {
			t.Error("member should be able to directly complete a todo task")
		}
	}

	{
		call := &agent.ToolCallHookRequest{
			Tool:      "read",
			Arguments: map[string]any{"entity_id": "task-100", "entity_type": "Task"},
			Context:   &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "member"}},
		}
		_, decision, err := hook.BeforeTool(context.Background(), call)
		if err != nil {
			t.Fatalf("BeforeTool(read for member): %v", err)
		}
		if decision.Action != agent.HookActionContinue {
			t.Logf("member tool call decision: %s (member has read on tasks in manifest)", decision.Action)
		}
	}

	{
		call := &agent.ToolCallHookRequest{
			Tool:      "task_delete",
			Arguments: map[string]any{"entity_id": "task-100"},
			Context:   &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "member"}},
		}
		_, decision, err := hook.BeforeTool(context.Background(), call)
		if err != nil {
			t.Fatalf("BeforeTool(task_delete for member): %v", err)
		}
		if decision.Action != agent.HookActionDenyTool {
			t.Errorf("member should be denied task_delete, got %s", decision.Action)
		}
	}

	{
		states := []string{}
		for entityType := range map[string]*workflow.FSM{"Task": nil} {
			_ = entityType
		}
		fsm, _ := workflow.NewFSM(fsmCfg)
		listStates := fsm.ListStates()
		states = append(states, listStates...)
		if len(states) == 0 {
			t.Error("FSM should list states")
		}
	}

	{
		entries, err := auditLog.Query(AuditFilter{Limit: 100})
		if err != nil {
			t.Fatalf("auditLog.Query: %v", err)
		}
		if len(entries) == 0 {
			t.Error("auditLog should have entries after tool calls")
		}
	}

	{
		payload := TurnEndPayload{
			Status:      agent.TurnEndStatusCompleted,
			UserMessage: "task created and workflow triggered",
			ToolCount:   2,
			Iteration:   1,
			Context:     &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "member"}},
		}
		err := hook.AfterTurn(context.Background(), payload)
		if err != nil {
			t.Fatalf("AfterTurn: %v", err)
		}
	}

	{
		entries, err := auditLog.Query(AuditFilter{ActorID: "member", Limit: 50})
		if err != nil {
			t.Fatalf("auditLog.Query(member): %v", err)
		}
		found := false
		for _, e := range entries {
			if e.Action == "turn_end" {
				found = true
				break
			}
		}
		if !found {
			t.Error("turn_end audit entry for member not found")
		}
	}
}

func TestIntegration_OpenAPI_ParkingTicketSpec(t *testing.T) {
	m := loadManifest(t, "parking-ticket.yaml")

	apiGen, err := api.NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	spec := apiGen.GenerateOpenAPI()

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("OpenAPI: want 3.0.3, got %s", spec.OpenAPI)
	}

	if spec.Info.Title != "parking-ticket-system" {
		t.Errorf("Info.Title: want parking-ticket-system, got %s", spec.Info.Title)
	}

	if len(spec.Paths) == 0 {
		t.Error("spec.Paths should not be empty")
	}

	for _, apiConfig := range m.Integrations.APIs {
		for _, ep := range apiConfig.Endpoints {
			path := strings.TrimSuffix(apiConfig.BasePath, "/") + "/" + strings.TrimPrefix(ep.Path, "/")
			path = strings.ReplaceAll(path, "{id}", "{id}")

			pathObj, ok := spec.Paths[path]
			if !ok {
				t.Errorf("Path %s not found in OpenAPI spec", path)
				continue
			}

			method := strings.ToLower(ep.Method)
			if _, ok := pathObj[method]; !ok {
				t.Errorf("Method %s for path %s not found in spec", method, path)
			}
		}
	}

	for entityName, schema := range spec.Components.Schemas {
		if schema.Type != "object" {
			t.Errorf("Schema %s: want type object, got %s", entityName, schema.Type)
		}
		if len(schema.Properties) == 0 {
			t.Errorf("Schema %s has no properties", entityName)
		}
	}

	if _, ok := spec.Components.SecuritySchemes["bearerAuth"]; !ok {
		t.Error("JWT security scheme not generated")
	}
}

func TestIntegration_OpenAPI_CafeteriaSpec(t *testing.T) {
	m := loadManifest(t, "cafeteria-loyalty.yaml")

	apiGen, err := api.NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	spec := apiGen.GenerateOpenAPI()

	if spec.Info.Version != "1.0.0" {
		t.Errorf("Info.Version: want 1.0.0, got %s", spec.Info.Version)
	}

	if len(spec.Components.Schemas) < 5 {
		t.Errorf("should have at least 5 entity schemas, got %d", len(spec.Components.Schemas))
	}

	for _, apiConfig := range m.Integrations.APIs {
		for _, ep := range apiConfig.Endpoints {
			if len(ep.Permissions) > 0 {
				path := strings.TrimSuffix(apiConfig.BasePath, "/") + "/" + strings.TrimPrefix(ep.Path, "/")
				path = strings.ReplaceAll(path, "{id}", "{id}")

				pathObj, ok := spec.Paths[path]
				if ok {
					method := strings.ToLower(ep.Method)
					if pathObj[method].Security == nil {
						t.Errorf("endpoint %s %s has permissions but no security in OpenAPI spec", method, path)
					}
				}
			}
		}
	}
}

func TestIntegration_AuditLog_FileBased(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.jsonl")

	log, err := NewFileAuditLog(logPath)
	if err != nil {
		t.Fatalf("NewFileAuditLog: %v", err)
	}

	for i := 0; i < 10; i++ {
		entry := AuditEntry{
			ActorID:  "user-" + string(rune('0'+i)),
			Action:   "test_action",
			Resource: "test_resource",
			Allowed:  i%2 == 0,
			Details:  map[string]interface{}{"index": i},
		}
		if err := log.Record(entry); err != nil {
			t.Fatalf("Record[%d]: %v", i, err)
		}
	}

	entries, err := log.Query(AuditFilter{Limit: 20})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 10 {
		t.Errorf("Query: want 10 entries, got %d", len(entries))
	}

	entries2, err := log.Query(AuditFilter{ActorID: "user-2", Limit: 5})
	if err != nil {
		t.Fatalf("Query(actor=user-2): %v", err)
	}
	if len(entries2) != 1 {
		t.Errorf("Query(actor=user-2): want 1 entry, got %d", len(entries2))
	}

	entries3, err := log.Query(AuditFilter{Limit: 3})
	if err != nil {
		t.Fatalf("Query(limit=3): %v", err)
	}
	if len(entries3) != 3 {
		t.Errorf("Query(limit=3): want 3 entries, got %d", len(entries3))
	}
}

func TestIntegration_WorkflowStore_FileBased(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "workflows.json")

	store, err := NewFileWorkflowStore(storePath)
	if err != nil {
		t.Fatalf("NewFileWorkflowStore: %v", err)
	}

	states := []*WorkflowState{
		{EntityID: "task-1", EntityType: "Task", CurrentState: "todo", UpdatedBy: "user1"},
		{EntityID: "task-2", EntityType: "Task", CurrentState: "done", UpdatedBy: "user2"},
		{EntityID: "tx-1", EntityType: "Transaction", CurrentState: "open", UpdatedBy: "admin"},
	}

	for _, s := range states {
		if err := store.Set(s); err != nil {
			t.Fatalf("Set(%s): %v", s.EntityID, err)
		}
	}

	retrieved, err := store.Get("Task", "task-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Get returned nil")
	}
	if retrieved.CurrentState != "todo" {
		t.Errorf("CurrentState: want todo, got %s", retrieved.CurrentState)
	}

	list, err := store.List("Task")
	if err != nil {
		t.Fatalf("List(Task): %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List(Task): want 2, got %d", len(list))
	}
}

func TestIntegration_BusinessRules_TriggerChains(t *testing.T) {
	m := loadManifest(t, "cafeteria-loyalty.yaml")

	ruleExecutor := api.NewRuleExecutor(m)

	data := map[string]interface{}{
		"type":        "purchase",
		"customer_id": "cust-001",
		"amount":      150.0,
	}

	err := ruleExecutor.ExecuteAfter(context.Background(), "create", "Transaction", data)
	if err != nil {
		t.Fatalf("ExecuteAfter(create, Transaction): %v", err)
	}

	data2 := map[string]interface{}{
		"type":   "purchase",
		"amount": 200.0,
	}

	err = ruleExecutor.ExecuteBefore(context.Background(), "create", "Transaction", data2)
	if err != nil {
		t.Fatalf("ExecuteBefore(create, Transaction): %v", err)
	}

	data3 := map[string]interface{}{
		"status":           "completed",
		"customer_id":      "cust-001",
		"points_to_redeem": 1000,
		"current_points":   500,
	}

	err = ruleExecutor.ExecuteAfter(context.Background(), "create", "Redemption", data3)
	if err != nil && !strings.Contains(err.Error(), "rejected") {
		t.Errorf("ExecuteAfter(create, Redemption) unexpected error: %v", err)
	}
}

func TestIntegration_NotificationBus_MultiActor(t *testing.T) {
	bus := NewNotificationBus(100)

	bus.Notify(Notification{
		FromActor: "admin",
		ToActor:   "manager",
		Type:      "workflow_complete",
		Title:     "Task Completed",
		Body:      "Task task-42 has been completed",
		Data:      map[string]interface{}{"task_id": "task-42"},
	})

	bus.Notify(Notification{
		FromActor: "manager",
		ToActor:   "member",
		Type:      "assignment",
		Title:     "New Task Assigned",
		Body:      "You have been assigned task-43",
		Data:      map[string]interface{}{"task_id": "task-43"},
	})

	bus.Notify(Notification{
		FromActor: "system",
		ToActor:   "*",
		Type:      "system_alert",
		Title:     "System Maintenance",
		Body:      "Maintenance scheduled for tonight",
	})

	managerNotifs := bus.GetUnread("manager")
	if len(managerNotifs) != 1 {
		t.Errorf("manager should have 1 notification, got %d", len(managerNotifs))
	}
	if managerNotifs[0].FromActor != "admin" {
		t.Errorf("notification from should be admin, got %s", managerNotifs[0].FromActor)
	}

	memberNotifs := bus.GetUnread("member")
	if len(memberNotifs) != 1 {
		t.Errorf("member should have 1 direct notification, got %d", len(memberNotifs))
	}
	if memberNotifs[0].FromActor != "manager" {
		t.Errorf("direct notification from should be manager, got %s", memberNotifs[0].FromActor)
	}

	broadcastNotifs := bus.GetLog()
	var broadcastCount int
	for _, n := range broadcastNotifs {
		if n.ToActor == "*" {
			broadcastCount++
		}
	}
	if broadcastCount != 1 {
		t.Errorf("should have 1 broadcast notification, got %d", broadcastCount)
	}

	bus.MarkRead(managerNotifs[0].ID)
	afterMark := bus.GetUnread("manager")
	if len(afterMark) != 0 {
		t.Errorf("after marking read, manager should have 0 unread, got %d", len(afterMark))
	}

	allNotifs := bus.GetLog()
	if len(allNotifs) != 3 {
		t.Errorf("GetLog should return all 3 notifications, got %d", len(allNotifs))
	}
}

func TestIntegration_Manifest_SerializationRoundTrip(t *testing.T) {
	m := loadManifest(t, "cafeteria-loyalty.yaml")

	data, err := m.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}

	var m2 manifest.Manifest
	if err := json.Unmarshal(data, &m2); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if m2.Metadata.Name != m.Metadata.Name {
		t.Errorf("Name: want %s, got %s", m.Metadata.Name, m2.Metadata.Name)
	}

	if len(m2.Actors) != len(m.Actors) {
		t.Errorf("Actors count: want %d, got %d", len(m.Actors), len(m2.Actors))
	}

	if len(m2.DataModel.Entities) != len(m.DataModel.Entities) {
		t.Errorf("Entities count: want %d, got %d", len(m.DataModel.Entities), len(m2.DataModel.Entities))
	}
}

func containsStr(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}
