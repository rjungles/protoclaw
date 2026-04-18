package agentos

import (
	"context"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/workflow"
)

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:        "test-system",
			Version:     "1.0.0",
			Description: "Test system for AgentOS",
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Administrator",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "tool:*", Actions: []string{"*"}},
					{Resource: "data", Actions: []string{"read", "write", "delete"}},
				},
			},
			{
				ID:    "user",
				Name:  "Regular User",
				Roles: []string{"user"},
				Permissions: []manifest.Permission{
					{Resource: "tool:read", Actions: []string{"execute"}},
					{Resource: "tool:write", Actions: []string{"execute"}},
					{Resource: "data", Actions: []string{"read"}},
				},
			},
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name:        "User",
					Description: "User entity",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "name", Type: "string", Required: true},
					},
				},
				{
					Name:        "Task",
					Description: "Task entity",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "title", Type: "string", Required: true},
						{Name: "status", Type: "string", Required: true},
					},
				},
			},
		},
		BusinessRules: []manifest.BusinessRule{
			{
				ID:        "rule_1",
				Name:      "Test Rule",
				Enabled:   true,
				Trigger:   manifest.Trigger{Event: "create", Entities: []string{"Task"}},
				Condition: "task.status == 'draft'",
				Actions: []manifest.RuleAction{
					{Type: "notify", Target: "admin"},
				},
			},
		},
		Security: manifest.SecurityPolicy{
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: true,
				RoleHierarchy: []manifest.RoleHierarchy{
					{Role: "admin", Inherits: []string{"user"}},
				},
			},
		},
	}
}

func TestNewAgentOSHook(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	if hook == nil {
		t.Fatal("hook is nil")
	}

	if hook.Manifest() == nil {
		t.Error("Manifest() returned nil")
	}

	if hook.Name() != "agentos" {
		t.Errorf("Name: want agentos, got %s", hook.Name())
	}

	if hook.Priority() != -100 {
		t.Errorf("Priority: want -100, got %d", hook.Priority())
	}
}

func TestAgentOSHook_BeforeLLM(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	req := &agent.LLMHookRequest{
		Context: &agent.TurnContext{
			Inbound: &bus.InboundContext{SenderID: "user"},
		},
		Messages: []providers.Message{
			{Role: "user", Content: "hello"},
		},
	}

	newReq, decision, err := hook.BeforeLLM(context.Background(), req)
	if err != nil {
		t.Fatalf("BeforeLLM: %v", err)
	}

	if decision.Action != agent.HookActionContinue {
		t.Errorf("decision.Action: want continue, got %s", decision.Action)
	}

	if len(newReq.Messages) < 2 {
		t.Errorf("Messages count: want >= 2, got %d", len(newReq.Messages))
	}

	if newReq.Messages[0].Role != "system" {
		t.Errorf("First message role: want system, got %s", newReq.Messages[0].Role)
	}

	if newReq.Messages[0].Content == "" {
		t.Error("Context block is empty")
	}

	if !contains(newReq.Messages[0].Content, "Current Actor") {
		t.Error("Context block missing Current Actor section")
	}

	if !contains(newReq.Messages[0].Content, "Available Entities") {
		t.Error("Context block missing Available Entities section")
	}

	if !contains(newReq.Messages[0].Content, "Active Business Rules") {
		t.Error("Context block missing Business Rules section")
	}
}

func TestAgentOSHook_BeforeLLM_Anonymous(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	req := &agent.LLMHookRequest{
		Messages: []providers.Message{
			{Role: "user", Content: "hello"},
		},
	}

	newReq, _, err := hook.BeforeLLM(context.Background(), req)
	if err != nil {
		t.Fatalf("BeforeLLM: %v", err)
	}

	if !contains(newReq.Messages[0].Content, "anonymous") {
		t.Error("Context block should reference anonymous actor")
	}
}

func TestAgentOSHook_BeforeTool_Denied(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	call := &agent.ToolCallHookRequest{
		Tool: "bash",
		Arguments: map[string]any{
			"command": "rm -rf /",
		},
		Context: &agent.TurnContext{
			Inbound: &bus.InboundContext{SenderID: "user"},
		},
	}

	newCall, decision, err := hook.BeforeTool(context.Background(), call)
	if err != nil {
		t.Fatalf("BeforeTool: %v", err)
	}

	if decision.Action != agent.HookActionDenyTool {
		t.Errorf("decision.Action: want deny_tool, got %s", decision.Action)
	}

	if newCall != call {
		t.Error("call should not be modified when denied")
	}
}

func TestAgentOSHook_BeforeTool_Allowed(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	call := &agent.ToolCallHookRequest{
		Tool: "read",
		Arguments: map[string]any{
			"path": "/some/file",
		},
		Context: &agent.TurnContext{
			Inbound: &bus.InboundContext{SenderID: "user"},
		},
	}

	_, decision, err := hook.BeforeTool(context.Background(), call)
	if err != nil {
		t.Fatalf("BeforeTool: %v", err)
	}

	if decision.Action != agent.HookActionContinue {
		t.Errorf("decision.Action: want continue, got %s", decision.Action)
	}
}

func TestAgentOSHook_BeforeTool_AdminAllowed(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	call := &agent.ToolCallHookRequest{
		Tool: "bash",
		Arguments: map[string]any{
			"command": "ls -la",
		},
		Context: &agent.TurnContext{
			Inbound: &bus.InboundContext{SenderID: "admin"},
		},
	}

	_, decision, err := hook.BeforeTool(context.Background(), call)
	if err != nil {
		t.Fatalf("BeforeTool: %v", err)
	}

	if decision.Action != agent.HookActionContinue {
		t.Errorf("decision.Action: want continue for admin, got %s", decision.Action)
	}
}

