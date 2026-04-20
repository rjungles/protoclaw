package agentos

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/api"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/infra/db"
	"github.com/sipeed/picoclaw/pkg/manifest"
)

// SystemInstance representa uma instância completa do sistema AgentOS
type SystemInstance struct {
	// Componentes principais
	Manifest *manifest.Manifest
	DB       *sql.DB
	Catalog  *OperationCatalog

	// Subsistemas
	ActorStore     ActorStore
	PolicyEngine   *policy.Engine
	RuleExecutor   *api.RuleExecutor
	APIGenerator   *api.Generator
	Migrator       *db.Migrator
	WorkflowEngine *WorkflowEngine
	HTTPMux        *http.ServeMux

	// Novos componentes para AgentOS
	ManifestStore    ManifestStore
	EvolutionManager *EvolutionManager
	CreatedAt        time.Time
	State            *InstanceState
	Metrics          *SystemMetrics
	AuditLog         *AuditLog

	// Controle de ciclo de vida
	shutdownFuncs []func(context.Context) error
	mu            sync.RWMutex
}

// InstanceState representa o estado da instância
type InstanceState struct {
	Status         string
	StartTime      time.Time
	LastUpdate     time.Time
	ErrorCount     int
	OperationCount int64
}

// SystemMetrics contém métricas do sistema
type SystemMetrics struct {
	TotalOperations     int64
	FailedOperations    int64
	AverageResponseTime time.Duration
	ActiveWorkflows     int
}

// AuditLog representa o log de auditoria
type AuditLog struct {
	Entries []AuditEntry
}

// AuditEntry representa uma entrada de auditoria
type AuditEntry struct {
	Timestamp time.Time
	ActorID   string
	Operation string
	Entity    string
	Action    string
	Success   bool
	Error     string
	Duration  time.Duration
	Metadata  map[string]interface{}
}

// NewSystemInstance cria uma nova instância do sistema
func NewSystemInstance(manifest *manifest.Manifest) *SystemInstance {
	return &SystemInstance{
		Manifest:      manifest,
		CreatedAt:     time.Now(),
		State:         &InstanceState{Status: "initializing", StartTime: time.Now()},
		Metrics:       &SystemMetrics{},
		AuditLog:      &AuditLog{Entries: make([]AuditEntry, 0)},
		shutdownFuncs: make([]func(context.Context) error, 0),
	}
}

// ExecuteOperation executa uma operação de forma unificada (API e MCP)
func (si *SystemInstance) ExecuteOperation(ctx context.Context, opName string, params map[string]interface{}, actorID string) (map[string]interface{}, error) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	// Registrar início da operação
	startTime := time.Now()
	si.Metrics.OperationCount++

	// Obter operação do catálogo
	op, err := si.Catalog.Get(opName)
	if err != nil {
		si.recordAuditEntry(actorID, opName, "", "", false, err.Error(), time.Since(startTime), nil)
		return nil, fmt.Errorf("operation not found: %w", err)
	}

	// Verificar permissões
	if si.PolicyEngine != nil {
		if err := si.PolicyEngine.CheckPermission(actorID, op); err != nil {
			si.recordAuditEntry(actorID, opName, op.Entity, op.Action, false, fmt.Sprintf("permission denied: %v", err), time.Since(startTime), nil)
			return nil, fmt.Errorf("permission denied: %w", err)
		}
	}

	// Executar validações de negócio
	if si.RuleExecutor != nil {
		if err := si.RuleExecutor.ValidateOperation(ctx, op, params); err != nil {
			si.recordAuditEntry(actorID, opName, op.Entity, op.Action, false, fmt.Sprintf("validation failed: %v", err), time.Since(startTime), nil)
			return nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	// Executar operação
	var result map[string]interface{}
	var execErr error

	switch op.Action {
	case "list":
		result, execErr = si.executeListOperation(ctx, op, params, actorID)
	case "get":
		result, execErr = si.executeGetOperation(ctx, op, params, actorID)
	case "create":
		result, execErr = si.executeCreateOperation(ctx, op, params, actorID)
	case "update":
		result, execErr = si.executeUpdateOperation(ctx, op, params, actorID)
	case "delete":
		result, execErr = si.executeDeleteOperation(ctx, op, params, actorID)
	case "transition":
		result, execErr = si.executeTransitionOperation(ctx, op, params, actorID)
	default:
		result, execErr = si.executeCustomOperation(ctx, op, params, actorID)
	}

	// Registrar resultado
	duration := time.Since(startTime)
	if execErr != nil {
		si.Metrics.FailedOperations++
		si.recordAuditEntry(actorID, opName, op.Entity, op.Action, false, execErr.Error(), duration, nil)
		return nil, execErr
	}

	si.recordAuditEntry(actorID, opName, op.Entity, op.Action, true, "", duration, result)
	return result, nil
}

// Métodos auxiliares de execução
func (si *SystemInstance) executeListOperation(ctx context.Context, op *Operation, params map[string]interface{}, actorID string) (map[string]interface{}, error) {
	// Implementar lógica de listagem
	return map[string]interface{}{
		"items": []interface{}{},
		"total": 0,
	}, nil
}

func (si *SystemInstance) executeGetOperation(ctx context.Context, op *Operation, params map[string]interface{}, actorID string) (map[string]interface{}, error) {
	// Implementar lógica de busca
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id parameter required")
	}

	return map[string]interface{}{
		"id":   id,
		"data": map[string]interface{}{},
	}, nil
}

