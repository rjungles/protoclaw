package stateful

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/workflow"
)

type WorkflowInstance struct {
	ID            string
	EntityType    string
	EntityID      string
	CurrentState  string
	PreviousState string
	UpdatedAt     time.Time
	UpdatedBy     string
	Metadata      map[string]interface{}
}

type TransitionRecord struct {
	ID         string
	EntityType string
	EntityID   string
	FromState  string
	ToState    string
	Action     string
	ActorID    string
	Timestamp  time.Time
	Metadata   map[string]interface{}
}

type WorkflowStateStore interface {
	GetState(entityType, entityID string) (*WorkflowInstance, error)
	SetState(instance *WorkflowInstance) error
	ListStates(entityType string) ([]*WorkflowInstance, error)
	GetHistory(entityType, entityID string) ([]TransitionRecord, error)
	RecordTransition(record *TransitionRecord) error
}

type MemoryWorkflowStateStore struct {
	states  map[string]*WorkflowInstance
	history []TransitionRecord
}

func NewMemoryWorkflowStateStore() *MemoryWorkflowStateStore {
	return &MemoryWorkflowStateStore{
		states:  make(map[string]*WorkflowInstance),
		history: make([]TransitionRecord, 0),
	}
}

func (s *MemoryWorkflowStateStore) key(entityType, entityID string) string {
	return entityType + ":" + entityID
}

func (s *MemoryWorkflowStateStore) GetState(entityType, entityID string) (*WorkflowInstance, error) {
	if inst, ok := s.states[s.key(entityType, entityID)]; ok {
		return inst, nil
	}
	return nil, nil
}

func (s *MemoryWorkflowStateStore) SetState(instance *WorkflowInstance) error {
	s.states[s.key(instance.EntityType, instance.EntityID)] = instance
	return nil
}

func (s *MemoryWorkflowStateStore) ListStates(entityType string) ([]*WorkflowInstance, error) {
	result := make([]*WorkflowInstance, 0)
	for _, inst := range s.states {
		if inst.EntityType == entityType {
			result = append(result, inst)
		}
	}
	return result, nil
}

func (s *MemoryWorkflowStateStore) GetHistory(entityType, entityID string) ([]TransitionRecord, error) {
	result := make([]TransitionRecord, 0)
	for _, rec := range s.history {
		if rec.EntityType == entityType && rec.EntityID == entityID {
			result = append(result, rec)
		}
	}
	return result, nil
}

func (s *MemoryWorkflowStateStore) RecordTransition(record *TransitionRecord) error {
	s.history = append(s.history, *record)
	return nil
}

type DBWorkflowStateStore struct {
	db *sql.DB
}

func NewDBWorkflowStateStore(db *sql.DB) *DBWorkflowStateStore {
	store := &DBWorkflowStateStore{db: db}
	store.createTables()
	return store
}

func (s *DBWorkflowStateStore) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS _workflow_states (
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			current_state TEXT NOT NULL,
			previous_state TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_by TEXT,
			metadata TEXT,
			PRIMARY KEY (entity_type, entity_id)
		)`,
		`CREATE TABLE IF NOT EXISTS _workflow_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			from_state TEXT,
			to_state TEXT NOT NULL,
			action TEXT NOT NULL,
			actor_id TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_history_entity ON _workflow_history(entity_type, entity_id)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

func (s *DBWorkflowStateStore) GetState(entityType, entityID string) (*WorkflowInstance, error) {
	query := `SELECT entity_type, entity_id, current_state, previous_state, updated_at, updated_by, metadata
			 FROM _workflow_states WHERE entity_type = ? AND entity_id = ?`

	var inst WorkflowInstance
	var updatedBy sql.NullString
	var metadata sql.NullString

	err := s.db.QueryRow(query, entityType, entityID).Scan(
		&inst.EntityType, &inst.EntityID, &inst.CurrentState, &inst.PreviousState,
		&inst.UpdatedAt, &updatedBy, &metadata,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if updatedBy.Valid {
		inst.UpdatedBy = updatedBy.String
	}
	if metadata.Valid {
		inst.Metadata = parseMetadata(metadata.String)
	}
	return &inst, nil
}

func (s *DBWorkflowStateStore) SetState(instance *WorkflowInstance) error {
	query := `INSERT OR REPLACE INTO _workflow_states (entity_type, entity_id, current_state, previous_state, updated_at, updated_by, metadata)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`

	metadata := ""
	if instance.Metadata != nil {
		metadata = formatMetadata(instance.Metadata)
	}

	_, err := s.db.Exec(query,
		instance.EntityType, instance.EntityID, instance.CurrentState,
		instance.PreviousState, instance.UpdatedAt, instance.UpdatedBy, metadata,
	)
	return err
}

func (s *DBWorkflowStateStore) ListStates(entityType string) ([]*WorkflowInstance, error) {
	query := `SELECT entity_type, entity_id, current_state, previous_state, updated_at, updated_by, metadata
			 FROM _workflow_states WHERE entity_type = ?`

	rows, err := s.db.Query(query, entityType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]*WorkflowInstance, 0)
	for rows.Next() {
		var inst WorkflowInstance
		var updatedBy sql.NullString
		var metadata sql.NullString
		if err := rows.Scan(&inst.EntityType, &inst.EntityID, &inst.CurrentState, &inst.PreviousState, &inst.UpdatedAt, &updatedBy, &metadata); err != nil {
			return nil, err
		}
		if updatedBy.Valid {
			inst.UpdatedBy = updatedBy.String
		}
		if metadata.Valid {
			inst.Metadata = parseMetadata(metadata.String)
		}
		result = append(result, &inst)
	}

	return result, rows.Err()
}

