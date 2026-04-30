# Tutorial de Instalação do PicoClaw com AgentOS

Guia completo de instalação e configuração do PicoClaw com os recursos avançados do **AgentOS** (Etapas 1, 6, 7, 8 e 9 implementadas).

---

## 📋 Sumário

1. [Pré-requisitos](#pré-requisitos)
2. [Instalação Rápida](#instalação-rápida)
3. [Instalação Completa do AgentOS](#instalação-completa-do-agentos)
4. [Configuração do Manifesto](#configuração-do-manifesto)
5. [Bootstrap do Sistema](#bootstrap-do-sistema)
6. [Execução via API REST](#execução-via-api-rest)
7. [Execução via MCP Server](#execução-via-mcp-server)
8. [Gestão de Workflows](#gestão-de-workflows)
9. [Evolução do Sistema](#evolução-do-sistema)
10. [Troubleshooting](#troubleshooting)

---

## Pré-requisitos

### Requisitos de Hardware

| Recurso | Mínimo | Recomendado |
|---------|--------|-------------|
| CPU | 0.6 GHz (single-core) | 1+ cores |
| RAM | 64 MB | 256 MB |
| Disco | 100 MB | 1 GB |
| Arquitetura | x86_64, ARM64, MIPS, RISC-V, LoongArch | x86_64 ou ARM64 |

### Requisitos de Software

- **Go** 1.25.7 ou superior
- **Git** (para clonar o repositório)
- **Banco de Dados** (um dos seguintes):
  - SQLite 3 (recomendado para desenvolvimento)
  - PostgreSQL 13+
  - MySQL 8+

### Sistemas Operacionais Suportados

- Linux (todas as distribuições)
- macOS 10.15+
- Windows 10/11 (via WSL ou nativo)
- Android (via Termux ou APK)

---

## Instalação Rápida

### Opção 1: Download do Binário (Recomendado)

```bash
# Linux/macOS
curl -fsSL https://picoclaw.io/install.sh | bash

# Ou baixe manualmente em https://picoclaw.io/download
```

### Opção 2: Compilação do Código Fonte

```bash
# 1. Clone o repositório
git clone https://github.com/sipeed/picoclaw.git
cd picoclaw

# 2. Verifique a versão do Go
go version  # Deve ser >= 1.25.7

# 3. Compile o binário
make build

# Ou compile manualmente:
CGO_ENABLED=0 go build -tags goolm,stdjson -ldflags="-s -w" -o picoclaw ./cmd/picoclaw

# 4. Instale (opcional)
sudo make install
# ou
sudo cp picoclaw /usr/local/bin/
```

### Verificação da Instalação

```bash
picoclaw --version
# Saída esperada: picoclaw v0.2.4 (ou superior)

picoclaw --help
# Deve exibir ajuda com comandos disponíveis
```

---

## Instalação Completa do AgentOS

O AgentOS adiciona capacidades avançadas ao PicoClaw. Siga os passos abaixo para habilitar todos os recursos.

### 1. Instalar Dependências Adicionais

```bash
# SQLite (driver CGO-free - já incluído)
# Não requer instalação adicional

# Para PostgreSQL (opcional)
# Ubuntu/Debian
sudo apt-get install postgresql postgresql-contrib

# Para MySQL (opcional)
# Ubuntu/Debian
sudo apt-get install mysql-server mysql-client
```

### 2. Estrutura de Diretórios do AgentOS

```bash
# Crie a estrutura de dados
mkdir -p ~/picoclaw-data/{manifests,db,logs,backups}

# Diretórios:
# ~/picoclaw-data/manifests/  -> Arquivos YAML/JSON de manifestos
# ~/picoclaw-data/db/         -> Banco de dados SQLite
# ~/picoclaw-data/logs/       -> Logs de auditoria
# ~/picoclaw-data/backups/    -> Backups automáticos
```

### 3. Configuração do PicoClaw com AgentOS

```bash
# Inicialize o modo AgentOS
picoclaw agentos init --data-dir ~/picoclaw-data

# Ou configure manualmente criando o arquivo:
# ~/.config/picoclaw/config.yaml
```

**Exemplo de `config.yaml`:**

```yaml
# Configuração do PicoClaw com AgentOS
mode: agentos

agentos:
  # Manifesto atual do sistema
  manifest_path: ~/picoclaw-data/manifests/system.yaml
  
  # Configuração do banco de dados
  database:
    driver: sqlite  # ou postgres, mysql
    connection: ~/picoclaw-data/db/agentos.db
    # Para PostgreSQL:
    # connection: "postgres://user:password@localhost:5432/picoclaw?sslmode=disable"
    # Para MySQL:
    # connection: "user:password@tcp(localhost:3306)/picoclaw?parseTime=true"
  
  # Diretório de dados
  data_dir: ~/picoclaw-data
  
  # Evolução automática (Etapa 9)
  auto_evolve: true
  
  # Serviços habilitados
  enable_api: true
  enable_mcp: true
  enable_workflows: true

# Configuração da API REST
api:
  host: 0.0.0.0
  port: 8080
  base_path: /api/v1

# Configuração do MCP Server (Etapa 7)
mcp:
  enabled: true
  name: "PicoClaw AgentOS"
  version: "1.0.0"

# Configuração de logging
logging:
  level: info
  file: ~/picoclaw-data/logs/picoclaw.log
  audit: true  # Habilita auditoria de operações
```

---

## Configuração do Manifesto

O manifesto é o coração do AgentOS. Ele define toda a infraestrutura do seu sistema.

### 1. Manifesto Mínimo (Hello World)

Crie o arquivo `~/picoclaw-data/manifests/hello-world.yaml`:

```yaml
# Manifesto mínimo do AgentOS
metadata:
  name: "hello-world-system"
  version: "1.0.0"
  description: "Sistema de exemplo"

# Atores do sistema
actors:
  - id: "admin"
    name: "Administrador"
    roles: ["admin"]
    permissions:
      - resource: "*"
        actions: ["*"]

# Modelo de dados
data_model:
  entities:
    - name: "Message"
      fields:
        - name: "id"
          type: "string"
          required: true
          unique: true
        - name: "content"
          type: "string"
          required: true
        - name: "created_at"
          type: "datetime"
          required: true

# Integrações
integrations:
  apis:
    - name: "Hello API"
      base_path: "/api/v1"
      endpoints:
        - path: "/messages"
          method: "GET"
          handler: "Message.list"
        - path: "/messages"
          method: "POST"
          handler: "Message.create"
```

### 2. Manifesto Completo (Experience Platform)

Copie o manifesto de exemplo:

```bash
# Copie o manifesto de experience platform
cp examples/manifests/experience-platform.yaml ~/picoclaw-data/manifests/

# Ou use o de cafeteria loyalty
cp examples/manifests/cafeteria-loyalty.yaml ~/picoclaw-data/manifests/
```

**Estrutura do manifesto `experience-platform.yaml`:**

```yaml
metadata:
  name: "experience-platform"
  version: "1.0.0"
  description: "Plataforma de experiências editoriais"

# Atores: Author, Editor, Reviewer, Admin
actors:
  - id: "author"
    name: "Autor"
    roles: ["author"]
    permissions:
      - resource: "experience"
        actions: ["create", "read", "update"]
        condition: "author_id==self"
      - resource: "experience"
        actions: ["transition.submit_review", "transition.resubmit"]
  
  - id: "editor"
    name: "Editor"
    roles: ["editor"]
    permissions:
      - resource: "experience"
        actions: ["read", "transition.approve", "transition.request_clarification"]

# Modelo de dados
# Entity: Experience com campos id, title, content, author_id, state
data_model:
  entities:
    - name: "Experience"
      fields:
        - name: "id"
          type: "string"
          required: true
          unique: true
        - name: "title"
          type: "string"
          required: true
        - name: "content"
          type: "string"
          required: false
        - name: "author_id"
          type: "string"
          required: true
        - name: "editor_id"
          type: "string"
          required: false
        - name: "state"
          type: "string"
          required: true

# Workflow complexo (Etapa 8)
# Estados: DRAFT → UNDER_REVIEW → CLARIFICATION/APPROVED → PUBLISHED → ARCHIVED
workflows:
  - entity: "Experience"
    initial_state: "DRAFT"
    states:
      - id: "DRAFT"
        transitions:
          - to: "UNDER_REVIEW"
            action: "submit_review"
            allowed_roles: ["author"]
            guards:
              - field: "content"
                condition: "not_empty"
      
      - id: "UNDER_REVIEW"
        transitions:
          - to: "DRAFT"
            action: "request_clarification"
            allowed_roles: ["editor"]
          - to: "APPROVED"
            action: "approve"
            allowed_roles: ["editor"]
        timeout:
          duration: "7d"
          transition_to: "DRAFT"
          action: "auto_return"
      
      - id: "APPROVED"
        transitions:
          - to: "PUBLISHED"
            action: "publish"
            allowed_roles: ["admin"]
```

---

## Bootstrap do Sistema

O Bootstrap é o processo que materializa o sistema descrito no manifesto.

### 1. Executar Bootstrap

```bash
# Bootstrap completo
picoclaw agentos bootstrap \
  --manifest ~/picoclaw-data/manifests/experience-platform.yaml \
  --data-dir ~/picoclaw-data \
  --db-driver sqlite \
  --db-connection ~/picoclaw-data/db/system.db

# Ou use o modo interativo
picoclaw agentos bootstrap --interactive
```

### 2. Pipeline de Bootstrap (12 Passos)

```
[1/12] Load Manifest       ✓ Parse e validação
[2/12] Validate            ✓ Consistência verificada
[3/12] Open Database       ✓ Conectado: system.db
[4/12] Run Migrations      ✓ 12 tabelas criadas
[5/12] Provision Actors    ✓ 4 atores provisionados
[6/12] Build Catalog       ✓ 47 operações registradas
[7/12] Create PolicyEng    ✓ RBAC/ABAC ativo
[8/12] Create RuleExec     ✓ Regras carregadas
[9/12] Create FSMs         ✓ 1 workflow configurado
[10/12] Create APIGen      ✓ Rotas HTTP geradas
[11/12] Mount HTTP Mux     ✓ Middleware configurado
[12/12] System Ready       ✓ Aguardando conexões
```

### 3. Verificar Status do Sistema

```bash
# Verificar status
curl http://localhost:8080/_system/health

# Resposta esperada:
{
  "status": "healthy",
  "version": "1.0.0",
  "manifest": "experience-platform",
  "actors": 4,
  "entities": 1,
  "operations": 47,
  "database": "connected",
  "workflows": 1
}
```

### 4. Listar Operações Disponíveis

```bash
# Listar todas as operações
curl http://localhost:8080/_system/operations | jq

# Operações geradas automaticamente:
# - experience.list, experience.get, experience.create, experience.update, experience.delete
# - experience.transition.submit_review
# - experience.transition.request_clarification
# - experience.transition.approve
# - experience.transition.publish
```

---

## Execução via API REST

A API REST expõe todas as operações definidas no manifesto.

### 1. Autenticação

```bash
# Configurar header de autenticação
export API_KEY="actor-api-key-here"
# Ou use X-Actor-ID para desenvolvimento
export ACTOR_ID="author"
```

### 2. Operações CRUD

```bash
# Criar uma experiência (autor)
curl -X POST http://localhost:8080/api/v1/experiences \
  -H "Content-Type: application/json" \
  -H "X-Actor-ID: author" \
  -d '{
    "id": "exp-001",
    "title": "Minha Primeira Experiência",
    "content": "Conteúdo inicial...",
    "author_id": "author"
  }'

# Listar experiências (com filtro de permissão)
curl http://localhost:8080/api/v1/experiences \
  -H "X-Actor-ID: author"

# Atualizar experiência
curl -X PUT http://localhost:8080/api/v1/experiences/exp-001 \
  -H "Content-Type: application/json" \
  -H "X-Actor-ID: author" \
  -d '{
    "title": "Experiência Atualizada",
    "content": "Novo conteúdo..."
  }'
```

### 3. Transições de Workflow

```bash
# 1. Autor submete para revisão (DRAFT → UNDER_REVIEW)
curl -X POST http://localhost:8080/api/v1/experiences/exp-001/actions/submit_review \
  -H "X-Actor-ID: author"

# Verificar estado
curl http://localhost:8080/api/v1/experiences/exp-001

# 2. Editor aprova (UNDER_REVIEW → APPROVED)
curl -X POST http://localhost:8080/api/v1/experiences/exp-001/actions/approve \
  -H "X-Actor-ID: editor"

# 3. Admin publica (APPROVED → PUBLISHED)
curl -X POST http://localhost:8080/api/v1/experiences/exp-001/actions/publish \
  -H "X-Actor-ID: admin"

# Verificar histórico de transições
curl http://localhost:8080/api/v1/experiences/exp-001/history
```

### 4. Verificar Ações Disponíveis

```bash
# Quais ações o autor pode executar?
curl http://localhost:8080/api/v1/experiences/exp-001/actions \
  -H "X-Actor-ID: author"

# Resposta depende do estado atual e permissões
{
  "state": "DRAFT",
  "available_actions": ["submit_review"],
  "allowed": true
}
```

---

## Execução via MCP Server

O MCP Server expõe as mesmas operações via protocolo MCP para integração com LLMs.

### 1. Iniciar MCP Server

```bash
# MCP via stdio (para uso com Claude Desktop)
picoclaw agentos mcp --stdio

# Ou MCP via SSE (Server-Sent Events)
picoclaw agentos mcp --sse --port 8081
```

### 2. Configurar Claude Desktop

Edite `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) ou equivalente:

```json
{
  "mcpServers": {
    "picoclaw-agentos": {
      "command": "/usr/local/bin/picoclaw",
      "args": [
        "agentos",
        "mcp",
        "--stdio",
        "--manifest",
        "/home/user/picoclaw-data/manifests/experience-platform.yaml"
      ],
      "env": {
        "PICOCLAW_DATA_DIR": "/home/user/picoclaw-data"
      }
    }
  }
}
```

### 3. Tools MCP Disponíveis

O MCP Server expõe automaticamente:

```
# Tools CRUD
- experience.list      → Lista experiências
- experience.get       → Obtém experiência por ID
- experience.create    → Cria nova experiência
- experience.update    → Atualiza experiência
- experience.delete    → Remove experiência

# Tools de Workflow
- experience.transition.submit_review
- experience.transition.request_clarification
- experience.transition.approve
- experience.transition.publish
- experience.transition.archive

# Tools de Sistema
- system.health        → Verifica saúde do sistema
- system.operations    → Lista operações disponíveis
- system.actors        → Lista atores
```

### 4. Exemplo de Uso com Claude

```
Usuário: Crie uma nova experiência chamada "Tutorial AgentOS"

Claude: Vou criar a experiência para você.
[Chama tool: experience.create]

✓ Experiência criada com sucesso!
- ID: exp-002
- Título: "Tutorial AgentOS"
- Estado: DRAFT
- Autor: author

Usuário: Submeta para revisão

Claude: 
[Chama tool: experience.transition.submit_review]

✓ Experiência submetida para revisão!
- Estado anterior: DRAFT
- Novo estado: UNDER_REVIEW
- Ação: submit_review
```

### 5. Paridade API/MCP

| Operação | API REST | MCP Tool |
|----------|----------|----------|
| Listar | `GET /experiences` | `experience.list` |
| Criar | `POST /experiences` | `experience.create` |
| Workflow | `POST /{id}/actions/{action}` | `experience.transition.{action}` |
| Permissões | Headers | Context |

---

## Gestão de Workflows

O AgentOS suporta workflows complexos com estados, guards e side effects.

### 1. Estados e Transições

```bash
# Visualizar workflow
curl http://localhost:8080/_system/workflows/experience

{
  "entity": "Experience",
  "initial_state": "DRAFT",
  "states": [
    {
      "id": "DRAFT",
      "transitions": [
        {"action": "submit_review", "to": "UNDER_REVIEW", "allowed_roles": ["author"]}
      ]
    },
    {
      "id": "UNDER_REVIEW",
      "transitions": [
        {"action": "request_clarification", "to": "DRAFT"},
        {"action": "approve", "to": "APPROVED"}
      ],
      "timeout": {"duration": "7d", "transition_to": "DRAFT"}
    }
  ]
}
```

### 2. Guards (Validações)

Guards impedem transições se condições não forem satisfeitas:

```bash
# Tentar submeter sem conteúdo (deve falhar)
curl -X POST http://localhost:8080/api/v1/experiences/exp-003/actions/submit_review \
  -H "X-Actor-ID: author"

# Resposta:
{
  "error": "Guard validation failed",
  "reason": "Field 'content' cannot be empty",
  "guard": "content_not_empty"
}
```

### 3. Side Effects

Side effects são ações automáticas nas transições:

```yaml
# No manifesto:
transitions:
  - to: "UNDER_REVIEW"
    action: "submit_review"
    on_enter:
      - type: "notify"
        target: "editor"
        message: "Nova experiência para revisão"
      - type: "update_field"
        field: "submitted_at"
        value: "{{now}}"
```

### 4. Histórico de Transições

```bash
# Obter histórico completo
curl http://localhost:8080/api/v1/experiences/exp-001/history

{
  "transitions": [
    {
      "id": "trans-001",
      "from": "DRAFT",
      "to": "UNDER_REVIEW",
      "action": "submit_review",
      "actor_id": "author",
      "timestamp": "2026-04-19T10:30:00Z"
    },
    {
      "id": "trans-002",
      "from": "UNDER_REVIEW",
      "to": "APPROVED",
      "action": "approve",
      "actor_id": "editor",
      "timestamp": "2026-04-19T14:15:00Z"
    }
  ]
}
```

---

## Evolução do Sistema

O AgentOS permite evoluir o manifesto sem perder dados (Etapa 9).

### 1. Modificar o Manifesto

```bash
# Copie para uma nova versão
cp ~/picoclaw-data/manifests/experience-platform.yaml \
   ~/picoclaw-data/manifests/experience-platform-v2.yaml

# Edite o v2 (adicione nova entidade, campo, etc.)
nano ~/picoclaw-data/manifests/experience-platform-v2.yaml
```

**Exemplo de mudança (v2):**

```yaml
metadata:
  name: "experience-platform"
  version: "2.0.0"  # Nova versão

data_model:
  entities:
    - name: "Experience"
      fields:
        # ... campos existentes ...
        - name: "category"  # NOVO CAMPO
          type: "string"
          required: false
    
    - name: "Category"  # NOVA ENTIDADE
      fields:
        - name: "id"
          type: "string"
          required: true
        - name: "name"
          type: "string"
          required: true
```

### 2. Detectar Mudanças

```bash
# Analisar diferenças
picoclaw agentos diff \
  --from ~/picoclaw-data/manifests/experience-platform.yaml \
  --to ~/picoclaw-data/manifests/experience-platform-v2.yaml

# Saída:
Detectando mudanças...
============================
Versão: 1.0.0 → 2.0.0

Mudanças Seguras:
  ✓ ADD_ENTITY: Category
  ✓ ADD_COLUMN: Experience.category

Mudanças de Revisão:
  - None

Mudanças Quebradoras:
  - None

Pode aplicar automaticamente: SIM
```

### 3. Aplicar Evolução

```bash
# Modo interativo (recomendado)
picoclaw agentos evolve \
  --to ~/picoclaw-data/manifests/experience-platform-v2.yaml \
  --interactive

# Ou modo automático
picoclaw agentos evolve \
  --to ~/picoclaw-data/manifests/experience-platform-v2.yaml \
  --auto

# Pipeline de evolução:
[1/5] Compare Manifests      ✓ 2 mudanças detectadas
[2/5] Create Plan            ✓ Plano gerado (2 steps)
[3/5] Backup Database        ✓ Backup: backups/2026-04-19-001.db
[4/5] Execute Safe Steps     ✓ Entity Category created
                               ✓ Column category added
[5/5] Update System          ✓ Sistema atualizado para v2.0.0
```

### 4. Estratégias de Proteção

```bash
# Se remover um campo (mudança breaking):
# O sistema renomeia em vez de deletar:
#   Campo "old_field" → "_deprecated_old_field"

# Se remover uma entidade:
# A tabela é renomeada:
#   Tabela "OldEntity" → "_archived_OldEntity"

# Se remover um ator:
# O ator é desativado:
#   is_active: true → false
```

### 5. Rollback

```bash
# Listar versões
picoclaw agentos versions

# Rollback para versão anterior
picoclaw agentos rollback --to-version 1.0.0
```

---

## Troubleshooting

### Problema: "Failed to open database: sql: unknown driver"

```bash
# Causa: Driver SQLite não encontrado
# Solução: O AgentOS usa modernc.org/sqlite (CGO-free)
# Não requer instalação adicional

# Verifique o go.mod:
grep "modernc.org/sqlite" go.mod

# Se não estiver presente, instale:
go get modernc.org/sqlite
```

### Problema: "Permission denied" ao executar transições

```bash
# Verifique as permissões do ator
curl http://localhost:8080/_system/actors \
  -H "X-Actor-ID: admin"

# Verifique as roles do ator
# Certifique-se de que o ator tem a role necessária para a transição
```

### Problema: Bootstrap falha na migração

```bash
# Limpe o banco e tente novamente
rm ~/picoclaw-data/db/*.db

# Ou use modo debug
picoclaw agentos bootstrap --verbose --skip-migrate=false
```

### Problema: MCP Server não conecta

```bash
# Verifique se o MCP está habilitado
picoclaw agentos config get mcp.enabled

# Verifique logs
picoclaw agentos logs --follow

# Teste o MCP manualmente
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | \
  picoclaw agentos mcp --stdio
```

### Problema: Guard validation falha inesperadamente

```bash
# Verifique o estado atual
curl http://localhost:8080/api/v1/experiences/{id}

# Liste transições disponíveis
curl http://localhost:8080/api/v1/experiences/{id}/actions \
  -H "X-Actor-ID: {actor}"

# Verifique se o campo obrigatório existe
```

---

## Referência Rápida

### Comandos Principais

```bash
# Instalação
make build                    # Compilar
sudo make install             # Instalar

# Configuração
picoclaw agentos init         # Inicializar estrutura
picoclaw agentos config       # Gerenciar configuração

# Bootstrap e Execução
picoclaw agentos bootstrap    # Materializar sistema
picoclaw agentos serve        # Iniciar servidor

# MCP
picoclaw agentos mcp --stdio  # Modo stdio
picoclaw agentos mcp --sse    # Modo SSE

# Evolução
picoclaw agentos diff         # Comparar manifestos
picoclaw agentos evolve       # Aplicar evolução
picoclaw agentos versions     # Listar versões
picoclaw agentos rollback     # Reverter versão

# Utilidades
picoclaw agentos validate     # Validar manifesto
picoclaw agentos status       # Status do sistema
picoclaw agentos logs         # Ver logs
```

### Variáveis de Ambiente

```bash
PICOCLAW_CONFIG_PATH          # Caminho do config.yaml
PICOCLAW_DATA_DIR             # Diretório de dados
PICOCLAW_LOG_LEVEL            # debug, info, warn, error
PICOCLAW_MANIFEST_PATH        # Manifesto padrão
PICOCLAW_DATABASE_DRIVER      # sqlite, postgres, mysql
PICOCLAW_DATABASE_CONNECTION  # Connection string
```

### Portas Padrão

| Serviço | Porta | Descrição |
|---------|-------|-----------|
| API REST | 8080 | Endpoints HTTP |
| MCP SSE | 8081 | Server-Sent Events |
| Health | 8080/_system/health | Health check |

---

## Recursos Adicionais

- [AgentOS Security Architecture](./AGENTOS_SECURITY_IMPLEMENTATION.md)
- [AgentOS Migration Guide](./AGENTOS_SECURITY_MIGRATION.md)
- [AgentOS Tools](./AGENTOS_TOOLS.md)
- [Exemplos de Manifestos](../examples/manifests/)
- [API Reference](https://docs.picoclaw.io/)
- [Comunidade Discord](https://discord.gg/V4sAZ9XWpN)

---

**Última atualização:** Abril 2026  
**Versão do AgentOS:** 1.0.0
