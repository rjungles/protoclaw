package providers

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/agentos/llm"
)

func TestNewProviderFactory(t *testing.T) {
	factory := llm.NewProviderFactory()
	if factory == nil {
		t.Fatal("Expected factory to be created")
	}
}

func TestProviderFactoryRegisterAndCreate(t *testing.T) {
	factory := llm.NewProviderFactory()

	// Register a mock provider
	factory.Register(llm.ProviderTypeLocal, func(name string, config *llm.ProviderConfig) (llm.Provider, error) {
		base := llm.NewBaseProvider(name, llm.ProviderTypeLocal, config)
		return &MockProvider{BaseProvider: base}, nil
	})

	// Create a provider
	config := &llm.ProviderConfig{
		Name:    "test",
		Type:    llm.ProviderTypeLocal,
		Enabled: true,
	}

	provider, err := factory.Create(llm.ProviderTypeLocal, "test", config)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.Name() != "test" {
		t.Errorf("Expected name 'test', got '%s'", provider.Name())
	}
	if provider.Type() != llm.ProviderTypeLocal {
		t.Errorf("Expected type 'local', got '%s'", provider.Type())
	}
}

func TestProviderFactoryUnknownType(t *testing.T) {
	factory := llm.NewProviderFactory()

	config := &llm.ProviderConfig{
		Name: "test",
		Type: llm.ProviderType("unknown"),
	}

	_, err := factory.Create(llm.ProviderType("unknown"), "test", config)
	if err == nil {
		t.Error("Expected error for unknown provider type")
	}
}

func TestInit(t *testing.T) {
	factory := llm.NewProviderFactory()
	Init(factory)

	// All providers should be registered
	providerTypes := []llm.ProviderType{
		llm.ProviderTypeOpenAI,
		llm.ProviderTypeAnthropic,
		llm.ProviderTypeGoogle,
		llm.ProviderTypeNVIDIA,
		llm.ProviderTypeStepfun,
		llm.ProviderTypeCompatible,
	}

	for _, pt := range providerTypes {
		// Try to create - should not fail with "unknown provider type"
		config := &llm.ProviderConfig{
			Name: "test",
			Type: pt,
			Config: map[string]interface{}{
				"base_url": "https://api.test.com/v1",
				"api_key":  "test-key",
			},
		}
		_, err := factory.Create(pt, "test", config)
		// May fail for other reasons (like missing API key in env), but not "unknown type"
		if err != nil && err.Error() == "unknown provider type: "+string(pt) {
			t.Errorf("Provider type %s not registered", pt)
		}
	}
}

func TestNewOpenAIProvider(t *testing.T) {
	config := &llm.ProviderConfig{
		Name: "openai-test",
		Type: llm.ProviderTypeOpenAI,
		Config: map[string]interface{}{
			"base_url": "https://api.openai.com/v1",
			"api_key":  "test-key",
		},
		Enabled: true,
	}

	provider, err := NewOpenAIProvider("openai-test", config)
	if err != nil {
		t.Fatalf("Failed to create OpenAI provider: %v", err)
	}

	if provider.Name() != "openai-test" {
		t.Errorf("Expected name 'openai-test', got '%s'", provider.Name())
	}
	if provider.Type() != llm.ProviderTypeOpenAI {
		t.Errorf("Expected type 'openai', got '%s'", provider.Type())
	}
	if !provider.IsConfigured() {
		t.Error("Expected provider to be configured")
	}
}

func TestNewCompatibleProvider(t *testing.T) {
	config := &llm.ProviderConfig{
		Name: "groq",
		Type: llm.ProviderTypeCompatible,
		Config: map[string]interface{}{
			"base_url": "https://api.groq.com/openai/v1",
			"api_key":  "test-key",
		},
		Enabled: true,
	}

	provider, err := NewCompatibleProvider("groq", config)
	if err != nil {
		t.Fatalf("Failed to create compatible provider: %v", err)
	}

	if provider.Name() != "groq" {
		t.Errorf("Expected name 'groq', got '%s'", provider.Name())
	}
}

func TestNewCompatibleProviderMissingBaseURL(t *testing.T) {
	config := &llm.ProviderConfig{
		Name:    "test",
		Type:    llm.ProviderTypeCompatible,
		Enabled: true,
		Config:  map[string]interface{}{},
	}

	_, err := NewCompatibleProvider("test", config)
	if err == nil {
		t.Error("Expected error for missing base URL")
	}
}

// MockProvider for testing
type MockProvider struct {
	*llm.BaseProvider
}

func (m *MockProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{ID: "mock", Content: "mock response"}, nil
}

func (m *MockProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamResponse, error) {
	return nil, nil
}

func (m *MockProvider) Embed(ctx context.Context, req llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return &llm.EmbeddingResponse{Embeddings: [][]float64{{0.1, 0.2}}}, nil
}
