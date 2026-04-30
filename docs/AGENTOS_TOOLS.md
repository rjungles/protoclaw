# AgentOS Tools Integration

Conversational tools that let the PicoClaw agent create, manage, and query AgentOS systems through natural language.

---

## Architecture

```
User (Telegram / WhatsApp / CLI)
       │
       ▼
  PicoClaw Agent (LLM)
       │
       ▼
  Tool Registry
       │
       ├── agentos                  System lifecycle (init, bootstrap, serve, status, validate, migrate)
       ├── agentos_generate_manifest  Generate manifest from description
       └── agentos_query             Query entity data
              │
              ▼
       Security Layer
       ├── SystemNameValidator      Reject path traversal, reserved words
       ├── SystemPaths              Hash-based directory isolation
       ├── DBRegistry               SQLite registry (WAL mode)
       └── Audit Logger             Immutable operation trail
              │
              ▼
       AgentOS Systems
       └── Database / Manifests
```

---

## Available Tools

### 1. `agentos` — System Lifecycle

Manages AgentOS system operations. All actions validate system names, use isolated directories, and log to the audit trail.

**Actions:**

| Action | Description | Required Params |
|--------|-------------|-----------------|
| `init` | Initialize system from manifest | `system_name`, `manifest_path` |
| `bootstrap` | Create database schema | `system_name` |
| `serve` | Start/stop system emulation | `system_name` |
| `status` | List all systems | — |
| `validate` | Validate system health | `system_name` |
| `migrate` | Migrate system to secure storage | `system_name` |

**Parameters:**

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `action` | string | Yes | One of: init, bootstrap, serve, status, validate, list, migrate |
| `system_name` | string | Most | Validated against security rules |
| `manifest_path` | string | init | Path to YAML manifest |
| `data_dir` | string | No | Override default data directory |
| `user_id` | string | No | For audit logging (defaults to "anonymous") |

**Example conversation:**

```
User: Create a system from my manifest for a restaurant
Agent: [calls agentos with action=init, system_name=restaurant, manifest_path=/path/manifest.yaml]
→ System initialized at ~/.picoclaw/agentos/sys/a1b2c/restaurant/
→ Registered in SQLite registry
→ Audit event logged

User: Bootstrap it
Agent: [calls agentos with action=bootstrap, system_name=restaurant]
→ Database created, status updated to "bootstrapped"

User: What systems do I have?
Agent: [calls agentos with action=status]
→ restaurant (bootstrapped) at sys/a1b2c/restaurant/
```

### 2. `agentos_generate_manifest` — Manifest Generation

Generates an AgentOS system manifest YAML from a natural language description.

**Parameters:**

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `description` | string | Yes | Natural language system description |
| `system_name` | string | Yes | Name for the system (validated) |
| `output_path` | string | No | Where to save the manifest |

**Entity extraction keywords:**

| Keywords | Entity |
|----------|--------|
| customer, client | Customer |
| vehicle, car | Vehicle |
| order, sale | Order |
| product | Product |
| appointment, booking | Appointment |
| employee, staff | Employee |
| menu, reservation, table | Menu/Reservation |

### 3. `agentos_query` — Data Queries

Queries entity data from a running system.

**Parameters:**

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `system_name` | string | Yes | System to query (validated) |
| `entity` | string | Yes | Entity name (e.g., Customer, Vehicle) |
| `limit` | int | No | Max results (default 10) |

---

## Security Integration

All three tools enforce security through the `ExecAgentOSTool` struct:

```go
type ExecAgentOSTool struct {
    dataDir   string
    validator *validation.SystemNameValidator  // Always present
}
```

**Every operation follows this pattern:**

1. **Validate** system name via `validator.Validate()` or `validator.ValidateAndSanitize()`
2. **Resolve** paths via `storage.NewSystemPaths()` (hash-isolated)
3. **Register/lookup** in `registry.NewDBRegistry()` (SQLite)
4. **Log** to audit trail via `logAuditEvent()`

---

## Configuration

### Enable AgentOS Tools

```yaml
# config.yaml
tools:
  agentos:
    enabled: true
```

### Data Directory

```bash
# Default: ~/.picoclaw/agentos/
export AGENTOS_DATA_DIR=/path/to/custom/data
```

---

## CLI Commands

```bash
# Provider configuration
picoclaw agentos configure-provider openai
picoclaw agentos configure-provider groq

# Key management
picoclaw agentos show-keys
picoclaw agentos delete-key <key-name>
picoclaw agentos rotate-key <key-name>

# System lifecycle
picoclaw agentos init --name my-system
picoclaw agentos bootstrap --system my-system
picoclaw agentos status
```

---

## File Structure

```
pkg/tools/
├── agentos.go                          # Tool implementations (security-integrated)
├── agentos_secure.go                   # SecureAgentOSManager (unified API)
├── agentos_test.go                     # Unit tests
├── agentos_integration_test.go         # Integration tests
└── agentos_integration_secure_test.go  # Security integration tests

cmd/picoclaw/internal/agentos/commands/
├── root.go                             # Command dispatcher
└── configure.go                        # Provider config + key management
```

---

## Testing

```bash
# Unit tests
go test ./pkg/tools -v -run AgentOS

# Integration tests
go test ./pkg/tools -v -run "TestAgentOSWorkflow|TestAgentOSConversationFlow"

# Security integration tests
go test ./pkg/tools -v -run TestSecureAgentOS
```

---

## Extending

### Adding a New Action

Add a case in `ExecAgentOSTool.Execute()`:

```go
case "new_action":
    return t.executeNewAction(ctx, args, dataDir, userID)

func (t *ExecAgentOSTool) executeNewAction(ctx context.Context, args map[string]any, dataDir, userID string) *ToolResult {
    systemName, _ := args["system_name"].(string)
    validatedName, err := t.validator.Validate(systemName)
    if err != nil {
        return ErrorResult(fmt.Sprintf("invalid system name: %v", err))
    }
    // ... implementation ...
    logAuditEvent(dataDir, audit.OpCustom, systemID, userID, details)
    return UserResult("Success!")
}
```

---

## See Also

- [AgentOS Security Architecture](./AGENTOS_SECURITY_IMPLEMENTATION.md)
- [AgentOS Migration Guide](./AGENTOS_SECURITY_MIGRATION.md)
- [LLM Integration](./LLM.md)
- [Installation Tutorial](./TUTORIAL-INSTALACAO-AGENTOS.md)
