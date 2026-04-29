package llm

import (
	"context"
	"testing"
	"time"
)

func TestMessage(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "Hello",
	}

	if msg.Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", msg.Role)
	}
	if msg.Content != "Hello" {
		t.Errorf("Expected content 'Hello', got '%s'", msg.Content)
	}
}

func TestCompletionRequest(t *testing.T) {
	req := CompletionRequest{
		Model:       "gpt-4",
		Messages:    []Message{{Role: "user", Content: "Test"}},
		Temperature: 0.7,
		MaxTokens:   100,
		TopP:        0.9,
		Stream:      false,
		System:      "You are helpful",
		Function:    "chat",
		Intent:      "conversation",
	}

	if req.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", req.Model)
	}
	if req.Temperature != 0.7 {
		t.Errorf("Expected temperature 0.7, got %f", req.Temperature)
	}
	if req.Function != "chat" {
		t.Errorf("Expected function 'chat', got '%s'", req.Function)
	}
}

func TestCompletionResponse(t *testing.T) {
	resp := CompletionResponse{
		ID:           "test-id",
		Model:        "gpt-4",
		Content:      "Hello world",
		Usage:        Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
		FinishReason: "stop",
		CreatedAt:    time.Now(),
	}

	if resp.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", resp.ID)
	}
	if resp.Usage.TotalTokens != 30 {
		t.Errorf("Expected total tokens 30, got %d", resp.Usage.TotalTokens)
	}
}

func TestUsage(t *testing.T) {
	usage := Usage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
	}

	if usage.PromptTokens != 10 {
		t.Errorf("Expected prompt tokens 10, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 20 {
		t.Errorf("Expected completion tokens 20, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 30 {
		t.Errorf("Expected total tokens 30, got %d", usage.TotalTokens)
	}
}

func TestStreamResponse(t *testing.T) {
	resp := StreamResponse{
		ID:      "stream-id",
		Content: "Hello",
		Done:    false,
	}

	if resp.ID != "stream-id" {
		t.Errorf("Expected ID 'stream-id', got '%s'", resp.ID)
	}
	if resp.Done {
		t.Error("Expected Done to be false")
	}
}

func TestEmbeddingRequest(t *testing.T) {
	req := EmbeddingRequest{
		Model: "text-embedding-ada-002",
		Input: []string{"Hello", "World"},
	}

	if req.Model != "text-embedding-ada-002" {
		t.Errorf("Expected model 'text-embedding-ada-002', got '%s'", req.Model)
	}
	if len(req.Input) != 2 {
		t.Errorf("Expected 2 inputs, got %d", len(req.Input))
	}
}

func TestEmbeddingResponse(t *testing.T) {
	resp := EmbeddingResponse{
		Embeddings: [][]float64{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}},
		Usage:      Usage{TotalTokens: 10},
	}

	if len(resp.Embeddings) != 2 {
		t.Errorf("Expected 2 embeddings, got %d", len(resp.Embeddings))
	}
	if len(resp.Embeddings[0]) != 3 {
		t.Errorf("Expected 3 dimensions, got %d", len(resp.Embeddings[0]))
	}
}

func TestModelInfo(t *testing.T) {
	info := ModelInfo{
		ID:          "gpt-4",
		Name:        "GPT-4",
		MaxTokens:   8192,
		Description: "Most capable model",
	}

	if info.ID != "gpt-4" {
		t.Errorf("Expected ID 'gpt-4', got '%s'", info.ID)
	}
	if info.MaxTokens != 8192 {
		t.Errorf("Expected max tokens 8192, got %d", info.MaxTokens)
	}
}

func TestAgentConfig(t *testing.T) {
	config := AgentConfig{
		Provider:     "openai",
		Model:        "gpt-4",
		Temperature:  0.7,
		MaxTokens:    2000,
		SystemPrompt: "You are helpful",
		Capabilities: []string{"text_generation", "code"},
	}

	if config.Provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", config.Provider)
	}
	if len(config.Capabilities) != 2 {
		t.Errorf("Expected 2 capabilities, got %d", len(config.Capabilities))
	}
}

func TestProviderChainEntry(t *testing.T) {
	entry := ProviderChainEntry{
		Provider: "openai",
		Model:    "gpt-4",
		Timeout:  30,
	}

	if entry.Provider != "openai" {
		t.Errorf("Expected provider 'openai', got '%s'", entry.Provider)
	}
	if entry.Timeout != 30 {
		t.Errorf("Expected timeout 30, got %f", entry.Timeout)
	}
}

func TestRoutingRule(t *testing.T) {
	rule := RoutingRule{
		Name:        "code-review",
		Condition:   "intent == 'code'",
		Priority:    1,
		Enabled:     true,
		Provider:    "anthropic",
		Model:       "claude-3",
		Temperature: 0.3,
	}

	if rule.Name != "code-review" {
		t.Errorf("Expected name 'code-review', got '%s'", rule.Name)
	}
	if rule.Provider != "anthropic" {
		t.Errorf("Expected provider 'anthropic', got '%s'", rule.Provider)
	}
}

func TestAlertConfig(t *testing.T) {
	alert := AlertConfig{
		Name:     "high-latency",
		Condition: "latency > 1000",
		Channels: []string{"slack", "email"},
	}

	if alert.Name != "high-latency" {
		t.Errorf("Expected name 'high-latency', got '%s'", alert.Name)
	}
	if len(alert.Channels) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(alert.Channels))
	}
}

func TestProviderType(t *testing.T) {
	tests := []struct {
		providerType ProviderType
		expected     string
	}{
		{ProviderTypeOpenAI, "openai"},
		{ProviderTypeAnthropic, "anthropic"},
		{ProviderTypeGoogle, "google"},
		{ProviderTypeStepfun, "stepfun"},
		{ProviderTypeNVIDIA, "nvidia"},
		{ProviderTypeLocal, "local"},
		{ProviderTypeCompatible, "compatible"},
	}

	for _, tt := range tests {
		if string(tt.providerType) != tt.expected {
			t.Errorf("Expected type '%s', got '%s'", tt.expected, tt.providerType)
		}
	}
}

// MockProvider is a mock implementation for testing
type MockProvider struct {
	*BaseProvider
	CompleteFunc func(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	StreamFunc   func(ctx context.Context, req CompletionRequest) (<-chan StreamResponse, error)
	EmbedFunc    func(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error)
}

func (m *MockProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, req)
	}
	return &CompletionResponse{ID: "mock-id", Content: "mock response"}, nil
}

func (m *MockProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamResponse, error) {
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, req)
	}
	return nil, nil
}

func (m *MockProvider) Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	if m.EmbedFunc != nil {
		return m.EmbedFunc(ctx, req)
	}
	return &EmbeddingResponse{Embeddings: [][]float64{{0.1, 0.2}}}, nil
}

func TestMockProvider(t *testing.T) {
	base := NewBaseProvider("mock", ProviderTypeLocal, &ProviderConfig{
		Name:    "mock",
		Type:    ProviderTypeLocal,
		Enabled: true,
	})

	mock := &MockProvider{BaseProvider: base}

	if mock.Name() != "mock" {
		t.Errorf("Expected name 'mock', got '%s'", mock.Name())
	}
	if mock.Type() != ProviderTypeLocal {
		t.Errorf("Expected type 'local', got '%s'", mock.Type())
	}
	if !mock.IsConfigured() {
		t.Error("Expected provider to be configured")
	}
}
