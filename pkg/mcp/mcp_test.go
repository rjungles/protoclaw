package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestProtocol_NewRequest(t *testing.T) {
	req, err := NewRequest("initialize", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	if req.JSONRPC != JSONRPCVersion {
		t.Errorf("JSONRPC: want %s, got %s", JSONRPCVersion, req.JSONRPC)
	}

	if req.Method != "initialize" {
		t.Errorf("Method: want initialize, got %s", req.Method)
	}

	if req.ID == 0 {
		t.Error("ID should not be zero")
	}
}

func TestProtocol_NextRequestID(t *testing.T) {
	id1 := NextRequestID()
	id2 := NextRequestID()

	if id1 == id2 {
		t.Error("IDs should be unique")
	}

	if id2 <= id1 {
		t.Error("IDs should be increasing")
	}
}

func TestProtocol_DecodeResult(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		ServerInfo: ServerInfo{
			Name:    "test-server",
			Version: "1.0.0",
		},
	}

	data, err := encodeToRawMessage(result)
	if err != nil {
		t.Fatalf("encodeToRawMessage: %v", err)
	}

	resp := &Response{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Result:  data,
	}

	decoded, err := DecodeResult[InitializeResult](resp)
	if err != nil {
		t.Fatalf("DecodeResult: %v", err)
	}

	if decoded.ServerInfo.Name != "test-server" {
		t.Errorf("ServerInfo.Name: want test-server, got %s", decoded.ServerInfo.Name)
	}
}

func TestProtocol_DecodeResult_Error(t *testing.T) {
	resp := &Response{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Error:   &Error{Code: -32600, Message: "Invalid Request"},
	}

	_, err := DecodeResult[InitializeResult](resp)
	if err == nil {
		t.Error("expected error for response with error")
	}
}

func TestMockTransport_Initialize(t *testing.T) {
	transport := NewMockTransport()
	defer transport.Close()

	req, _ := NewRequest("initialize", InitializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientInfo:      ClientInfo{Name: "test", Version: "1.0"},
	})

	resp, err := transport.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	result, err := DecodeResult[InitializeResult](resp)
	if err != nil {
		t.Fatalf("DecodeResult: %v", err)
	}

	if result.ServerInfo.Name != "mock-server" {
		t.Errorf("ServerInfo.Name: want mock-server, got %s", result.ServerInfo.Name)
	}
}

func TestMockTransport_ListTools(t *testing.T) {
	transport := NewMockTransport()
	defer transport.Close()

	req, _ := NewRequest("tools/list", ListToolsParams{})

	resp, err := transport.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	result, err := DecodeResult[ListToolsResult](resp)
	if err != nil {
		t.Fatalf("DecodeResult: %v", err)
	}

	if len(result.Tools) == 0 {
		t.Error("expected at least one tool")
	}
}

func TestMockTransport_CallTool(t *testing.T) {
	transport := NewMockTransport()
	defer transport.Close()

	req, _ := NewRequest("tools/call", CallToolParams{
		Name:      "test_tool",
		Arguments: map[string]interface{}{"arg1": "value1"},
	})

	resp, err := transport.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	result, err := DecodeResult[CallToolResult](resp)
	if err != nil {
		t.Fatalf("DecodeResult: %v", err)
	}

	if len(result.Content) == 0 {
		t.Error("expected content in result")
	}
}

func TestClient_Initialize(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)

	result, err := client.Initialize(context.Background(), "test-client", "1.0.0")
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if result.ServerInfo.Name != "mock-server" {
		t.Errorf("ServerInfo.Name: want mock-server, got %s", result.ServerInfo.Name)
	}

	if !client.IsInitialized() {
		t.Error("client should be initialized")
	}
}

func TestClient_ListTools(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)

	_, _ = client.Initialize(context.Background(), "test-client", "1.0.0")

	result, err := client.ListTools(context.Background(), "")
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(result.Tools) == 0 {
		t.Error("expected at least one tool")
	}

	tools := client.Tools()
	if len(tools) == 0 {
		t.Error("client should have cached tools")
	}
}

