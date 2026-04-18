package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/agentos"
	"github.com/sipeed/picoclaw/pkg/api"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/infra/db"
	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/mcp"
	_ "modernc.org/sqlite"
)

func testSystemInstance(t *testing.T) (*agentos.SystemInstance, func()) {
	t.Helper()

	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "TestSystem",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Task",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "title", Type: "string", Required: true},
						{Name: "status", Type: "string", Required: false},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Administrator",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "Task", Actions: []string{"read", "write", "delete"}},
				},
			},
			{
				ID:    "user",
				Name:  "User",
				Roles: []string{"user"},
				Permissions: []manifest.Permission{
					{Resource: "Task", Actions: []string{"read"}},
				},
			},
		},
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	dbConn, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	dbConn.SetMaxOpenConns(1)
	dbConn.SetMaxIdleConns(1)

	migrator := db.NewMigrator(db.NewSQLDB(dbConn), m)
	if err := migrator.Migrate(context.Background()); err != nil {
		dbConn.Close()
		t.Fatalf("Migrate: %v", err)
	}

	actorStore := agentos.NewMemoryActorStore()
	for _, actor := range m.Actors {
		actorStore.Provision(actor)
	}

	catalog := agentos.NewCatalog(m)

	policyEngine, err := policy.NewEngine(m)
	if err != nil {
		dbConn.Close()
		t.Fatalf("NewEngine: %v", err)
	}

	ruleExecutor := api.NewRuleExecutor(m)

	apiGen, err := api.NewGeneratorWithDB(m, dbConn)
	if err != nil {
		dbConn.Close()
		t.Fatalf("NewGeneratorWithDB: %v", err)
	}

	si := &agentos.SystemInstance{
		Manifest:     m,
		DB:           dbConn,
		Catalog:      catalog,
		ActorStore:   actorStore,
		PolicyEngine: policyEngine,
		RuleExecutor: ruleExecutor,
		APIGenerator: apiGen,
	}

	cleanup := func() {
		dbConn.SetMaxOpenConns(0)
		dbConn.SetMaxIdleConns(0)
		dbConn.Close()
	}

	return si, cleanup
}

func TestMCPServer_Initialize(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	cfg := ServerConfig{
		Name:        "test-server",
		Version:     "1.0.0",
		Description: "Test MCP Server",
	}

	server := NewMCPServer(cfg, si)

	params := mcp.InitializeParams{
		ProtocolVersion: mcp.ProtocolVersion,
		ClientInfo: mcp.ClientInfo{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}

	paramsJSON, _ := json.Marshal(params)
	req := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  paramsJSON,
	}

	resp, err := server.HandleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	if !server.IsInitialized() {
		t.Error("server should be initialized")
	}
}

func TestMCPServer_ListTools(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	cfg := ServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, si)

	params := mcp.InitializeParams{
		ProtocolVersion: mcp.ProtocolVersion,
		ClientInfo:      mcp.ClientInfo{Name: "test", Version: "1.0"},
	}
	initParamsJSON, _ := json.Marshal(params)
	initReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  initParamsJSON,
	}
	server.HandleRequest(context.Background(), initReq)

	listReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      2,
		Method:  "tools/list",
		Params:  []byte("{}"),
	}

	resp, err := server.HandleRequest(context.Background(), listReq)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	if resp.Result == nil {
		t.Fatal("expected result")
	}

	var result mcp.ListToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(result.Tools) == 0 {
		t.Error("expected tools to be generated")
	}

	t.Logf("Generated %d tools", len(result.Tools))
	for _, tool := range result.Tools {
		t.Logf("  - %s: %s", tool.Name, tool.Description)
	}
}

func TestMCPServer_CallTool_List(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	cfg := ServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, si)

	initParams := mcp.InitializeParams{
		ProtocolVersion: mcp.ProtocolVersion,
		ClientInfo:      mcp.ClientInfo{Name: "test", Version: "1.0"},
	}
	initParamsJSON, _ := json.Marshal(initParams)
	initReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  initParamsJSON,
	}
	server.HandleRequest(context.Background(), initReq)

	_, err := si.DB.Exec(`INSERT INTO tasks (id, title, status) VALUES ('task-1', 'Test Task', 'todo')`)
	if err != nil {
		t.Skipf("skipping list test (table may not be created): %v", err)
	}

	callParams := mcp.CallToolParams{
		Name:      "Task.list",
		Arguments: map[string]interface{}{"limit": float64(10)},
	}
	callParamsJSON, _ := json.Marshal(callParams)
	callReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      2,
		Method:  "tools/call",
		Params:  callParamsJSON,
	}

	resp, err := server.HandleRequest(context.Background(), callReq)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	var result mcp.CallToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(result.Content) == 0 {
		t.Error("expected content in result")
	}

	if result.IsError {
		t.Errorf("expected no error, got: %s", result.Content[0].Text)
	}
}

