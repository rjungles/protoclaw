# Etapa 9: Evolução de Manifesto e Migração Sem Perda de Dados

## Objetivo

Permitir que o manifesto evolua ao longo do tempo — novas entidades, novos campos, novas regras, novos atores — sem perda de dados existentes. O sistema deve **detectar diferenças** entre o manifesto atual e o anterior e aplicar as mudanças de forma segura.

## Estrutura de Arquivos a Criar

```
pkg/agentos/
    evolution/
        diff.go           # ManifestDiff - comparação de manifestos
        plan.go           # MigrationPlan - plano de migração
        executor.go       # EvolutionExecutor - execução de migrações
        versioning.go    # ManifestVersioning - versionamento de manifestos
        evolution_test.go # Testes unitários
        evolution_integration_test.go  # Testes de integração
```

## Componentes a Implementar

### 1. ManifestDiff (`diff.go`)

Comparar dois manifestos e produzir lista de mudanças.

```go
type ChangeType string

const (
    ChangeTypeAdd    ChangeType = "add"
    ChangeTypeModify ChangeType = "modify"
    ChangeTypeRemove ChangeType = "remove"
)

type ChangeSeverity string

const (
    SeveritySafe     ChangeSeverity = "safe"
    SeverityReview   ChangeSeverity = "review"
    SeverityBreaking ChangeSeverity = "breaking"
)

type Change struct {
    Type       ChangeType
    Path       string           // "data_model.entities.User.fields.email"
    Severity   ChangeSeverity
    OldValue   interface{}
    NewValue   interface{}
    Description string
}

type ManifestDiff struct {
    FromVersion string
    ToVersion   string
    Changes     []Change
}

func DiffManifests(old, new *manifest.Manifest) *ManifestDiff

func (d *ManifestDiff) HasBreakingChanges() bool

func (d *ManifestDiff) GetSafeChanges() []Change

func (d *ManifestDiff) GetReviewChanges() []Change

func (d *ManifestDiff) GetBreakingChanges() []Change
```

### 2. MigrationPlan (`plan.go`)

Gerar plano ordenado de ações a partir do diff.

```go
type MigrationAction string

const (
    ActionAddColumn    MigrationAction = "ADD_COLUMN"
    ActionAddEntity   MigrationAction = "ADD_ENTITY"
    ActionAddActor    MigrationAction = "ADD_ACTOR"
    ActionModifyField  MigrationAction = "MODIFY_FIELD"
    ActionRemoveField MigrationAction = "REMOVE_FIELD"
    ActionRemoveEntity MigrationAction = "REMOVE_ENTITY"
    ActionDeprecateField MigrationAction = "DEPRECATE_FIELD"
    ActionArchiveEntity MigrationAction = "ARCHIVE_ENTITY"
    ActionMigrateData  MigrationAction = "MIGRATE_DATA"
)

type MigrationStep struct {
    Action      MigrationAction
    Entity     string
    Field      string
    SQL        string
    RollbackSQL string
    DataMigration string  // SQL para migrar dados antes da mudança
}

type MigrationPlan struct {
    Steps      []MigrationStep
    Safe      []MigrationStep
    Review    []MigrationStep
    Breaking  []MigrationStep
}

func CreateMigrationPlan(diff *ManifestDiff, db *sql.DB) *MigrationPlan

func (p *MigrationPlan) CanApplyAutomatically() bool

func (p *MigrationPlan) RequiresConfirmation() bool

func (p *MigrationPlan) GetExecutableSteps() []MigrationStep
```

### 3. EvolutionExecutor (`executor.go`)

Executar o plano de migração.

```go
type EvolutionResult struct {
    Success     bool
    AppliedSteps []MigrationStep
    FailedStep  *MigrationStep
    Error       error
    Warnings    []string
}

type EvolutionExecutor struct {
    manifest   *manifest.Manifest
    db         *sql.DB
    versioning *ManifestVersioning
}

func NewEvolutionExecutor(manifest *manifest.Manifest, db *sql.DB) *EvolutionExecutor

func (e *EvolutionExecutor) Evolve(ctx context.Context, newManifest *manifest.Manifest) (*EvolutionResult, error)

func (e *EvolutionExecutor) ApplyPlan(ctx context.Context, plan *MigrationPlan) (*EvolutionResult, error)

func (e *EvolutionExecutor) Rollback(ctx context.Context, version string) error

func (e *EvolutionExecutor) GetCurrentVersion() string

func (e *EvolutionExecutor) GetVersionHistory() ([]ManifestVersion, error)
```