func TestAgentOSHook_AfterTool(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	result := &agent.ToolResultHookResponse{
		Tool:      "read",
		Arguments: map[string]any{"path": "/file.txt"},
	}

	newResult, decision, err := hook.AfterTool(context.Background(), result)
	if err != nil {
		t.Fatalf("AfterTool: %v", err)
	}

	if decision.Action != agent.HookActionContinue {
		t.Errorf("decision.Action: want continue, got %s", decision.Action)
	}

	if newResult != result {
		t.Error("result should not be modified")
	}
}

func TestAgentOSHook_ApproveTool(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	req := &agent.ToolApprovalRequest{
		Tool:      "bash",
		Arguments: map[string]any{"command": "ls"},
		Context: &agent.TurnContext{
			Inbound: &bus.InboundContext{SenderID: "admin"},
		},
	}

	decision, err := hook.ApproveTool(context.Background(), req)
	if err != nil {
		t.Fatalf("ApproveTool: %v", err)
	}

	if !decision.Approved {
		t.Error("admin should be approved for bash")
	}
}

func TestAgentOSHook_ApproveTool_Denied(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	req := &agent.ToolApprovalRequest{
		Tool:      "bash",
		Arguments: map[string]any{"command": "ls"},
		Context: &agent.TurnContext{
			Inbound: &bus.InboundContext{SenderID: "user"},
		},
	}

	decision, err := hook.ApproveTool(context.Background(), req)
	if err != nil {
		t.Fatalf("ApproveTool: %v", err)
	}

	if decision.Approved {
		t.Error("user should not be approved for bash")
	}
}

func TestAgentOSHook_AfterTurn(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	payload := TurnEndPayload{
		Status:      agent.TurnEndStatusCompleted,
		UserMessage: "test message",
		ToolCount:   5,
		Iteration:   2,
		Context: &agent.TurnContext{
			Inbound: &bus.InboundContext{SenderID: "admin"},
		},
	}

	err = hook.AfterTurn(context.Background(), payload)
	if err != nil {
		t.Fatalf("AfterTurn: %v", err)
	}

	entries, err := auditLog.Query(AuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.Action == "turn_end" {
			found = true
			break
		}
	}
	if !found {
		t.Error("audit entry for turn_end not found")
	}
}

func TestAgentOSHook_ExportContextJSON(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	data, err := hook.ExportContextJSON()
	if err != nil {
		t.Fatalf("ExportContextJSON: %v", err)
	}

	if len(data) == 0 {
		t.Error("exported data is empty")
	}

	if !contains(string(data), "test-system") {
		t.Error("exported data missing manifest name")
	}

	if !contains(string(data), "actors") {
		t.Error("exported data missing actors count")
	}
}

func TestAgentOSHook_GetWorkflowState(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	state := &WorkflowState{
		EntityID:     "task-123",
		EntityType:   "task",
		CurrentState: "draft",
		UpdatedAt:    time.Now(),
		UpdatedBy:    "admin",
	}

	err = workflowStore.Set(state)
	if err != nil {
		t.Fatalf("workflowStore.Set: %v", err)
	}

	retrieved, err := hook.GetWorkflowState("task", "task-123")
	if err != nil {
		t.Fatalf("GetWorkflowState: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetWorkflowState returned nil")
	}

	if retrieved.CurrentState != "draft" {
		t.Errorf("CurrentState: want draft, got %s", retrieved.CurrentState)
	}
}

func TestAgentOSHook_SendNotification(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	hook.SendNotification(Notification{
		FromActor: "admin",
		ToActor:   "user",
		Type:      "test",
		Title:     "Test notification",
		Body:      "This is a test",
	})

	notifications := hook.GetNotifications("user")
	if len(notifications) == 0 {
		t.Error("notification not found for user")
	}
}

func TestAgentOSHook_GetNotifications_Empty(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	notifications := hook.GetNotifications("unknown_actor")
	if notifications != nil && len(notifications) > 0 {
		t.Error("should have no notifications for unknown actor")
	}
}

func TestAgentOSHook_RegisterFSM(t *testing.T) {
	m := testManifest()
	workflowStore := NewMemoryWorkflowStore()
	auditLog := NewMemoryAuditLog(100)
	notifyBus := NewNotificationBus(100)

	hook, err := NewAgentOSHook(m, workflowStore, auditLog, notifyBus)
	if err != nil {
		t.Fatalf("NewAgentOSHook: %v", err)
	}

	cfg := workflow.FSMConfig{
		EntityName:   "article",
		InitialState: "draft",
		States: map[workflow.State]workflow.StateConfig{
			"draft": {
				Transitions: []workflow.Transition{
					{To: "review", Action: "submit", AllowedRoles: []string{"author"}},
				},
			},
			"review": {
				Transitions: []workflow.Transition{
					{To: "published", Action: "publish", AllowedRoles: []string{"editor"}},
				},
			},
		},
	}

	err = hook.RegisterFSM("article", cfg)
	if err != nil {
		t.Fatalf("RegisterFSM: %v", err)
	}

	state, err := hook.GetWorkflowState("article", "article-1")
	if err != nil {
		t.Fatalf("GetWorkflowState: %v", err)
	}

	if state != nil {
		t.Error("state should be nil for non-existent article")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
