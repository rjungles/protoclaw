package llm

import (
	"context"
	"time"
)

// ProviderType represents the type of LLM provider
type ProviderType string

const (
	ProviderTypeOpenAI      ProviderType = "openai"
	ProviderTypeAnthropic ProviderType = "anthropic"
	ProviderTypeGoogle    ProviderType = "google"
	ProviderTypeStepfun   ProviderType = "stepfun"
	ProviderTypeNVIDIA    ProviderType = "nvidia"
	ProviderTypeLocal     ProviderType = "local"
	ProviderTypeCompatible ProviderType = "compatible"
)

// Message represents a message with role and content
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest represents a request to complete text
type CompletionRequest struct {
	Model       string `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int `json:"max_tokens,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	Stream      bool `json:"stream,omitempty"`
	System      string `json:"system,omitempty"`
	Function    string `json:"function,omitempty"` // Business function for routing
	Intent      string `json:"intent,omitempty"`   // Intent for routing
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CompletionResponse represents a response from an LLM
type CompletionResponse struct {
	ID         string        `json:"id"`
	Model      string        `json:"model"`
	Content    string        `json:"content"`
	Usage      Usage         `json:"usage"`
	FinishReason string      `json:"finish_reason"`
	CreatedAt  time.Time     `json:"created_at"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamResponse represents a streaming response chunk
type StreamResponse struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

// EmbeddingRequest represents a request to generate embeddings
type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// EmbeddingResponse represents an embedding response
type EmbeddingResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Usage      Usage       `json:"usage"`
}

// Provider represents an LLM provider interface
type Provider interface {
	// Type returns the provider type
	Type() ProviderType

	// Name returns the provider name
	Name() string

	// Complete generates a completion
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// Stream generates a streaming completion
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamResponse, error)

	// Embed generates embeddings
	Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error)

	// IsConfigured returns true if the provider is properly configured
	IsConfigured() bool

	// GetModels returns available models
	GetModels() []ModelInfo

	// Close closes the provider
	Close() error
}

// ModelInfo represents information about an LLM model
type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MaxTokens   int    `json:"max_tokens"`
	Description string `json:"description,omitempty"`
}

// AgentConfig represents LLM configuration for an agent
type AgentConfig struct {
	Provider       string                 `json:"provider"`
	Model          string                 `json:"model"`
	Temperature    float64                `json:"temperature"`
	MaxTokens      int                    `json:"max_tokens"`
	SystemPrompt   string                 `json:"system_prompt"`
	ProviderChain  []ProviderChainEntry   `json:"provider_chain,omitempty"`
	Capabilities   []string               `json:"capabilities"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// ProviderChainEntry represents a provider in a fallback chain
type ProviderChainEntry struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Timeout  float64 `json:"timeout"` // seconds
}

// RoutingRule represents a rule for routing LLM requests
type RoutingRule struct {
	Name        string  `json:"name"`
	Condition   string  `json:"condition,omitempty"`
	Action      RoutingAction `json:"action,omitempty"`
	Priority    int     `json:"priority,omitempty"`
	Enabled     bool    `json:"enabled,omitempty"`
	Provider    string  `json:"provider,omitempty"`
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	Timeout     int     `json:"timeout,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

// RoutingAction represents the action to take when a rule matches
type RoutingAction struct {
	Agent          string  `json:"agent,omitempty"`
	Provider       string  `json:"provider,omitempty"`
	Model          string  `json:"model,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
	MaxTokens      int     `json:"max_tokens,omitempty"`
}

// AlertConfig represents an alert configuration
type AlertConfig struct {
	Name      string      `json:"name"`
	Condition string      `json:"condition"`
	Action    AlertAction `json:"action"`
	Channels  []string    `json:"channels,omitempty"`
}

// AlertAction represents the action to take when an alert triggers
type AlertAction struct {
	Type     string                 `json:"type"` // notify, switch_model, etc
	Target   string                 `json:"target,omitempty"`
	Fallback string                 `json:"fallback,omitempty"`
	Message  string                 `json:"message,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// MediaConfig represents media processing configuration
type MediaConfig struct {
	Image  MediaTypeConfig `json:"image,omitempty"`
	Audio  MediaTypeConfig `json:"audio,omitempty"`
	Video  MediaTypeConfig `json:"video,omitempty"`
}

// MediaTypeConfig represents configuration for a media type
type MediaTypeConfig struct {
	Enabled   bool     `json:"enabled"`
	Providers []string `json:"providers,omitempty"`
	Formats   []string `json:"formats,omitempty"`
	MaxSize   string   `json:"max_size,omitempty"`
}
