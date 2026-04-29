package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLLMAgent(t *testing.T) {
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
agents:
  test-agent:
    provider: "mock"
    model: "test-model"
    temperature: 0.7
    max_tokens: 500
    system_prompt: "You are a test agent."
    capabilities:
      - chat
      - analyze
routing:
  functions: {}
  intents: {}
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	service, _ := NewService(configPath)
	defer service.Shutdown()

	agent, err := NewLLMAgent("test-agent", service)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	if agent.Name != "test-agent" {
		t.Errorf("Expected name 'test-agent', got '%s'", agent.Name)
	}
	if agent.SystemPrompt != "You are a test agent." {
		t.Errorf("Expected system prompt 'You are a test agent.', got '%s'", agent.SystemPrompt)
	}
	if agent.MaxMemory != 10 {
		t.Errorf("Expected max memory 10, got %d", agent.MaxMemory)
	}
}

func TestNewLLMAgentNotFound(t *testing.T) {
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

	_, err := NewLLMAgent("nonexistent-agent", service)
	if err == nil {
		t.Error("Expected error for nonexistent agent")
	}
}

func TestLLMAgentMemory(t *testing.T) {
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
agents:
  test-agent:
    provider: "mock"
    model: "test-model"
    temperature: 0.7
    max_tokens: 500
    system_prompt: "You are a test agent."
    capabilities:
      - chat
routing:
  functions: {}
  intents: {}
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	service, _ := NewService(configPath)
	defer service.Shutdown()

	agent, _ := NewLLMAgent("test-agent", service)

	// Test initial memory state
	if len(agent.GetMemory()) != 0 {
		t.Errorf("Expected empty memory, got %d items", len(agent.GetMemory()))
	}

	// Test setting max memory
	agent.SetMaxMemory(5)
	if agent.MaxMemory != 5 {
		t.Errorf("Expected max memory 5, got %d", agent.MaxMemory)
	}

	// Test clearing memory
	agent.Memory = []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}
	agent.ClearMemory()
	if len(agent.GetMemory()) != 0 {
		t.Errorf("Expected empty memory after clear, got %d items", len(agent.GetMemory()))
	}
}

func TestFormatParams(t *testing.T) {
	params := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}

	result := formatParams(params)
	if result == "" {
		t.Error("Expected non-empty result")
	}
	if result == "{}" {
		t.Error("Expected JSON output, got empty object")
	}
}

func TestFormatParamsNil(t *testing.T) {
	result := formatParams(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil params, got '%s'", result)
	}
}