func (si *SystemInstance) executeCreateOperation(ctx context.Context, op *Operation, params map[string]interface{}, actorID string) (map[string]interface{}, error) {
	// Implementar lógica de criação
	return params, nil
}

func (si *SystemInstance) executeUpdateOperation(ctx context.Context, op *Operation, params map[string]interface{}, actorID string) (map[string]interface{}, error) {
	// Implementar lógica de atualização
	return params, nil
}

func (si *SystemInstance) executeDeleteOperation(ctx context.Context, op *Operation, params map[string]interface{}, actorID string) (map[string]interface{}, error) {
	// Implementar lógica de exclusão
	return map[string]interface{}{"success": true}, nil
}

func (si *SystemInstance) executeTransitionOperation(ctx context.Context, op *Operation, params map[string]interface{}, actorID string) (map[string]interface{}, error) {
	// Implementar lógica de transição de workflow
	if si.WorkflowEngine == nil {
		return nil, fmt.Errorf("workflow engine not configured")
	}

	entityID, _ := params["id"].(string)
	action, _ := params["action"].(string)

	result, err := si.WorkflowEngine.Transition(ctx, op.Entity, entityID, action, actorID, []string{"user"})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":    true,
		"from_state": result.PreviousState,
		"to_state":   result.CurrentState,
	}, nil
}

func (si *SystemInstance) executeCustomOperation(ctx context.Context, op *Operation, params map[string]interface{}, actorID string) (map[string]interface{}, error) {
	// Implementar lógica de operação customizada
	return map[string]interface{}{"result": "custom operation executed"}, nil
}

// recordAuditEntry registra uma entrada de auditoria
func (si *SystemInstance) recordAuditEntry(actorID, operation, entity, action string, success bool, errorMsg string, duration time.Duration, result map[string]interface{}) {
	entry := AuditEntry{
		Timestamp: time.Now(),
		ActorID:   actorID,
		Operation: operation,
		Entity:    entity,
		Action:    action,
		Success:   success,
		Error:     errorMsg,
		Duration:  duration,
		Metadata:  result,
	}

	si.mu.Lock()
	si.AuditLog.Entries = append(si.AuditLog.Entries, entry)
	si.mu.Unlock()
}

// AddShutdownFunc adiciona uma função de shutdown
func (si *SystemInstance) AddShutdownFunc(fn func(context.Context) error) {
	si.mu.Lock()
	defer si.mu.Unlock()
	si.shutdownFuncs = append(si.shutdownFuncs, fn)
}

// Shutdown executa o shutdown graceful do sistema
func (si *SystemInstance) Shutdown(ctx context.Context) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	si.State.Status = "shutting_down"

	var errors []error
	for _, fn := range si.shutdownFuncs {
		if err := fn(ctx); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	si.State.Status = "shutdown"
	return nil
}

// GetMetrics retorna as métricas atuais do sistema
func (si *SystemInstance) GetMetrics() *SystemMetrics {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return si.Metrics
}

// GetAuditLog retorna o log de auditoria
func (si *SystemInstance) GetAuditLog() *AuditLog {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return si.AuditLog
}

// GetState retorna o estado atual da instância
func (si *SystemInstance) GetState() *InstanceState {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return si.State
}

// UpdateState atualiza o estado da instância
func (si *SystemInstance) UpdateState(status string) {
	si.mu.Lock()
	defer si.mu.Unlock()
	si.State.Status = status
	si.State.LastUpdate = time.Now()
}
