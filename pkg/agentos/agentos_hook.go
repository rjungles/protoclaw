package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/workflow"
)

type AgentOSHook struct {
	manifest      *manifest.Manifest
	policyEngine  *policy.Engine
	workflowStore WorkflowStore
	auditLog      AuditLog
	notifyBus     *NotificationBus

	activeFSMs map[string]*workflow.FSM
}

func NewAgentOSHook(
	m *manifest.Manifest,
	workflowStore WorkflowStore,
	auditLog AuditLog,
	notifyBus *NotificationBus,
) (*AgentOSHook, error) {
	policyEngine, err := policy.NewEngine(m)
	if err != nil {
		return nil, fmt.Errorf("failed to create policy engine: %w", err)
	}

	return &AgentOSHook{
		manifest:      m,
		policyEngine:  policyEngine,
		workflowStore: workflowStore,
		auditLog:      auditLog,
		notifyBus:     notifyBus,
		activeFSMs:    make(map[string]*workflow.FSM),
	}, nil
}

func (h *AgentOSHook) Manifest() *manifest.Manifest {
	return h.manifest
}

func (h *AgentOSHook) Name() string {
	return "agentos"
}

func (h *AgentOSHook) Priority() int {
	return -100
}

func (h *AgentOSHook) Source() agent.HookSource {
	return agent.HookSourceInProcess
}

