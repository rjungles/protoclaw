# Etapa 8: Gestão de Estados Complexos e Comportamentos Avançados

## Objetivo

Evoluir o sistema para suportar **comportamentos complexos** que vão além de CRUD simples — especialmente cenários que exigem gestão de estados com múltiplas etapas, como o fluxo editorial do experience-platform.yaml.

## Estrutura de Arquivos a Criar

```
pkg/agentos/
    stateful/
        engine.go           # WorkflowEngine com persistência
        guards.go           # Guards para validação de transições
        side_effects.go     # Side effects (on_enter, on_exit)
        timeouts.go         # Timeouts de estado
        history.go          # Histórico de transições
        computed.go         # Computed fields
        lifecycle.go        # Lifecycle hooks (OnCreate, OnUpdate, etc.)
        queries.go          # Contextual queries
        stateful_test.go   # Testes unitários
        stateful_integration_test.go  # Testes de integração
```

## Componentes a Implementar

### 1. WorkflowEngine (`engine.go`)

Engine de workflow persistente que gerencia instâncias de workflow por entidade.

```go
type WorkflowInstance struct {
    ID          string
    EntityID    string
    EntityType  string
    CurrentState string
    PreviousState string
    UpdatedAt   time.Time
    UpdatedBy   string
    Metadata    map[string]interface{}
}

type WorkflowEngine struct {
    manifest     *manifest.Manifest
    db           *sql.DB
    fsms         map[string]*workflow.FSM
    store        WorkflowStateStore
    guards       *GuardEvaluator
    sideEffects  *SideEffectExecutor
    timeouts     *TimeoutManager
}

type WorkflowStateStore interface {
    GetState(entityType, entityID string) (*WorkflowInstance, error)
    SetState(instance *WorkflowInstance) error
    ListStates(entityType string) ([]*WorkflowInstance, error)
    GetHistory(entityType, entityID string) ([]TransitionRecord, error)
}

func (e *WorkflowEngine) Transition(ctx context.Context, entityType, entityID, action, actorID string, roles []string) (*WorkflowInstance, error)

func (e *WorkflowEngine) GetCurrentState(entityType, entityID string) (*WorkflowInstance, error)

func (e *WorkflowEngine) CanTransition(entityType, entityID, action string, roles []string) (bool, error)

func (e *WorkflowEngine) ListAvailableActions(entityType, entityID string, roles []string) []string
```

### 2. Guards (`guards.go`)

Validadores que devem ser satisfeitos para permitir uma transição.

```go
type Guard struct {
    Type      string                 // "field_not_empty", "field_equals", "custom"
    Field     string                 // Campo a ser validado
    Value     interface{}            // Valor esperado
    Condition string                 // "not_empty", "equals", "greater_than", etc.
}

type GuardEvaluator struct {
    manifest *manifest.Manifest
    db       *sql.DB
}

func (e *GuardEvaluator) Evaluate(guards []Guard, entityID string) (bool, error)

func (e *GuardEvaluator) EvaluateFieldNotEmpty(field, entityID string) (bool, error)

func (e *GuardEvaluator) EvaluateFieldEquals(field, entityID string, expected interface{}) (bool, error)
```

### 3. Side Effects (`side_effects.go`)

Ações executadas automaticamente ao entrar ou sair de um estado.

```go
type SideEffect struct {
    Type    string                 // "notify", "webhook", "update_field", "log"
    Target  string                 // Destinatário ou campo
    Message string                 // Mensagem ou template
    Config  map[string]interface{}
}

type SideEffectExecutor struct {
    manifest     *manifest.Manifest
    notifyBus    NotificationBus
    db           *sql.DB
}

func (e *SideEffectExecutor) ExecuteOnEnter(state, entityID, actorID string) error

func (e *SideEffectExecutor) ExecuteOnExit(state, entityID, actorID string) error

func (e *SideEffectExecutor) ExecuteNotify(target, message, entityID string) error

func (e *SideEffectExecutor) ExecuteUpdateField(field, value, entityID string) error
```

### 4. Timeouts (`timeouts.go`)

Transições automáticas após tempo em um estado.

```go
type TimeoutConfig struct {
    Duration    time.Duration
    TransitionTo string
    Action      string
}

type TimeoutManager struct {
    manifest *manifest.Manifest
    db       *sql.DB
    store    WorkflowStateStore
}

func (m *TimeoutManager) CheckTimeouts(ctx context.Context) ([]TransitionRecord, error)

func (m *TimeoutManager) GetTimeoutForState(entityType, state string) *TimeoutConfig

func (m *TimeoutManager) SetTimeout(entityType, entityID, state string, deadline time.Time) error
```

### 5. History (`history.go`)

Registro completo de transições.

```go
type TransitionRecord struct {
    ID           string
    EntityType   string
    EntityID     string
    FromState    string
    ToState      string
    Action       string
    ActorID      string
    Timestamp    time.Time
    Metadata     map[string]interface{}
}

type HistoryStore interface {
    Record(record *TransitionRecord) error
    GetHistory(entityType, entityID string) ([]TransitionRecord, error)
    GetLastTransition(entityType, entityID string) (*TransitionRecord, error)
}
```

