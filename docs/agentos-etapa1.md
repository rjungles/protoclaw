# Fundação de Governança e Validação de Manifesto - Etapa 1

Esta é a primeira etapa da evolução do PicoClaw para um **Sistema Operacional de Agentes (AgentOS)** baseado em manifestos declarativos.

## Visão Geral

O sistema permite definir toda a infraestrutura de uma aplicação através de um arquivo de manifesto YAML/JSON, incluindo:
- Atores e suas permissões
- Modelo de dados completo
- Regras de negócio
- Integrações (APIs, MCPs, canais)
- Políticas de segurança
- Requisitos não funcionais

## Componentes Implementados

### 1. Pacote `pkg/manifest`

Responsável pelo parsing, validação e serialização de manifestos.

#### Estruturas Principais

```go
type Manifest struct {
    Metadata        Metadata        // Metadados do sistema
    Actors          []Actor         // Definição de atores
    DataModel       DataModel       // Modelo de dados
    BusinessRules   []BusinessRule  // Regras de negócio
    Integrations    Integrations    // APIs, MCPs, canais
    Security        SecurityPolicy  // Políticas de segurança
    NonFunctional   NonFunctional   // Requisitos não funcionais
}
```

#### Funcionalidades

- **ParseFile(path string)**: Carrega manifesto de arquivo YAML ou JSON
- **ParseYAML(data []byte)**: Parseia dados YAML
- **ParseJSON(data []byte)**: Parseia dados JSON
- **Validate(manifest *Manifest)**: Valida estrutura e consistência
- **ToYAML()/ToJSON()**: Serializa o manifesto

#### Validações Implementadas

- Metadata obrigatório (name, version)
- IDs únicos para atores e regras de negócio
- Entidades com campos válidos e tipos definidos
- Regras de negócio com triggers configurados corretamente
- APIs com nome e base_path obrigatórios
- Hierarquia de papéis sem ciclos

### 2. Pacote `pkg/governance/policy`

Engine de controle de acesso baseada em RBAC/ABAC.

#### Estruturas Principais

```go
type Engine struct {
    manifest         *manifest.Manifest
    roleHierarchy    map[string][]string
    actorRoles       map[string][]string
    actorPermissions map[string][]manifest.Permission
    defaultDeny      bool
}

type Context struct {
    ActorID    string
    Resource   string
    Action     string
    Attributes map[string]interface{}
    Time       time.Time
}

type Result struct {
    Allowed   bool
    Denied    bool
    Reason    string
    Roles     []string
    Condition string
}
```

#### Funcionalidades

- **NewEngine(manifest)**: Cria engine a partir do manifesto
- **GetAllRoles(actorID)**: Retorna papéis com herança
- **CheckPermission(ctx)**: Verifica permissão básica
- **CheckAccess(ctx)**: Verifica permissão com condições contextuais
- **ValidateManifest(manifest)**: Valida configurações de segurança

#### Modelos de Autorização Suportados

- **RBAC** (Role-Based Access Control): Permissões baseadas em papéis
- **ABAC** (Attribute-Based Access Control): Condições baseadas em atributos
- **ACL** (Access Control List): Listas de controle de acesso

#### Recursos de Segurança

- **Hierarquia de Papéis**: Papéis podem herdar permissões de outros papéis
- **Condições Contextuais**: Expressões avaliadas em tempo de execução
- **Wildcards**: Suporte a `*` para recursos e ações
- **Default Deny/Allow**: Política padrão configurável
- **Detecção de Ciclos**: Validação de hierarquias circulares

## Exemplo de Uso

### 1. Carregar e Validar Manifesto

```go
import (
    "github.com/sipeed/picoclaw/pkg/manifest"
    "github.com/sipeed/picoclaw/pkg/governance/policy"
)

// Carregar manifesto
m, err := manifest.ParseFile("manifest.yaml")
if err != nil {
    log.Fatal(err)
}

// Validar manifesto
parser := &manifest.Parser{}
err = parser.Validate(m)
if err != nil {
    log.Fatal(err)
}

// Criar engine de políticas
engine, err := policy.NewEngine(m)
if err != nil {
    log.Fatal(err)
}

// Validar segurança
err = policy.ValidateManifest(m)
if err != nil {
    log.Fatal(err)
}
```

### 2. Verificar Permissões

