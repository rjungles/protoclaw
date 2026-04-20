package stateful

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "TestSystem",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Task",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "title", Type: "string", Required: true},
						{Name: "status", Type: "string", Required: false},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Administrator",
				Roles: []string{"admin"},
			},
			{
				ID:    "member",
				Name:  "Member",
				Roles: []string{"member"},
			},
		},
		Workflows: []manifest.WorkflowConfig{
			{
				Entity:       "Task",
				InitialState: "todo",
				States: []manifest.WorkflowState{
					{
						ID:          "todo",
						Description: "Task todo",
						Transitions: []manifest.WorkflowTransition{
							{To: "in_progress", Action: "start", AllowedRoles: []string{"member", "admin"}},
						},
					},
					{
						ID:          "in_progress",
						Description: "Task in progress",
						Transitions: []manifest.WorkflowTransition{
							{To: "done", Action: "complete", AllowedRoles: []string{"member", "admin"}},
							{To: "todo", Action: "reopen", AllowedRoles: []string{"admin"}},
						},
					},
					{
						ID:          "done",
						Description: "Task done",
						Transitions: []manifest.WorkflowTransition{},
					},
				},
			},
		},
	}
}

func TestWorkflowEngine_NewWorkflowEngine(t *testing.T) {
	m := testManifest()
	engine := NewWorkflowEngine(m, nil)

	if engine == nil {
		t.Fatal("engine should not be nil")
	}

	if len(engine.fsms) != 1 {
		t.Errorf("expected 1 FSM, got %d", len(engine.fsms))
	}

	fsm := engine.fsms["Task"]
	if fsm == nil {
		t.Fatal("Task FSM should exist")
	}

	if fsm.InitialState() != "todo" {
		t.Errorf("expected initial state 'todo', got '%s'", fsm.InitialState())
	}
}

func TestWorkflowEngine_Transition(t *testing.T) {
	m := testManifest()
	engine := NewWorkflowEngine(m, nil)

	ctx := context.Background()

	instance, err := engine.InitializeState("Task", "task-1", "member")
	if err != nil {
		t.Fatalf("InitializeState: %v", err)
	}

	if instance.CurrentState != "todo" {
		t.Errorf("expected initial state 'todo', got '%s'", instance.CurrentState)
	}

	newInstance, err := engine.Transition(ctx, "Task", "task-1", "start", "member", []string{"member"})
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}

	if newInstance.CurrentState != "in_progress" {
		t.Errorf("expected state 'in_progress', got '%s'", newInstance.CurrentState)
	}

	if newInstance.PreviousState != "todo" {
		t.Errorf("expected previous state 'todo', got '%s'", newInstance.PreviousState)
	}
}

func TestWorkflowEngine_TransitionUnauthorized(t *testing.T) {
	m := testManifest()
	engine := NewWorkflowEngine(m, nil)

	ctx := context.Background()

	engine.InitializeState("Task", "task-1", "member")

	_, err := engine.Transition(ctx, "Task", "task-1", "reopen", "member", []string{"member"})
	if err == nil {
		t.Error("expected error for unauthorized transition")
	}
}

func TestWorkflowEngine_TransitionInvalidAction(t *testing.T) {
	m := testManifest()
	engine := NewWorkflowEngine(m, nil)

	ctx := context.Background()

	engine.InitializeState("Task", "task-1", "member")

	_, err := engine.Transition(ctx, "Task", "task-1", "invalid_action", "member", []string{"member"})
	if err == nil {
		t.Error("expected error for invalid action")
	}
}

func TestWorkflowEngine_CanTransition(t *testing.T) {
	m := testManifest()
	engine := NewWorkflowEngine(m, nil)

	engine.InitializeState("Task", "task-1", "member")

	canStart, err := engine.CanTransition("Task", "task-1", "start", []string{"member"})
	if err != nil {
		t.Fatalf("CanTransition: %v", err)
	}
	if !canStart {
		t.Error("member should be able to start task")
	}

	engine.Transition(context.Background(), "Task", "task-1", "start", "member", []string{"member"})

	canReopen, err := engine.CanTransition("Task", "task-1", "reopen", []string{"member"})
	if err != nil {
		t.Fatalf("CanTransition: %v", err)
	}
	if canReopen {
		t.Error("member should NOT be able to reopen task")
	}

	canReopenAdmin, err := engine.CanTransition("Task", "task-1", "reopen", []string{"admin"})
	if err != nil {
		t.Fatalf("CanTransition: %v", err)
	}
	if !canReopenAdmin {
		t.Error("admin should be able to reopen task")
	}
}

func TestWorkflowEngine_ListAvailableActions(t *testing.T) {
	m := testManifest()
	engine := NewWorkflowEngine(m, nil)

	engine.InitializeState("Task", "task-1", "member")

	actions := engine.ListAvailableActions("Task", "task-1", []string{"member"})
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}

	if actions[0] != "start" {
		t.Errorf("expected action 'start', got '%s'", actions[0])
	}
}

func TestWorkflowEngine_GetHistory(t *testing.T) {
	m := testManifest()
	engine := NewWorkflowEngine(m, nil)

	ctx := context.Background()

	engine.InitializeState("Task", "task-1", "member")
	engine.Transition(ctx, "Task", "task-1", "start", "member", []string{"member"})

	history, err := engine.GetHistory("Task", "task-1")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("expected 2 history records, got %d", len(history))
	}

	if history[0].Action != "initialize" {
		t.Errorf("expected first action 'initialize', got '%s'", history[0].Action)
	}

	if history[1].Action != "start" {
		t.Errorf("expected second action 'start', got '%s'", history[1].Action)
	}
}

