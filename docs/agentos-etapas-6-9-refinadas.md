# Refinamento das Etapas 6-9: Sistema Operacional de Agentes (AgentOS)

## Visão Geral

O objetivo é transformar o PicoClaw em um **Sistema Operacional de Agentes (AgentOS)** que, a partir de um único arquivo de manifesto, seja capaz de gerar automaticamente toda a infraestrutura necessária para um sistema completo com:

- **Configuração automática** de banco de dados e persistência
- **Controle de acesso** com permissões granulares
- **Regras de negócio** executáveis e validáveis
- **APIs REST** e **MCP Server** com paridade total
- **Workflows complexos** com gestão de estados
- **Evolução sem perda** de dados

## Etapa 6: Sistema de Orquestração Central (AgentOS Core)

### Objetivo
Criar o núcleo do sistema que orquestra todos os subsistemas a partir do manifesto, garantindo inicialização consistente e gerenciamento de ciclo de vida.

### Componentes Principais

#### 1. AgentOS Engine (`pkg/agentos/engine.go`)
```go
type AgentOSEngine struct {
    config *EngineConfig
    instance *SystemInstance
    manifestStore *ManifestStore
    evolutionManager *EvolutionManager
}

type EngineConfig struct {
    ManifestPath string
    DataDir string
    DBDriver string
    DBConnection string
    AutoEvolve bool
    EnableMCP bool
    EnableAPI bool
    EnableWorkflows bool
}

func (e *AgentOSEngine) Bootstrap(ctx context.Context) (*SystemInstance, error) {
    // 1. Carregar e validar manifesto atual
    manifest, err := e.loadCurrentManifest()
    if err != nil {
        return nil, fmt.Errorf("failed to load manifest: %w", err)
    }

    // 2. Detectar evolução se houver manifesto anterior
    if e.hasPreviousManifest() {
        if e.config.AutoEvolve {
            if err := e.evolveSystem(ctx, manifest); err != nil {
                return nil, fmt.Errorf("evolution failed: %w", err)
            }
        }
    }

    // 3. Criar nova instância do sistema
    instance := NewSystemInstance(manifest)
    
    // 4. Inicializar subsistemas
    if err := e.initializeSubsystems(ctx, instance); err != nil {
        return nil, fmt.Errorf("subsystem initialization failed: %w", err)
    }

    // 5. Registrar shutdown handlers
    e.setupShutdownHandlers(instance)
    
    return instance, nil
}
```

#### 2. System Instance (`pkg/agentos/instance.go`)
```go
type SystemInstance struct {
    // Core components
    Manifest *manifest.Manifest
    DB *sql.DB
    Engine *AgentOSEngine
    
    // Subsystems
    ActorManager *ActorManager
    PolicyEngine *PolicyEngine
    WorkflowEngine *WorkflowEngine
    APIManager *APIManager
    MCPManager *MCPManager
    RuleEngine *RuleEngine
    EvolutionManager *EvolutionManager
    
    // State management
    State *InstanceState
    Metrics *SystemMetrics
    AuditLog *AuditLog
    
    // Lifecycle
    shutdownFuncs []func(context.Context) error
    mu sync.RWMutex
}

func (si *SystemInstance) GetOperation(name string) (*Operation, error) {
    return si.APIManager.GetOperation(name)
}

func (si *SystemInstance) ExecuteOperation(ctx context.Context, opName string, params map[string]interface{}, actorID string) (map[string]interface{}, error) {
    // Unified operation execution for both API and MCP
    op, err := si.GetOperation(opName)
    if err != nil {
        return nil, err
    }
    
    // Check permissions
    if err := si.PolicyEngine.CheckPermission(actorID, op); err != nil {
        return nil, fmt.Errorf("permission denied: %w", err)
    }
    
    // Execute operation
    return si.APIManager.ExecuteOperation(ctx, op, params, actorID)
}
```

