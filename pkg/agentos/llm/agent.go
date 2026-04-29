package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// LLMAgent represents an agent powered by LLM
type LLMAgent struct {
	Name         string
	SystemPrompt string
	Config       AgentConfig
	Service      *Service
	Memory       []Message
	MaxMemory    int
}

// NewLLMAgent creates a new LLM-powered agent
func NewLLMAgent(name string, service *Service) (*LLMAgent, error) {
	config := service.GetManager().GetConfig()

	agentConfig, ok := config.Agents[name]
	if !ok {
		return nil, fmt.Errorf("agent configuration not found: %s", name)
	}

	return &LLMAgent{
		Name:         name,
		SystemPrompt: agentConfig.SystemPrompt,
		Config:       agentConfig,
		Service:      service,
		Memory:       make([]Message, 0),
		MaxMemory:    10,
	}, nil
}

// Chat sends a message to the agent and gets a response
func (a *LLMAgent) Chat(ctx context.Context, message string) (string, error) {
	// Build messages with memory
	messages := a.buildMessages(message)

	// Create request
	req := CompletionRequest{
		Model:       a.Config.Model,
		Messages:    messages,
		System:      a.SystemPrompt,
		Temperature: a.Config.Temperature,
		MaxTokens:   a.Config.MaxTokens,
	}

	// Execute
	resp, err := a.Service.GetManager().Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("agent chat failed: %w", err)
	}

	// Update memory
	a.updateMemory(message, resp.Content)

	return resp.Content, nil
}

// ChatWithContext sends a message with additional context
func (a *LLMAgent) ChatWithContext(ctx context.Context, message string, context map[string]interface{}) (string, error) {
	// Enhance system prompt with context
	enhancedPrompt := a.enhancePromptWithContext(context)

	// Build messages with memory
	messages := a.buildMessages(message)

	// Create request
	req := CompletionRequest{
		Model:       a.Config.Model,
		Messages:    messages,
		System:      enhancedPrompt,
		Temperature: a.Config.Temperature,
		MaxTokens:   a.Config.MaxTokens,
	}

	// Execute
	resp, err := a.Service.GetManager().Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("agent chat failed: %w", err)
	}

	// Update memory
	a.updateMemory(message, resp.Content)

	return resp.Content, nil
}

// ExecuteTool executes a tool/function call
func (a *LLMAgent) ExecuteTool(ctx context.Context, toolName string, params map[string]interface{}) (string, error) {
	// Build tool execution prompt
	toolPrompt := fmt.Sprintf("Execute tool '%s' with parameters:\n%s", toolName, formatParams(params))

	// Add tool instructions to system prompt
	enhancedPrompt := a.SystemPrompt + "\n\nYou are executing a tool. Return only the result in the requested format."

	// Create request
	req := CompletionRequest{
		Model:       a.Config.Model,
		Messages:    []Message{{Role: "user", Content: toolPrompt}},
		System:      enhancedPrompt,
		Temperature: 0.1, // Low temperature for tool execution
		MaxTokens:   a.Config.MaxTokens,
	}

	// Execute
	resp, err := a.Service.GetManager().Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("tool execution failed: %w", err)
	}

	return resp.Content, nil
}

// Analyze analyzes content and extracts structured information
func (a *LLMAgent) Analyze(ctx context.Context, content string, schema map[string]interface{}) (map[string]interface{}, error) {
	// Build analysis prompt
	schemaJSON, _ := json.MarshalIndent(schema, "", "  ")
	analysisPrompt := fmt.Sprintf(`Analyze the following content and extract information according to this schema:
%s

Content to analyze:
%s

Return ONLY a valid JSON object matching the schema.`, string(schemaJSON), content)

	// Create request
	req := CompletionRequest{
		Model:       a.Config.Model,
		Messages:    []Message{{Role: "user", Content: analysisPrompt}},
		System:      "You are an analyzer. Extract structured data from content.",
		Temperature: 0.1,
		MaxTokens:   a.Config.MaxTokens,
	}

	// Execute
	resp, err := a.Service.GetManager().Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("analysis failed: %w", err)
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		// Return raw content if JSON parsing fails
		return map[string]interface{}{"raw": resp.Content}, nil
	}

	return result, nil
}

