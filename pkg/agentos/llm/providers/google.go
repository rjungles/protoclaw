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

// GoogleProvider implements the Google Vertex AI / Gemini API
type GoogleProvider struct {
	*llm.BaseProvider
	client    *http.Client
	baseURL   string
	apiKey    string
	projectID string
}

// NewGoogleProvider creates a new Google provider
func NewGoogleProvider(name string, config *llm.ProviderConfig) (llm.Provider, error) {
	base := llm.NewBaseProvider(name, llm.ProviderTypeGoogle, config)

	apiKey, err := config.GetAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	baseURL := config.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1"
	}

	return &GoogleProvider{
		BaseProvider: base,
		client:       &http.Client{Timeout: 30 * time.Second},
		baseURL:      baseURL,
		apiKey:       apiKey,
	}, nil
}

// Complete generates a completion using Google Gemini API
func (p *GoogleProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Google provider not configured")
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.baseURL, req.Model, p.apiKey)

	// Build contents
	contents := []map[string]interface{}{}
	for _, msg := range req.Messages {
		contents = append(contents, map[string]interface{}{
			"role": msg.Role,
			"parts": []map[string]string{
				{"text": msg.Content},
			},
		})
	}

	body := map[string]interface{}{
		"contents": contents,
		"generationConfig": map[string]interface{}{
			"temperature":     req.Temperature,
			"maxOutputTokens": req.MaxTokens,
			"topP":            req.TopP,
		},
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
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates returned")
	}

	content := ""
	for _, part := range result.Candidates[0].Content.Parts {
		content += part.Text
	}

	return &llm.CompletionResponse{
		ID:           fmt.Sprintf("gemini-%d", time.Now().Unix()),
		Model:        req.Model,
		Content:      content,
		FinishReason: result.Candidates[0].FinishReason,
		Usage: llm.Usage{
			PromptTokens:     result.UsageMetadata.PromptTokenCount,
			CompletionTokens: result.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      result.UsageMetadata.TotalTokenCount,
		},
		CreatedAt: time.Now(),
	}, nil
}

// Stream generates a streaming completion
func (p *GoogleProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamResponse, error) {
	return nil, fmt.Errorf("streaming not implemented")
}

// Embed generates embeddings
func (p *GoogleProvider) Embed(ctx context.Context, req llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return nil, fmt.Errorf("embeddings not implemented")
}

// RegisterGoogle registers the Google provider
func RegisterGoogle(factory *llm.ProviderFactory) {
	factory.Register(llm.ProviderTypeGoogle, func(name string, config *llm.ProviderConfig) (llm.Provider, error) {
		return NewGoogleProvider(name, config)
	})
}