### 6. Computed Fields (`computed.go`)

Campos calculados automaticamente.

```go
type ComputedField struct {
    Name       string
    Expression string  // "days_in_state", "progress_percentage", etc.
    DependsOn  []string
}

type ComputedFieldEvaluator struct {
    manifest *manifest.Manifest
    db       *sql.DB
}

func (e *ComputedFieldEvaluator) Evaluate(entityID string, fields []ComputedField) (map[string]interface{}, error)

func (e *ComputedFieldEvaluator) EvaluateDaysInState(entityID string) (int, error)

func (e *ComputedFieldEvaluator) EvaluateProgressPercentage(entityID string) (float64, error)
```

### 7. Lifecycle Hooks (`lifecycle.go`)

Hooks de ciclo de vida de entidade.

```go
type LifecycleHook struct {
    Event     string  // "OnCreate", "OnUpdate", "OnDelete", "OnStateChange"
    Entity    string
    Actions   []SideEffect
}

type LifecycleManager struct {
    manifest  *manifest.Manifest
    hooks     map[string][]LifecycleHook
    executor  *SideEffectExecutor
}

func (m *LifecycleManager) TriggerOnCreate(ctx context.Context, entityID, actorID string) error

func (m *LifecycleManager) TriggerOnUpdate(ctx context.Context, entityID, actorID string, changes map[string]interface{}) error

func (m *LifecycleManager) TriggerOnStateChange(ctx context.Context, entityID, fromState, toState, actorID string) error
```

### 8. Contextual Queries (`queries.go`)

Filtros automáticos baseados no ator.

```go
type ContextualQuery struct {
    ActorID    string
    Roles      []string
    EntityType string
}

func (q *ContextualQuery) ApplyFilters(sqlQuery string, args []interface{}) (string, []interface{})

func (q *ContextualQuery) GetAuthorFilter() (string, []interface{})

func (q *ContextualQuery) GetStateFilter() (string, []interface{})
```

## Extensão do Manifesto

O manifesto `workflow` será estendido para incluir:

```yaml
workflow:
  entity: "experience"
  initial_state: "DRAFT"
  states:
    - id: "UNDER_REVIEW"
      on_enter:
        - action: "notify"
          target: "editor"
          message: "Nova experiência para revisão"
      guards:
        - field: "raw_content"
          condition: "not_empty"
      timeout:
        duration: "7d"
        transition_to: "DRAFT"
        action: "auto_return"
    - id: "DRAFTING"
      on_enter:
        - action: "update_field"
          field: "editor_id"
          value: "{{current_actor}}"
```

## Fluxo de Transição Completo

```
1. Receive transition request
   └─ entityType, entityID, action, actorID, roles

2. Load current state
   └─ store.GetState(entityType, entityID)

3. Find FSM and transition
   └─ fsm.FindTransition(currentState, action)

4. Check role permissions
   └─ transition.AllowedRoles contains any of userRoles

5. Evaluate guards
   └─ guardEvaluator.Evaluate(guards, entityID)
   └─ Se falhar: return error with guard failure reason

6. Execute on_exit side effects
   └─ sideEffectExecutor.ExecuteOnExit(fromState, entityID, actorID)

7. Execute transition
   └─ Update state in store
   └─ Record in history

8. Execute on_enter side effects
   └─ sideEffectExecutor.ExecuteOnEnter(toState, entityID, actorID)

9. Check timeout
   └─ timeoutManager.SetTimeout(entityType, entityID, toState, deadline)

10. Return new state
```

## Testes

### Unit Tests (`stateful_test.go`)

1. `TestWorkflowEngine_Transition` - Transição básica
2. `TestWorkflowEngine_TransitionWithGuards` - Transição com guards
3. `TestWorkflowEngine_TransitionDeniedByGuard` - Guard falhando
4. `TestWorkflowEngine_SideEffects` - Side effects executados
5. `TestWorkflowEngine_History` - Histórico registrado
6. `TestWorkflowEngine_Timeout` - Timeout configurado
7. `TestComputedFields_DaysInState` - Campo calculado
8. `TestLifecycleHooks_OnCreate` - Hook de criação
9. `TestLifecycleHooks_OnStateChange` - Hook de mudança de estado
10. `TestContextualQueries_AuthorFilter` - Filtro por autor

### Integration Tests (`stateful_integration_test.go`)

1. `TestWorkflowEngine_ExperiencePlatform` - Fluxo editorial completo
2. `TestWorkflowEngine_ClarificationFlow` - Fluxo de esclarecimento
3. `TestWorkflowEngine_TimeoutAutoReturn` - Timeout com retorno automático

## Verificação

```bash
# Rodar testes unitários
go test ./pkg/agentos/stateful/... -v -count=1

# Rodar testes de integração
go test ./pkg/agentos/stateful/... -v -count=1 -run "Integration"
```
