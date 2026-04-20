package stateful

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type TimeoutConfig struct {
	Duration     time.Duration
	TransitionTo string
	Action       string
}

type TimeoutManager struct {
	manifest *manifest.Manifest
	db       *sql.DB
	store    WorkflowStateStore
}

func NewTimeoutManager(manifest *manifest.Manifest, db *sql.DB, store WorkflowStateStore) *TimeoutManager {
	return &TimeoutManager{
		manifest: manifest,
		db:       db,
		store:    store,
	}
}

func (m *TimeoutManager) CheckTimeouts(ctx context.Context) ([]TransitionRecord, error) {
	if m.db == nil {
		return nil, nil
	}

	// Query for states that have timeout configurations and are past their deadline
	query := `
		SELECT ws.entity_type, ws.entity_id, ws.current_state, ws.updated_at
		FROM _workflow_states ws
		WHERE EXISTS (
			SELECT 1 FROM _workflow_timeouts wt 
			WHERE wt.entity_type = ws.entity_type 
			AND wt.entity_id = ws.entity_id 
			AND wt.state = ws.current_state
			AND wt.deadline <= datetime('now')
		)`

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query timeouts: %w", err)
	}
	defer rows.Close()

	var results []TransitionRecord

	for rows.Next() {
		var entityType, entityID, currentState string
		var updatedAt time.Time

		if err := rows.Scan(&entityType, &entityID, &currentState, &updatedAt); err != nil {
			continue
		}

		// Get timeout configuration for this state
		timeoutConfig := m.GetTimeoutForState(entityType, currentState)
		if timeoutConfig == nil {
			continue
		}

		// Create transition record for the timeout
		record := TransitionRecord{
			EntityType: entityType,
			EntityID:   entityID,
			FromState:  currentState,
			ToState:    timeoutConfig.TransitionTo,
			Action:     timeoutConfig.Action,
			ActorID:    "system",
			Timestamp:  time.Now(),
			Metadata: map[string]interface{}{
				"timeout_triggered": true,
				"timeout_duration":  timeoutConfig.Duration.String(),
			},
		}

		results = append(results, record)
	}

	return results, rows.Err()
}

func (m *TimeoutManager) GetTimeoutForState(entityType, state string) *TimeoutConfig {
	if m.manifest == nil {
		return nil
	}

	for _, wf := range m.manifest.Workflows {
		if wf.Entity != entityType {
			continue
		}

		for _, s := range wf.States {
			if s.ID != state {
				continue
			}

			if s.Timeout != nil {
				duration, err := time.ParseDuration(s.Timeout.Duration)
				if err != nil {
					continue
				}

				return &TimeoutConfig{
					Duration:     duration,
					TransitionTo: s.Timeout.TransitionTo,
					Action:       s.Timeout.Action,
				}
			}
		}
	}

	return nil
}

func (m *TimeoutManager) SetTimeout(entityType, entityID, state string, deadline time.Time) error {
	if m.db == nil {
		return nil
	}

	// Clear any existing timeout for this entity/state
	if err := m.ClearTimeout(entityType, entityID, state); err != nil {
		return err
	}

	// Insert new timeout
	query := `INSERT INTO _workflow_timeouts (entity_type, entity_id, state, deadline) VALUES (?, ?, ?, ?)`
	_, err := m.db.Exec(query, entityType, entityID, state, deadline)
	return err
}

func (m *TimeoutManager) ClearTimeout(entityType, entityID, state string) error {
	if m.db == nil {
		return nil
	}

	query := `DELETE FROM _workflow_timeouts WHERE entity_type = ? AND entity_id = ? AND state = ?`
	_, err := m.db.Exec(query, entityType, entityID, state)
	return err
}

func (m *TimeoutManager) CreateTables() error {
	if m.db == nil {
		return nil
	}

	query := `
	CREATE TABLE IF NOT EXISTS _workflow_timeouts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		state TEXT NOT NULL,
		deadline DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(entity_type, entity_id, state)
	)`

	_, err := m.db.Exec(query)
	return err
}

func (m *TimeoutManager) CleanupExpiredTimeouts() error {
	if m.db == nil {
		return nil
	}

	query := `DELETE FROM _workflow_timeouts WHERE deadline <= datetime('now')`
	_, err := m.db.Exec(query)
	return err
}
