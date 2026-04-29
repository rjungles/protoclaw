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

// OpenAIProvider implements the OpenAI API
type OpenAIProvider struct {
	*llm.BaseProvider
	client  *http.Client
	baseURL string
	apiKey  string
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(name string, config *llm.ProviderConfig) (llm.Provider, error) {
	base := llm.NewBaseProvider(name, llm.ProviderTypeOpenAI, config)

	apiKey, err := config.GetAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	baseURL := config.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return &OpenAIProvider{
		BaseProvider: base,
		client:       &http.Client{Timeout: 30 * time.Second},
		baseURL:      baseURL,
		apiKey:       apiKey,
	}, nil
}

// Complete generates a completion
func (p *OpenAIProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("OpenAI provider not configured")
	}

	url := p.baseURL + "/chat/completions"

	body := map[string]interface{}{
		"model":       req.Model,
		"messages":    req.Messages,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
		"stream":      false,
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
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

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
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	return &llm.CompletionResponse{
		ID:           result.ID,
		Model:        result.Model,
		Content:      result.Choices[0].Message.Content,
		FinishReason: result.Choices[0].FinishReason,
		Usage: llm.Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		},
		CreatedAt: time.Now(),
	}, nil
}

// Stream generates a streaming completion
func (p *OpenAIProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamResponse, error) {
	return nil, fmt.Errorf("streaming not implemented")
}

// Embed generates embeddings
func (p *OpenAIProvider) Embed(ctx context.Context, req llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, fmt.Errorf("embeddings not implemented")
}

// Register registers the OpenAI provider
func RegisterOpenAI(factory *llm.ProviderFactory) {
	factory.Register(llm.ProviderTypeOpenAI, func(name string, config *llm.ProviderConfig) (llm.Provider, error) {
		return NewOpenAIProvider(name, config)
	})
}