```go
// Contexto de requisição
ctx := &policy.Context{
    ActorID:  "user123",
    Resource: "projects",
    Action:   "delete",
    Attributes: map[string]interface{}{
        "owner": "user456",
    },
    Time: time.Now(),
}

// Verificar permissão
result := engine.CheckPermission(ctx)
if !result.Allowed || result.Denied {
    fmt.Printf("Acesso negado: %s\n", result.Reason)
} else {
    fmt.Println("Acesso permitido")
}

// Verificar com condições contextuais
result = engine.CheckAccess(ctx)
```

### 3. Obter Informações de Papéis

```go
// Todos os papéis de um ator (com herança)
roles := engine.GetAllRoles("manager")
// Retorna: [manager, member, viewer]

// Listar recursos protegidos
resources := engine.ListResources()
```

## Arquivos de Exemplo

### `examples/manifests/task-management.yaml`

Manifesto completo de um sistema de gestão de tarefas com:
- 4 atores (admin, manager, member, viewer)
- 6 entidades (User, Project, Task, Comment, Team, TeamMember)
- 5 regras de negócio
- 1 API REST com 10 endpoints
- 2 canais (web, slack)
- Políticas de segurança completas

### `examples/manifests/test_manifest.go`

Programa de demonstração que:
1. Carrega o manifesto de exemplo
2. Valida a estrutura
3. Inicializa a engine de políticas
4. Executa testes de permissão
5. Demonstra serialização

## Executando os Testes

```bash
# Testar pacote manifest
go test ./pkg/manifest/... -v

# Testar pacote policy
go test ./pkg/governance/policy/... -v

# Executar demonstração
go run examples/manifests/test_manifest.go
```

## Próximas Etapas

### Etapa 2: Sistema de Migração Automática de Banco de Dados
- Gerar schemas SQL a partir do DataModel
- Criar migrations automáticas
- Suporte a múltiplos bancos (PostgreSQL, MySQL, SQLite)

### Etapa 3: Geração Dinâmica de APIs e Integrações
- Auto-generar handlers de API baseados nos endpoints
- Integrar com sistema de tools do PicoClaw
- Configurar automaticamente canais de comunicação

### Etapa 4: Refinamento do Comportamento do Agente
- Interpretador de regras de negócio determinísticas
- Hooks para interceptar operações do agente
- Auditoria obrigatória das ações

## Estrutura de Diretórios

```
/workspace/
├── pkg/
│   ├── manifest/
│   │   ├── manifest.go          # Estruturas e parser
│   │   └── manifest_test.go     # Testes unitários
│   └── governance/
│       └── policy/
│           ├── engine.go        # Engine de políticas
│           └── engine_test.go   # Testes unitários
└── examples/
    └── manifests/
        ├── task-management.yaml # Manifesto de exemplo
        └── test_manifest.go     # Programa de teste
```

## Formato do Manifesto

O manifesto usa YAML (ou JSON) com as seguintes seções principais:

```yaml
metadata:
  name: "SystemName"
  version: "1.0.0"
  description: "Descrição do sistema"

actors:
  - id: "actor_id"
    name: "Nome do Ator"
    roles: ["role1", "role2"]
    permissions:
      - resource: "resource_name"
        actions: ["read", "write"]
        condition: "owner == self"

data_model:
  entities:
    - name: "EntityName"
      fields:
        - name: "field_name"
          type: "string|int|float|bool|datetime|reference"
          required: true
          unique: false

business_rules:
  - id: "BR001"
    name: "Nome da Regra"
    trigger:
      event: "create|update|delete"
      entities: ["EntityName"]
    actions:
      - type: "validate|transform|notify|execute"

integrations:
  apis:
    - name: "API Name"
      base_path: "/api/v1"
      endpoints: [...]
  channels:
    - type: "web|telegram|slack"
      config: {...}

security:
  authentication:
    methods: ["jwt", "api_key"]
  authorization:
    model: "rbac"
    default_deny: true
    role_hierarchy: [...]
```

## Considerações de Segurança

1. **Default Deny**: Sempre configure `default_deny: true` para exigir permissões explícitas
2. **Princípio do Menor Privilégio**: Conceda apenas permissões necessárias
3. **Validação de Entrada**: Todas as entradas devem ser validadas contra o schema
4. **Auditoria**: Habilite logging de todas as operações sensíveis
5. **Campos Sensíveis**: Use masking e encryption para dados sensíveis

## Contribuição

Para contribuir com melhorias:
1. Adicione testes cobrindo novas funcionalidades
2. Mantenha a compatibilidade com manifestos existentes
3. Documente novas estruturas no README
4. Siga as convenções de código Go do projeto
