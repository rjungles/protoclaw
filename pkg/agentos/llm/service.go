package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Service provides LLM capabilities to business functions
type Service struct {
	manager     *Manager
	functionMap map[string]FunctionHandler
	agentMap    map[string]AgentHandler
	mu          sync.RWMutex
}

// FunctionHandler handles LLM requests for a business function
type FunctionHandler interface {
	Handle(ctx context.Context, req FunctionRequest) (*FunctionResponse, error)
}

// FunctionHandlerFunc is a function type that implements FunctionHandler
type FunctionHandlerFunc func(ctx context.Context, req FunctionRequest) (*FunctionResponse, error)

// Handle implements FunctionHandler
func (f FunctionHandlerFunc) Handle(ctx context.Context, req FunctionRequest) (*FunctionResponse, error) {
	return f(ctx, req)
}

// AgentHandler handles LLM requests for an agent
type AgentHandler interface {
	Process(ctx context.Context, input string, context map[string]interface{}) (*AgentResponse, error)
}

// AgentHandlerFunc is a function type that implements AgentHandler
type AgentHandlerFunc func(ctx context.Context, input string, context map[string]interface{}) (*AgentResponse, error)

// Process implements AgentHandler
func (a AgentHandlerFunc) Process(ctx context.Context, input string, context map[string]interface{}) (*AgentResponse, error) {
	return a(ctx, input, context)
}

// FunctionRequest represents a request to a business function
type FunctionRequest struct {
	Function    string                 `json:"function"`
	Input       string                 `json:"input"`
	Context     map[string]interface{} `json:"context,omitempty"`
	UserID      string                 `json:"user_id,omitempty"`
	SessionID   string                 `json:"session_id,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// FunctionResponse represents a response from a business function
type FunctionResponse struct {
	Output      string                 `json:"output"`
	Function    string                 `json:"function"`
	Model       string                 `json:"model"`
	Provider    string                 `json:"provider"`
	Usage       Usage                  `json:"usage"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CompletedAt string                 `json:"completed_at"`
}

