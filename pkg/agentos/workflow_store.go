package agentos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type WorkflowState struct {
	EntityID    string    `json:"entity_id"`
	EntityType  string    `json:"entity_type"`
	CurrentState string   `json:"current_state"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   string    `json:"updated_by"`
}

type WorkflowStore interface {
	Get(entityType, entityID string) (*WorkflowState, error)
	Set(state *WorkflowState) error
	List(entityType string) ([]*WorkflowState, error)
}

type MemoryWorkflowStore struct {
	mu     sync.RWMutex
	states map[string]*WorkflowState
}

func NewMemoryWorkflowStore() *MemoryWorkflowStore {
	return &MemoryWorkflowStore{
		states: make(map[string]*WorkflowState),
	}
}

func (s *MemoryWorkflowStore) Get(entityType, entityID string) (*WorkflowState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := entityType + ":" + entityID
	state, ok := s.states[key]
	if !ok {
		return nil, nil
	}
	cp := *state
	return &cp, nil
}

func (s *MemoryWorkflowStore) Set(state *WorkflowState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := state.EntityType + ":" + state.EntityID
	cp := *state
	s.states[key] = &cp
	return nil
}

func (s *MemoryWorkflowStore) List(entityType string) ([]*WorkflowState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*WorkflowState
	for _, state := range s.states {
		if state.EntityType == entityType {
			cp := *state
			result = append(result, &cp)
		}
	}
	return result, nil
}

type FileWorkflowStore struct {
	mu   sync.Mutex
	path string
}

func NewFileWorkflowStore(path string) (*FileWorkflowStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	return &FileWorkflowStore{path: path}, nil
}

func (s *FileWorkflowStore) Get(entityType, entityID string) (*WorkflowState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return nil, err
	}
	key := entityType + ":" + entityID
	state, ok := states[key]
	if !ok {
		return nil, nil
	}
	return state, nil
}

func (s *FileWorkflowStore) Set(state *WorkflowState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return err
	}
	key := state.EntityType + ":" + state.EntityID
	cp := *state
	states[key] = &cp
	return s.save(states)
}

func (s *FileWorkflowStore) List(entityType string) ([]*WorkflowState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	states, err := s.load()
	if err != nil {
		return nil, err
	}
	var result []*WorkflowState
	for _, state := range states {
		if state.EntityType == entityType {
			result = append(result, state)
		}
	}
	return result, nil
}

func (s *FileWorkflowStore) load() (map[string]*WorkflowState, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return make(map[string]*WorkflowState), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow store: %w", err)
	}
	var states map[string]*WorkflowState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, fmt.Errorf("failed to parse workflow store: %w", err)
	}
	return states, nil
}

func (s *FileWorkflowStore) save(states map[string]*WorkflowState) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal workflow store: %w", err)
	}
	return os.WriteFile(s.path, data, 0o644)
}
