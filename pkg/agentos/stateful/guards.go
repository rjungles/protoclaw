package stateful

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/workflow"
)

type Guard struct {
	Type      string
	Field     string
	Value     interface{}
	Condition string
}

type GuardEvaluator struct {
	manifest *manifest.Manifest
	db       *sql.DB
}

func NewGuardEvaluator(manifest *manifest.Manifest, db *sql.DB) *GuardEvaluator {
	return &GuardEvaluator{
		manifest: manifest,
		db:       db,
	}
}

func (e *GuardEvaluator) EvaluateGuards(entityType, entityID string, currentState workflow.State, action workflow.Action) (bool, string) {
	if e.manifest == nil {
		return true, ""
	}

	for _, wf := range e.manifest.Workflows {
		if wf.Entity != entityType {
			continue
		}

		for _, state := range wf.States {
			if state.ID != string(currentState) {
				continue
			}

			for _, trans := range state.Transitions {
				if trans.Action != string(action) {
					continue
				}

				if state.Guards == nil || len(state.Guards) == 0 {
					return true, ""
				}

				for _, guard := range state.Guards {
					allowed, reason := e.evaluateGuard(&guard, entityID)
					if !allowed {
						return false, reason
					}
				}

				return true, ""
			}
		}
	}

	return true, ""
}

func (e *GuardEvaluator) evaluateGuard(guard *manifest.WorkflowGuard, entityID string) (bool, string) {
	if guard.Field == "" {
		return true, ""
	}

	fieldValue, err := e.getFieldValue(entityID, guard.Field)
	if err != nil {
		return false, fmt.Sprintf("failed to get field '%s': %v", guard.Field, err)
	}

	switch guard.Condition {
	case "not_empty":
		return e.evaluateNotEmpty(fieldValue), fmt.Sprintf("field '%s' must not be empty", guard.Field)
	case "equals":
		return e.evaluateEquals(fieldValue, guard.Value), fmt.Sprintf("field '%s' must equal '%v'", guard.Field, guard.Value)
	case "not_equals":
		return e.evaluateNotEquals(fieldValue, guard.Value), fmt.Sprintf("field '%s' must not equal '%v'", guard.Field, guard.Value)
	case "greater_than":
		return e.evaluateGreaterThan(fieldValue, guard.Value), fmt.Sprintf("field '%s' must be greater than '%v'", guard.Field, guard.Value)
	case "less_than":
		return e.evaluateLessThan(fieldValue, guard.Value), fmt.Sprintf("field '%s' must be less than '%v'", guard.Field, guard.Value)
	case "contains":
		return e.evaluateContains(fieldValue, guard.Value), fmt.Sprintf("field '%s' must contain '%v'", guard.Field, guard.Value)
	default:
		return true, ""
	}
}

func (e *GuardEvaluator) getFieldValue(entityID, fieldName string) (interface{}, error) {
	if e.db == nil {
		return nil, nil
	}

	// Validate inputs to prevent SQL injection
	if err := validateIdentifier(entityID); err != nil {
		return nil, fmt.Errorf("invalid entityID: %w", err)
	}
	if err := validateIdentifier(fieldName); err != nil {
		return nil, fmt.Errorf("invalid fieldName: %w", err)
	}

	// Use a more efficient approach - query the specific entity table directly
	// based on the workflow configuration
	for _, wf := range e.manifest.Workflows {
		if wf.Entity == "" {
			continue
		}
		
		tableName := toSnakeCase(wf.Entity)
		
		// Build parameterized query to prevent SQL injection
		query := fmt.Sprintf("SELECT %s FROM %s WHERE id = ? LIMIT 1", 
			toSnakeCase(fieldName), tableName)
		
		var value interface{}
		err := e.db.QueryRow(query, entityID).Scan(&value)
		
		if err == nil {
			return value, nil
		}
		if err == sql.ErrNoRows {
			continue
		}
		return nil, err
	}

	return nil, fmt.Errorf("field '%s' not found in any entity", fieldName)
}

func (e *GuardEvaluator) evaluateNotEmpty(value interface{}) bool {
	if value == nil {
		return false
	}

	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []byte:
		return len(v) > 0
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	default:
		return true
	}
}

func (e *GuardEvaluator) evaluateEquals(value interface{}, expected interface{}) bool {
	if value == nil && expected == nil {
		return true
	}
	if value == nil || expected == nil {
		return false
	}

	strValue := fmt.Sprintf("%v", value)
	strExpected := fmt.Sprintf("%v", expected)
	return strValue == strExpected
}

func (e *GuardEvaluator) evaluateNotEquals(value interface{}, expected interface{}) bool {
	return !e.evaluateEquals(value, expected)
}

func (e *GuardEvaluator) evaluateGreaterThan(value interface{}, threshold interface{}) bool {
	floatValue, ok1 := toFloat64(value)
	floatThreshold, ok2 := toFloat64(threshold)
	if !ok1 || !ok2 {
		return false
	}
	return floatValue > floatThreshold
}

func (e *GuardEvaluator) evaluateLessThan(value interface{}, threshold interface{}) bool {
	floatValue, ok1 := toFloat64(value)
	floatThreshold, ok2 := toFloat64(threshold)
	if !ok1 || !ok2 {
		return false
	}
	return floatValue < floatThreshold
}

func (e *GuardEvaluator) evaluateContains(value interface{}, substring interface{}) bool {
	strValue := fmt.Sprintf("%v", value)
	strSubstring := fmt.Sprintf("%v", substring)
	return strings.Contains(strValue, strSubstring)
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, []rune(strings.ToLower(string(r)))...)
	}
	return string(result)
}

// validateIdentifier validates that a string is safe to use in SQL queries
func validateIdentifier(s string) error {
	if s == "" {
		return fmt.Errorf("identifier cannot be empty")
	}
	// Allow only alphanumeric characters and underscores
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return fmt.Errorf("identifier contains invalid character: %c", r)
		}
	}
	// Check for SQL keywords that could be dangerous
	sqlKeywords := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER", "EXEC", "SCRIPT"}
	upperS := strings.ToUpper(s)
	for _, keyword := range sqlKeywords {
		if strings.Contains(upperS, keyword) {
			return fmt.Errorf("identifier contains SQL keyword: %s", keyword)
		}
	}
	return nil
}
