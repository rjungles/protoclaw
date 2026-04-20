# AgentOS - Etapas Implementadas: Documentação Sintetizada

## Visão Geral

O **AgentOS** é um Sistema Operacional de Agentes que transforma o PicoClaw em uma plataforma capaz de gerar automaticamente infraestrutura completa a partir de um manifesto declarativo YAML/JSON.

As etapas implementadas estabelecem uma arquitetura em camadas que vai da fundação de governança até a evolução inteligente do sistema.

---

## 📋 Resumo das Etapas

| Etapa | Nome | Propósito | Componentes Principais |
|-------|------|-----------|------------------------|
| **1** | Fundação de Governança | Parser e validação de manifestos, controle de acesso RBAC/ABAC | `pkg/manifest`, `pkg/governance/policy`, `pkg/workflow` |
| **6** | Orquestrador de Sistema | Bootstrap unificado que materializa o sistema descrito no manifesto | `pkg/agentos/bootstrap`, OperationCatalog, ActorStore |
| **7** | MCP Server Nativo | Exposição das operações via protocolo MCP com paridade total à API REST | `pkg/mcp/server`, OperationHandler, ToolGenerator |
| **8** | Gestão de Estados Complexos | Workflows persistentes com guards, side effects, timeouts e histórico | `pkg/agentos/stateful`, WorkflowEngine, GuardEvaluator |
| **9** | Evolução Inteligente | Migração automática sem perda de dados entre versões de manifesto | `pkg/agentos/evolution`, ManifestDiff, MigrationPlan |

---

## 🔧 Etapa 1: Fundação de Governança e Validação

### Propósito
Estabelecer a base declarativa do sistema, permitindo que toda a infraestrutura seja definida via manifesto.

### Comportamento

#### Parser de Manifesto (`pkg/manifest`)
- **Entrada**: Arquivo YAML/JSON descrevendo o sistema
- **Processamento**: Validação estrutural e semântica
- **Saída**: Estrutura `Manifest` tipada em Go

**Validações Automáticas:**
- Metadata obrigatória (name, version)
- IDs únicos para atores e regras
- Tipos de campos válidos (string, int, float, bool, datetime, reference)
- Hierarquia de papéis sem ciclos
- Referências cruzadas consistentes

#### Engine de Políticas (`pkg/governance/policy`)
- **Modelo**: RBAC (Role-Based) + ABAC (Attribute-Based)
- **Features**: Hierarquia de papéis, wildcards (`*`), condições contextuais
- **Decisão**: Default deny (negação por padrão)

**Exemplo de Avaliação:**
```go
ctx := &policy.Context{
    ActorID: "user123",
    Resource: "projects",
    Action: "delete",
    Attributes: map[string]interface{}{"owner": "user456"},
}
result := engine.CheckPermission(ctx) // Allowed/Denied + Reason
```

#### Máquina de Estados (`pkg/workflow`)
- **Função**: Gerencia transições de estado de entidades
- **Características**: Validação por papel, listagem de ações disponíveis
- **Uso**: Workflows simples e complexos

---

## 🚀 Etapa 6: Orquestrador de Sistema (Bootstrapper)

### Propósito
Materializar o sistema descrito no manifesto, orquestrando todos os subsistemas em uma instância funcional.

### Comportamento

#### Pipeline de Bootstrap (12 passos)
```
1. Load Manifest     → Parse e validação completa
2. Validate         → Verifica consistência
3. Open Database    → Conexão ao banco configurado
4. Run Migrations     → Cria/ atualiza schema
5. Provision Actors   → Gera API keys para atores
6. Build Catalog      → Registra todas as operações
7. Create PolicyEng   → Inicializa controle de acesso
8. Create RuleExec    → Prepara regras de negócio
9. Create FSMs        → Instancia workflows
10. Create APIGen    → Gera handlers REST
11. Mount HTTP Mux   → Configura rotas + middleware
12. Return Instance  → Sistema pronto para uso
```

#### OperationCatalog
Registra automaticamente:
- **CRUD**: list, get, create, update, delete para cada entidade
- **Workflow**: POST /{entity}/{id}/actions/{action}
- **Custom**: Operações definidas em regras de negócio

#### ActorStore
- **Memória**: Para desenvolvimento/testes
- **Banco de Dados**: Para produção (tabela `_actors`)
- **Geração**: API keys criptograficamente seguras

---

## 🔌 Etapa 7: MCP Server Nativo

### Propósito
Expor exatamente as mesmas operações da API REST via protocolo MCP, permitindo consumo por agentes LLM.

### Comportamento

#### Paridade Total
| Aspecto | API REST | MCP Server |
|---------|----------|------------|
| Autenticação | X-Actor-ID, Bearer, X-API-Key | Idêntico |
| Permissões | policy.CheckPermission | Mesma verificação |
| Regras Before/After | ruleExecutor.Execute | Mesma execução |
| Workflows | fsm.Transition | Mesma transição |
| Auditoria | auditLog.Record | Mesmos logs |

