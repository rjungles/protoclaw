# Etapa 7: MCP Server Nativo e Operações Unificadas

## Objetivo

Gerar um **MCP Server** que expõe exatamente as mesmas operações disponíveis via API REST, com o mesmo modelo de permissões, regras de negócio e gestão de estados. Isso permite que o sistema gerado pelo manifesto seja consumido tanto por humanos (via API) quanto por agentes LLM (via MCP).

## Estrutura de Arquivos a Criar

```
pkg/mcp/
    server/
        server.go          # MCPServer + ServerGenerator
        handler.go         # OperationHandler para cada operação
        tools.go           # Conversão de Operation → Tool MCP
        resources.go       # Recursos MCP (entidades como resources)
        server_test.go    # Testes unitários
        server_integration_test.go  # Testes de integração
```

## Componentes a Implementar

### 1. MCPServer (`server.go`)

Servidor MCP que implementa o protocolo JSON-RPC 2.0.

```go
type MCPServer struct {
    manifest     *manifest.Manifest
    catalog      *agentos.OperationCatalog
    actorStore   agentos.ActorStore
    policyEngine *policy.Engine
    ruleExecutor *api.RuleExecutor
    db           *sql.DB
    
    mu           sync.RWMutex
    initialized bool
}

type ServerConfig struct {
    Name        string
    Version     string
    Description string
}

func NewMCPServer(cfg ServerConfig, systemInstance *agentos.SystemInstance) *MCPServer

func (s *MCPServer) HandleRequest(ctx context.Context, req *Request) (*Response, error)

func (s *MCPServer) HandleInitialize(ctx context.Context, params InitializeParams) (*InitializeResult, error)

func (s *MCPServer) HandleListTools(ctx context.Context, params ListToolsParams) (*ListToolsResult, error)

func (s *MCPServer) HandleCallTool(ctx context.Context, params CallToolParams) (*CallToolResult, error)

func (s *MCPServer) HandleListResources(ctx context.Context, params ListResourcesParams) (*ListResourcesResult, error)

func (s *MCPServer) HandleReadResource(ctx context.Context, params ReadResourceParams) (*ReadResourceResult, error)
```

### 2. OperationHandler (`handler.go`)

Handler que executa operações do catálogo com verificação de permissões.

```go
type OperationHandler struct {
    manifest     *manifest.Manifest
    actorStore   agentos.ActorStore
    policyEngine *policy.Engine
    ruleExecutor *api.RuleExecutor
    db           *sql.DB
}

func (h *OperationHandler) ExecuteOperation(ctx context.Context, op *agentos.Operation, args map[string]interface{}, actorID string) (map[string]interface{}, error)

func (h *OperationHandler) CheckPermission(actorID string, op *agentos.Operation) error

func (h *OperationHandler) ExecuteBeforeRules(ctx context.Context, op *agentos.Operation, data map[string]interface{}) error

func (h *OperationHandler) ExecuteAfterRules(ctx context.Context, op *agentos.Operation, data map[string]interface{}) error
```

### 3. ToolGenerator (`tools.go`)

Conversão de Operations do catálogo para Tools MCP.

```go
type ToolGenerator struct {
    catalog *agentos.OperationCatalog
}

func (g *ToolGenerator) GenerateTools() []Tool

func (g *ToolGenerator) OperationToTool(op *agentos.Operation) Tool

func (g *ToolGenerator) BuildInputSchema(op *agentos.Operation) map[string]interface{}
```

### 4. ResourceGenerator (`resources.go`)

Conversão de entidades do DataModel para Resources MCP.

```go
type ResourceGenerator struct {
    manifest *manifest.Manifest
    db       *sql.DB
}

func (g *ResourceGenerator) GenerateResources() []Resource

func (g *ResourceGenerator) EntityToResource(entity *manifest.Entity) Resource

func (g *ResourceGenerator) ReadEntityData(uri string) (*ReadResourceResult, error)
```

## Fluxo de Execução de uma Tool MCP

```
1. Receive tools/call request
   └─ Parse CallToolParams (name, arguments)

2. Resolve actor from context
   └─ X-Actor-ID header ou Authorization header

3. Find operation in catalog by tool name
   └─ Map: "entity.list" → Operation{Entity, Action, Permissions}

4. Check permission
   └─ policyEngine.CheckPermission(actorID, resource, action)
   └─ Se negado: return error response

5. Execute before rules
   └─ ruleExecutor.ExecuteBefore(event, entity, data)
   └─ Se rejeitado: return error response

6. Execute operation
   └─ CRUD: repository.FindAll/FindByID/Create/Update/Delete
   └─ Workflow: fsm.Transition(roles, action)

7. Execute after rules
   └─ ruleExecutor.ExecuteAfter(event, entity, data)

8. Audit log
   └─ auditLog.Record(entry)

9. Return result
   └─ CallToolResult{Content: [{Type: "text", Text: result}]}
```

## Paridade de Comportamento

| Aspecto | API REST | MCP Server |
|---------|----------|------------|
| Autenticação | X-Actor-ID, Bearer, X-API-Key | X-Actor-ID, Authorization |
| Permissões | policyEngine.CheckPermission | Mesma verificação |
| Regras Before | ruleExecutor.ExecuteBefore | Mesma execução |
| Regras After | ruleExecutor.ExecuteAfter | Mesma execução |
| Workflow FSM | fsm.Transition | Mesma transição |
| Auditoria | auditLog.Record | Mesmo registro |
| Notificações | notifyBus.Notify | Mesma notificação |

## Nomenclatura de Tools

Para manter consistência, as tools MCP seguem o padrão:

```
{entity}.{action}
```

Exemplos:
- `customer.list` - Lista clientes
- `customer.get` - Obtém cliente por ID
- `customer.create` - Cria cliente
- `customer.update` - Atualiza cliente
- `customer.delete` - Deleta cliente
- `experience.transition.submit_review` - Executa transição de workflow

## Recursos MCP

Cada entidade do DataModel é exposta como um Resource MCP:

```
resource://{system}/{entity}/{id}
```

Exemplos:
- `resource://cafeteria/customer/cust-001`
- `resource://parking/ticket/ticket-123`
- `resource://task-management/task/task-456`

## Testes

### Unit Tests (`server_test.go`)

1. `TestMCPServer_Initialize` - Verifica inicialização do servidor
2. `TestMCPServer_ListTools` - Verifica listagem de tools
3. `TestMCPServer_CallTool_List` - Verifica execução de list
4. `TestMCPServer_CallTool_Create` - Verifica criação com validação
5. `TestMCPServer_CallTool_PermissionDenied` - Verifica negação de permissão
6. `TestMCPServer_CallTool_BeforeRuleReject` - Verifica rejeição por regra before
7. `TestToolGenerator_GenerateTools` - Verifica conversão de operations para tools
8. `TestResourceGenerator_GenerateResources` - Verifica geração de resources

### Integration Tests (`server_integration_test.go`)

1. `TestMCPServer_FullPipeline` - Pipeline completo com manifesto
2. `TestMCPServer_CafeteriaLoyalty` - Sistema de cafeteria via MCP
3. `TestMCPServer_ParkingTicket` - Sistema de estacionamento via MCP
4. `TestMCPServer_WorkflowTransitions` - Transições de workflow via MCP

## Verificação

```bash
# Rodar testes unitários
go test ./pkg/mcp/server/... -v -count=1

# Rodar testes de integração
go test ./pkg/mcp/server/... -v -count=1 -run "Integration"

# Exemplo de uso (após implementação):
go run examples/mcp-server/main.go examples/manifests/cafeteria-loyalty.yaml
```