func TestClient_CallTool(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)

	_, _ = client.Initialize(context.Background(), "test-client", "1.0.0")

	result, err := client.CallTool(context.Background(), "test_tool", map[string]interface{}{"arg": "value"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	if len(result.Content) == 0 {
		t.Error("expected content in result")
	}
}

func TestClient_FindTool(t *testing.T) {
	transport := NewMockTransport()
	transport.SetTools([]Tool{
		{Name: "tool_a", Description: "Tool A"},
		{Name: "tool_b", Description: "Tool B"},
	})
	client := NewClient(transport)

	_, _ = client.Initialize(context.Background(), "test-client", "1.0.0")
	_, _ = client.ListTools(context.Background(), "")

	tool := client.FindTool("tool_a")
	if tool == nil {
		t.Fatal("tool_a not found")
	}

	if tool.Description != "Tool A" {
		t.Errorf("Description: want Tool A, got %s", tool.Description)
	}

	tool = client.FindTool("nonexistent")
	if tool != nil {
		t.Error("nonexistent tool should not be found")
	}
}

func TestRegistry_RegisterConfig(t *testing.T) {
	registry := NewRegistry()

	cfg := manifest.MCPConfig{
		Name:      "test-server",
		Server:    "test-server-binary",
		Transport: "stdio",
	}

	registry.RegisterConfig("test-server", cfg)

	servers := registry.ListServers()
	if len(servers) != 1 {
		t.Errorf("ListServers: want 1, got %d", len(servers))
	}

	retrieved, ok := registry.GetConfig("test-server")
	if !ok {
		t.Fatal("config not found")
	}

	if retrieved.Server != "test-server-binary" {
		t.Errorf("Server: want test-server-binary, got %s", retrieved.Server)
	}
}

func TestRegistry_GetClient_Mock(t *testing.T) {
	registry := NewRegistry()

	transport := NewMockTransport()
	client := NewClient(transport)

	_, err := client.Initialize(context.Background(), "test-client", "1.0.0")
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	registry.mu.Lock()
	registry.clients["mock-server"] = client
	registry.configs["mock-server"] = manifest.MCPConfig{Name: "mock-server"}
	registry.mu.Unlock()

	retrieved, err := registry.GetClient(context.Background(), "mock-server")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}

	if !retrieved.IsInitialized() {
		t.Error("client should be initialized")
	}

	client2, err := registry.GetClient(context.Background(), "mock-server")
	if err != nil {
		t.Fatalf("GetClient (cached): %v", err)
	}

	if retrieved != client2 {
		t.Error("should return cached client")
	}
}

func TestRegistry_FindTool(t *testing.T) {
	registry := NewRegistry()

	transport := NewMockTransport()
	transport.SetTools([]Tool{
		{Name: "test_tool", Description: "Test Tool"},
	})
	client := NewClient(transport)

	_, err := client.Initialize(context.Background(), "test-client", "1.0.0")
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	_, _ = client.ListTools(context.Background(), "")

	registry.mu.Lock()
	registry.clients["test-server"] = client
	registry.configs["test-server"] = manifest.MCPConfig{Name: "test-server"}
	registry.mu.Unlock()

	serverName, tool, err := registry.FindTool(context.Background(), "test_tool")
	if err != nil {
		t.Fatalf("FindTool: %v", err)
	}

	if serverName != "test-server" {
		t.Errorf("serverName: want test-server, got %s", serverName)
	}

	if tool == nil {
		t.Fatal("tool should not be nil")
	}

	if tool.Name != "test_tool" {
		t.Errorf("tool.Name: want test_tool, got %s", tool.Name)
	}
}

func TestRegistry_CloseAll(t *testing.T) {
	registry := NewRegistry()

	transport := NewMockTransport()
	client := NewClient(transport)

	_, err := client.Initialize(context.Background(), "test-client", "1.0.0")
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	registry.mu.Lock()
	registry.clients["mock-server"] = client
	registry.configs["mock-server"] = manifest.MCPConfig{Name: "mock-server"}
	registry.mu.Unlock()

	if err := registry.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
}

func TestMCPHook_BeforeLLM(t *testing.T) {
	registry := NewRegistry()

	transport := NewMockTransport()
	transport.SetTools([]Tool{
		{Name: "test_tool", Description: "Test Tool"},
	})
	client := NewClient(transport)

	_, err := client.Initialize(context.Background(), "test-client", "1.0.0")
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	_, _ = client.ListTools(context.Background(), "")

	registry.mu.Lock()
	registry.clients["test-server"] = client
	registry.configs["test-server"] = manifest.MCPConfig{Name: "test-server"}
	registry.mu.Unlock()

	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "test-system",
			Version: "1.0.0",
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "*", Actions: []string{"*"}},
				},
			},
		},
	}

	hook, err := NewMCPHook(m, registry)
	if err != nil {
		t.Fatalf("NewMCPHook: %v", err)
	}

	req := &LLMHookRequest{
		Context: &TurnContext{
			Inbound: &InboundContext{SenderID: "admin"},
		},
		Messages: []providers.Message{
			{Role: "user", Content: "hello"},
		},
	}

	newReq, decision, err := hook.BeforeLLM(context.Background(), req)
	if err != nil {
		t.Fatalf("BeforeLLM: %v", err)
	}

	if decision.Action != HookActionContinue {
		t.Errorf("decision.Action: want continue, got %s", decision.Action)
	}

	if len(newReq.Messages) < 2 {
		t.Error("should prepend context block")
	}

	if newReq.Messages[0].Role != "system" {
		t.Errorf("first message role: want system, got %s", newReq.Messages[0].Role)
	}

	if newReq.Messages[0].Content == "" {
		t.Error("context block should not be empty")
	}
}