func (s *DBWorkflowStateStore) GetHistory(entityType, entityID string) ([]TransitionRecord, error) {
	query := `SELECT id, entity_type, entity_id, from_state, to_state, action, actor_id, timestamp, metadata
			 FROM _workflow_history WHERE entity_type = ? AND entity_id = ? ORDER BY timestamp ASC`

	rows, err := s.db.Query(query, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]TransitionRecord, 0)
	for rows.Next() {
		var rec TransitionRecord
		var actorID sql.NullString
		var metadata sql.NullString
		if err := rows.Scan(&rec.ID, &rec.EntityType, &rec.EntityID, &rec.FromState, &rec.ToState, &rec.Action, &actorID, &rec.Timestamp, &metadata); err != nil {
			return nil, err
		}
		if actorID.Valid {
			rec.ActorID = actorID.String
		}
		if metadata.Valid {
			rec.Metadata = parseMetadata(metadata.String)
		}
		result = append(result, rec)
	}

	return result, rows.Err()
}

func (s *DBWorkflowStateStore) RecordTransition(record *TransitionRecord) error {
	query := `INSERT INTO _workflow_history (entity_type, entity_id, from_state, to_state, action, actor_id, timestamp, metadata)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	metadata := ""
	if record.Metadata != nil {
		metadata = formatMetadata(record.Metadata)
	}

	_, err := s.db.Exec(query,
		record.EntityType, record.EntityID, record.FromState,
		record.ToState, record.Action, record.ActorID, record.Timestamp, metadata,
	)
	return err
}

type WorkflowEngine struct {
	manifest    *manifest.Manifest
	db          *sql.DB
	store       WorkflowStateStore
	fsms        map[string]*workflow.FSM
	guards      *GuardEvaluator
	sideEffects *SideEffectExecutor
	timeouts    *TimeoutManager
}

func NewWorkflowEngine(manifest *manifest.Manifest, db *sql.DB) *WorkflowEngine {
	var store WorkflowStateStore = NewMemoryWorkflowStateStore()
	if db != nil {
		store = NewDBWorkflowStateStore(db)
	}

	fsms := make(map[string]*workflow.FSM)
	for _, wf := range manifest.Workflows {
		fsmConfig := workflow.FSMConfig{
			EntityName:   wf.Entity,
			InitialState: workflow.State(wf.InitialState),
			States:       convertWorkflowStates(wf.States),
		}
		if fsm, err := workflow.NewFSM(fsmConfig); err == nil {
			fsms[wf.Entity] = fsm
		}
	}

	return &WorkflowEngine{
		manifest:    manifest,
		db:          db,
		store:       store,
		fsms:        fsms,
		guards:      NewGuardEvaluator(manifest, db),
		sideEffects: NewSideEffectExecutor(manifest, db, nil),
		timeouts:    NewTimeoutManager(manifest, db, store),
	}
}

func (e *WorkflowEngine) Transition(ctx context.Context, entityType, entityID, action, actorID string, roles []string) (*WorkflowInstance, error) {
	fsm, ok := e.fsms[entityType]
	if !ok {
		return nil, nil
	}

	current, err := e.store.GetState(entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current state: %w", err)
	}

	currentState := workflow.State(fsm.InitialState())
	if current != nil {
		currentState = workflow.State(current.CurrentState)
	}

	trans := fsm.FindTransition(currentState, workflow.Action(action))
	if trans == nil {
		return nil, fmt.Errorf("action '%s' is not valid in state '%s'", action, currentState)
	}

	// Verificar roles
	hasRole := false
	for _, role := range roles {
		for _, allowed := range trans.AllowedRoles {
			if role == allowed {
				hasRole = true
				break
			}
		}
		if hasRole {
			break
		}
	}
	if !hasRole {
		return nil, fmt.Errorf("user with roles %v is not authorized for action '%s'", roles, action)
	}

	// Avaliar guards
	if e.guards != nil {
		allowed, reason := e.guards.EvaluateGuards(entityType, entityID, currentState, workflow.Action(action))
		if !allowed {
			return nil, fmt.Errorf("guard evaluation failed: %s", reason)
		}
	}

	previousState := string(currentState)
	newState := string(trans.To)

	// Executar side effects on exit
	if e.sideEffects != nil {
		if err := e.sideEffects.ExecuteOnExit(previousState, entityID, actorID); err != nil {
			return nil, fmt.Errorf("on_exit side effect failed: %w", err)
		}
	}

	instance := &WorkflowInstance{
		ID:            fmt.Sprintf("%s:%s", entityType, entityID),
		EntityType:    entityType,
		EntityID:      entityID,
		CurrentState:  newState,
		PreviousState: previousState,
		UpdatedAt:     time.Now(),
		UpdatedBy:     actorID,
	}

	if err := e.store.SetState(instance); err != nil {
		return nil, fmt.Errorf("failed to save state: %w", err)
	}

	record := &TransitionRecord{
		EntityType: entityType,
		EntityID:   entityID,
		FromState:  previousState,
		ToState:    newState,
		Action:     action,
		ActorID:    actorID,
		Timestamp:  time.Now(),
	}

	if err := e.store.RecordTransition(record); err != nil {
		return nil, fmt.Errorf("failed to record transition: %w", err)
	}

	// Executar side effects on enter
	if e.sideEffects != nil {
		if err := e.sideEffects.ExecuteOnEnter(newState, entityID, actorID); err != nil {
			return nil, fmt.Errorf("on_enter side effect failed: %w", err)
		}
	}

	return instance, nil
}

func (e *WorkflowEngine) GetCurrentState(entityType, entityID string) (*WorkflowInstance, error) {
	return e.store.GetState(entityType, entityID)
}

func (e *WorkflowEngine) CanTransition(entityType, entityID, action string, roles []string) (bool, error) {
	fsm, ok := e.fsms[entityType]
	if !ok {
		return false, fmt.Errorf("no workflow defined for entity type: %s", entityType)
	}

	current, err := e.store.GetState(entityType, entityID)
	if err != nil {
		return false, err
	}

	currentState := workflow.State(fsm.InitialState())
	if current != nil {
		currentState = workflow.State(current.CurrentState)
	}

	trans := fsm.FindTransition(currentState, workflow.Action(action))
	if trans == nil {
		return false, nil
	}

	for _, role := range roles {
		for _, allowed := range trans.AllowedRoles {
			if role == allowed {
				return true, nil
			}
		}
	}

	return false, nil
}

func (e *WorkflowEngine) ListAvailableActions(entityType, entityID string, roles []string) []string {
	fsm, ok := e.fsms[entityType]
	if !ok {
		return nil
	}

	current, _ := e.store.GetState(entityType, entityID)
	currentState := workflow.State(fsm.InitialState())
	if current != nil {
		currentState = workflow.State(current.CurrentState)
	}

	transitions := fsm.ListTransitions(currentState, roles)
	actions := make([]string, 0, len(transitions))
	for _, t := range transitions {
		actions = append(actions, string(t.Action))
	}

	return actions
}

func (e *WorkflowEngine) GetHistory(entityType, entityID string) ([]TransitionRecord, error) {
	return e.store.GetHistory(entityType, entityID)
}

func (e *WorkflowEngine) InitializeState(entityType, entityID, actorID string) (*WorkflowInstance, error) {
	fsm, ok := e.fsms[entityType]
	if !ok {
		return nil, fmt.Errorf("no workflow defined for entity type: %s", entityType)
	}

	instance := &WorkflowInstance{
		ID:           fmt.Sprintf("%s:%s", entityType, entityID),
		EntityType:   entityType,
		EntityID:     entityID,
		CurrentState: string(fsm.InitialState()),
		UpdatedAt:    time.Now(),
		UpdatedBy:    actorID,
	}

	if err := e.store.SetState(instance); err != nil {
		return nil, err
	}

	record := &TransitionRecord{
		EntityType: entityType,
		EntityID:   entityID,
		FromState:  "",
		ToState:    string(fsm.InitialState()),
		Action:     "initialize",
		ActorID:    actorID,
		Timestamp:  time.Now(),
	}

	e.store.RecordTransition(record)
	return instance, nil
}

func convertWorkflowStates(states []manifest.WorkflowState) map[workflow.State]workflow.StateConfig {
	result := make(map[workflow.State]workflow.StateConfig)
	for _, s := range states {
		transitions := make([]workflow.Transition, len(s.Transitions))
		for i, t := range s.Transitions {
			transitions[i] = workflow.Transition{
				To:           workflow.State(t.To),
				Action:       workflow.Action(t.Action),
				AllowedRoles: t.AllowedRoles,
			}
		}
		result[workflow.State(s.ID)] = workflow.StateConfig{
			ID:          workflow.State(s.ID),
			Transitions: transitions,
		}
	}
	return result
}

func parseMetadata(s string) map[string]interface{} {
	if s == "" || s == "{}" {
		return make(map[string]interface{})
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return make(map[string]interface{})
	}
	return result
}

func formatMetadata(m map[string]interface{}) string {
	if len(m) == 0 {
		return "{}"
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(data)
}
