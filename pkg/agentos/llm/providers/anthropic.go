package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sipeed/picoclaw/pkg/agentos/llm"
)

// AnthropicProvider implements the Anthropic Claude API
type AnthropicProvider struct {
	*llm.BaseProvider
	client  *http.Client
	baseURL string
	apiKey  string
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(name string, config *llm.ProviderConfig) (llm.Provider, error) {
	base := llm.NewBaseProvider(name, llm.ProviderTypeAnthropic, config)

	apiKey, err := config.GetAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	baseURL := config.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	return &AnthropicProvider{
		BaseProvider: base,
		client:       &http.Client{Timeout: 30 * time.Second},
		baseURL:      baseURL,
		apiKey:       apiKey,
	}, nil
}

// Complete generates a completion
func (p *AnthropicProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Anthropic provider not configured")
	}

	url := p.baseURL + "/messages"

	body := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   req.Messages,
	}

	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}

	if req.System != "" {
		body["system"] = req.System
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	content := ""
	for _, c := range result.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	return &llm.CompletionResponse{
		ID:           result.ID,
		Model:        result.Model,
		Content:      content,
		FinishReason: result.StopReason,
		Usage: llm.Usage{
			PromptTokens:     result.Usage.InputTokens,
			CompletionTokens: result.Usage.OutputTokens,
			TotalTokens:      result.Usage.InputTokens + result.Usage.OutputTokens,
		},
		CreatedAt: time.Now(),
	}, nil
}

// Stream generates a streaming completion
func (p *AnthropicProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamResponse, error) {
	return nil, fmt.Errorf("streaming not implemented")
}

// Embed generates embeddings
func (p *AnthropicProvider) Embed(ctx context.Context, req llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, fmt.Errorf("embeddings not implemented")
}

// RegisterAnthropic registers the Anthropic provider
func RegisterAnthropic(factory *llm.ProviderFactory) {
	factory.Register(llm.ProviderTypeAnthropic, func(name string, config *llm.ProviderConfig) (llm.Provider, error) {
		return NewAnthropicProvider(name, config)
	})
}
