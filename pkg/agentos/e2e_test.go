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
	"github.com/sipeed/picoclaw/pkg/agentos/evolution"
	"github.com/sipeed/picoclaw/pkg/agentos/stateful"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/infra/db"
	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/workflow"

	_ "modernc.org/sqlite"
)

func loadTestManifest(t *testing.T, name string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.ParseFile("../../examples/manifests/" + name)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", name, err)
	}
	return m
}

func setupE2EDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "e2e_test.db")

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
	}

	return dbConn, cleanup
}

func TestE2E_CafeteriaLoyalty_FullSystemBootstrap(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")

	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	bootstrapper := NewBootstrapper(BootstrapConfig{
		Manifest: m,
		DBDriver: "sqlite",
	})

	instance, err := bootstrapper.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer instance.Shutdown(ctx)

	t.Run("ManifestLoaded", func(t *testing.T) {
		if instance.Manifest == nil {
			t.Fatal("Manifest should be loaded")
		}
		if instance.Manifest.Metadata.Name != "cafeteria-loyalty-system" {
			t.Errorf("Manifest name: want cafeteria-loyalty-system, got %s", instance.Manifest.Metadata.Name)
		}
	})

	t.Run("DatabaseMigrated", func(t *testing.T) {
		var count int
		err := dbConn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
		if err != nil {
			t.Fatalf("Query tables: %v", err)
		}
		if count < 5 {
			t.Errorf("Expected at least 5 tables, got %d", count)
		}

		for _, entity := range m.DataModel.Entities {
			var tableName string
			err := dbConn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", strings.ToLower(entity.Name)).Scan(&tableName)
			if err == sql.ErrNoRows {
				t.Errorf("Table %s not created", entity.Name)
			}
		}
	})

	t.Run("ActorsProvisioned", func(t *testing.T) {
		actors, err := instance.ActorStore.ListAll()
		if err != nil {
			t.Fatalf("ListAll actors: %v", err)
		}
		if len(actors) < 4 {
			t.Errorf("Expected at least 4 actors, got %d", len(actors))
		}

		for _, actor := range m.Actors {
			cred, err := instance.ActorStore.GetByID(actor.ID)
			if err != nil {
				t.Errorf("GetByID actor %s: %v", actor.ID, err)
				continue
			}
			if cred == nil {
				t.Errorf("Actor %s not provisioned", actor.ID)
				continue
			}
			if cred.APIKey == "" {
				t.Errorf("Actor %s has no API key", actor.ID)
			}
		}
	})

	t.Run("OperationCatalogBuilt", func(t *testing.T) {
		ops := instance.Catalog.ListAll()
		if len(ops) == 0 {
			t.Error("OperationCatalog should have operations")
		}

		var crudOps int
		for _, op := range ops {
			if op.Action == "create" || op.Action == "read" || op.Action == "update" || op.Action == "delete" {
				crudOps++
			}
		}

		if crudOps == 0 {
			t.Error("Should have CRUD operations")
		}
	})

	t.Run("PolicyEngineCreated", func(t *testing.T) {
		if instance.PolicyEngine == nil {
			t.Fatal("PolicyEngine should be created")
		}

		policyCtx := &policy.Context{
			ActorID:    "customer",
			Resource:   "loyalty_account",
			Action:     "read",
			Attributes: map[string]interface{}{"owner": "customer"},
			Time:       time.Now(),
		}
		result := instance.PolicyEngine.CheckPermission(policyCtx)
		if !result.Allowed {
			t.Error("customer should read own loyalty_account")
		}
	})

	t.Run("HTTPMuxReady", func(t *testing.T) {
		if instance.HTTPMux == nil {
			t.Fatal("HTTPMux should be created")
		}

		srv := httptest.NewServer(instance)
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/_health")
		if err != nil {
			t.Fatalf("GET /_health: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("/_health: want 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		resp, err = http.Get(srv.URL + "/_system/info")
		if err != nil {
			t.Fatalf("GET /_system/info: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("/_system/info: want 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

func TestE2E_ParkingTicket_WorkflowTransitions(t *testing.T) {
	m := loadTestManifest(t, "parking-ticket.yaml")

	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	ctx := context.Background()

	migrator := db.NewMigrator(db.NewSQLDB(dbConn), m)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	workflowEngine := stateful.NewWorkflowEngine(m, dbConn)

	t.Run("InitializeWorkflowState", func(t *testing.T) {
		instance, err := workflowEngine.InitializeState("Ticket", "ticket-001", "attendant")
		if err != nil {
			t.Fatalf("InitializeState: %v", err)
		}
		if instance.CurrentState != "active" {
			t.Errorf("Initial state: want active, got %s", instance.CurrentState)
		}
	})

	t.Run("TransitionWorkflow", func(t *testing.T) {
		instance, err := workflowEngine.Transition(ctx, "Ticket", "ticket-001", "pay", "attendant", []string{"operator", "staff"})
		if err != nil {
			t.Fatalf("Transition(pay): %v", err)
		}
		if instance.CurrentState != "paid" {
			t.Errorf("After pay: want paid, got %s", instance.CurrentState)
		}
	})

	t.Run("WorkflowHistory", func(t *testing.T) {
		history, err := workflowEngine.GetHistory("Ticket", "ticket-001")
		if err != nil {
			t.Fatalf("GetHistory: %v", err)
		}
		if len(history) < 2 {
			t.Errorf("Expected at least 2 history entries, got %d", len(history))
		}
	})

	t.Run("ListAvailableActions", func(t *testing.T) {
		actions := workflowEngine.ListAvailableActions("Ticket", "ticket-001", []string{"operator"})
		if len(actions) == 0 {
			t.Error("Should have available actions")
		}
	})

	t.Run("UnauthorizedTransition", func(t *testing.T) {
		_, err := workflowEngine.Transition(ctx, "Ticket", "ticket-001", "void", "attendant", []string{"operator"})
		if err == nil {
			t.Error("Unauthorized transition should fail")
		}
	})
}

func TestE2E_TaskManagement_RBACWithWorkflow(t *testing.T) {
	m := loadTestManifest(t, "task-management.yaml")

	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	ctx := context.Background()

	migrator := db.NewMigrator(db.NewSQLDB(dbConn), m)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	engine, err := policy.NewEngine(m)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
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
	fsm, err := workflow.NewFSM(fsmCfg)
	if err != nil {
		t.Fatalf("NewFSM: %v", err)
	}

	t.Run("MemberCanStartTask", func(t *testing.T) {
		allowed, _, err := fsm.CanTransition(workflow.State("todo"), workflow.Action("start"), []string{"member"})
		if err != nil {
			t.Fatalf("CanTransition: %v", err)
		}
		if !allowed {
			t.Error("member should be able to start task")
		}
	})

	t.Run("ViewerCannotStartTask", func(t *testing.T) {
		allowed, _, err := fsm.CanTransition(workflow.State("todo"), workflow.Action("start"), []string{"viewer"})
		if err != nil {
			t.Fatalf("CanTransition: %v", err)
		}
		if allowed {
			t.Error("viewer should NOT be able to start task")
		}
	})

	t.Run("MemberCanOnlyWriteOwnTasks", func(t *testing.T) {
		policyCtx := &policy.Context{
			ActorID:    "member",
			Resource:   "tasks",
			Action:     "write",
			Attributes: map[string]interface{}{"owner": "member"},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(policyCtx)
		if !result.Allowed {
			t.Error("member should write own tasks")
		}

		policyCtx2 := &policy.Context{
			ActorID:    "member",
			Resource:   "tasks",
			Action:     "write",
			Attributes: map[string]interface{}{"owner": "other"},
			Time:       time.Now(),
		}
		result2 := engine.CheckPermission(policyCtx2)
		if result2.Allowed {
			t.Error("member should NOT write tasks owned by others")
		}
	})

	t.Run("AdminHasWildcardPermissions", func(t *testing.T) {
		policyCtx := &policy.Context{
			ActorID:    "admin",
			Resource:   "*",
			Action:     "*",
			Attributes: map[string]interface{}{},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(policyCtx)
		if !result.Allowed {
			t.Error("admin should have wildcard permissions")
		}
	})

	t.Run("RoleInheritance", func(t *testing.T) {
		roles := engine.GetAllRoles("admin")
		hasAdmin := false
		for _, r := range roles {
			if r == "admin" {
				hasAdmin = true
				break
			}
		}
		if !hasAdmin {
			t.Errorf("admin should have admin role via inheritance, got %v", roles)
		}
	})
}

func TestE2E_ManifestEvolution(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")

	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	ctx := context.Background()

	migrator := db.NewMigrator(db.NewSQLDB(dbConn), m)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	executor := evolution.NewEvolutionExecutor(m, dbConn)

	t.Run("InitialVersion", func(t *testing.T) {
		version := executor.GetCurrentVersion()
		if version != "1.0.0" {
			t.Errorf("Initial version: want 1.0.0, got %s", version)
		}
	})

	t.Run("DiffWithSameManifest", func(t *testing.T) {
		diff := executor.DiffWith(m)
		if diff.HasChanges() {
			t.Error("Diff with same manifest should have no changes")
		}
	})

	t.Run("EvolveWithNewField", func(t *testing.T) {
		newManifest := *m
		newManifest.Metadata.Version = "1.1.0"

		newEntity := manifest.Entity{
			Name: "Promotion",
			Fields: []manifest.Field{
				{Name: "id", Type: "string", Required: true, Unique: true},
				{Name: "name", Type: "string", Required: true},
				{Name: "discount_percent", Type: "float", Required: true},
				{Name: "active", Type: "bool", Required: true, Default: true},
			},
		}
		newManifest.DataModel.Entities = append(newManifest.DataModel.Entities, newEntity)

		result, err := executor.Evolve(ctx, &newManifest)
		if err != nil {
			t.Fatalf("Evolve: %v", err)
		}
		if !result.Success {
			t.Errorf("Evolve should succeed, warnings: %v", result.Warnings)
		}

		var tableName string
		err = dbConn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='promotion'").Scan(&tableName)
		if err == sql.ErrNoRows {
			t.Error("promotion table should be created")
		}
	})

	t.Run("VersionHistory", func(t *testing.T) {
		versions, err := executor.GetVersionHistory()
		if err != nil {
			t.Fatalf("GetVersionHistory: %v", err)
		}
		if len(versions) == 0 {
			t.Error("Should have version history")
		}
	})
}

func TestE2E_FullPipeline_CafeteriaLoyalty(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")

	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	bootstrapper := NewBootstrapper(BootstrapConfig{
		Manifest: m,
		DBDriver: "sqlite",
	})

	instance, err := bootstrapper.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer instance.Shutdown(ctx)

	srv := httptest.NewServer(instance)
	defer srv.Close()

	t.Run("1_SystemHealth", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/_health")
		if err != nil {
			t.Fatalf("GET /_health: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("/_health: want 200, got %d", resp.StatusCode)
		}
	})

	t.Run("2_SystemInfo", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/_system/info")
		if err != nil {
			t.Fatalf("GET /_system/info: %v", err)
		}
		defer resp.Body.Close()

		var info map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			t.Fatalf("Decode info: %v", err)
		}

		if info["name"] != "cafeteria-loyalty-system" {
			t.Errorf("System name: want cafeteria-loyalty-system, got %v", info["name"])
		}
	})

	t.Run("3_ListActors", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/_system/actors")
		if err != nil {
			t.Fatalf("GET /_system/actors: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Decode actors: %v", err)
		}

		actors := result["actors"].([]interface{})
		if len(actors) < 4 {
			t.Errorf("Expected at least 4 actors, got %d", len(actors))
		}
	})

	t.Run("4_ListOperations", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/_system/operations")
		if err != nil {
			t.Fatalf("GET /_system/operations: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Decode operations: %v", err)
		}

		ops := result["operations"].([]interface{})
		if len(ops) == 0 {
			t.Error("Should have operations")
		}
	})

	t.Run("5_OpenAPISpec", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/_openapi.json")
		if err != nil {
			t.Fatalf("GET /_openapi.json: %v", err)
		}
		defer resp.Body.Close()

		var spec map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
			t.Fatalf("Decode OpenAPI: %v", err)
		}

		if spec["openapi"] != "3.0.3" {
			t.Errorf("OpenAPI version: want 3.0.3, got %v", spec["openapi"])
		}
	})

	t.Run("6_WorkflowEngine", func(t *testing.T) {
		workflowEngine := stateful.NewWorkflowEngine(m, dbConn)

		inst, err := workflowEngine.InitializeState("Transaction", "tx-e2e-001", "barista")
		if err != nil {
			t.Fatalf("InitializeState: %v", err)
		}
		if inst.CurrentState == "" {
			t.Error("Should have initial state")
		}

		actions := workflowEngine.ListAvailableActions("Transaction", "tx-e2e-001", []string{"staff"})
		t.Logf("Available actions for staff: %v", actions)
	})

	t.Run("7_EvolutionExecutor", func(t *testing.T) {
		executor := evolution.NewEvolutionExecutor(m, dbConn)

		version := executor.GetCurrentVersion()
		if version == "" {
			t.Error("Should have current version")
		}

		versions, err := executor.GetVersionHistory()
		if err != nil {
			t.Fatalf("GetVersionHistory: %v", err)
		}
		t.Logf("Version history count: %d", len(versions))
	})

	t.Run("8_PolicyEngine", func(t *testing.T) {
		policyCtx := &policy.Context{
			ActorID:    "barista",
			Resource:   "transactions",
			Action:     "create",
			Attributes: map[string]interface{}{},
			Time:       time.Now(),
		}
		result := instance.PolicyEngine.CheckPermission(policyCtx)
		t.Logf("Barista can create transactions: %v", result.Allowed)
	})

	t.Run("9_RuleExecutor", func(t *testing.T) {
		data := map[string]interface{}{
			"type":        "purchase",
			"customer_id": "cust-e2e-001",
			"amount":      50.0,
		}

		err := instance.RuleExecutor.ExecuteBefore(ctx, "create", "Transaction", data)
		if err != nil {
			t.Logf("ExecuteBefore warning: %v", err)
		}

		err = instance.RuleExecutor.ExecuteAfter(ctx, "create", "Transaction", data)
		if err != nil {
			t.Logf("ExecuteAfter warning: %v", err)
		}
	})
}

func TestE2E_FullPipeline_ParkingTicket(t *testing.T) {
	m := loadTestManifest(t, "parking-ticket.yaml")

	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	bootstrapper := NewBootstrapper(BootstrapConfig{
		Manifest: m,
		DBDriver: "sqlite",
	})

	instance, err := bootstrapper.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer instance.Shutdown(ctx)

	t.Run("ComplexRBAC", func(t *testing.T) {
		testCases := []struct {
			actorID     string
			resource    string
			action      string
			attrs       map[string]interface{}
			shouldAllow bool
		}{
			{"driver", "own_tickets", "read", map[string]interface{}{"owner": "driver"}, true},
			{"driver", "tickets", "create", map[string]interface{}{"shift_active": true}, false},
			{"attendant", "tickets", "create", map[string]interface{}{"shift_active": true}, true},
			{"attendant", "tickets", "create", map[string]interface{}{"shift_active": false}, false},
			{"supervisor", "payments", "refund", map[string]interface{}{"amount": 1500.0}, true},
			{"supervisor", "payments", "refund", map[string]interface{}{"amount": 3000.0}, false},
			{"manager", "rates", "create", map[string]interface{}{}, true},
			{"admin", "*", "*", map[string]interface{}{}, true},
		}

		for _, tc := range testCases {
			policyCtx := &policy.Context{
				ActorID:    tc.actorID,
				Resource:   tc.resource,
				Action:     tc.action,
				Attributes: tc.attrs,
				Time:       time.Now(),
			}
			result := instance.PolicyEngine.CheckPermission(policyCtx)
			if result.Allowed != tc.shouldAllow {
				t.Errorf("Actor %s, resource %s, action %s: want allowed=%v, got %v",
					tc.actorID, tc.resource, tc.action, tc.shouldAllow, result.Allowed)
			}
		}
	})

	t.Run("WorkflowWithGuards", func(t *testing.T) {
		workflowEngine := stateful.NewWorkflowEngine(m, dbConn)

		_, err := workflowEngine.InitializeState("Ticket", "parking-001", "attendant")
		if err != nil {
			t.Fatalf("InitializeState: %v", err)
		}

		actions := workflowEngine.ListAvailableActions("Ticket", "parking-001", []string{"operator"})
		t.Logf("Available actions for operator: %v", actions)
	})

	t.Run("APIGeneration", func(t *testing.T) {
		spec := instance.APIGenerator.GenerateOpenAPI()

		if spec.Info.Title != "parking-ticket-system" {
			t.Errorf("API title: want parking-ticket-system, got %s", spec.Info.Title)
		}

		if len(spec.Paths) == 0 {
			t.Error("Should have API paths")
		}

		if len(spec.Components.Schemas) < 5 {
			t.Errorf("Expected at least 5 schemas, got %d", len(spec.Components.Schemas))
		}
	})
}

func TestE2E_FullPipeline_TaskManagement(t *testing.T) {
	m := loadTestManifest(t, "task-management.yaml")

	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	bootstrapper := NewBootstrapper(BootstrapConfig{
		Manifest: m,
		DBDriver: "sqlite",
	})

	instance, err := bootstrapper.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer instance.Shutdown(ctx)

	t.Run("TeamBasedPermissions", func(t *testing.T) {
		policyCtx := &policy.Context{
			ActorID:    "member",
			Resource:   "tasks",
			Action:     "write",
			Attributes: map[string]interface{}{"owner": "member"},
			Time:       time.Now(),
		}
		result := instance.PolicyEngine.CheckPermission(policyCtx)
		if !result.Allowed {
			t.Error("member should write own tasks")
		}

		policyCtx2 := &policy.Context{
			ActorID:    "member",
			Resource:   "tasks",
			Action:     "write",
			Attributes: map[string]interface{}{"owner": "other"},
			Time:       time.Now(),
		}
		result2 := instance.PolicyEngine.CheckPermission(policyCtx2)
		if result2.Allowed {
			t.Error("member should NOT write others' tasks")
		}
	})

	t.Run("ProjectWorkflow", func(t *testing.T) {
		workflowEngine := stateful.NewWorkflowEngine(m, dbConn)

		_, err := workflowEngine.InitializeState("Task", "task-001", "member")
		if err != nil {
			t.Fatalf("InitializeState: %v", err)
		}

		actions := workflowEngine.ListAvailableActions("Task", "task-001", []string{"member"})
		t.Logf("Available actions for member: %v", actions)

		actions2 := workflowEngine.ListAvailableActions("Task", "task-001", []string{"viewer"})
		if len(actions2) > 0 {
			t.Errorf("viewer should have no actions, got %v", actions2)
		}
	})

	t.Run("BusinessRules", func(t *testing.T) {
		if len(m.BusinessRules) == 0 {
			t.Log("No business rules defined in manifest")
			return
		}

		for _, rule := range m.BusinessRules {
			t.Logf("Business rule: %s (%s)", rule.Name, rule.ID)
		}
	})
}

func TestE2E_AuditAndNotifications(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")

	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(1000)
	notifyBus := NewNotificationBus(500)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	ctx := context.Background()

	t.Run("AuditLogging", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			entry := AuditEntry{
				ActorID:  "barista",
				Action:   "create_transaction",
				Resource: "Transaction",
				Allowed:  true,
				Details:  map[string]interface{}{"amount": 50.0 + float64(i*10)},
			}
			if err := auditLog.Record(entry); err != nil {
				t.Fatalf("Record: %v", err)
			}
		}

		entries, err := auditLog.Query(AuditFilter{Limit: 10})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(entries) != 5 {
			t.Errorf("Expected 5 entries, got %d", len(entries))
		}

		entries2, err := auditLog.Query(AuditFilter{ActorID: "barista", Limit: 10})
		if err != nil {
			t.Fatalf("Query(actor=barista): %v", err)
		}
		if len(entries2) != 5 {
			t.Errorf("Expected 5 entries for barista, got %d", len(entries2))
		}
	})

	t.Run("Notifications", func(t *testing.T) {
		notifyBus.Notify(Notification{
			FromActor: "system",
			ToActor:   "manager",
			Type:      "alert",
			Title:     "Test Alert",
			Body:      "This is a test notification",
		})

		notifyBus.Notify(Notification{
			FromActor: "barista",
			ToActor:   "customer",
			Type:      "points_earned",
			Title:     "Points Earned",
			Body:      "You earned 50 points!",
		})

		managerNotifs := notifyBus.GetUnread("manager")
		if len(managerNotifs) != 1 {
			t.Errorf("Manager should have 1 notification, got %d", len(managerNotifs))
		}

		customerNotifs := notifyBus.GetUnread("customer")
		if len(customerNotifs) != 1 {
			t.Errorf("Customer should have 1 notification, got %d", len(customerNotifs))
		}

		notifyBus.MarkRead(managerNotifs[0].ID)
		if len(notifyBus.GetUnread("manager")) != 0 {
			t.Error("Manager should have 0 unread after marking read")
		}
	})

	t.Run("HookIntegration", func(t *testing.T) {
		call := &agent.ToolCallHookRequest{
			Tool:      "read",
			Arguments: map[string]any{"entity_id": "cust-001", "entity_type": "Customer"},
			Context:   &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "barista"}},
		}
		_, decision, err := hook.BeforeTool(ctx, call)
		if err != nil {
			t.Fatalf("BeforeTool: %v", err)
		}
		t.Logf("Tool call decision: %s", decision.Action)

		payload := TurnEndPayload{
			Status:      agent.TurnEndStatusCompleted,
			UserMessage: "Transaction processed",
			ToolCount:   3,
			Iteration:   1,
			Context:     &agent.TurnContext{Inbound: &bus.InboundContext{SenderID: "barista"}},
		}
		if err := hook.AfterTurn(ctx, payload); err != nil {
			t.Fatalf("AfterTurn: %v", err)
		}

		entries, err := auditLog.Query(AuditFilter{ActorID: "barista", Limit: 10})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(entries) == 0 {
			t.Error("Should have audit entries after hook operations")
		}
	})
}

