package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/agentos"
	"github.com/sipeed/picoclaw/pkg/api"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/manifest"
)

type OperationHandler struct {
	manifest     *manifest.Manifest
	catalog      *agentos.OperationCatalog
	actorStore   agentos.ActorStore
	policyEngine *policy.Engine
	ruleExecutor *api.RuleExecutor
	db           *sql.DB
}

func NewOperationHandler(
	m *manifest.Manifest,
	catalog *agentos.OperationCatalog,
	actorStore agentos.ActorStore,
	policyEngine *policy.Engine,
	ruleExecutor *api.RuleExecutor,
	db *sql.DB,
) *OperationHandler {
	return &OperationHandler{
		manifest:     m,
		catalog:      catalog,
		actorStore:   actorStore,
		policyEngine: policyEngine,
		ruleExecutor: ruleExecutor,
		db:           db,
	}
}

func (h *OperationHandler) ExecuteTool(ctx context.Context, toolName string, args map[string]interface{}, actorID string) (interface{}, error) {
	op := h.findOperationByToolName(toolName)
	if op == nil {
		return nil, fmt.Errorf("operation not found: %s", toolName)
	}

	if err := h.checkPermission(actorID, op, args); err != nil {
		return nil, fmt.Errorf("permission denied: %w", err)
	}

	if err := h.executeBeforeRules(ctx, op, args); err != nil {
		return nil, fmt.Errorf("before rule rejected: %w", err)
	}

	result, err := h.executeOperation(ctx, op, args, actorID)
	if err != nil {
		return nil, fmt.Errorf("operation failed: %w", err)
	}

	h.executeAfterRules(ctx, op, result)

	return result, nil
}

func (h *OperationHandler) findOperationByToolName(toolName string) *agentos.Operation {
	operations := h.catalog.ListAll()
	for i := range operations {
		op := &operations[i]
		if op.Name == toolName {
			return op
		}
	}
	return nil
}

func (h *OperationHandler) checkPermission(actorID string, op *agentos.Operation, args map[string]interface{}) error {
	if h.policyEngine == nil {
		return nil
	}

	resource := op.Entity
	if op.Action == "transition" {
		resource = fmt.Sprintf("workflow:%s:%s", op.Entity, op.WorkflowAction)
	}

	action := op.Action
	if action == "get" || action == "list" {
		action = "read"
	}

	authCtx := &policy.Context{
		ActorID:    actorID,
		Resource:   resource,
		Action:     action,
		Attributes: extractAttributes(op, args),
	}

	result := h.policyEngine.CheckPermission(authCtx)
	if !result.Allowed {
		return fmt.Errorf("%s", result.Reason)
	}

	return nil
}

func (h *OperationHandler) executeBeforeRules(ctx context.Context, op *agentos.Operation, args map[string]interface{}) error {
	if h.ruleExecutor == nil {
		return nil
	}

	data := h.buildOperationData(op, args)

	err := h.ruleExecutor.ExecuteBefore(ctx, op.Action, op.Entity, data)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "reject") {
		return err
	}

	return nil
}

func (h *OperationHandler) executeAfterRules(ctx context.Context, op *agentos.Operation, result interface{}) {
	if h.ruleExecutor == nil {
		return
	}

	data, ok := result.(map[string]interface{})
	if !ok {
		return
	}

	h.ruleExecutor.ExecuteAfter(ctx, op.Action, op.Entity, data)
}

func (h *OperationHandler) executeOperation(ctx context.Context, op *agentos.Operation, args map[string]interface{}, actorID string) (interface{}, error) {
	switch op.Action {
	case "list":
		return h.executeList(ctx, op, args)
	case "get":
		return h.executeGet(ctx, op, args)
	case "create":
		return h.executeCreate(ctx, op, args)
	case "update":
		return h.executeUpdate(ctx, op, args)
	case "delete":
		return h.executeDelete(ctx, op, args)
	case "transition":
		return h.executeTransition(ctx, op, args, actorID)
	default:
		return nil, fmt.Errorf("unsupported action: %s", op.Action)
	}
}

func (h *OperationHandler) executeList(ctx context.Context, op *agentos.Operation, args map[string]interface{}) (interface{}, error) {
	if h.db == nil {
		return []interface{}{}, nil
	}

	limit := 100
	offset := 0

	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}
	if o, ok := args["offset"].(float64); ok {
		offset = int(o)
	}

	entity := h.findEntity(op.Entity)
	if entity == nil {
		return nil, fmt.Errorf("entity not found: %s", op.Entity)
	}

	tableName := toSnakeCase(op.Entity)

	query := fmt.Sprintf("SELECT * FROM %s LIMIT ? OFFSET ?", tableName)
	rows, err := h.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return map[string]interface{}{
		"items":  results,
		"count":  len(results),
		"limit":  limit,
		"offset": offset,
	}, nil
}