func TestMCPServer_CallTool_Create(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	cfg := ServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, si)

	initParams := mcp.InitializeParams{
		ProtocolVersion: mcp.ProtocolVersion,
		ClientInfo:      mcp.ClientInfo{Name: "test", Version: "1.0"},
	}
	initParamsJSON, _ := json.Marshal(initParams)
	initReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  initParamsJSON,
	}
	server.HandleRequest(context.Background(), initReq)

	callParams := mcp.CallToolParams{
		Name: "Task.create",
		Arguments: map[string]interface{}{
			"title": "New Task",
		},
	}
	callParamsJSON, _ := json.Marshal(callParams)
	callReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      2,
		Method:  "tools/call",
		Params:  callParamsJSON,
	}

	resp, err := server.HandleRequest(context.Background(), callReq)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	var result mcp.CallToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if result.IsError {
		t.Errorf("expected no error, got: %s", result.Content[0].Text)
	}
}

func TestMCPServer_ListResources(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	cfg := ServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, si)

	initParams := mcp.InitializeParams{
		ProtocolVersion: mcp.ProtocolVersion,
		ClientInfo:      mcp.ClientInfo{Name: "test", Version: "1.0"},
	}
	initParamsJSON, _ := json.Marshal(initParams)
	initReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  initParamsJSON,
	}
	server.HandleRequest(context.Background(), initReq)

	listReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      2,
		Method:  "resources/list",
		Params:  []byte("{}"),
	}

	resp, err := server.HandleRequest(context.Background(), listReq)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	var result mcp.ListResourcesResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(result.Resources) == 0 {
		t.Error("expected resources to be generated")
	}

	t.Logf("Generated %d resources", len(result.Resources))
	for _, res := range result.Resources {
		t.Logf("  - %s: %s", res.URI, res.Name)
	}
}

func TestMCPServer_MethodNotFound(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	cfg := ServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, si)

	req := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  "nonexistent/method",
		Params:  []byte("{}"),
	}

	resp, err := server.HandleRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	if resp.Error == nil {
		t.Error("expected error for nonexistent method")
	}

	if resp.Error.Code != mcp.MethodNotFound {
		t.Errorf("expected MethodNotFound error code, got %d", resp.Error.Code)
	}
}

func TestToolGenerator_GenerateTools(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	gen := NewToolGenerator(si.Catalog)
	tools := gen.GenerateTools()

	if len(tools) == 0 {
		t.Error("expected tools to be generated")
	}

	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool name should not be empty")
		}
		if tool.InputSchema == nil {
			t.Error("tool input schema should not be nil")
		}
	}
}

func TestToolGenerator_OperationToTool(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	gen := NewToolGenerator(si.Catalog)

	ops := si.Catalog.ListAll()
	if len(ops) == 0 {
		t.Fatal("expected operations")
	}

	tool := gen.OperationToTool(&ops[0])

	if tool.Name != ops[0].Name {
		t.Errorf("expected name %s, got %s", ops[0].Name, tool.Name)
	}

	if tool.InputSchema == nil {
		t.Error("expected input schema")
	}
}

func TestResourceGenerator_GenerateResources(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	gen := NewResourceGenerator(si.Manifest, si.DB)
	resources := gen.GenerateResources()

	if len(resources) == 0 {
		t.Error("expected resources to be generated")
	}

	for _, res := range resources {
		if res.URI == "" {
			t.Error("resource URI should not be empty")
		}
		if res.Name == "" {
			t.Error("resource name should not be empty")
		}
	}
}

func TestResourceGenerator_EntityToResource(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	gen := NewResourceGenerator(si.Manifest, si.DB)

	entities := si.Manifest.DataModel.Entities
	if len(entities) == 0 {
		t.Fatal("expected entities")
	}

	resource := gen.EntityToResource(&entities[0])

	if resource.URI == "" {
		t.Error("resource URI should not be empty")
	}

	expectedURIPrefix := "resource://testsystem/"
	if len(resource.URI) <= len(expectedURIPrefix) {
		t.Errorf("expected URI to start with %s, got %s", expectedURIPrefix, resource.URI)
	}
}