func TestE2E_ErrorHandling(t *testing.T) {
	t.Run("InvalidManifest", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidPath := filepath.Join(tmpDir, "invalid.yaml")
		os.WriteFile(invalidPath, []byte("invalid: yaml: content: ["), 0644)

		_, err := manifest.ParseFile(invalidPath)
		if err == nil {
			t.Error("Should fail to parse invalid manifest")
		}
	})

	t.Run("MissingManifest", func(t *testing.T) {
		_, err := manifest.ParseFile("nonexistent.yaml")
		if err == nil {
			t.Error("Should fail for nonexistent manifest")
		}
	})

	t.Run("UnauthorizedOperation", func(t *testing.T) {
		m := loadTestManifest(t, "cafeteria-loyalty.yaml")

		engine, err := policy.NewEngine(m)
		if err != nil {
			t.Fatalf("NewEngine: %v", err)
		}

		policyCtx := &policy.Context{
			ActorID:    "customer",
			Resource:   "customers",
			Action:     "delete",
			Attributes: map[string]interface{}{},
			Time:       time.Now(),
		}
		result := engine.CheckPermission(policyCtx)
		if result.Allowed {
			t.Error("customer should NOT be able to delete customers")
		}
	})

	t.Run("InvalidWorkflowTransition", func(t *testing.T) {
		_ = loadTestManifest(t, "task-management.yaml")

		fsmCfg := workflow.FSMConfig{
			EntityName:   "Task",
			InitialState: workflow.State("todo"),
			States: map[workflow.State]workflow.StateConfig{
				"todo": {
					Transitions: []workflow.Transition{
						{To: "done", Action: "complete", AllowedRoles: []string{"member"}},
					},
				},
				"done": {},
			},
		}
		fsm, err := workflow.NewFSM(fsmCfg)
		if err != nil {
			t.Fatalf("NewFSM: %v", err)
		}

		allowed, _, err := fsm.CanTransition(workflow.State("todo"), workflow.Action("invalid_action"), []string{"member"})
		if allowed {
			t.Error("Invalid action should not be allowed")
		}
	})
}