#### 3. Manifest Store (`pkg/agentos/manifest_store.go`)
```go
type ManifestStore interface {
    SaveVersion(version *ManifestVersion) error
    GetVersion(version string) (*ManifestVersion, error)
    GetLatestVersion() (*ManifestVersion, error)
    ListVersions() ([]*ManifestVersion, error)
    GetDiff(fromVersion, toVersion string) (*ManifestDiff, error)
}

type ManifestVersion struct {
    Version string
    Manifest *manifest.Manifest
    CreatedAt time.Time
    CreatedBy string
    DiffFromPrevious *ManifestDiff
    Checksum string
}
```

### Fluxo de Inicialização
```
1. Load Manifest → Parse e validação completa
2. Check Evolution → Detectar mudanças do manifesto
3. Plan Migration → Gerar plano de migração se necessário
4. Execute Migration → Aplicar mudanças com segurança
5. Initialize DB → Conectar e configurar banco
6. Setup Actors → Provisionar atores e permissões
7. Create Engines → Inicializar todos os subsistemas
8. Start Services → APIs, MCP, Workflows
9. System Ready → Disponível para uso
```

## Etapa 7: MCP Server Nativo com Paridade Total

### Objetivo
Criar um servidor MCP que exponha exatamente as mesmas operações disponíveis via API REST, garantindo paridade total de funcionalidades e segurança.

### Componentes Principais

#### 1. MCPServer (`pkg/mcp/server.go`)
```go
type MCPServer struct {
    instance *agentos.SystemInstance
    catalog *OperationCatalog
    config *MCPConfig
    
    tools []Tool
    resources []Resource
    prompts []Prompt
}

func (s *MCPServer) HandleCallTool(ctx context.Context, params CallToolParams) (*CallToolResult, error) {
    // 1. Resolver operação pelo nome da tool
    opName := s.mapToolToOperation(params.Name)
    op, err := s.instance.GetOperation(opName)
    if err != nil {
        return nil, fmt.Errorf("operation not found: %w", err)
    }
    
    // 2. Extrair ator do contexto
    actorID := s.extractActorID(ctx)
    
    // 3. Executar operação via SystemInstance (mesma lógica que API)
    result, err := s.instance.ExecuteOperation(ctx, opName, params.Arguments, actorID)
    if err != nil {
        return nil, err
    }
    
    // 4. Converter resultado para formato MCP
    return s.convertToMCPResult(result), nil
}

func (s *MCPServer) mapToolToOperation(toolName string) string {
    // Mapeamento consistente: "entity.action" → "entity.action"
    return toolName
}
```

#### 2. Operation Bridge (`pkg/mcp/bridge.go`)
```go
type OperationBridge struct {
    instance *agentos.SystemInstance
}

func (b *OperationBridge) ConvertAPIToMCPOperation(op *api.Operation) *Operation {
    return &Operation{
        Name: op.Name,
        Description: op.Description,
        InputSchema: b.convertJSONSchemaToMCP(op.InputSchema),
        OutputSchema: b.convertJSONSchemaToMCP(op.OutputSchema),
    }
}

func (b *OperationBridge) ExecuteViaMCP(ctx context.Context, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
    // Usa exatamente a mesma lógica de execução que a API
    return b.instance.ExecuteOperation(ctx, toolName, args, b.getActorID(ctx))
}
```

#### 3. Resource Manager (`pkg/mcp/resources.go`)
```go
type ResourceManager struct {
    instance *agentos.SystemInstance
}

func (rm *ResourceManager) ListResources() []Resource {
    resources := make([]Resource, 0)
    
    for _, entity := range rm.instance.Manifest.DataModel.Entities {
        resource := Resource{
            URI:      fmt.Sprintf("resource://%s/%s", rm.instance.Manifest.Metadata.Name, entity.Name),
            Name:     entity.Name,
            MimeType: "application/json",
        }
        resources = append(resources, resource)
    }
    
    return resources
}

func (rm *ResourceManager) ReadResource(uri string) (*ReadResourceResult, error) {
    // Extrair entidade e ID do URI
    parts := strings.Split(uri, "/")
    if len(parts) < 4 {
        return nil, fmt.Errorf("invalid resource URI")
    }
    
    entityName := parts[2]
    entityID := parts[3]
    
    // Usar mesma lógica de leitura que a API
    opName := fmt.Sprintf("%s.get", entityName)
    result, err := rm.instance.ExecuteOperation(context.Background(), opName, map[string]interface{}{"id": entityID}, "system")
    if err != nil {
        return nil, err
    }
    
    return &ReadResourceResult{
        Contents: []TextContent{{
            Type: "text",
            Text: string(rm.marshalToJSON(result)),
        }},
    }, nil
}
```