### 4. ManifestVersioning (`versioning.go`)

Versionar manifestos no banco de dados.

```go
type ManifestVersion struct {
    Version     string
    Manifest    string  // YAML ou JSON do manifesto
    CreatedAt   time.Time
    CreatedBy   string
    Description string
    DiffFromPrevious string  // Diff em relação à versão anterior
}

type ManifestVersionStore interface {
    SaveVersion(version *ManifestVersion) error
    GetVersion(version string) (*ManifestVersion, error)
    GetLatestVersion() (*ManifestVersion, error)
    ListVersions() ([]ManifestVersion, error)
    DeleteVersion(version string) error
}

type DBManifestVersionStore struct {
    db *sql.DB
}

func NewDBManifestVersionStore(db *sql.DB) *DBManifestVersionStore

func (s *DBManifestVersionStore) createTable() error
```

## Fluxo de Evolução

```
1. Load new manifest
   └─ ParseFile(newManifestPath)

2. Get current manifest version
   └─ versioning.GetLatestVersion()

3. Diff manifests
   └─ DiffManifests(old, new)

4. Classify changes by severity
   ├─ Safe: ADD_COLUMN, ADD_ENTITY, ADD_ACTOR
   ├─ Review: MODIFY_FIELD, MIGRATE_DATA
   └─ Breaking: REMOVE_FIELD, REMOVE_ENTITY

5. Create migration plan
   └─ CreateMigrationPlan(diff, db)

6. Save new manifest version
   └─ versioning.SaveVersion(newManifest)

7. Execute safe changes automatically
   └─ executor.ApplyPlan(plan.Safe)

8. Report review changes for manual confirmation
   └─ Return result with review changes

9. For breaking changes:
   ├─ Fields: Rename to _deprecated_{name}
   ├─ Entities: Rename to _archived_{name}
   └─ Actors: Deactivate instead of delete
```

## Estratégias de Migração

### Para Mudanças Seguras (Automáticas)

| Mudança | Ação |
|---------|------|
| Nova entidade | CREATE TABLE |
| Novo campo | ALTER TABLE ADD COLUMN |
| Novo ator | INSERT INTO _actors |
| Novo workflow | Adicionar FSM |

### Para Mudanças de Revisão

| Mudança | Ação |
|---------|------|
| Tipo de campo alterado | Converter dados se possível |
| Campo renomeado | UPDATE com mapeamento |
| Permissão alterada | Invalidar sessões ativas |

### Para Mudanças Quebradoras (Proteção de Dados)

| Mudança | Ação |
|---------|------|
| Campo removido | ALTER TABLE ADD COLUMN _deprecated_{name} |
| Entidade removida | RENAME TABLE TO _archived_{name} |
| Ator removido | UPDATE _actors SET is_active = FALSE |

## Testes

### Unit Tests (`evolution_test.go`)

1. `TestManifestDiff_AddEntity` - Detectar nova entidade
2. `TestManifestDiff_RemoveField` - Detectar campo removido
3. `TestManifestDiff_ModifyField` - Detectar campo modificado
4. `TestMigrationPlan_SafeChanges` - Plano para mudanças seguras
5. `TestMigrationPlan_BreakingChanges` - Plano para mudanças quebradoras
6. `TestEvolutionExecutor_Evolve` - Evolução completa
7. `TestManifestVersioning_SaveVersion` - Salvar versão
8. `TestManifestVersioning_GetHistory` - Histórico de versões

### Integration Tests (`evolution_integration_test.go`)

1. `TestEvolution_AddNewField` - Adicionar novo campo
2. `TestEvolution_RenameField` - Renomear campo (sem perda)
3. `TestEvolution_AddNewEntity` - Adicionar nova entidade
4. `TestEvolution_VersionHistory` - Histórico de versões

## Verificação

```bash
# Rodar testes unitários
go test ./pkg/agentos/evolution/... -v -count=1

# Rodar testes de integração
go test ./pkg/agentos/evolution/... -v -count=1 -run "Integration"
```