func TestE2E_ActorStore_CRUD(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")

	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	t.Run("MemoryActorStore", func(t *testing.T) {
		store := NewMemoryActorStore()

		for _, actor := range m.Actors {
			cred, err := store.Provision(actor)
			if err != nil {
				t.Errorf("Provision %s: %v", actor.ID, err)
				continue
			}
			if cred.APIKey == "" {
				t.Errorf("Actor %s should have API key", actor.ID)
			}
		}

		actors, err := store.ListAll()
		if err != nil {
			t.Fatalf("ListAll: %v", err)
		}
		if len(actors) != len(m.Actors) {
			t.Errorf("Expected %d actors, got %d", len(m.Actors), len(actors))
		}

		for _, actor := range m.Actors {
			cred, err := store.GetByID(actor.ID)
			if err != nil {
				t.Errorf("GetByID %s: %v", actor.ID, err)
				continue
			}
			if cred.ActorID != actor.ID {
				t.Errorf("ActorID mismatch: want %s, got %s", actor.ID, cred.ActorID)
			}
		}

		if len(actors) > 0 {
			cred, _ := store.GetByID(actors[0].ActorID)
			credByAPIKey, err := store.GetByAPIKey(cred.APIKey)
			if err != nil {
				t.Errorf("GetByAPIKey: %v", err)
			}
			if credByAPIKey.ActorID != cred.ActorID {
				t.Errorf("API key lookup mismatch")
			}
		}
	})

	t.Run("DBActorStore", func(t *testing.T) {
		store := NewDBActorStore(dbConn)

		for _, actor := range m.Actors {
			_, err := store.Provision(actor)
			if err != nil && err.Error() != "" && !strings.Contains(err.Error(), "already provisioned") {
				t.Errorf("Provision %s: %v", actor.ID, err)
			}
		}

		actors, err := store.ListAll()
		if err != nil {
			t.Fatalf("ListAll: %v", err)
		}
		if len(actors) < len(m.Actors) {
			t.Errorf("Expected at least %d actors, got %d", len(m.Actors), len(actors))
		}

		for _, actor := range m.Actors {
			cred, err := store.GetByID(actor.ID)
			if err != nil {
				t.Errorf("GetByID %s: %v", actor.ID, err)
				continue
			}
			if cred.ActorID != actor.ID {
				t.Errorf("ActorID mismatch: want %s, got %s", actor.ID, cred.ActorID)
			}
		}
	})
}