// Classify classifies content into categories
func (a *LLMAgent) Classify(ctx context.Context, content string, categories []string) (string, error) {
	// Build classification prompt
	categoriesJSON, _ := json.Marshal(categories)
	classifyPrompt := fmt.Sprintf(`Classify the following content into one of these categories: %s

Content:
%s

Return ONLY the category name.`, string(categoriesJSON), content)

	// Create request
	req := CompletionRequest{
		Model:       a.Config.Model,
		Messages:    []Message{{Role: "user", Content: classifyPrompt}},
		System:      "You are a classifier. Classify content accurately.",
		Temperature: 0.1,
		MaxTokens:   50,
	}

	// Execute
	resp, err := a.Service.GetManager().Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("classification failed: %w", err)
	}

	return resp.Content, nil
}

// Summarize creates a summary of content
func (a *LLMAgent) Summarize(ctx context.Context, content string, maxLength int) (string, error) {
	summaryPrompt := fmt.Sprintf(`Summarize the following content in %d words or less:

%s`, maxLength, content)

	req := CompletionRequest{
		Model:       a.Config.Model,
		Messages:    []Message{{Role: "user", Content: summaryPrompt}},
		System:      "You are a summarizer. Create concise summaries.",
		Temperature: 0.3,
		MaxTokens:   maxLength * 2,
	}

	resp, err := a.Service.GetManager().Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("summarization failed: %w", err)
	}

	return resp.Content, nil
}

// Translate translates content to a target language
func (a *LLMAgent) Translate(ctx context.Context, content string, targetLanguage string) (string, error) {
	translatePrompt := fmt.Sprintf(`Translate the following to %s:

%s`, targetLanguage, content)

	req := CompletionRequest{
		Model:       a.Config.Model,
		Messages:    []Message{{Role: "user", Content: translatePrompt}},
		System:      fmt.Sprintf("You are a translator. Translate accurately to %s.", targetLanguage),
		Temperature: 0.3,
		MaxTokens:   a.Config.MaxTokens,
	}

	resp, err := a.Service.GetManager().Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("translation failed: %w", err)
	}

	return resp.Content, nil
}

// ClearMemory clears the agent's memory
func (a *LLMAgent) ClearMemory() {
	a.Memory = make([]Message, 0)
}

// GetMemory returns the agent's current memory
func (a *LLMAgent) GetMemory() []Message {
	result := make([]Message, len(a.Memory))
	copy(result, a.Memory)
	return result
}

// SetMaxMemory sets the maximum memory size
func (a *LLMAgent) SetMaxMemory(size int) {
	a.MaxMemory = size
	// Trim if needed
	if len(a.Memory) > a.MaxMemory {
		a.Memory = a.Memory[len(a.Memory)-a.MaxMemory:]
	}
}

// buildMessages builds the message list including memory
func (a *LLMAgent) buildMessages(newMessage string) []Message {
	messages := make([]Message, 0, len(a.Memory)+1)
	messages = append(messages, a.Memory...)
	messages = append(messages, Message{Role: "user", Content: newMessage})
	return messages
}

// updateMemory adds new exchange to memory
func (a *LLMAgent) updateMemory(userMsg, assistantMsg string) {
	a.Memory = append(a.Memory, Message{Role: "user", Content: userMsg})
	a.Memory = append(a.Memory, Message{Role: "assistant", Content: assistantMsg})

	// Trim if exceeds max
	if len(a.Memory) > a.MaxMemory*2 {
		a.Memory = a.Memory[len(a.Memory)-a.MaxMemory*2:]
	}
}

// enhancePromptWithContext enhances system prompt with context
func (a *LLMAgent) enhancePromptWithContext(ctx map[string]interface{}) string {
	if ctx == nil || len(ctx) == 0 {
		return a.SystemPrompt
	}

	var additions []string
	for key, value := range ctx {
		additions = append(additions, fmt.Sprintf("%s: %v", key, value))
	}

	return a.SystemPrompt + "\n\nContext:\n" + formatParams(ctx)
}

// formatParams formats parameters for display
func formatParams(params map[string]interface{}) string {
	if params == nil {
		return ""
	}

	result, _ := json.MarshalIndent(params, "", "  ")
	return string(result)
}
