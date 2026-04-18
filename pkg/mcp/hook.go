package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type HookAction string

const (
	HookActionContinue HookAction = "continue"
	HookActionDenyTool HookAction = "deny_tool"
)

type HookDecision struct {
	Action HookAction
	Reason string
}

type HookSource string

const (
	HookSourceInProcess HookSource = "in_process"
)

type LLMHookRequest struct {
	Context  *TurnContext
	Messages []providers.Message
}

type LLMHookResponse struct {
	Messages []providers.Message
}

type ToolCallHookRequest struct {
	Tool      string
	Arguments map[string]any
	Channel   string
	ChatID    string
	Context   *TurnContext
}

type ToolResultHookResponse struct {
	Tool      string
	Arguments map[string]any
	Result    string
	Error     error
}

type ToolApprovalRequest struct {
	Tool      string
	Arguments map[string]any
	Context   *TurnContext
}

type ApprovalDecision struct {
	Approved bool
	Reason   string
}

type TurnContext struct {
	Inbound *InboundContext
}

type InboundContext struct {
	SenderID string
}

type LLMInterceptor interface {
	BeforeLLM(ctx context.Context, req *LLMHookRequest) (*LLMHookRequest, HookDecision, error)
	AfterLLM(ctx context.Context, resp *LLMHookResponse) (*LLMHookResponse, HookDecision, error)
}

type ToolInterceptor interface {
	BeforeTool(ctx context.Context, call *ToolCallHookRequest) (*ToolCallHookRequest, HookDecision, error)
	AfterTool(ctx context.Context, result *ToolResultHookResponse) (*ToolResultHookResponse, HookDecision, error)
	ApproveTool(ctx context.Context, req *ToolApprovalRequest) (ApprovalDecision, error)
}

type MCPHook struct {
	registry     *Registry
	manifest     *manifest.Manifest
	policyEngine *policy.Engine
}

func NewMCPHook(m *manifest.Manifest, registry *Registry) (*MCPHook, error) {
	engine, err := policy.NewEngine(m)
	if err != nil {
		return nil, fmt.Errorf("failed to create policy engine: %w", err)
	}
	return &MCPHook{
		registry:     registry,
		manifest:     m,
		policyEngine: engine,
	}, nil
}

func (h *MCPHook) Name() string {
	return "mcp"
}

func (h *MCPHook) Priority() int {
	return -50
}

func (h *MCPHook) Source() HookSource {
	return HookSourceInProcess
}

func (h *MCPHook) BeforeLLM(
	ctx context.Context,
	req *LLMHookRequest,
) (*LLMHookRequest, HookDecision, error) {
	if req == nil || len(req.Messages) == 0 {
		return req, HookDecision{Action: HookActionContinue}, nil
	}

	actorID := resolveActorID(req)

	contextBlock := h.buildMCPContextBlock(ctx, actorID)

	newMessages := make([]providers.Message, 0, len(req.Messages)+1)
	newMessages = append(newMessages, providers.Message{
		Role:    "system",
		Content: contextBlock,
	})
	newMessages = append(newMessages, req.Messages...)

	req.Messages = newMessages

	return req, HookDecision{Action: HookActionContinue}, nil
}