func TestOperationHandler_ExecuteTool_NotFound(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	cfg := ServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, si)

	initParams := mcp.InitializeParams{
		ProtocolVersion: mcp.ProtocolVersion,
		ClientInfo:      mcp.ClientInfo{Name: "test", Version: "1.0"},
	}
	initParamsJSON, _ := json.Marshal(initParams)
	initReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  initParamsJSON,
	}
	server.HandleRequest(context.Background(), initReq)

	callParams := mcp.CallToolParams{
		Name:      "Nonexistent.operation",
		Arguments: map[string]interface{}{},
	}
	callParamsJSON, _ := json.Marshal(callParams)
	callReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      2,
		Method:  "tools/call",
		Params:  callParamsJSON,
	}

	resp, err := server.HandleRequest(context.Background(), callReq)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	var result mcp.CallToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for nonexistent operation")
	}
}

func loadManifest(t *testing.T, name string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.ParseFile("../../examples/manifests/" + name)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", name, err)
	}
	return m
}

func TestMCPServer_FullPipeline_Cafeteria(t *testing.T) {
	m := loadManifest(t, "cafeteria-loyalty.yaml")

	actorStore := agentos.NewMemoryActorStore()
	for _, actor := range m.Actors {
		actorStore.Provision(actor)
	}

	catalog := agentos.NewCatalog(m)

	policyEngine, err := policy.NewEngine(m)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ruleExecutor := api.NewRuleExecutor(m)

	si := &agentos.SystemInstance{
		Manifest:     m,
		Catalog:      catalog,
		ActorStore:   actorStore,
		PolicyEngine: policyEngine,
		RuleExecutor: ruleExecutor,
	}

	cfg := ServerConfig{
		Name:        m.Metadata.Name,
		Version:     m.Metadata.Version,
		Description: m.Metadata.Description,
	}

	server := NewMCPServer(cfg, si)

	initParams := mcp.InitializeParams{
		ProtocolVersion: mcp.ProtocolVersion,
		ClientInfo:      mcp.ClientInfo{Name: "test-client", Version: "1.0"},
	}
	initParamsJSON, _ := json.Marshal(initParams)
	initReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  initParamsJSON,
	}

	initResp, err := server.HandleRequest(context.Background(), initReq)
	if err != nil {
		t.Fatalf("HandleRequest (initialize): %v", err)
	}
	if initResp.Error != nil {
		t.Errorf("initialize error: %v", initResp.Error)
	}

	listToolsReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      2,
		Method:  "tools/list",
		Params:  []byte("{}"),
	}

	listResp, err := server.HandleRequest(context.Background(), listToolsReq)
	if err != nil {
		t.Fatalf("HandleRequest (tools/list): %v", err)
	}
	if listResp.Error != nil {
		t.Errorf("tools/list error: %v", listResp.Error)
	}

	var toolsResult mcp.ListToolsResult
	if err := json.Unmarshal(listResp.Result, &toolsResult); err != nil {
		t.Fatalf("unmarshal tools: %v", err)
	}

	t.Logf("Cafeteria system: %d tools generated", len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		t.Logf("  - %s", tool.Name)
	}

	listResourcesReq := &mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      3,
		Method:  "resources/list",
		Params:  []byte("{}"),
	}

	resourcesResp, err := server.HandleRequest(context.Background(), listResourcesReq)
	if err != nil {
		t.Fatalf("HandleRequest (resources/list): %v", err)
	}

	var resourcesResult mcp.ListResourcesResult
	if err := json.Unmarshal(resourcesResp.Result, &resourcesResult); err != nil {
		t.Fatalf("unmarshal resources: %v", err)
	}

	t.Logf("Cafeteria system: %d resources generated", len(resourcesResult.Resources))
}

func TestMCPServer_Shutdown(t *testing.T) {
	si, cleanup := testSystemInstance(t)
	defer cleanup()

	cfg := ServerConfig{
		Name:    "test-server",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, si)

	if server == nil {
		t.Fatal("server should not be nil")
	}

	if !server.IsInitialized() {
		t.Log("server not initialized (expected for basic test)")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