### Paridade de Funcionalidades

| Aspecto | API REST | MCP Server |
|---------|----------|------------|
| Autenticação | Headers (X-Actor-ID, Bearer) | Headers (X-Actor-ID, Authorization) |
| Permissões | PolicyEngine.CheckPermission | Mesma verificação |
| Regras de Negócio | RuleExecutor.Execute | Mesma execução |
| Workflows | FSM.Transition | Mesma transição |
| Validação | Mesmas validações | Idênticas |
| Auditoria | Mesmos logs | Idênticos |
| Performance | Otimizado | Otimizado |

### Nomenclatura de Tools
```
{entity}.{action}                    # CRUD operations
{entity}.transition.{action}         # Workflow transitions
{entity}.custom.{handler}              # Custom operations
system.{operation}                     # System operations
```

## Etapa 8: Gestão de Estados Complexos e Workflows Persistentes

### Objetivo
Suportar workflows complexos com múltiplas etapas, validações, efeitos colaterais, histórico completo e gestão avançada de estados.

### Componentes Principais

#### 1. Stateful Workflow Engine (`pkg/agentos/stateful/engine.go`)
```go
type StatefulWorkflowEngine struct {
    instance *agentos.SystemInstance
    store *WorkflowStore
    guardEvaluator *GuardEvaluator
    effectExecutor *SideEffectExecutor
    timeoutManager *TimeoutManager
    historyManager *HistoryManager
}

type WorkflowInstance struct {
    ID string
    EntityType string
    EntityID string
    CurrentState string
    PreviousState string
    Metadata map[string]interface{}
    CreatedAt time.Time
    UpdatedAt time.Time
    UpdatedBy string
    ComputedFields map[string]interface{}
}

func (e *StatefulWorkflowEngine) Transition(ctx context.Context, req *TransitionRequest) (*TransitionResult, error) {
    // 1. Carregar instância atual
    instance, err := e.store.GetInstance(req.EntityType, req.EntityID)
    if err != nil {
        return nil, err
    }
    
    // 2. Verificar permissões
    if err := e.checkPermissions(req, instance); err != nil {
        return nil, err
    }
    
    // 3. Avaliar guardas
    if err := e.evaluateGuards(req, instance); err != nil {
        return nil, err
    }
    
    // 4. Executar efeitos de saída
    if err := e.executeOnExit(ctx, instance, req); err != nil {
        return nil, err
    }
    
    // 5. Realizar transição
    newInstance, err := e.performTransition(ctx, instance, req)
    if err != nil {
        return nil, err
    }
    
    // 6. Executar efeitos de entrada
    if err := e.executeOnEnter(ctx, newInstance, req); err != nil {
        // Rollback se falhar
        e.rollbackTransition(ctx, instance)
        return nil, err
    }
    
    // 7. Registrar histórico
    if err := e.recordHistory(ctx, instance, newInstance, req); err != nil {
        return nil, err
    }
    
    // 8. Configurar timeouts
    if err := e.setupTimeouts(ctx, newInstance); err != nil {
        return nil, err
    }
    
    return &TransitionResult{
        Success: true,
        FromState: instance.CurrentState,
        ToState: newInstance.CurrentState,
        Instance: newInstance,
    }, nil
}
```