func TestE2E_OperationCatalog_Generation(t *testing.T) {
	m := loadTestManifest(t, "parking-ticket.yaml")

	catalog := NewCatalog(m)

	t.Run("ListAllOperations", func(t *testing.T) {
		ops := catalog.ListAll()
		if len(ops) == 0 {
			t.Error("Should have operations")
		}

		entityCount := len(m.DataModel.Entities)
		minOps := entityCount * 4
		if len(ops) < minOps {
			t.Errorf("Expected at least %d operations (4 per entity), got %d", minOps, len(ops))
		}
	})

	t.Run("GetOperation", func(t *testing.T) {
		op := catalog.Get("create_Ticket")
		if op == nil {
			t.Error("Should have create_Ticket operation")
		} else {
			if op.Entity != "Ticket" {
				t.Errorf("Operation entity: want Ticket, got %s", op.Entity)
			}
			if op.Action != "create" {
				t.Errorf("Operation action: want create, got %s", op.Action)
			}
		}
	})

	t.Run("GetByEntity", func(t *testing.T) {
		ticketOps := catalog.ListByEntity("Ticket")
		if len(ticketOps) < 4 {
			t.Errorf("Ticket should have at least 4 operations, got %d", len(ticketOps))
		}
	})
}

func TestE2E_WorkflowStateStore_Persistence(t *testing.T) {
	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	t.Run("MemoryStore", func(t *testing.T) {
		store := stateful.NewMemoryWorkflowStateStore()

		inst := &stateful.WorkflowInstance{
			ID:           "Task:task-001",
			EntityType:   "Task",
			EntityID:     "task-001",
			CurrentState: "todo",
			UpdatedBy:    "member",
		}

		if err := store.SetState(inst); err != nil {
			t.Fatalf("SetState: %v", err)
		}

		retrieved, err := store.GetState("Task", "task-001")
		if err != nil {
			t.Fatalf("GetState: %v", err)
		}
		if retrieved == nil {
			t.Fatal("Should retrieve state")
		}
		if retrieved.CurrentState != "todo" {
			t.Errorf("CurrentState: want todo, got %s", retrieved.CurrentState)
		}

		record := &stateful.TransitionRecord{
			EntityType: "Task",
			EntityID:   "task-001",
			FromState:  "todo",
			ToState:    "in_progress",
			Action:     "start",
			ActorID:    "member",
		}
		if err := store.RecordTransition(record); err != nil {
			t.Fatalf("RecordTransition: %v", err)
		}

		history, err := store.GetHistory("Task", "task-001")
		if err != nil {
			t.Fatalf("GetHistory: %v", err)
		}
		if len(history) != 1 {
			t.Errorf("Expected 1 history entry, got %d", len(history))
		}
	})

	t.Run("DBStore", func(t *testing.T) {
		store := stateful.NewDBWorkflowStateStore(dbConn)

		inst := &stateful.WorkflowInstance{
			ID:           "Task:task-002",
			EntityType:   "Task",
			EntityID:     "task-002",
			CurrentState: "todo",
			UpdatedBy:    "member",
		}

		if err := store.SetState(inst); err != nil {
			t.Fatalf("SetState: %v", err)
		}

		retrieved, err := store.GetState("Task", "task-002")
		if err != nil {
			t.Fatalf("GetState: %v", err)
		}
		if retrieved == nil {
			t.Fatal("Should retrieve state")
		}
		if retrieved.CurrentState != "todo" {
			t.Errorf("CurrentState: want todo, got %s", retrieved.CurrentState)
		}

		list, err := store.ListStates("Task")
		if err != nil {
			t.Fatalf("ListStates: %v", err)
		}
		if len(list) == 0 {
			t.Error("Should have at least one state")
		}
	})
}

