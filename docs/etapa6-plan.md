# Etapa 6: Orquestrador de Sistema (System Bootstrapper)

## Objetivo

Criar o componente central que, dado um manifesto, orquestra todos os subsistemas existentes para materializar o sistema descrito. O `Bootstrapper` recebe um manifesto e executa sequencialmente: validação, migração de banco, provisionamento de atores, registro de FSMs, montagem da API REST e ativação dos hooks.

## Estrutura de Arquivos a Criar

```
pkg/agentos/
    bootstrap.go          # Bootstrapper + SystemInstance
    operations.go         # OperationCatalog + Operation
    actor_store.go        # ActorStore para credenciais
    middleware.go         # AuthMiddleware
    bootstrap_test.go    # Testes unitários
    bootstrap_integration_test.go  # Testes de integração
```

## Componentes a Implementar

### 1. OperationCatalog (`operations.go`)

Catálogo central que registra todas as operações do sistema, consumido tanto pela API REST quanto pelo MCP Server.

```go
type Operation struct {
    Name          string                 // "experiences.list", "tasks.create"
    Entity        string                 // "Experience", "Task"
    Action        string                 // "list", "create", "update", "delete", "transition"
    Method        string                 // "GET", "POST", "PUT", "DELETE"
    Path          string                 // "/api/v1/experiences/{id}/actions/{action}"
    Description   string
    InputSchema   map[string]interface{}  // JSON Schema para input
    OutputSchema  map[string]interface{}  // JSON Schema para output
    Permissions   []string               // ["experience:read", "task:write"]
    WorkflowAction string                // Se for ação de workflow: "submit_review"
}

type OperationCatalog struct {
    operations []Operation
    byEntity  map[string][]Operation
    byName    map[string]*Operation
}
```

**Métodos:**
- `NewCatalog(manifest)`: Constrói catálogo automaticamente a partir do manifesto
- `Register(op)`: Adiciona operação manualmente
- `Get(name)`: Retorna operação por nome
- `ListByEntity(entity)`: Lista operações de uma entidade
- `ListAll()`: Lista todas as operações
- `BuildOperationCatalog` gera automaticamente:
  - CRUD para cada entidade do DataModel
  - Ações de workflow a partir das transições
  - Operações customizadas das regras de negócio

### 2. ActorStore (`actor_store.go`)

Armazena atores e credenciais no banco de dados.

```go
type ActorCredential struct {
    ActorID    string    // "author_001"
    ActorType  string    // "author"
    APIKey     string    // Hash da API key
    Roles      []string  // ["creator"]
    CreatedAt  time.Time
    IsActive   bool
}

type ActorStore interface {
    Provision(actor manifest.Actor) (*ActorCredential, error)
    GetByID(actorID string) (*ActorCredential, error)
    GetByAPIKey(apiKey string) (*ActorCredential, error)
    ListAll() ([]*ActorCredential, error)
    Deactivate(actorID string) error
}

type MemoryActorStore struct {
    actors map[string]*ActorCredential
    mu     sync.RWMutex
}

type DBActorStore struct {
    db *sql.DB
}
```

**Tabela `_actors`:**
```sql
CREATE TABLE IF NOT EXISTS _actors (
    actor_id TEXT PRIMARY KEY,
    actor_type TEXT NOT NULL,
    api_key_hash TEXT NOT NULL,
    roles TEXT NOT NULL,  -- JSON array
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT TRUE
);
```

### 3. AuthMiddleware (`middleware.go`)

Middleware HTTP que autentica requisições e injeta o `ActorID` no contexto.

```go
type contextKey string

const ActorIDKey contextKey = "actor_id"

type AuthMiddleware struct {
    actorStore ActorStore
    manifest   *manifest.Manifest
}

func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler

func (m *AuthMiddleware) authenticate(r *http.Request) (string, error)
// Extrai ator de:
// 1. X-Actor-ID header (para desenvolvimento)
// 2. Authorization: Bearer <api_key>
// 3. X-API-Key header
// 4. Retorna "anonymous" como fallback
```

### 4. Bootstrapper + SystemInstance (`bootstrap.go`)

Componente principal que orquestra todos os subsistemas.

