package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "llm.yaml")

	configContent := `
version: "1.0"
system: "test-system"
settings:
  hot_reload: true
  reload_interval: 5
  provider_chain:
    timeout: 30
    max_retries: 2
    fallback: true
  default_routing:
    provider: "openai"
    model: "gpt-4"
    timeout: 30
    max_tokens: 4096
providers:
  - name: "openai"
    type: "openai"
    enabled: true
    priority: 1
    models:
      - id: "gpt-4"
        name: "GPT-4"
        max_tokens: 8192
agents: {}
routing:
  functions: {}
  intents: {}
  cost_based:
    enabled: false
  ab_testing:
    enabled: false
defaults:
  temperature: 0.7
  max_tokens: 2000
  timeout: 30s
  retry:
    max_attempts: 3
    backoff: exponential
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load the config
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify config
	if config.System != "test-system" {
		t.Errorf("Expected system 'test-system', got '%s'", config.System)
	}
	if config.Settings.HotReload != true {
		t.Error("Expected hot_reload to be true")
	}
	if len(config.Providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(config.Providers))
	}
	if config.Providers[0].Name != "openai" {
		t.Errorf("Expected provider name 'openai', got '%s'", config.Providers[0].Name)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				System: "test",
				Providers: []ProviderConfig{
					{
						Name: "openai",
						Type: ProviderTypeOpenAI,
						Models: []ModelConfig{
							{ID: "gpt-4", Name: "GPT-4", MaxTokens: 8192},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing system",
			config: Config{
				Providers: []ProviderConfig{
					{Name: "openai", Type: ProviderTypeOpenAI, Models: []ModelConfig{{ID: "gpt-4", Name: "GPT-4", MaxTokens: 8192}}},
				},
			},
			wantErr: true,
		},
		{
			name: "no providers",
			config: Config{
				System: "test",
			},
			wantErr: true,
		},
		{
			name: "provider without name",
			config: Config{
				System: "test",
				Providers: []ProviderConfig{
					{Type: ProviderTypeOpenAI, Models: []ModelConfig{{ID: "gpt-4", Name: "GPT-4", MaxTokens: 8192}}},
				},
			},
			wantErr: true,
		},
		{
			name: "provider without type",
			config: Config{
				System: "test",
				Providers: []ProviderConfig{
					{Name: "openai", Models: []ModelConfig{{ID: "gpt-4", Name: "GPT-4", MaxTokens: 8192}}},
				},
			},
			wantErr: true,
		},
		{
			name: "provider without models",
			config: Config{
				System: "test",
				Providers: []ProviderConfig{
					{Name: "openai", Type: ProviderTypeOpenAI},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestGetProvider(t *testing.T) {
	config := &Config{
		Providers: []ProviderConfig{
			{Name: "openai", Type: ProviderTypeOpenAI},
			{Name: "anthropic", Type: ProviderTypeAnthropic},
		},
	}

	provider := config.GetProvider("openai")
	if provider == nil {
		t.Error("Expected to find provider 'openai'")
	}
	if provider != nil && provider.Name != "openai" {
		t.Errorf("Expected provider name 'openai', got '%s'", provider.Name)
	}

	provider = config.GetProvider("nonexistent")
	if provider != nil {
		t.Error("Expected nil for nonexistent provider")
	}
}

func TestGetEnabledProviders(t *testing.T) {
	config := &Config{
		Providers: []ProviderConfig{
			{Name: "openai", Type: ProviderTypeOpenAI, Enabled: true},
			{Name: "anthropic", Type: ProviderTypeAnthropic, Enabled: false},
			{Name: "google", Type: ProviderTypeGoogle, Enabled: true},
		},
	}

	enabled := config.GetEnabledProviders()
	if len(enabled) != 2 {
		t.Errorf("Expected 2 enabled providers, got %d", len(enabled))
	}
}

func TestGetAgent(t *testing.T) {
	config := &Config{
		Agents: map[string]AgentConfig{
			"chat-agent": {
				Provider: "openai",
				Model:    "gpt-4",
			},
		},
	}

	agent := config.GetAgent("chat-agent")
	if agent == nil {
		t.Error("Expected to find agent 'chat-agent'")
	}
	if agent != nil && agent.Provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", agent.Provider)
	}

	agent = config.GetAgent("nonexistent")
	if agent != nil {
		t.Error("Expected nil for nonexistent agent")
	}
}

func TestProviderConfigGetAPIKey(t *testing.T) {
	// Test with API key in config
	pc := &ProviderConfig{
		Name: "test",
		Config: map[string]interface{}{
			"api_key": "test-key-123",
		},
	}

	key, err := pc.GetAPIKey()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if key != "test-key-123" {
		t.Errorf("Expected key 'test-key-123', got '%s'", key)
	}

	// Test with no API key
	pc2 := &ProviderConfig{
		Name:   "test",
		Config: map[string]interface{}{},
	}

	_, err = pc2.GetAPIKey()
	if err == nil {
		t.Error("Expected error for missing API key")
	}
}

func TestProviderConfigGetBaseURL(t *testing.T) {
	// Test with base URL
	pc := &ProviderConfig{
		Config: map[string]interface{}{
			"base_url": "https://api.example.com/v1",
		},
	}

	url := pc.GetBaseURL()
	if url != "https://api.example.com/v1" {
		t.Errorf("Expected URL 'https://api.example.com/v1', got '%s'", url)
	}

	// Test without base URL
	pc2 := &ProviderConfig{
		Config: map[string]interface{}{},
	}

	url = pc2.GetBaseURL()
	if url != "" {
		t.Errorf("Expected empty URL, got '%s'", url)
	}
}