#### 2. Workflow Store (`pkg/agentos/stateful/store.go`)
```go
type WorkflowStore interface {
    // Instance management
    GetInstance(entityType, entityID string) (*WorkflowInstance, error)
    CreateInstance(instance *WorkflowInstance) error
    UpdateInstance(instance *WorkflowInstance) error
    ListInstances(entityType string, filters map[string]interface{}) ([]*WorkflowInstance, error)
    
    // History management
    RecordTransition(record *TransitionRecord) error
    GetHistory(entityType, entityID string) ([]*TransitionRecord, error)
    GetLastTransition(entityType, entityID string) (*TransitionRecord, error)
    
    // Timeout management
    SetTimeout(timeout *TimeoutRecord) error
    GetTimeoutsToProcess() ([]*TimeoutRecord, error)
    MarkTimeoutProcessed(id string) error
}
```

#### 3. Guard System (`pkg/agentos/stateful/guards.go`)
```go
type GuardEvaluator struct {
    instance *agentos.SystemInstance
}

type Guard struct {
    Type string // "field_not_empty", "field_equals", "custom", "expression"
    Field string
    Value interface{}
    Condition string // "equals", "not_empty", "greater_than", "custom"
    Expression string // Para condições complexas
}

func (e *GuardEvaluator) EvaluateGuards(guards []Guard, instance *WorkflowInstance, req *TransitionRequest) error {
    for _, guard := range guards {
        switch guard.Type {
        case "field_not_empty":
            if err := e.checkFieldNotEmpty(guard.Field, instance); err != nil {
                return err
            }
        case "field_equals":
            if err := e.checkFieldEquals(guard.Field, guard.Value, instance); err != nil {
                return err
            }
        case "custom":
            if err := e.evaluateCustomGuard(guard, instance, req); err != nil {
                return err
            }
        case "expression":
            if err := e.evaluateExpression(guard.Expression, instance, req); err != nil {
                return err
            }
        }
    }
    return nil
}
```

#### 4. Side Effects Manager (`pkg/agentos/stateful/effects.go`)
```go
type SideEffectExecutor struct {
    instance *agentos.SystemInstance
}

type SideEffect struct {
    Type string // "notify", "webhook", "update_field", "log", "execute_script"
    Target string
    Message string
    Config map[string]interface{}
    Script string // Para efeitos customizados
}

func (e *SideEffectExecutor) ExecuteOnEnter(ctx context.Context, effects []SideEffect, instance *WorkflowInstance, req *TransitionRequest) error {
    for _, effect := range effects {
        switch effect.Type {
        case "notify":
            if err := e.executeNotification(effect, instance, req); err != nil {
                return err
            }
        case "update_field":
            if err := e.executeFieldUpdate(ctx, effect, instance); err != nil {
                return err
            }
        case "webhook":
            if err := e.executeWebhook(ctx, effect, instance); err != nil {
                return err
            }
        case "execute_script":
            if err := e.executeScript(ctx, effect.Script, instance, req); err != nil {
                return err
            }
        }
    }
    return nil
}
```

### Extensão do Manifesto para Workflows Complexos
```yaml
workflow:
  entity: "experience"
  initial_state: "DRAFT"
  states:
    - id: "DRAFT"
      description: "Rascunho inicial"
      transitions:
        - to: "UNDER_REVIEW"
          action: "submit_review"
          allowed_roles: ["author", "editor"]
      guards:
        - field: "raw_content"
          condition: "not_empty"
          message: "Conteúdo é obrigatório"
      on_exit:
        - type: "log"
          message: "Experiência enviada para revisão"
    
    - id: "UNDER_REVIEW"
      description: "Em revisão editorial"
      timeout:
        duration: "7d"
        transition_to: "DRAFT"
        action: "auto_return"
      guards:
        - field: "raw_content"
          condition: "min_length"
          value: 100
          message: "Conteúdo muito curto"
      on_enter:
        - type: "notify"
          target: "editor"
          message: "Nova experiência para revisão: {{entity.title}}"
        - type: "update_field"
          field: "reviewer_id"
          value: "{{current_actor}}"
      on_exit:
        - type: "log"
          message: "Revisão concluída"
    
    - id: "PUBLISHED"
      description: "Publicado"
      on_enter:
        - type: "webhook"
          url: "https://api.example.com/publish"
          method: "POST"
          payload:
            event: "experience_published"
            entity_id: "{{entity.id}}"
        - type: "notify"
          target: "author"
          message: "Sua experiência foi publicada!"
```