func TestE2E_ManifestVersionStore_Persistence(t *testing.T) {
	dbConn, cleanup := setupE2EDB(t)
	defer cleanup()

	store := evolution.NewDBManifestVersionStore(dbConn)

	t.Run("SaveAndGetVersion", func(t *testing.T) {
		version := &evolution.ManifestVersion{
			Version:      "1.0.0",
			ManifestYAML: "metadata:\n  name: test\n  version: 1.0.0",
			CreatedAt:    time.Now(),
			CreatedBy:    "test",
			Description:  "Initial version",
		}

		if err := store.SaveVersion(version); err != nil {
			t.Fatalf("SaveVersion: %v", err)
		}

		retrieved, err := store.GetVersion("1.0.0")
		if err != nil {
			t.Fatalf("GetVersion: %v", err)
		}
		if retrieved == nil {
			t.Fatal("Should retrieve version")
		}
		if retrieved.Version != "1.0.0" {
			t.Errorf("Version: want 1.0.0, got %s", retrieved.Version)
		}
	})

	t.Run("GetLatestVersion", func(t *testing.T) {
		version2 := &evolution.ManifestVersion{
			Version:      "1.1.0",
			ManifestYAML: "metadata:\n  name: test\n  version: 1.1.0",
			CreatedAt:    time.Now(),
			CreatedBy:    "test",
			Description:  "Second version",
		}
		store.SaveVersion(version2)

		latest, err := store.GetLatestVersion()
		if err != nil {
			t.Fatalf("GetLatestVersion: %v", err)
		}
		if latest == nil {
			t.Fatal("Should have latest version")
		}
		if latest.Version != "1.1.0" {
			t.Errorf("Latest version: want 1.1.0, got %s", latest.Version)
		}
	})

	t.Run("ListVersions", func(t *testing.T) {
		versions, err := store.ListVersions()
		if err != nil {
			t.Fatalf("ListVersions: %v", err)
		}
		if len(versions) < 2 {
			t.Errorf("Expected at least 2 versions, got %d", len(versions))
		}
	})
}