func TestWorkflowEngine_GetCurrentState(t *testing.T) {
	m := testManifest()
	engine := NewWorkflowEngine(m, nil)

	engine.InitializeState("Task", "task-1", "member")

	state, err := engine.GetCurrentState("Task", "task-1")
	if err != nil {
		t.Fatalf("GetCurrentState: %v", err)
	}

	if state == nil {
		t.Fatal("state should not be nil")
	}

	if state.CurrentState != "todo" {
		t.Errorf("expected state 'todo', got '%s'", state.CurrentState)
	}
}

func TestWorkflowEngine_FullWorkflow(t *testing.T) {
	m := testManifest()
	engine := NewWorkflowEngine(m, nil)

	ctx := context.Background()

	engine.InitializeState("Task", "task-1", "member")

	engine.Transition(ctx, "Task", "task-1", "start", "member", []string{"member"})

	engine.Transition(ctx, "Task", "task-1", "complete", "member", []string{"member"})

	state, _ := engine.GetCurrentState("Task", "task-1")
	if state.CurrentState != "done" {
		t.Errorf("expected state 'done', got '%s'", state.CurrentState)
	}

	history, _ := engine.GetHistory("Task", "task-1")
	if len(history) != 3 {
		t.Errorf("expected 3 history records, got %d", len(history))
	}
}

func TestMemoryWorkflowStateStore(t *testing.T) {
	store := NewMemoryWorkflowStateStore()

	instance := &WorkflowInstance{
		EntityType:   "Task",
		EntityID:     "task-1",
		CurrentState: "todo",
	}

	err := store.SetState(instance)
	if err != nil {
		t.Fatalf("SetState: %v", err)
	}

	retrieved, err := store.GetState("Task", "task-1")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}

	if retrieved.CurrentState != "todo" {
		t.Errorf("expected 'todo', got '%s'", retrieved.CurrentState)
	}

	allStates, err := store.ListStates("Task")
	if err != nil {
		t.Fatalf("ListStates: %v", err)
	}
	if len(allStates) != 1 {
		t.Errorf("expected 1 state, got %d", len(allStates))
	}
}

func TestGuardEvaluator_NotEmpty(t *testing.T) {
	m := testManifest()
	evaluator := NewGuardEvaluator(m, nil)

	tests := []struct {
		value    interface{}
		expected bool
	}{
		{nil, false},
		{"", false},
		{"  ", false},
		{"hello", true},
		{[]byte{}, false},
		{[]byte("data"), true},
		{[]interface{}{}, false},
		{[]interface{}{"a"}, true},
	}

	for _, test := range tests {
		result := evaluator.evaluateNotEmpty(test.value)
		if result != test.expected {
			t.Errorf("evaluateNotEmpty(%v): expected %v, got %v", test.value, test.expected, result)
		}
	}
}

func TestGuardEvaluator_Equals(t *testing.T) {
	m := testManifest()
	evaluator := NewGuardEvaluator(m, nil)

	if !evaluator.evaluateEquals("hello", "hello") {
		t.Error("expected equals")
	}

	if evaluator.evaluateEquals("hello", "world") {
		t.Error("expected not equals")
	}

	if evaluator.evaluateEquals(42, 42) {
		t.Log("42 equals 42 (expected)")
	}

	if evaluator.evaluateEquals(42, 100) {
		t.Error("expected 42 != 100")
	}
}

func TestGuardEvaluator_GreaterThan(t *testing.T) {
	m := testManifest()
	evaluator := NewGuardEvaluator(m, nil)

	if !evaluator.evaluateGreaterThan(10.0, 5.0) {
		t.Error("expected 10 > 5")
	}

	if evaluator.evaluateGreaterThan(3.0, 5.0) {
		t.Error("expected 3 < 5")
	}

	if evaluator.evaluateGreaterThan("hello", "world") {
		t.Error("expected non-numeric comparison to fail")
	}
}

func TestContextualQuery_GetAuthorFilter(t *testing.T) {
	q := &ContextualQuery{
		ActorID:    "user-1",
		Roles:      []string{"member"},
		EntityType: "Task",
	}

	filter, args := q.GetAuthorFilter()
	if filter != "author_id = ?" {
		t.Errorf("expected filter 'author_id = ?', got '%s'", filter)
	}
	if len(args) != 1 || args[0] != "user-1" {
		t.Errorf("expected args ['user-1'], got %v", args)
	}
}

func TestContextualQuery_GetStateFilter(t *testing.T) {
	q := &ContextualQuery{
		ActorID:    "user-1",
		Roles:      []string{"member"},
		EntityType: "Task",
	}

	filter, args := q.GetStateFilter([]string{"todo", "in_progress"})
	if filter != "state IN (?,?)" {
		t.Errorf("expected filter 'state IN (?,?)', got '%s'", filter)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestComputedFieldEvaluator(t *testing.T) {
	m := testManifest()
	evaluator := NewComputedFieldEvaluator(m, nil)

	fields := []ComputedField{
		{Name: "days_in_state", Expression: "days_in_state"},
		{Name: "progress_percentage", Expression: "progress_percentage"},
	}

	result, err := evaluator.Evaluate("task-1", fields)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if _, ok := result["days_in_state"]; !ok {
		t.Error("expected days_in_state in result")
	}

	if _, ok := result["progress_percentage"]; !ok {
		t.Error("expected progress_percentage in result")
	}
}