## Etapa 9: Sistema de Evolução e Migração Inteligente

### Objetivo
Permitir que o manifesto evolua ao longo do tempo com detecção automática de mudanças, migração segura de dados e proteção contra perdas.

### Componentes Principais

#### 1. Manifest Diff Engine (`pkg/agentos/evolution/diff.go`)
```go
type ManifestDiffEngine struct {
    instance *agentos.SystemInstance
}

type ManifestDiff struct {
    FromVersion string
    ToVersion string
    Changes []Change
    BreakingChanges []Change
    SafeChanges []Change
    ReviewChanges []Change
}

type Change struct {
    Type ChangeType // "add", "modify", "remove"
    Severity ChangeSeverity // "safe", "review", "breaking"
    Path string // "data_model.entities.User.fields.email"
    OldValue interface{}
    NewValue interface{}
    Description string
    MigrationStrategy string
}

func (e *ManifestDiffEngine) Compare(oldManifest, newManifest *manifest.Manifest) (*ManifestDiff, error) {
    diff := &ManifestDiff{
        FromVersion: oldManifest.Metadata.Version,
        ToVersion: newManifest.Metadata.Version,
    }
    
    // Comparar estruturas principais
    e.compareMetadata(oldManifest, newManifest, diff)
    e.compareActors(oldManifest, newManifest, diff)
    e.compareDataModel(oldManifest, newManifest, diff)
    e.compareBusinessRules(oldManifest, newManifest, diff)
    e.compareWorkflows(oldManifest, newManifest, diff)
    e.compareSecurity(oldManifest, newManifest, diff)
    
    // Classificar mudanças
    diff.classifyChanges()
    
    return diff, nil
}

func (d *ManifestDiff) classifyChanges() {
    for _, change := range d.Changes {
        switch {
        case strings.HasPrefix(change.Path, "data_model.entities.") && strings.Contains(change.Path, ".fields."):
            if change.Type == "add" {
                change.Severity = "safe"
                change.MigrationStrategy = "ADD_COLUMN"
            } else if change.Type == "remove" {
                change.Severity = "breaking"
                change.MigrationStrategy = "DEPRECATE_FIELD"
            } else if change.Type == "modify" {
                change.Severity = "review"
                change.MigrationStrategy = "ALTER_COLUMN"
            }
        case strings.HasPrefix(change.Path, "data_model.entities.") && !strings.Contains(change.Path, ".fields."):
            if change.Type == "add" {
                change.Severity = "safe"
                change.MigrationStrategy = "CREATE_TABLE"
            } else if change.Type == "remove" {
                change.Severity = "breaking"
                change.MigrationStrategy = "ARCHIVE_TABLE"
            }
        case strings.HasPrefix(change.Path, "actors."):
            if change.Type == "add" {
                change.Severity = "safe"
                change.MigrationStrategy = "ADD_ACTOR"
            } else if change.Type == "remove" {
                change.Severity = "breaking"
                change.MigrationStrategy = "DEACTIVATE_ACTOR"
            }
        }
    }
}
```