func TestMCPHook_BeforeTool_Allowed(t *testing.T) {
	registry := NewRegistry()

	transport := NewMockTransport()
	transport.SetTools([]Tool{
		{Name: "test_tool", Description: "Test Tool"},
	})
	client := NewClient(transport)

	_, err := client.Initialize(context.Background(), "test-client", "1.0.0")
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	_, _ = client.ListTools(context.Background(), "")

	registry.mu.Lock()
	registry.clients["test-server"] = client
	registry.configs["test-server"] = manifest.MCPConfig{Name: "test-server"}
	registry.mu.Unlock()

	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "test-system",
			Version: "1.0.0",
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "*", Actions: []string{"*"}},
				},
			},
		},
	}

	hook, err := NewMCPHook(m, registry)
	if err != nil {
		t.Fatalf("NewMCPHook: %v", err)
	}

	call := &ToolCallHookRequest{
		Tool:      "test_tool",
		Arguments: map[string]any{"arg": "value"},
		Context:   &TurnContext{Inbound: &InboundContext{SenderID: "admin"}},
	}

	_, decision, err := hook.BeforeTool(context.Background(), call)
	if err != nil {
		t.Fatalf("BeforeTool: %v", err)
	}

	if decision.Action != HookActionContinue {
		t.Errorf("decision.Action: want continue, got %s", decision.Action)
	}
}

func TestMCPHook_BeforeTool_Denied(t *testing.T) {
	registry := NewRegistry()

	transport := NewMockTransport()
	transport.SetTools([]Tool{
		{Name: "test_tool", Description: "Test Tool"},
	})
	client := NewClient(transport)

	_, err := client.Initialize(context.Background(), "test-client", "1.0.0")
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	_, _ = client.ListTools(context.Background(), "")

	registry.mu.Lock()
	registry.clients["test-server"] = client
	registry.configs["test-server"] = manifest.MCPConfig{Name: "test-server"}
	registry.mu.Unlock()

	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "test-system",
			Version: "1.0.0",
		},
		Actors: []manifest.Actor{
			{
				ID:    "user",
				Roles: []string{"user"},
				Permissions: []manifest.Permission{
					{Resource: "read-only", Actions: []string{"read"}},
				},
			},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				DefaultDeny: true,
			},
		},
	}

	hook, err := NewMCPHook(m, registry)
	if err != nil {
		t.Fatalf("NewMCPHook: %v", err)
	}

	call := &ToolCallHookRequest{
		Tool:      "test_tool",
		Arguments: map[string]any{"arg": "value"},
		Context:   &TurnContext{Inbound: &InboundContext{SenderID: "user"}},
	}

	_, decision, err := hook.BeforeTool(context.Background(), call)
	if err != nil {
		t.Fatalf("BeforeTool: %v", err)
	}

	if decision.Action != HookActionDenyTool {
		t.Errorf("decision.Action: want deny_tool, got %s", decision.Action)
	}
}

func TestMCPHook_ApproveTool(t *testing.T) {
	registry := NewRegistry()

	transport := NewMockTransport()
	transport.SetTools([]Tool{
		{Name: "test_tool", Description: "Test Tool"},
	})
	client := NewClient(transport)

	_, err := client.Initialize(context.Background(), "test-client", "1.0.0")
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	_, _ = client.ListTools(context.Background(), "")

	registry.mu.Lock()
	registry.clients["test-server"] = client
	registry.configs["test-server"] = manifest.MCPConfig{Name: "test-server"}
	registry.mu.Unlock()

	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "test-system",
			Version: "1.0.0",
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "*", Actions: []string{"*"}},
				},
			},
		},
	}

	hook, err := NewMCPHook(m, registry)
	if err != nil {
		t.Fatalf("NewMCPHook: %v", err)
	}

	req := &ToolApprovalRequest{
		Tool:      "test_tool",
		Arguments: map[string]any{"arg": "value"},
		Context:   &TurnContext{Inbound: &InboundContext{SenderID: "admin"}},
	}

	decision, err := hook.ApproveTool(context.Background(), req)
	if err != nil {
		t.Fatalf("ApproveTool: %v", err)
	}

	if !decision.Approved {
		t.Error("admin should be approved for test_tool")
	}
}

func encodeToRawMessage(v interface{}) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