```go
type BootstrapConfig struct {
    ManifestPath  string  // Caminho para arquivo de manifesto
    Manifest      *manifest.Manifest  // Ou manifesto já carregado
    DBDriver      string  // "sqlite", "postgres", "mysql"
    DBConnection  string  // Connection string
    DataDir       string  // Diretório para arquivos (audit, workflows)

    // Opções de inicialização
    SkipMigrate   bool    // Pula migração de banco
    SkipAudit     bool    // Pula sistema de auditoria
    SkipMCP       bool    // Pula inicialização MCP
}

type SystemInstance struct {
    // Subsistemas inicializados
    Manifest     *manifest.Manifest
    DB           *sql.DB
    Catalog      *OperationCatalog
    ActorStore   ActorStore
    PolicyEngine *policy.Engine
    RuleExecutor *api.RuleExecutor
    APIGenerator *api.Generator
    Migrator     *db.Migrator

    // Workflow
    WorkflowEngines map[string]*workflow.FSM  // FSMs por entidade

    // Infra
    HTTPMux       *http.ServeMux
    ShutdownFuncs []func(context.Context) error
}

type Bootstrapper struct {
    config BootstrapConfig
}

func NewBootstrapper(cfg BootstrapConfig) *Bootstrapper

func (b *Bootstrapper) Bootstrap(ctx context.Context) (*SystemInstance, error) {
    // 1. Carregar manifesto (se não fornecido)
    // 2. Validar manifesto
    // 3. Conectar ao banco de dados
    // 4. Executar migrações
    // 5. Provisionar atores
    // 6. Criar catálogo de operações
    // 7. Criar engine de políticas
    // 8. Criar executor de regras
    // 9. Criar FSMs de workflow
    // 10. Criar generator de API
    // 11. Montar HTTP mux com middleware
    // 12. Retornar SystemInstance
}

func (si *SystemInstance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    si.HTTPMux.ServeHTTP(w, r)
}

func (si *SystemInstance) Shutdown(ctx context.Context) error {
    for _, fn := range si.ShutdownFuncs {
        if err := fn(ctx); err != nil {
            return err
        }
    }
    return nil
}

func (si *SystemInstance) GetOperationCatalog() *OperationCatalog {
    return si.Catalog
}

func (si *SystemInstance) GetActorStore() ActorStore {
    return si.ActorStore
}
```

**Pipeline de Bootstrap:**

```
1. Load Manifest
   └─ manifest.ParseFile(path) ou usa config.Manifest

2. Validate Manifest
   └─ manifest.Parser{}.Validate()

3. Open Database
   ├─ sql.Open(driver, connection)
   ├─ SetMaxOpenConns(1) para SQLite
   └─ Ping()

4. Run Migrations (se não SkipMigrate)
   ├─ db.NewMigrator(sqlDB, manifest)
   └─ migrator.Migrate(ctx)

5. Provision Actors
   ├─ actorStore.Provision(actor) para cada actor
   └─ Gera API keys e armazena hash

6. Build Operation Catalog
   ├─ NewCatalog(manifest)
   └─ Adiciona CRUD + workflow actions

7. Create Policy Engine
   └─ policy.NewEngine(manifest)

8. Create Rule Executor
   └─ api.NewRuleExecutor(manifest)

9. Create Workflow FSMs
   ├─ Para cada entity com workflow no manifesto
   └─ workflow.NewFSM(config)

10. Create API Generator
    ├─ api.NewGeneratorWithDB(manifest, db)
    └─ gen.BuildMux()

11. Mount HTTP Mux
    ├─ http.NewServeMux()
    ├─ authMiddleware.Wrap(genMux)
    ├─ /_system/info (status)
    ├─ /_system/actors (listar atores)
    └─ /_system/health
```

## Validações e Decisões de Design

### Geração de Operações

Para cada **entidade** no `DataModel`:
- `list` → GET /{entityPlural}
- `get` → GET /{entityPlural}/{id}
- `create` → POST /{entityPlural}
- `update` → PUT /{entityPlural}/{id}
- `delete` → DELETE /{entityPlural}/{id}

Para cada **workflow**:
- Para cada transição com action "submit_review", "approve", etc.
- POST /{entityPlural}/{id}/actions/{action}

### Validação do Manifesto

Antes de prosseguir, validar:
1. Metadata (name, version)
2. Se há pelo menos uma API definida em `integrations.apis`
3. Se há pelo menos um ator definido
4. Se entidades referenciadas em permissions existem no `data_model`

### API Key Generation

```go
func generateAPIKey() string {
    b := make([]byte, 32)
    rand.Read(b)
    return base64.URLEncoding.EncodeToString(b)
}

func hashAPIKey(key string) string {
    h := sha256.Sum256([]byte(key))
    return hex.EncodeToString(h[:])
}
```

## Arquivos Críticos a Modificar

Nenhum arquivo existente precisa ser modificado. A Etapa 6 é inteiramente aditiva, criando um novo pacote `pkg/agentos` com novos arquivos que consomem os pacotes existentes.

## Testes

### Unit Tests (`bootstrap_test.go`)

1. `TestOperationCatalog_NewCatalog`: Verifica geração automática de operações CRUD
2. `TestOperationCatalog_GetByEntity`: Verifica filtro por entidade
3. `TestActorStore_MemoryStore`: Teste do store em memória
4. `TestActorStore_Provision`: Teste de provisionamento
5. `TestAuthMiddleware_ExtractActorID`: Teste de extração de ator de headers
6. `TestBootstrapper_ValidateManifest`: Teste de validação
7. `TestBootstrapper_Bootstrap_Minimal`: Teste com manifesto mínimo

### Integration Tests (`bootstrap_integration_test.go`)

1. `TestBootstrap_FullPipeline`: Carrega cafeteria-loyalty.yaml, faz bootstrap completo, verifica todas as operações registradas
2. `TestBootstrap_ParkingTicket`: Carrega parking-ticket.yaml, verifica migrations e API keys
3. `TestSystemInstance_HTTPEndpoints`: Verifica que endpoints funcionam com autenticação

## Verificação

```bash
# Rodar testes unitários
go test ./pkg/agentos/... -v -run "TestBootstrap|TestOperation|TestActor|TestMiddleware"

# Rodar testes de integração
go test ./pkg/agentos/... -v -run "Integration"

# Exemplo de uso (após implementação):
go run examples/bootstrap/main.go examples/manifests/cafeteria-loyalty.yaml
```