#### 2. Migration Planner (`pkg/agentos/evolution/planner.go`)
```go
type MigrationPlanner struct {
    instance *agentos.SystemInstance
}

type MigrationPlan struct {
    Version string
    Steps []MigrationStep
    SafeSteps []MigrationStep
    ReviewSteps []MigrationStep
    BreakingSteps []MigrationStep
    EstimatedTime time.Duration
    BackupRequired bool
    CanRollback bool
}

type MigrationStep struct {
    ID string
    Type MigrationType
    Description string
    SQL string
    RollbackSQL string
    DataMigration func(ctx context.Context, db *sql.DB) error
    Validation func(ctx context.Context, db *sql.DB) error
    Priority int
}

func (p *MigrationPlanner) CreatePlan(diff *ManifestDiff) (*MigrationPlan, error) {
    plan := &MigrationPlan{
        Version: diff.ToVersion,
        Steps: make([]MigrationStep, 0),
    }
    
    // Ordenar mudanças por prioridade
    orderedChanges := p.orderChangesByPriority(diff.Changes)
    
    for _, change := range orderedChanges {
        step := p.createStepForChange(change)
        plan.Steps = append(plan.Steps, step)
        
        switch change.Severity {
        case "safe":
            plan.SafeSteps = append(plan.SafeSteps, step)
        case "review":
            plan.ReviewSteps = append(plan.ReviewSteps, step)
        case "breaking":
            plan.BreakingSteps = append(plan.BreakingSteps, step)
        }
    }
    
    // Calcular estimativas
    plan.EstimatedTime = p.estimateExecutionTime(plan.Steps)
    plan.BackupRequired = len(plan.BreakingSteps) > 0
    plan.CanRollback = p.canRollback(plan.Steps)
    
    return plan, nil
}

func (p *MigrationPlanner) createStepForChange(change Change) MigrationStep {
    switch change.MigrationStrategy {
    case "ADD_COLUMN":
        return MigrationStep{
            ID: fmt.Sprintf("add_column_%s", uuid.New()),
            Type: "ADD_COLUMN",
            Description: fmt.Sprintf("Add column %s to table %s", change.Path, extractTableName(change.Path)),
            SQL: p.generateAddColumnSQL(change),
            RollbackSQL: p.generateDropColumnSQL(change),
            Priority: 1,
        }
    case "DEPRECATE_FIELD":
        return MigrationStep{
            ID: fmt.Sprintf("deprecate_field_%s", uuid.New()),
            Type: "DEPRECATE_FIELD",
            Description: fmt.Sprintf("Deprecate field %s (rename to _deprecated_%s)", change.Path, extractFieldName(change.Path)),
            SQL: p.generateDeprecateFieldSQL(change),
            RollbackSQL: p.generateRestoreFieldSQL(change),
            DataMigration: p.createDataMigrationForField(change),
            Priority: 10, // Alta prioridade para proteção
        }
    case "CREATE_TABLE":
        return MigrationStep{
            ID: fmt.Sprintf("create_table_%s", uuid.New()),
            Type: "CREATE_TABLE",
            Description: fmt.Sprintf("Create table for new entity %s", extractEntityName(change.Path)),
            SQL: p.generateCreateTableSQL(change),
            RollbackSQL: p.generateDropTableSQL(change),
            Priority: 1,
        }
    }
}
```

