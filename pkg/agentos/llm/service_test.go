package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewService(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "llm.yaml")

	// Create a minimal config
	configContent := `
version: "1.0"
system: "test"
settings:
  hot_reload: false
  provider_chain:
    timeout: 30
    max_retries: 2
    fallback: false
  default_routing:
    provider: "mock"
    model: "test"
providers:
  - name: "mock"
    type: "local"
    enabled: true
    models:
      - id: "test-model"
        name: "Test Model"
        max_tokens: 1000
agents: {}
routing:
  functions: {}
  intents: {}
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	service, err := NewService(configPath)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.GetManager() == nil {
		t.Error("Expected manager to be initialized")
	}

	service.Shutdown()
}

func TestServiceRegisterFunction(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "llm.yaml")

	configContent := `
version: "1.0"
system: "test"
settings:
  hot_reload: false
  provider_chain:
    timeout: 30
    max_retries: 2
    fallback: false
  default_routing:
    provider: "mock"
    model: "test"
providers:
  - name: "mock"
    type: "local"
    enabled: true
    models:
      - id: "test-model"
        name: "Test Model"
        max_tokens: 1000
agents: {}
routing:
  functions: {}
  intents: {}
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	service, _ := NewService(configPath)
	defer service.Shutdown()

	// Register a custom function handler
	
	service.RegisterFunctionFunc("custom-function", func(ctx context.Context, req FunctionRequest) (*FunctionResponse, error) {
		_ = ctx
		return &FunctionResponse{
			Output:   "custom output",
			Function: req.Function,
		}, nil
	})

	// Verify function is registered
	functions := service.ListFunctions()
	found := false
	for _, f := range functions {
		if f == "custom-function" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'custom-function' to be registered")
	}
}

func TestServiceRegisterAgent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "llm.yaml")

	configContent := `
version: "1.0"
system: "test"
settings:
  hot_reload: false
  provider_chain:
    timeout: 30
    max_retries: 2
    fallback: false
  default_routing:
    provider: "mock"
    model: "test"
providers:
  - name: "mock"
    type: "local"
    enabled: true
    models:
      - id: "test-model"
        name: "Test Model"
        max_tokens: 1000
agents: {}
routing:
  functions: {}
  intents: {}
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	service, _ := NewService(configPath)
	defer service.Shutdown()

	// Register a custom agent handler
	service.RegisterAgentFunc("custom-agent", func(ctx context.Context, input string, context map[string]interface{}) (*AgentResponse, error) {
		return &AgentResponse{
			Response: "custom response",
			Agent:    "custom-agent",
		}, nil
	})

	// Verify agent is registered
	agents := service.ListAgents()
	found := false
	for _, a := range agents {
		if a == "custom-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'custom-agent' to be registered")
	}
}

func TestFunctionRequest(t *testing.T) {
	req := FunctionRequest{
		Function:  "test-function",
		Input:     "test input",
		UserID:    "user-123",
		SessionID: "session-456",
		Metadata: map[string]interface{}{
			"key": "value",
		},
	}

	if req.Function != "test-function" {
		t.Errorf("Expected function 'test-function', got '%s'", req.Function)
	}
	if req.UserID != "user-123" {
		t.Errorf("Expected user ID 'user-123', got '%s'", req.UserID)
	}
}

func TestFunctionResponse(t *testing.T) {
	resp := FunctionResponse{
		Output:   "test output",
		Function: "test-function",
		Model:    "gpt-4",
		Provider: "openai",
		Usage:    Usage{TotalTokens: 100},
		Metadata: map[string]interface{}{"key": "value"},
	}

	if resp.Output != "test output" {
		t.Errorf("Expected output 'test output', got '%s'", resp.Output)
	}
	if resp.Usage.TotalTokens != 100 {
		t.Errorf("Expected 100 tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestAgentResponse(t *testing.T) {
	resp := AgentResponse{
		Response: "test response",
		Agent:    "test-agent",
		Model:    "claude",
		Actions: []AgentAction{
			{Type: "notify", Target: "user", Payload: map[string]interface{}{"message": "hello"}},
		},
	}

	if resp.Response != "test response" {
		t.Errorf("Expected response 'test response', got '%s'", resp.Response)
	}
	if len(resp.Actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(resp.Actions))
	}
}

func TestAgentAction(t *testing.T) {
	action := AgentAction{
		Type:   "send_email",
		Target: "user@example.com",
		Payload: map[string]interface{}{
			"subject": "Test",
			"body":    "Hello",
		},
	}

	if action.Type != "send_email" {
		t.Errorf("Expected type 'send_email', got '%s'", action.Type)
	}
	if action.Target != "user@example.com" {
		t.Errorf("Expected target 'user@example.com', got '%s'", action.Target)
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "llm.yaml")

	configContent := `
version: "1.0"
system: "test"
settings:
  hot_reload: false
  provider_chain:
    timeout: 30
    max_retries: 2
    fallback: false
  default_routing:
    provider: "mock"
    model: "test"
providers:
  - name: "mock"
    type: "local"
    enabled: true
    models:
      - id: "test-model"
        name: "Test Model"
        max_tokens: 1000
agents: {}
routing:
  functions: {}
  intents: {}
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	service, _ := NewService(configPath)
	defer service.Shutdown()

	// Test various function prompts
	tests := []struct {
		function string
		context  map[string]interface{}
		contains string
	}{
		{"code-review", nil, "code reviewer"},
		{"summarize", nil, "summarizer"},
		{"translate", nil, "translator"},
		{"classify", nil, "classifier"},
		{"generate", nil, "content generator"},
		{"analyze", nil, "analyzer"},
		{"unknown", nil, "helpful assistant"},
		{"custom", map[string]interface{}{"style": "formal"}, "Style: formal"},
		{"custom", map[string]interface{}{"format": "json"}, "Format: json"},
		{"custom", map[string]interface{}{"language": "pt"}, "Language: pt"},
	}

	for _, tt := range tests {
		prompt := service.buildSystemPrompt(tt.function, tt.context)
		if prompt == "" {
			t.Errorf("Expected non-empty prompt for function '%s'", tt.function)
		}
		if tt.contains != "" && !contains(prompt, tt.contains) {
			t.Errorf("Expected prompt for '%s' to contain '%s', got:\n%s", tt.function, tt.contains, prompt)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
