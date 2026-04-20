package stateful

import (
	"database/sql"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type NotificationBus interface {
	Notify(toActor, fromActor, notifType, title, body string, data map[string]interface{}) error
}

type SideEffectExecutor struct {
	manifest  *manifest.Manifest
	db        *sql.DB
	notifyBus NotificationBus
}

func NewSideEffectExecutor(manifest *manifest.Manifest, db *sql.DB, notifyBus NotificationBus) *SideEffectExecutor {
	return &SideEffectExecutor{
		manifest:  manifest,
		db:        db,
		notifyBus: notifyBus,
	}
}

func (e *SideEffectExecutor) ExecuteOnEnter(state, entityID, actorID string) error {
	if e.manifest == nil {
		return nil
	}
	for _, wf := range e.manifest.Workflows {
		for _, s := range wf.States {
			if s.ID != state {
				continue
			}
			for _, action := range s.OnEnter {
				if err := e.executeAction(&action, entityID, actorID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (e *SideEffectExecutor) ExecuteOnExit(state, entityID, actorID string) error {
	if e.manifest == nil {
		return nil
	}
	for _, wf := range e.manifest.Workflows {
		for _, s := range wf.States {
			if s.ID != state {
				continue
			}
			for _, action := range s.OnExit {
				if err := e.executeAction(&action, entityID, actorID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (e *SideEffectExecutor) executeAction(action *manifest.WorkflowAction, entityID, actorID string) error {
	switch action.Action {
	case "notify":
		return e.executeNotify(action, entityID, actorID)
	case "update_field":
		return e.executeUpdateField(action, entityID)
	case "log":
		return e.executeLog(action, entityID, actorID)
	default:
		return nil
	}
}

func (e *SideEffectExecutor) executeNotify(action *manifest.WorkflowAction, entityID, actorID string) error {
	if e.notifyBus == nil {
		return nil
	}
	target := action.Target
	if target == "" {
		target = "*"
	}
	message := action.Message
	if message == "" {
		message = fmt.Sprintf("State changed to %s", entityID)
	}
	return e.notifyBus.Notify(target, actorID, "state_change", "State Update", message, map[string]interface{}{
		"entity_id": entityID,
	})
}

func (e *SideEffectExecutor) executeUpdateField(action *manifest.WorkflowAction, entityID string) error {
	if e.db == nil {
		return nil
	}
	field := action.Config["field"]
	value := action.Config["value"]
	if field == nil || value == nil {
		return nil
	}
	fieldName, ok := field.(string)
	if !ok {
		return fmt.Errorf("field must be a string")
	}

	// Validate inputs to prevent SQL injection
	if err := validateIdentifier(entityID); err != nil {
		return fmt.Errorf("invalid entityID: %w", err)
	}
	if err := validateIdentifier(fieldName); err != nil {
		return fmt.Errorf("invalid fieldName: %w", err)
	}

	// Use a more efficient approach - find the correct entity based on workflow
	for _, wf := range e.manifest.Workflows {
		if wf.Entity == "" {
			continue
		}
		tableName := toSnakeCase(wf.Entity)

		// Build parameterized query to prevent SQL injection
		query := fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", tableName, toSnakeCase(fieldName))
		result, err := e.db.Exec(query, value, entityID)
		if err != nil {
			continue
		}

		// Check if any rows were affected
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			continue
		}
		if rowsAffected > 0 {
			return nil
		}
	}
	return fmt.Errorf("field '%s' not found in any entity or no rows updated", fieldName)
}

func (e *SideEffectExecutor) executeLog(action *manifest.WorkflowAction, entityID, actorID string) error {
	return nil
}

type ContextualQuery struct {
	ActorID    string
	Roles      []string
	EntityType string
}

func (q *ContextualQuery) ApplyFilters(sqlQuery string, args []interface{}) (string, []interface{}) {
	return sqlQuery, args
}

func (q *ContextualQuery) GetAuthorFilter() (string, []interface{}) {
	if q.ActorID == "" {
		return "", nil
	}
	return "author_id = ?", []interface{}{q.ActorID}
}

func (q *ContextualQuery) GetStateFilter(states []string) (string, []interface{}) {
	if len(states) == 0 {
		return "", nil
	}

	placeholders := make([]string, len(states))
	args := make([]interface{}, len(states))
	for i, s := range states {
		placeholders[i] = "?"
		args[i] = s
	}

	return fmt.Sprintf("state IN (%s)", joinStrings(placeholders, ",")), args
}

// hasRole verifica se o ator tem uma role específica
func (q *ContextualQuery) hasRole(role string) bool {
	for _, r := range q.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// GetVisibilityFilter retorna filtro baseado na visibilidade permitida
func (q *ContextualQuery) GetVisibilityFilter() (string, []interface{}) {
	// Admin pode ver tudo
	if q.hasRole("admin") {
		return "", nil
	}

	// Outros usuários só podem ver itens públicos ou seus próprios
	return "(visibility = ? OR created_by = ?)", []interface{}{"public", q.ActorID}
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