// AgentResponse represents a response from an agent
type AgentResponse struct {
	Response    string                 `json:"response"`
	Agent       string                 `json:"agent"`
	Model       string                 `json:"model"`
	Provider    string                 `json:"provider"`
	Usage       Usage                  `json:"usage"`
	Actions     []AgentAction          `json:"actions,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// AgentAction represents an action taken by an agent
type AgentAction struct {
	Type    string                 `json:"type"`
	Target  string                 `json:"target"`
	Payload map[string]interface{} `json:"payload"`
}

// NewService creates a new LLM service
func NewService(configPath string) (*Service, error) {
	manager := NewManager(configPath)
	if err := manager.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize LLM manager: %w", err)
	}

	return &Service{
		manager:     manager,
		functionMap: make(map[string]FunctionHandler),
		agentMap:    make(map[string]AgentHandler),
	}, nil
}

// RegisterFunction registers a handler for a business function
func (s *Service) RegisterFunction(name string, handler FunctionHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.functionMap[name] = handler
}

// RegisterFunctionFunc registers a function handler for a business function
func (s *Service) RegisterFunctionFunc(name string, handler func(ctx context.Context, req FunctionRequest) (*FunctionResponse, error)) {
	s.RegisterFunction(name, FunctionHandlerFunc(handler))
}

// RegisterAgent registers a handler for an agent
func (s *Service) RegisterAgent(name string, handler AgentHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentMap[name] = handler
}

// RegisterAgentFunc registers a function handler for an agent
func (s *Service) RegisterAgentFunc(name string, handler func(ctx context.Context, input string, context map[string]interface{}) (*AgentResponse, error)) {
	s.RegisterAgent(name, AgentHandlerFunc(handler))
}

// ExecuteFunction executes an LLM request for a business function
func (s *Service) ExecuteFunction(ctx context.Context, req FunctionRequest) (*FunctionResponse, error) {
	s.mu.RLock()
	handler, ok := s.functionMap[req.Function]
	s.mu.RUnlock()

	if ok {
		// Use custom handler if registered
		return handler.Handle(ctx, req)
	}

	// Default implementation using LLM manager
	return s.executeWithLLM(ctx, req)
}

// executeWithLLM executes a function using the LLM manager
func (s *Service) executeWithLLM(ctx context.Context, req FunctionRequest) (*FunctionResponse, error) {
	// Get routing rule for this function
	rule := s.getRoutingRule(req.Function)

	// Build system prompt based on function
	systemPrompt := s.buildSystemPrompt(req.Function, req.Context)

	// Build completion request
	llmReq := CompletionRequest{
		Model:       rule.Model,
		Messages:    []Message{{Role: "user", Content: req.Input}},
		System:      systemPrompt,
		Temperature: rule.Temperature,
		MaxTokens:   rule.MaxTokens,
		Function:    req.Function,
	}

	// Execute via LLM manager
	resp, err := s.manager.Complete(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("LLM execution failed: %w", err)
	}

	return &FunctionResponse{
		Output:   resp.Content,
		Function: req.Function,
		Model:    resp.Model,
		Provider: rule.Provider,
		Usage:    resp.Usage,
		Metadata: map[string]interface{}{
			"finish_reason": resp.FinishReason,
		},
		CompletedAt: resp.CreatedAt.String(),
	}, nil
}

// getRoutingRule gets the routing rule for a function
func (s *Service) getRoutingRule(function string) RoutingRule {
	config := s.manager.GetConfig()

	// Check function-specific routing
	if rule, ok := config.Routing.Functions[function]; ok {
		return rule
	}

	// Return default routing
	return config.Settings.DefaultRouting
}

// buildSystemPrompt builds a system prompt based on function and context
func (s *Service) buildSystemPrompt(function string, context map[string]interface{}) string {
	var parts []string

	// Base instruction based on function
	switch function {
	case "code-review":
		parts = append(parts, "You are a code reviewer. Analyze the provided code and give constructive feedback.")
	case "summarize":
		parts = append(parts, "You are a text summarizer. Create a concise summary of the provided text.")
	case "translate":
		parts = append(parts, "You are a translator. Translate the provided text accurately.")
	case "classify":
		parts = append(parts, "You are a classifier. Categorize the provided content into appropriate categories.")
	case "generate":
		parts = append(parts, "You are a content generator. Create content based on the provided requirements.")
	case "analyze":
		parts = append(parts, "You are an analyzer. Analyze the provided data and extract insights.")
	default:
		parts = append(parts, "You are a helpful assistant.")
	}

	// Add context-based instructions
	if context != nil {
		if style, ok := context["style"]; ok {
			parts = append(parts, fmt.Sprintf("Style: %v", style))
		}
		if format, ok := context["format"]; ok {
			parts = append(parts, fmt.Sprintf("Format: %v", format))
		}
		if tone, ok := context["tone"]; ok {
			parts = append(parts, fmt.Sprintf("Tone: %v", tone))
		}
		if language, ok := context["language"]; ok {
			parts = append(parts, fmt.Sprintf("Language: %v", language))
		}
	}

	return strings.Join(parts, "\n")
}

// ProcessAgent processes input through an agent
func (s *Service) ProcessAgent(ctx context.Context, agentName string, input string, context map[string]interface{}) (*AgentResponse, error) {
	s.mu.RLock()
	handler, ok := s.agentMap[agentName]
	s.mu.RUnlock()

	if ok {
		// Use custom handler if registered
		return handler.Process(ctx, input, context)
	}

	// Default implementation using agent configuration
	return s.processWithAgentConfig(ctx, agentName, input, context)
}

// processWithAgentConfig processes using agent configuration
func (s *Service) processWithAgentConfig(ctx context.Context, agentName string, input string, context map[string]interface{}) (*AgentResponse, error) {
	config := s.manager.GetConfig()

	// Get agent configuration
	agentConfig, ok := config.Agents[agentName]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentName)
	}

	// Build completion request
	llmReq := CompletionRequest{
		Model:       agentConfig.Model,
		Messages:    []Message{{Role: "user", Content: input}},
		System:      agentConfig.SystemPrompt,
		Temperature: agentConfig.Temperature,
		MaxTokens:   agentConfig.MaxTokens,
	}

	// Execute via LLM manager
	resp, err := s.manager.Complete(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	return &AgentResponse{
		Response: resp.Content,
		Agent:    agentName,
		Model:    resp.Model,
		Provider: agentConfig.Provider,
		Usage:    resp.Usage,
		Metadata: map[string]interface{}{
			"capabilities":  agentConfig.Capabilities,
			"finish_reason": resp.FinishReason,
		},
	}, nil
}

// ListFunctions returns all registered function names
func (s *Service) ListFunctions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.functionMap))
	for name := range s.functionMap {
		names = append(names, name)
	}
	return names
}

// ListAgents returns all registered agent names
func (s *Service) ListAgents() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.agentMap))
	for name := range s.agentMap {
		names = append(names, name)
	}
	return names
}

// GetManager returns the underlying LLM manager
func (s *Service) GetManager() *Manager {
	return s.manager
}

// Shutdown stops the service
func (s *Service) Shutdown() {
	s.manager.Shutdown()
}
