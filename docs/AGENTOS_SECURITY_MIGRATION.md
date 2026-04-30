# AgentOS Migration Guide

Migrating from pre-security AgentOS to the hardened version.

---

## What Changed

| Aspect | Before | After |
|--------|--------|-------|
| System names | Unvalidated | Regex + reserved words + sanitization |
| Storage | Flat dirs: `~/.picoclaw/agentos/<name>/` | Hash-isolated: `~/.picoclaw/agentos/sys/<hash>/<name>/` |
| Registry | YAML file (`registry.yaml`) | SQLite WAL (`registry.db`) |
| API keys | Plaintext env vars | AES-256-GCM encrypted keystore |
| Audit | None | Immutable SQLite trail |
| LLM calls | Direct HTTP, no protection | Connection pool + circuit breaker |
| Long ops | Synchronous/blocking | Async job queue |
| Monitoring | None | Background health checks |

---

## Automatic Migration

Migration is **automatic and transparent**. When you access an existing system:

1. `storage.FindSystem()` detects the old flat directory
2. Migrates it to `sys/<hash>/<name>/`
3. Registry is created in SQLite on first use
4. Operations continue normally

```bash
# No special command needed — just use the system as before
picoclaw agentos status
```

---

## Manual Migration

### 1. Backup First

```bash
cp -r ~/.picoclaw/agentos ~/.picoclaw/agentos.backup.$(date +%Y%m%d)
```

### 2. Migrate API Keys

```bash
# Interactive: prompts for each provider
picoclaw agentos configure-provider openai
picoclaw agentos configure-provider groq
picoclaw agentos configure-provider anthropic

# The CLI detects existing env vars and offers migration
```

### 3. Verify

```bash
picoclaw agentos status          # Systems listed from SQLite registry
picoclaw agentos show-keys       # Keys listed from encrypted keystore
```

---

## New Directory Structure

```
~/.picoclaw/agentos/
├── registry.db          # SQLite registry (new)
├── audit.db             # Audit log (new)
├── jobs.db              # Job queue (new)
├── .keys.db             # Encrypted keystore (new)
├── .master.key          # AES-256 master key, 0600 (new)
├── health.db            # Health check history (new)
└── sys/                 # Hash-isolated system dirs (new)
    ├── a1b2c/
    │   └── my-system/
    │       ├── system.yaml
    │       ├── .serving
    │       ├── config/llm/llm.yaml
    │       └── data/data.db
    └── d3e4f/
        └── other-system/
```

---

## System Name Validation

Names must match `^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`.

| Input | Result | Reason |
|-------|--------|--------|
| `my-system` | Valid | - |
| `my_system` | Valid | - |
| `MySystem` | Valid | - |
| `123system` | Invalid | Starts with number |
| `../../../etc` | Invalid | Path traversal |
| `con` | Invalid | Reserved word |
| `.hidden` | Invalid | Starts with dot |
| `my system` | Sanitized → `my_system` | Space replaced |

---

## Secure Provider Configuration

### Before (Insecure)

```bash
# .env file — visible via /proc/*/environ, ps e
OPENAI_API_KEY=sk-...
```

### After (Secure)

```bash
# Interactive CLI — key encrypted at rest
picoclaw agentos configure-provider openai
# Enter API key: ********
# Key stored securely in keystore.

# Or programmatically
manager, _ := tools.NewSecureAgentOSManager(dataDir)
manager.StoreAPIKey("llm.provider.openai.api_key", apiKey, "Production")
```

---

## Audit Log

All operations are logged immutably. Query via Go API or export.

```go
// Go API
logger, _ := audit.NewLogger("audit.db")
events, _ := logger.Query(ctx, audit.Filter{
    SystemID:  "my-system",
    Operation: "system_created",
    Limit:     100,
})

// Export
data, _ := logger.Export(ctx, audit.Filter{SystemID: "my-system"})
os.WriteFile("audit-export.json", data, 0644)
```

---

## Rollback

```bash
rm -rf ~/.picoclaw/agentos
cp -r ~/.picoclaw/agentos.backup.20260429 ~/.picoclaw/agentos
```

Note: Systems migrated to the new directory structure cannot be automatically un-migrated.

---

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| "system exists in both locations" | Old and new dirs both present | Remove one: `rm -rf ~/.picoclaw/agentos/my-system` (old) or `rm -rf ~/.picoclaw/agentos/sys/<hash>/my-system` (new) |
| "keystore locked" | Permission issue | `chmod 600 ~/.picoclaw/agentos/.keys.db` |
| "registry migration failed" | Corrupt YAML | `rm ~/.picoclaw/agentos/registry.yaml && picoclaw agentos init` |
| "name validation failed" | Invalid characters | Use `ValidateAndSanitize()` or pick a valid name |

---

## References

- [AgentOS Security Architecture](./AGENTOS_SECURITY_IMPLEMENTATION.md)
- [AgentOS Tools](./AGENTOS_TOOLS.md)
- [LLM Integration](./LLM.md)
- [OWASP Path Traversal](https://owasp.org/www-community/attacks/Path_Traversal)
- [SQLite WAL Mode](https://sqlite.org/wal.html)
- [Circuit Breaker Pattern](https://martinfowler.com/bliki/CircuitBreaker.html)