func (h *AgentOSHook) BeforeLLM(
	ctx context.Context,
	req *agent.LLMHookRequest,
) (*agent.LLMHookRequest, agent.HookDecision, error) {
	if req == nil || len(req.Messages) == 0 {
		return req, agent.HookDecision{Action: agent.HookActionContinue}, nil
	}

	actorID := resolveActorID(req)

	contextBlock := h.buildContextBlock(actorID)

	newMessages := make([]providers.Message, 0, len(req.Messages)+1)
	newMessages = append(newMessages, providers.Message{
		Role:    "system",
		Content: contextBlock,
	})
	newMessages = append(newMessages, req.Messages...)

	req.Messages = newMessages

	return req, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

func (h *AgentOSHook) buildContextBlock(actorID string) string {
	var sb strings.Builder

	sb.WriteString("## AgentOS Context\n\n")

	sb.WriteString("### Current Actor\n")
	sb.WriteString(fmt.Sprintf("- Actor ID: %s\n", actorID))

	roles := h.policyEngine.GetAllRoles(actorID)
	if len(roles) > 0 {
		sb.WriteString(fmt.Sprintf("- Roles: %s\n", strings.Join(roles, ", ")))
	}

	if role := h.findActorRole(actorID); role != "" {
		sb.WriteString(fmt.Sprintf("- Role: %s\n", role))
	}
	sb.WriteString("\n")

	if len(h.manifest.DataModel.Entities) > 0 {
		sb.WriteString("### Available Entities\n")
		for _, entity := range h.manifest.DataModel.Entities {
			sb.WriteString(fmt.Sprintf("- %s", entity.Name))
			if entity.Description != "" {
				sb.WriteString(fmt.Sprintf(": %s", entity.Description))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(h.manifest.BusinessRules) > 0 {
		sb.WriteString("### Active Business Rules\n")
		for _, rule := range h.manifest.BusinessRules {
			if !rule.Enabled {
				continue
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", rule.ID, rule.Name))
			if rule.Description != "" {
				sb.WriteString(fmt.Sprintf("  %s\n", rule.Description))
			}
			sb.WriteString(fmt.Sprintf("  Trigger: %s on %s\n",
				rule.Trigger.Event, strings.Join(rule.Trigger.Entities, ", ")))
			if rule.Condition != "" {
				sb.WriteString(fmt.Sprintf("  Condition: %s\n", rule.Condition))
			}
		}
		sb.WriteString("\n")
	}

	if len(h.activeFSMs) > 0 {
		sb.WriteString("### Workflow State Machines\n")
		for entityType, fsm := range h.activeFSMs {
			states := fsm.ListStates()
			if len(states) > 0 {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", entityType, strings.Join(states, ", ")))
			}
		}
		sb.WriteString("\n")
	}

	allowedActions := h.enumerateAllowedActions(actorID)
	if len(allowedActions) > 0 {
		sb.WriteString("### Permitted Actions\n")
		for _, action := range allowedActions {
			sb.WriteString(fmt.Sprintf("- %s\n", action))
		}
		sb.WriteString("\n")
	}

	toolPermissions := h.enumerateToolPermissions(actorID)
	if len(toolPermissions) > 0 {
		sb.WriteString("### Tool Permissions\n")
		for tool, allowed := range toolPermissions {
			status := "ALLOWED"
			if !allowed {
				status = "DENIED"
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", tool, status))
		}
		sb.WriteString("\n")
	}

	if h.manifest.Security.Authorization.DefaultDeny {
		sb.WriteString("### Security Policy\n")
		sb.WriteString("- Default: DENY (explicit permission required)\n\n")
	} else {
		sb.WriteString("### Security Policy\n")
		sb.WriteString("- Default: ALLOW (restrictions only)\n\n")
	}

	return sb.String()
}

func (h *AgentOSHook) enumerateAllowedActions(actorID string) []string {
	var actions []string

	for _, actor := range h.manifest.Actors {
		if actor.ID != actorID {
			continue
		}
		for _, perm := range actor.Permissions {
			for _, action := range perm.Actions {
				entry := fmt.Sprintf("%s:%s", perm.Resource, action)
				if !slices.Contains(actions, entry) {
					actions = append(actions, entry)
				}
			}
		}
	}

	slices.Sort(actions)
	return actions
}

func (h *AgentOSHook) enumerateToolPermissions(actorID string) map[string]bool {
	result := make(map[string]bool)

	allTools := []string{
		"bash", "read", "write", "edit", "grep", "list",
		"task_create", "task_update", "task_delete",
		"file_search", "web_search", "web_fetch",
	}

	for _, tool := range allTools {
		action := tool
		if strings.Contains(action, "_") {
			action = strings.ReplaceAll(action, "_", "-")
		}

		result[tool] = false

		allRoles := h.policyEngine.GetAllRoles(actorID)
		if len(allRoles) == 0 {
			allRoles = []string{"anonymous"}
		}

		for _, role := range allRoles {
			for _, actor := range h.manifest.Actors {
				for _, actorRole := range actor.Roles {
					if actorRole == role {
						for _, perm := range actor.Permissions {
							for _, permAction := range perm.Actions {
								if permAction == "*" || permAction == action {
									if perm.Resource == "tool" || perm.Resource == "all" || perm.Resource == tool {
										result[tool] = true
										break
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return result
}

func (h *AgentOSHook) findActorRole(actorID string) string {
	for _, actor := range h.manifest.Actors {
		if actor.ID == actorID {
			if len(actor.Roles) > 0 {
				return actor.Roles[0]
			}
		}
	}
	return ""
}

func resolveActorID(req *agent.LLMHookRequest) string {
	if req == nil || req.Context == nil {
		return "anonymous"
	}
	if req.Context.Inbound != nil && req.Context.Inbound.SenderID != "" {
		return req.Context.Inbound.SenderID
	}
	return "anonymous"
}

func (h *AgentOSHook) AfterLLM(
	ctx context.Context,
	resp *agent.LLMHookResponse,
) (*agent.LLMHookResponse, agent.HookDecision, error) {
	return resp, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

func (h *AgentOSHook) BeforeTool(
	ctx context.Context,
	call *agent.ToolCallHookRequest,
) (*agent.ToolCallHookRequest, agent.HookDecision, error) {
	if call == nil {
		return call, agent.HookDecision{Action: agent.HookActionContinue}, nil
	}

	toolName := call.Tool
	actorID := resolveToolActorID(call)
	channel := call.Channel
	chatID := call.ChatID

	toolResource := "tool:" + toolName

	authCtx := &policy.Context{
		ActorID:    actorID,
		Resource:   toolResource,
		Action:     "execute",
		Attributes: extractToolAttributes(toolName, call.Arguments),
		Time:       time.Now(),
	}

	result := h.policyEngine.CheckPermission(authCtx)

	if h.auditLog != nil {
		entry := AuditEntry{
			ActorID:  actorID,
			Action:   toolName,
			Resource: toolResource,
			Allowed:  result.Allowed,
			Reason:   result.Reason,
			Details: map[string]interface{}{
				"tool":      toolName,
				"channel":   channel,
				"chat_id":   chatID,
				"arguments": call.Arguments,
				"roles":     result.Roles,
			},
		}
		_ = h.auditLog.Record(entry)
	}

	if !result.Allowed {
		return call, agent.HookDecision{
			Action: agent.HookActionDenyTool,
			Reason: fmt.Sprintf("access denied: %s (reason: %s)", result.Reason, result.Condition),
		}, nil
	}

	h.handleWorkflowTransition(actorID, toolName, call.Arguments)

	return call, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

func (h *AgentOSHook) AfterTool(
	ctx context.Context,
	result *agent.ToolResultHookResponse,
) (*agent.ToolResultHookResponse, agent.HookDecision, error) {
	return result, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

func (h *AgentOSHook) ApproveTool(
	ctx context.Context,
	req *agent.ToolApprovalRequest,
) (agent.ApprovalDecision, error) {
	if req == nil {
		return agent.ApprovalDecision{Approved: true}, nil
	}

	actorID := resolveToolApprovalActorID(req)
	toolName := req.Tool

	toolResource := "tool:" + toolName

	authCtx := &policy.Context{
		ActorID:    actorID,
		Resource:   toolResource,
		Action:     "execute",
		Attributes: extractToolAttributes(toolName, req.Arguments),
		Time:       time.Now(),
	}

	result := h.policyEngine.CheckPermission(authCtx)

	if !result.Allowed {
		return agent.ApprovalDecision{
			Approved: false,
			Reason:   result.Reason,
		}, nil
	}

	return agent.ApprovalDecision{Approved: true}, nil
}

func resolveToolActorID(call *agent.ToolCallHookRequest) string {
	if call == nil || call.Context == nil {
		return "anonymous"
	}
	if call.Context.Inbound != nil && call.Context.Inbound.SenderID != "" {
		return call.Context.Inbound.SenderID
	}
	return "anonymous"
}

func resolveToolApprovalActorID(req *agent.ToolApprovalRequest) string {
	if req == nil || req.Context == nil {
		return "anonymous"
	}
	if req.Context.Inbound != nil && req.Context.Inbound.SenderID != "" {
		return req.Context.Inbound.SenderID
	}
	return "anonymous"
}

func extractToolAttributes(toolName string, args map[string]any) map[string]interface{} {
	attrs := make(map[string]interface{})

	if args == nil {
		return attrs
	}

	if path, ok := args["path"].(string); ok {
		attrs["path"] = path
		attrs["path_depth"] = strings.Count(path, "/")
	}

	if cmd, ok := args["command"].(string); ok {
		attrs["command"] = cmd
		attrs["is_shell"] = strings.Contains(cmd, " ") || strings.Contains(cmd, "|")
	}

	if entityID, ok := args["entity_id"].(string); ok {
		attrs["entity_id"] = entityID
	}

	if entityType, ok := args["entity_type"].(string); ok {
		attrs["entity_type"] = entityType
	}

	if target, ok := args["target"].(string); ok {
		attrs["target"] = target
	}

	attrs["tool"] = toolName

	return attrs
}

func (h *AgentOSHook) handleWorkflowTransition(actorID string, toolName string, args map[string]any) {
	if len(h.activeFSMs) == 0 {
		return
	}

	entityID := ""
	entityType := "task"
	if id, ok := args["entity_id"].(string); ok {
		entityID = id
	}
	if t, ok := args["entity_type"].(string); ok {
		entityType = t
	}

	fsm, ok := h.activeFSMs[entityType]
	if !ok {
		fsm, ok = h.activeFSMs["task"]
		if !ok {
			return
		}
	}

	state, err := h.workflowStore.Get(entityType, entityID)
	if err != nil {
		return
	}

	var currentState workflow.State
	if state != nil {
		currentState = workflow.State(state.CurrentState)
	} else {
		currentState = fsm.InitialState()
	}

	action := toolNameToWorkflowAction(toolName)

	roles := h.policyEngine.GetAllRoles(actorID)
	if len(roles) == 0 {
		roles = []string{"anonymous"}
	}

	allowed, _, err := fsm.CanTransition(currentState, workflow.Action(action), roles)
	if err != nil || !allowed {
		return
	}

	transition := fsm.FindTransition(currentState, workflow.Action(action))
	if transition == nil {
		return
	}

	fsm.Transition(roles, workflow.Action(action))

	newState := transition.To
	newStateRecord := &WorkflowState{
		EntityID:     entityID,
		EntityType:   entityType,
		CurrentState: string(newState),
		UpdatedAt:    time.Now(),
		UpdatedBy:    actorID,
	}

	if err := h.workflowStore.Set(newStateRecord); err != nil {
		return
	}

	h.handleStateChangeNotifications(actorID, entityID, entityType, string(currentState), string(newState))
}

func toolNameToWorkflowAction(toolName string) string {
	switch toolName {
	case "task_create":
		return "create"
	case "task_update":
		return "update"
	case "submit":
		return "submit"
	case "approve":
		return "approve"
	case "reject":
		return "reject"
	case "publish":
		return "publish"
	case "archive":
		return "archive"
	default:
		return toolName
	}
}

func (h *AgentOSHook) handleStateChangeNotifications(
	actorID, entityID, entityType, fromState, toState string,
) {
	if h.notifyBus == nil {
		return
	}

	for _, rule := range h.manifest.BusinessRules {
		if !rule.Enabled || rule.Trigger.Event != "update" {
			continue
		}

		for _, entity := range rule.Trigger.Entities {
			if entity != entityType {
				continue
			}

			for _, action := range rule.Actions {
				if action.Type == "notify" && action.Target != "" {
					notifyActor := action.Target
					if notifyActor == "author" || notifyActor == "owner" {
						notifyActor = actorID
					}

					h.notifyBus.Notify(Notification{
						FromActor: actorID,
						ToActor:   notifyActor,
						Type:      "workflow_state_change",
						Title:     fmt.Sprintf("Workflow state changed for %s", entityID),
						Body:      fmt.Sprintf("State changed from %s to %s", fromState, toState),
						Data: map[string]interface{}{
							"entity_type": entityType,
							"entity_id":   entityID,
							"from_state":  fromState,
							"to_state":    toState,
							"rule_id":     rule.ID,
						},
					})
				}
			}
		}
	}
}

func (h *AgentOSHook) AfterTurn(
	ctx context.Context,
	payload TurnEndPayload,
) error {
	if h.auditLog == nil {
		return nil
	}

	actorID := "anonymous"
	if payload.Context != nil && payload.Context.Inbound != nil {
		actorID = payload.Context.Inbound.SenderID
	}

	entry := AuditEntry{
		ActorID:  actorID,
		Action:   "turn_end",
		Resource: "agent:turn",
		Allowed:  payload.Status == agent.TurnEndStatusCompleted,
		Reason:   string(payload.Status),
		Details: map[string]interface{}{
			"status":       payload.Status,
			"user_message": payload.UserMessage,
			"tool_count":   payload.ToolCount,
			"iteration":    payload.Iteration,
		},
	}

	return h.auditLog.Record(entry)
}

type TurnEndPayload struct {
	Status      agent.TurnEndStatus
	UserMessage string
	ToolCount   int
	Iteration   int
	Context     *agent.TurnContext
}

func (h *AgentOSHook) RegisterFSM(entityType string, fsmCfg workflow.FSMConfig) error {
	fsm, err := workflow.NewFSM(fsmCfg)
	if err != nil {
		return err
	}
	h.activeFSMs[entityType] = fsm
	return nil
}

func (h *AgentOSHook) GetWorkflowState(entityType, entityID string) (*WorkflowState, error) {
	if h.workflowStore == nil {
		return nil, nil
	}
	return h.workflowStore.Get(entityType, entityID)
}

func (h *AgentOSHook) QueryAudit(filter AuditFilter) ([]AuditEntry, error) {
	if h.auditLog == nil {
		return nil, nil
	}
	return h.auditLog.Query(filter)
}

func (h *AgentOSHook) SendNotification(n Notification) {
	if h.notifyBus != nil {
		h.notifyBus.Notify(n)
	}
}

func (h *AgentOSHook) GetNotifications(actorID string) []Notification {
	if h.notifyBus == nil {
		return nil
	}
	return h.notifyBus.GetUnread(actorID)
}

func (h *AgentOSHook) ExportContextJSON() ([]byte, error) {
	ctx := map[string]interface{}{
		"manifest_name":    h.manifest.Metadata.Name,
		"manifest_version": h.manifest.Metadata.Version,
		"actors":           len(h.manifest.Actors),
		"entities":         len(h.manifest.DataModel.Entities),
		"business_rules":   len(h.manifest.BusinessRules),
		"active_workflows": len(h.activeFSMs),
	}

	actorCtx := make(map[string][]string)
	for _, actor := range h.manifest.Actors {
		actorCtx[actor.ID] = actor.Roles
	}
	ctx["actor_roles"] = actorCtx

	return json.MarshalIndent(ctx, "", "  ")
}