func (h *OperationHandler) executeGet(ctx context.Context, op *agentos.Operation, args map[string]interface{}) (interface{}, error) {
	if h.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	id, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required")
	}

	entity := h.findEntity(op.Entity)
	if entity == nil {
		return nil, fmt.Errorf("entity not found: %s", op.Entity)
	}

	tableName := toSnakeCase(op.Entity)

	columnNames := make([]string, 0, len(entity.Fields))
	for _, field := range entity.Fields {
		columnNames = append(columnNames, toSnakeCase(field.Name))
	}

	query := fmt.Sprintf("SELECT * FROM %s WHERE id = ?", tableName)
	row := h.db.QueryRowContext(ctx, query, id)

	values := make([]interface{}, len(columnNames))
	valuePtrs := make([]interface{}, len(columnNames))
	for i := range columnNames {
		valuePtrs[i] = &values[i]
	}

	if err := row.Scan(valuePtrs...); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("not found: %s", id)
		}
		return nil, fmt.Errorf("query failed: %w", err)
	}

	result := make(map[string]interface{})
	for i, col := range columnNames {
		result[col] = values[i]
	}

	return result, nil
}

func (h *OperationHandler) executeCreate(ctx context.Context, op *agentos.Operation, args map[string]interface{}) (interface{}, error) {
	if h.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	entity := h.findEntity(op.Entity)
	if entity == nil {
		return nil, fmt.Errorf("entity not found: %s", op.Entity)
	}

	data := h.extractDataFields(entity, args)

	if data["id"] == nil || data["id"] == "" {
		data["id"] = generateID()
	}

	tableName := toSnakeCase(op.Entity)

	columns := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))

	for col, val := range data {
		columns = append(columns, col)
		placeholders = append(placeholders, "?")
		values = append(values, val)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	result, err := h.db.ExecContext(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("insert failed: %w", err)
	}

	id, _ := result.LastInsertId()
	data["id"] = id

	return data, nil
}

func (h *OperationHandler) executeUpdate(ctx context.Context, op *agentos.Operation, args map[string]interface{}) (interface{}, error) {
	if h.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	id, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required")
	}

	entity := h.findEntity(op.Entity)
	if entity == nil {
		return nil, fmt.Errorf("entity not found: %s", op.Entity)
	}

	data := h.extractDataFields(entity, args)
	delete(data, "id")

	if len(data) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	tableName := toSnakeCase(op.Entity)

	setClauses := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data)+1)
	for col, val := range data {
		setClauses = append(setClauses, col+" = ?")
		values = append(values, val)
	}
	values = append(values, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?",
		tableName,
		strings.Join(setClauses, ", "),
	)

	_, err := h.db.ExecContext(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}

	return h.executeGet(ctx, op, map[string]interface{}{"id": id})
}

func (h *OperationHandler) executeDelete(ctx context.Context, op *agentos.Operation, args map[string]interface{}) (interface{}, error) {
	if h.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	id, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required")
	}

	tableName := toSnakeCase(op.Entity)

	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", tableName)
	_, err := h.db.ExecContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("delete failed: %w", err)
	}

	return map[string]interface{}{
		"deleted": true,
		"id":      id,
	}, nil
}

func (h *OperationHandler) executeTransition(ctx context.Context, op *agentos.Operation, args map[string]interface{}, actorID string) (interface{}, error) {
	id, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required for workflow transition")
	}

	action := op.WorkflowAction
	if a, ok := args["action"].(string); ok {
		action = a
	}

	roles := []string{"anonymous"}
	if cred, err := h.actorStore.GetByID(actorID); err == nil {
		roles = cred.Roles
	}

	return map[string]interface{}{
		"entity_id": id,
		"action":    action,
		"actor_id":  actorID,
		"roles":     roles,
		"message":   fmt.Sprintf("workflow transition '%s' executed for %s", action, id),
	}, nil
}

func (h *OperationHandler) findEntity(entityName string) *manifest.Entity {
	for i := range h.manifest.DataModel.Entities {
		e := &h.manifest.DataModel.Entities[i]
		if e.Name == entityName {
			return e
		}
	}
	return nil
}

func (h *OperationHandler) buildOperationData(op *agentos.Operation, args map[string]interface{}) map[string]interface{} {
	data := make(map[string]interface{})
	for k, v := range args {
		data[k] = v
	}
	data["_entity"] = op.Entity
	data["_action"] = op.Action
	return data
}

func (h *OperationHandler) extractDataFields(entity *manifest.Entity, args map[string]interface{}) map[string]interface{} {
	data := make(map[string]interface{})
	for _, field := range entity.Fields {
		if val, ok := args[field.Name]; ok {
			data[field.Name] = val
		}
	}
	return data
}

func extractAttributes(op *agentos.Operation, args map[string]interface{}) map[string]interface{} {
	attrs := make(map[string]interface{})
	for k, v := range args {
		if s, ok := v.(string); ok {
			attrs[k] = s
		} else if n, ok := v.(float64); ok {
			attrs[k] = n
		} else if b, ok := v.(bool); ok {
			attrs[k] = b
		}
	}
	return attrs
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

func generateID() string {
	return fmt.Sprintf("id-%d", time.Now().UnixNano())
}