#### Nomenclatura de Tools MCP
```
{entity}.{action}                    # CRUD
{entity}.transition.{action}          # Workflow
system.{operation}                    # Operações de sistema

Exemplos:
- experience.list
- experience.create
- experience.transition.submit_review
- system.health
```

#### Fluxo de Execução MCP
```
1. Receive tools/call request
2. Resolve actor from context
3. Find operation in catalog
4. Check permission
5. Execute before rules
6. Execute operation (CRUD ou Workflow)
7. Execute after rules
8. Record audit log
9. Return CallToolResult
```

---

## 🔄 Etapa 8: Gestão de Estados Complexos (Stateful)

### Propósito
Suportar workflows sofisticados com múltiplas etapas, validações, efeitos colaterais e persistência.

### Comportamento

#### WorkflowEngine (`pkg/agentos/stateful/engine.go`)
Gerencia o ciclo de vida completo de transições de estado.

**Fluxo de Transição:**
```
1. Receive Request    → entityType, entityID, action, actorID, roles
2. Load State        → Recupera instância atual do store
3. Find Transition   → Valida se ação existe no estado atual
4. Check Roles       → Verifica allowed_roles
5. Evaluate Guards   → Executa validações customizadas
6. Execute OnExit    → Efeitos colaterais de saída
7. Perform Transition → Atualiza estado no banco
8. Execute OnEnter   → Efeitos colaterais de entrada
9. Record History    → Salva transição no histórico
10. Setup Timeouts   → Configura timeouts se houver
11. Return Result    → Nova instância + metadados
```

#### Guards (`guards.go`)
Validações obrigatórias para permitir transições:
- `field_not_empty`: Campo deve ter valor
- `field_equals`: Campo deve igualar valor esperado
- `custom`: Lógica customizada
- `expression`: Expressão complexa

**Exemplo no Manifesto:**
```yaml
transitions:
  - to: "UNDER_REVIEW"
    action: "submit_review"
    guards:
      - field: "raw_content"
        condition: "not_empty"
        message: "Conteúdo é obrigatório"
      - field: "title"
        condition: "min_length"
        value: 10
```

#### Side Effects (`side_effects.go`)
Ações automáticas executadas nas transições:
- `notify`: Notifica atores específicos
- `update_field`: Atualiza campos da entidade
- `webhook`: Chama endpoint externo
- `log`: Registra evento

**Exemplo:**
```yaml
on_enter:
  - type: "notify"
    target: "editor"
    message: "Nova experiência: {{entity.title}}"
  - type: "update_field"
    field: "reviewer_id"
    value: "{{current_actor}}"
```

#### Timeouts (`timeouts.go`)
Transições automáticas após período no estado:
```yaml
timeout:
  duration: "7d"
  transition_to: "DRAFT"
  action: "auto_return"
```

#### Histórico (`history.go`)
Registro imutável de todas as transições com:
- Estados de origem e destino
- Ação executada
- Ator responsável
- Timestamp
- Metadados adicionais

---

## 📈 Etapa 9: Evolução e Migração Inteligente

### Propósito
Permitir que o sistema evolua ao longo do tempo sem perda de dados.

### Comportamento

#### ManifestDiff (`evolution/diff.go`)
Compara manifestos e classifica mudanças:

| Tipo | Severidade | Exemplos |
|------|------------|----------|
| **Safe** | Automática | Nova entidade, novo campo, novo ator |
| **Review** | Requer confirmação | Alterar tipo de campo, renomear campo |
| **Breaking** | Proteção de dados | Remover campo, remover entidade |

#### MigrationPlan (`evolution/plan.go`)
Gera plano ordenado de migração:
- **Safe Steps**: Executadas automaticamente
- **Review Steps**: Requerem confirmação manual
- **Breaking Steps**: Protegem dados (renomeia, não deleta)

#### Estratégias de Proteção
- **Campo removido** → Renomeado para `_deprecated_{nome}`
- **Entidade removida** → Tabela renomeada para `_archived_{nome}`
- **Ator removido** → Desativado (`is_active = false`)

#### Fluxo de Evolução
```
1. Load new manifest
2. Get current version from database
3. Diff manifests → Detecta mudanças
4. Classify by severity
5. Create migration plan
6. Save new version
7. Execute safe changes
8. Report review/breaking changes
9. Update system instance
```

---

## 🏗️ Arquitetura Integrada

### Fluxo de Requisição Unificado
```
┌─────────────────────────────────────────────────────────────┐
│  Requisição (API REST ou MCP)                               │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  1. Extração de Ator (X-Actor-ID ou Authorization)           │
│  2. Autenticação (ActorStore lookup)                         │
│  3. Autorização (PolicyEngine.CheckPermission)              │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  4. Regras Before (RuleExecutor)                           │
│  5. Validação de Entrada                                   │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Operação Principal                                         │
│  ├── CRUD: Repository pattern                                │
│  └── Workflow: StatefulWorkflowEngine.Transition           │
│      ├── Guards evaluation                                 │
│      ├── Side effects (on_exit/on_enter)                   │
│      ├── History recording                                 │
│      └── Timeout setup                                     │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  6. Regras After (RuleExecutor)                            │
│  7. Auditoria (AuditLog.Record)                             │
│  8. Formatação de Resposta (JSON ou MCP)                    │
└─────────────────────────────────────────────────────────────┘
```