#### 3. Evolution Executor (`pkg/agentos/evolution/executor.go`)
```go
type EvolutionExecutor struct {
    instance *agentos.SystemInstance
    planner *MigrationPlanner
    backupManager *BackupManager
}

type EvolutionResult struct {
    Success bool
    AppliedSteps []MigrationStep
    FailedStep *MigrationStep
    Error error
    Warnings []string
    Duration time.Duration
    BackupPath string
}

func (e *EvolutionExecutor) Evolve(ctx context.Context, newManifest *manifest.Manifest) (*EvolutionResult, error) {
    result := &EvolutionResult{
        AppliedSteps: make([]MigrationStep, 0),
        Warnings: make([]string, 0),
    }
    
    startTime := time.Now()
    
    // 1. Comparar manifestos
    currentManifest := e.instance.Manifest
    diff, err := e.diffEngine.Compare(currentManifest, newManifest)
    if err != nil {
        result.Error = err
        return result, err
    }
    
    if len(diff.Changes) == 0 {
        result.Warnings = append(result.Warnings, "No changes detected")
        return result, nil
    }
    
    // 2. Criar plano de migração
    plan, err := e.planner.CreatePlan(diff)
    if err != nil {
        result.Error = err
        return result, err
    }
    
    // 3. Fazer backup se necessário
    if plan.BackupRequired {
        backupPath, err := e.backupManager.CreateBackup(ctx)
        if err != nil {
            result.Error = fmt.Errorf("backup failed: %w", err)
            return result, err
        }
        result.BackupPath = backupPath
    }
    
    // 4. Executar passos seguros automaticamente
    if err := e.executeSafeSteps(ctx, plan.SafeSteps, result); err != nil {
        result.Error = err
        return result, err
    }
    
    // 5. Solicitar confirmação para passos de revisão
    if len(plan.ReviewSteps) > 0 {
        if err := e.executeReviewSteps(ctx, plan.ReviewSteps, result); err != nil {
            result.Error = err
            return result, err
        }
    }
    
    // 6. Executar passos quebradores com proteção
    if len(plan.BreakingSteps) > 0 {
        if err := e.executeBreakingSteps(ctx, plan.BreakingSteps, result); err != nil {
            result.Error = err
            return result, err
        }
    }
    
    // 7. Atualizar manifesto no sistema
    if err := e.updateSystemManifest(newManifest); err != nil {
        result.Error = err
        return result, err
    }
    
    result.Success = true
    result.Duration = time.Since(startTime)
    
    return result, nil
}

func (e *EvolutionExecutor) executeSafeSteps(ctx context.Context, steps []MigrationStep, result *EvolutionResult) error {
    for _, step := range steps {
        if err := e.executeStep(ctx, step); err != nil {
            result.FailedStep = &step
            return err
        }
        result.AppliedSteps = append(result.AppliedSteps, step)
    }
    return nil
}

func (e *EvolutionExecutor) executeStep(ctx context.Context, step MigrationStep) error {
    // Executar SQL
    if step.SQL != "" {
        if _, err := e.instance.DB.ExecContext(ctx, step.SQL); err != nil {
            return fmt.Errorf("SQL execution failed for step %s: %w", step.ID, err)
        }
    }
    
    // Executar migração de dados se houver
    if step.DataMigration != nil {
        if err := step.DataMigration(ctx, e.instance.DB); err != nil {
            return fmt.Errorf("data migration failed for step %s: %w", step.ID, err)
        }
    }
    
    // Validar resultado
    if step.Validation != nil {
        if err := step.Validation(ctx, e.instance.DB); err != nil {
            return fmt.Errorf("validation failed for step %s: %w", step.ID, err)
        }
    }
    
    return nil
}
```

### Estratégias de Migração por Tipo de Mudança

#### Mudanças Seguras (Automáticas)
| Mudança | Estratégia | SQL Gerado |
|---------|------------|------------|
| Nova entidade | CREATE TABLE | `CREATE TABLE new_entity (...)` |
| Novo campo | ADD COLUMN | `ALTER TABLE entity ADD COLUMN new_field TYPE` |
| Novo ator | INSERT | `INSERT INTO _actors (actor_id, ...) VALUES (...)` |
| Nova permissão | UPDATE | `UPDATE _actors SET permissions = ...` |

#### Mudanças de Revisão (Requer Confirmação)
| Mudança | Estratégia | Considerações |
|---------|------------|----------------|
| Tipo de campo alterado | Conversão | Verificar compatibilidade de dados |
| Campo renomeado | ADD + UPDATE + DROP | Migrar dados antes de dropar |
| Permissão alterada | UPDATE | Invalidar sessões ativas |
| Regra de negócio modificada | UPDATE | Testar impacto em dados existentes |

#### Mudanças Quebradoras (Proteção de Dados)
| Mudança | Estratégia | Proteção |
|---------|------------|----------|
| Campo removido | Renomear para `_deprecated_` | Preservar dados originais |
| Entidade removida | Renomear para `_archived_` | Manter tabela com dados |
| Ator removido | Desativar (is_active = false) | Preservar histórico |
| Workflow removido | Marcar como inativo | Manter instâncias existentes |

## Integração Completa do Sistema AgentOS