func (h *MCPHook) buildMCPContextBlock(ctx context.Context, actorID string) string {
	var sb strings.Builder

	sb.WriteString("## MCP Tools Available\n\n")

	allTools, err := h.registry.ListAllTools(ctx)
	if err != nil {
		sb.WriteString("Error listing MCP tools: " + err.Error() + "\n")
		return sb.String()
	}

	if len(allTools) == 0 {
		sb.WriteString("No MCP tools configured.\n")
		return sb.String()
	}

	for serverName, tools := range allTools {
		sb.WriteString(fmt.Sprintf("### %s\n", serverName))
		for _, tool := range tools {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name, tool.Description))
			if len(tool.InputSchema) > 0 {
				if props, ok := tool.InputSchema["properties"].(map[string]interface{}); ok {
					sb.WriteString("  Arguments:\n")
					for propName, propDef := range props {
						if pd, ok := propDef.(map[string]interface{}); ok {
							propType := ""
							if t, ok := pd["type"].(string); ok {
								propType = t
							}
							propDesc := ""
							if d, ok := pd["description"].(string); ok {
								propDesc = d
							}
							sb.WriteString(fmt.Sprintf("    - %s (%s): %s\n", propName, propType, propDesc))
						}
					}
				}
			}
		}
		sb.WriteString("\n")
	}

	allowedTools := h.enumerateAllowedMCPTools(ctx, actorID, allTools)
	if len(allowedTools) > 0 {
		sb.WriteString("### Tools You Can Use\n")
		for _, tool := range allowedTools {
			sb.WriteString(fmt.Sprintf("- %s\n", tool))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (h *MCPHook) enumerateAllowedMCPTools(ctx context.Context, actorID string, allTools map[string][]Tool) []string {
	var allowed []string

	for serverName, tools := range allTools {
		for _, tool := range tools {
			resource := fmt.Sprintf("mcp:%s:%s", serverName, tool.Name)

			authCtx := &policy.Context{
				ActorID:    actorID,
				Resource:   resource,
				Action:     "execute",
				Attributes: map[string]interface{}{},
				Time:       time.Now(),
			}

			result := h.policyEngine.CheckPermission(authCtx)
			if result.Allowed {
				allowed = append(allowed, fmt.Sprintf("%s.%s", serverName, tool.Name))
			}
		}
	}

	slices.Sort(allowed)
	return allowed
}

func (h *MCPHook) AfterLLM(
	ctx context.Context,
	resp *LLMHookResponse,
) (*LLMHookResponse, HookDecision, error) {
	return resp, HookDecision{Action: HookActionContinue}, nil
}

func (h *MCPHook) BeforeTool(
	ctx context.Context,
	call *ToolCallHookRequest,
) (*ToolCallHookRequest, HookDecision, error) {
	if call == nil {
		return call, HookDecision{Action: HookActionContinue}, nil
	}

	toolName := call.Tool
	actorID := resolveToolActorID(call)

	serverName, _, err := h.registry.FindTool(ctx, toolName)
	if err != nil {
		return call, HookDecision{Action: HookActionContinue}, nil
	}

	resource := fmt.Sprintf("mcp:%s:%s", serverName, toolName)

	authCtx := &policy.Context{
		ActorID:    actorID,
		Resource:   resource,
		Action:     "execute",
		Attributes: call.Arguments,
		Time:       time.Now(),
	}

	result := h.policyEngine.CheckPermission(authCtx)

	if !result.Allowed {
		return call, HookDecision{
			Action: HookActionDenyTool,
			Reason: fmt.Sprintf("MCP tool access denied: %s (reason: %s)", result.Reason, result.Condition),
		}, nil
	}

	return call, HookDecision{Action: HookActionContinue}, nil
}

func (h *MCPHook) AfterTool(
	ctx context.Context,
	result *ToolResultHookResponse,
) (*ToolResultHookResponse, HookDecision, error) {
	return result, HookDecision{Action: HookActionContinue}, nil
}

func (h *MCPHook) ApproveTool(
	ctx context.Context,
	req *ToolApprovalRequest,
) (ApprovalDecision, error) {
	if req == nil {
		return ApprovalDecision{Approved: true}, nil
	}

	toolName := req.Tool
	actorID := resolveToolApprovalActorID(req)

	serverName, _, err := h.registry.FindTool(ctx, toolName)
	if err != nil {
		return ApprovalDecision{Approved: true}, nil
	}

	resource := fmt.Sprintf("mcp:%s:%s", serverName, toolName)

	authCtx := &policy.Context{
		ActorID:    actorID,
		Resource:   resource,
		Action:     "execute",
		Attributes: req.Arguments,
		Time:       time.Now(),
	}

	result := h.policyEngine.CheckPermission(authCtx)

	if !result.Allowed {
		return ApprovalDecision{
			Approved: false,
			Reason:   result.Reason,
		}, nil
	}

	return ApprovalDecision{Approved: true}, nil
}

func (h *MCPHook) Registry() *Registry {
	return h.registry
}

func resolveActorID(req *LLMHookRequest) string {
	if req == nil || req.Context == nil {
		return "anonymous"
	}
	if req.Context.Inbound != nil && req.Context.Inbound.SenderID != "" {
		return req.Context.Inbound.SenderID
	}
	return "anonymous"
}

func resolveToolActorID(call *ToolCallHookRequest) string {
	if call == nil || call.Context == nil {
		return "anonymous"
	}
	if call.Context.Inbound != nil && call.Context.Inbound.SenderID != "" {
		return call.Context.Inbound.SenderID
	}
	return "anonymous"
}

func resolveToolApprovalActorID(req *ToolApprovalRequest) string {
	if req == nil || req.Context == nil {
		return "anonymous"
	}
	if req.Context.Inbound != nil && req.Context.Inbound.SenderID != "" {
		return req.Context.Inbound.SenderID
	}
	return "anonymous"
}
