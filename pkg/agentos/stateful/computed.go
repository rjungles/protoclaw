package stateful

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type ComputedField struct {
	Name       string
	Expression string // "days_in_state", "progress_percentage", "time_since_update", etc.
	DependsOn  []string
}

type ComputedFieldEvaluator struct {
	manifest *manifest.Manifest
	db       *sql.DB
}

func NewComputedFieldEvaluator(manifest *manifest.Manifest, db *sql.DB) *ComputedFieldEvaluator {
	return &ComputedFieldEvaluator{
		manifest: manifest,
		db:       db,
	}
}

func (e *ComputedFieldEvaluator) Evaluate(entityID string, fields []ComputedField) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, field := range fields {
		switch field.Expression {
		case "days_in_state":
			days, err := e.EvaluateDaysInState(entityID)
			if err == nil {
				result[field.Name] = days
			}
		case "progress_percentage":
			progress, err := e.EvaluateProgressPercentage(entityID)
			if err == nil {
				result[field.Name] = progress
			}
		case "time_since_update":
			timeSince, err := e.EvaluateTimeSinceUpdate(entityID)
			if err == nil {
				result[field.Name] = timeSince
			}
		case "state_history_count":
			count, err := e.EvaluateStateHistoryCount(entityID)
			if err == nil {
				result[field.Name] = count
			}
		case "current_state_duration":
			duration, err := e.EvaluateCurrentStateDuration(entityID)
			if err == nil {
				result[field.Name] = duration
			}
		default:
			// Try to evaluate as a custom expression
			value, err := e.evaluateCustomExpression(entityID, field.Expression)
			if err == nil {
				result[field.Name] = value
			}
		}
	}

	return result, nil
}

func (e *ComputedFieldEvaluator) EvaluateDaysInState(entityID string) (int, error) {
	if e.db == nil {
		return 0, nil
	}

	// Get the current state and its last update time
	query := `
		SELECT ws.updated_at 
		FROM _workflow_states ws 
		WHERE ws.entity_id = ? 
		ORDER BY ws.updated_at DESC 
		LIMIT 1`

	var updatedAt time.Time
	err := e.db.QueryRow(query, entityID).Scan(&updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get state update time: %w", err)
	}

	// Calculate days since last update
	duration := time.Since(updatedAt)
	days := int(duration.Hours() / 24)

	return days, nil
}

func (e *ComputedFieldEvaluator) EvaluateProgressPercentage(entityID string) (float64, error) {
	if e.db == nil || e.manifest == nil {
		return 0.0, nil
	}

	// Get current state
	query := `SELECT current_state FROM _workflow_states WHERE entity_id = ?`
	var currentState string
	err := e.db.QueryRow(query, entityID).Scan(&currentState)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0.0, nil
		}
		return 0.0, fmt.Errorf("failed to get current state: %w", err)
	}

	// Find the workflow for this entity
	var workflow *manifest.WorkflowConfig
	for _, wf := range e.manifest.Workflows {
		if wf.Entity != "" {
			workflow = &wf
			break
		}
	}

	if workflow == nil {
		return 0.0, nil
	}

	totalStates := len(workflow.States)
	if totalStates == 0 {
		return 0.0, nil
	}

	// Find current state index
	currentIndex := -1
	for i, state := range workflow.States {
		if state.ID == currentState {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		return 0.0, nil
	}

	// Calculate progress percentage
	progress := float64(currentIndex+1) / float64(totalStates) * 100
	return progress, nil
}

func (e *ComputedFieldEvaluator) EvaluateTimeSinceUpdate(entityID string) (string, error) {
	if e.db == nil {
		return "", nil
	}

	query := `
		SELECT ws.updated_at 
		FROM _workflow_states ws 
		WHERE ws.entity_id = ? 
		ORDER BY ws.updated_at DESC 
		LIMIT 1`

	var updatedAt time.Time
	err := e.db.QueryRow(query, entityID).Scan(&updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return "never", nil
		}
		return "", fmt.Errorf("failed to get update time: %w", err)
	}

	duration := time.Since(updatedAt)

	// Format duration in a human-readable way
	if duration < time.Minute {
		return fmt.Sprintf("%d seconds ago", int(duration.Seconds())), nil
	} else if duration < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(duration.Minutes())), nil
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(duration.Hours())), nil
	} else {
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%d days ago", days), nil
	}
}

func (e *ComputedFieldEvaluator) EvaluateStateHistoryCount(entityID string) (int, error) {
	if e.db == nil {
		return 0, nil
	}

	query := `SELECT COUNT(*) FROM _workflow_history WHERE entity_id = ?`
	var count int
	err := e.db.QueryRow(query, entityID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count history: %w", err)
	}

	return count, nil
}

func (e *ComputedFieldEvaluator) EvaluateCurrentStateDuration(entityID string) (string, error) {
	if e.db == nil {
		return "", nil
	}

	// Get when the current state started
	query := `
		SELECT wh.timestamp 
		FROM _workflow_history wh 
		WHERE wh.entity_id = ? 
		AND wh.to_state = (
			SELECT current_state 
			FROM _workflow_states 
			WHERE entity_id = ?
		)
		ORDER BY wh.timestamp DESC 
		LIMIT 1`

	var stateStart time.Time
	err := e.db.QueryRow(query, entityID, entityID).Scan(&stateStart)
	if err != nil {
		if err == sql.ErrNoRows {
			return "unknown", nil
		}
		return "", fmt.Errorf("failed to get state start time: %w", err)
	}

	duration := time.Since(stateStart)

	// Format duration
	if duration < time.Minute {
		return fmt.Sprintf("%d seconds", int(duration.Seconds())), nil
	} else if duration < time.Hour {
		return fmt.Sprintf("%d minutes", int(duration.Minutes())), nil
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(duration.Hours())), nil
	} else {
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%d days", days), nil
	}
}

func (e *ComputedFieldEvaluator) evaluateCustomExpression(entityID string, expression string) (interface{}, error) {
	// Placeholder for custom expression evaluation
	// This could support more complex expressions like:
	// - Field comparisons
	// - Mathematical operations
	// - Conditional logic
	// - External API calls

	return nil, fmt.Errorf("custom expression '%s' not implemented", expression)
}

// RegisterComputedField allows registering custom computed field functions
func (e *ComputedFieldEvaluator) RegisterComputedField(name string, evaluator func(entityID string) (interface{}, error)) {
	// This would allow users to register custom computed field functions
	// For now, this is a placeholder for future extensibility
}