---

## 📊 Cobertura de Testes

| Pacote | Testes | Status |
|--------|--------|--------|
| `pkg/manifest` | 12+ | ✅ Passando |
| `pkg/governance/policy` | 14+ | ✅ Passando |
| `pkg/workflow` | 24+ | ✅ Passando |
| `pkg/agentos/stateful` | 20+ | ✅ Passando |
| `pkg/agentos/evolution` | 14+ | ✅ Passando |
| **Total** | **~84** | **✅ 100%** |

### Testes de Integração Principais

1. **TestWorkflowEngine_ExperiencePlatform**: Fluxo editorial completo
2. **TestWorkflowEngine_ClarificationFlow**: Fluxo de esclarecimento
3. **TestWorkflowEngine_TimeoutAutoReturn**: Timeout automático
4. **TestMigrationPlan_SafeSteps**: Migrações seguras
5. **TestMigrationPlan_BreakingSteps**: Proteção de dados

---

## 🎯 Benefícios do Sistema

1. **Declarativo**: Configuração única via manifesto YAML/JSON
2. **Automático**: Geração completa de infraestrutura
3. **Seguro**: Controle de acesso em todos os níveis
4. **Persistente**: Estados e dados consistentes
5. **Unificado**: Mesmas operações via API e MCP
6. **Evoluível**: Mudanças sem perda de dados
7. **Auditável**: Histórico completo de operações
8. **Escalável**: Arquitetura modular e extensível

---

## 📝 Exemplo de Uso Completo

```go
// 1. Inicializar o sistema
engine := agentos.NewEngine(&agentos.EngineConfig{
    ManifestPath: "experience-platform.yaml",
    AutoEvolve:  true,
    EnableMCP:   true,
    EnableAPI:   true,
})

instance, err := engine.Bootstrap(context.Background())
if err != nil {
    log.Fatal(err)
}

// 2. Executar operação (mesma lógica para API e MCP)
result, err := instance.ExecuteOperation(
    context.Background(),
    "experience.transition.submit_review",
    map[string]interface{}{"id": "exp-123"},
    "user-456",
)

// 3. Evoluir o sistema
newManifest, _ := manifest.ParseFile("manifest-v2.yaml")
evolutionResult, err := engine.Evolve(context.Background(), newManifest)
```

---

## 📁 Estrutura de Arquivos

```
pkg/
├── manifest/           # Parser e validador de manifestos
├── governance/policy/  # Engine RBAC/ABAC
├── workflow/           # Máquina de estados FSM
├── agentos/
│   ├── bootstrap/     # Orquestrador do sistema
│   ├── stateful/        # Workflow engine persistente
│   │   ├── engine.go    # WorkflowEngine
│   │   ├── guards.go    # GuardEvaluator
│   │   ├── side_effects.go
│   │   ├── timeouts.go
│   │   ├── history.go
│   │   └── stateful_integration_test.go
│   └── evolution/       # Sistema de evolução
│       ├── diff.go      # ManifestDiff
│       ├── plan.go      # MigrationPlan
│       └── executor.go  # EvolutionExecutor
└── mcp/
    └── server/          # MCP Server nativo

docs/
├── agentos-etapa1.md
├── agentos-etapa1-resumo.md
├── agentos-etapa1-resultados.md
├── etapa6-plan.md
├── etapa7-plan.md
├── etapa8-plan.md
├── etapa9-plan.md
├── agentos-etapas-6-9-refinadas.md
└── agentos-etapas-sintetizado.md  # Este arquivo
```

---

## ✅ Status de Implementação

| Etapa | Componente | Status |
|-------|------------|--------|
| 1 | Parser de Manifesto | ✅ Completo |
| 1 | Engine RBAC/ABAC | ✅ Completo |
| 1 | FSM Básico | ✅ Completo |
| 6 | Bootstrapper | ✅ Completo |
| 6 | OperationCatalog | ✅ Completo |
| 6 | ActorStore | ✅ Completo |
| 7 | MCP Server | ✅ Completo |
| 7 | Paridade API/MCP | ✅ Completo |
| 8 | WorkflowEngine Stateful | ✅ Completo |
| 8 | Guards | ✅ Completo |
| 8 | Side Effects | ✅ Completo |
| 8 | Timeouts | ✅ Completo |
| 8 | History | ✅ Completo |
| 9 | ManifestDiff | ✅ Completo |
| 9 | MigrationPlan | ✅ Completo |
| 9 | EvolutionExecutor | ✅ Completo |

---

*Documentação sintetizada em Abril 2026*