### Arquitetura Final
```
┌─────────────────────────────────────────────────────────────────┐
│                    Manifesto YAML/JSON                          │
└─────────────────────────┬─────────────────────────────────────────┘
                        │
┌─────────────────────────▼─────────────────────────────────────────┐
│                    AgentOS Engine                                 │
├─────────────────────────┬─────────────────────────────────────────┤
│  Core Services          │  Subsystem Managers                     │
│  - Manifest Store       │  - Actor Manager                        │
│  - System Instance      │  - Policy Engine                        │
│  - Evolution Manager    │  - Workflow Engine                      │
│  - Audit System         │  - API Manager                          │
│                         │  - MCP Manager                          │
├─────────────────────────┼─────────────────────────────────────────┤
│  Data Layer             │  Business Logic Layer                   │
│  - Database Migrations  │  - Rule Engine                          │
│  - Entity Repositories  │  - Validation Engine                    │
│  - Workflow Store       │  - Permission Engine                    │
├─────────────────────────┼─────────────────────────────────────────┤
│  Integration Layer      │  Evolution Layer                        │
│  - REST API Generator  │  - Manifest Diff                        │
│  - MCP Server          │  - Migration Planner                    │
│  - Webhook System      │  - Evolution Executor                   │
└─────────────────────────┴─────────────────────────────────────────┘
                        │
┌─────────────────────────▼─────────────────────────────────────────┐
│                    Database Layer                                 │
│  - System Tables       - Entity Tables     - Workflow Tables     │
│  - _actors             - User              - _workflow_instances │
│  - _manifest_versions  - Project          - _transition_history│
│  - _audit_log          - Task              - _timeout_records    │
└─────────────────────────────────────────────────────────────────┘
```

### Fluxo de Operação Unificado
```
1. Receber Requisição (API ou MCP)
   ├─ Extrair ator e permissões
   ├─ Validar autenticação
   └─ Verificar autorização

2. Processar Operação
   ├─ Executar regras de negócio (before)
   ├─ Validar dados de entrada
   ├─ Executar operação principal
   ├─ Executar regras de negócio (after)
   └─ Registrar auditoria

3. Retornar Resultado
   ├─ Formatar resposta (JSON para API, MCP para MCP)
   ├─ Incluir metadados e validações
   └─ Garantir consistência
```

### Benefícios do Sistema AgentOS

1. **Declarativo**: Toda configuração em um único manifesto
2. **Automático**: Geração completa de infraestrutura
3. **Seguro**: Controle de acesso em todos os níveis
4. **Persistente**: Dados e estados consistentes
5. **Unificado**: Mesmas operações via API e MCP
6. **Evoluível**: Mudanças sem perda de dados
7. **Auditável**: Histórico completo de operações
8. **Escalável**: Arquitetura modular e extensível

### Exemplo de Uso Completo
```go
// 1. Inicializar sistema a partir do manifesto
engine := agentos.NewEngine(&agentos.EngineConfig{
    ManifestPath: "manifest.yaml",
    AutoEvolve: true,
    EnableMCP: true,
    EnableAPI: true,
})

instance, err := engine.Bootstrap(context.Background())
if err != nil {
    log.Fatal(err)
}

// 2. Sistema pronto - mesmas operações disponíveis via API e MCP
// API: POST /api/v1/experiences
// MCP: callTool("experience.create", {...})

// 3. Executar workflow complexo
result, err := instance.ExecuteOperation(context.Background(), 
    "experience.transition.submit_review",
    map[string]interface{}{
        "id": "exp-123",
        "action": "submit_review",
    },
    "user-456",
)

// 4. Evoluir sistema
newManifest, _ := manifest.ParseFile("manifest-v2.yaml")
evolutionResult, err := engine.Evolve(context.Background(), newManifest)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Evolução concluída: %d passos aplicados\n", len(evolutionResult.AppliedSteps))
```

Este refinamento transforma o PicoClaw em um verdadeiro Sistema Operacional de Agentes, capaz de gerenciar sistemas complexos a partir de definições declarativas, com segurança, persistência e evolução controlada.