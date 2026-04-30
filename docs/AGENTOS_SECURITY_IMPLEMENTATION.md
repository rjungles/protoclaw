# AgentOS Security Architecture

## Overview

AgentOS is PicoClaw's subsystem for declarative system creation and management. This document covers the security layer that hardens all AgentOS operations.

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     CLI / Chat Tools                      │
│  (commands/root.go, agentos.go, agentos_secure.go)       │
└──────────────┬───────────────────────────────┬───────────┘
               │                               │
               ▼                               ▼
┌──────────────────────────┐   ┌───────────────────────────┐
│   ExecAgentOSTool        │   │  SecureAgentOSManager     │
│   (pkg/tools/agentos.go) │   │  (pkg/tools/              │
│   Validation + Audit     │   │   agentos_secure.go)      │
│   SQLite Registry        │   │  Full security stack      │
└──────────┬───────────────┘   └──────────┬────────────────┘
           │                               │
           ▼                               ▼
┌──────────────────────────────────────────────────────────┐
│                    Security Layer                         │
│                                                          │
│  ┌─────────────────┐  ┌──────────────┐  ┌─────────────┐ │
│  │ SystemName      │  │ KeyStore     │  │ Encryptor   │ │
│  │ Validator       │  │ (SQLite)     │  │ AES-256-GCM │ │
│  │ (validation/)   │  │ (security/)  │  │ (security/) │ │
│  └─────────────────┘  └──────────────┘  └─────────────┘ │
│                                                          │
│  ┌─────────────────┐  ┌──────────────┐  ┌─────────────┐ │
│  │ Storage Paths   │  │ Audit Logger │  │ Registry    │ │
│  │ (Hash-isolated) │  │ (Immutable)  │  │ (SQLite WAL)│ │
│  │ (storage/)      │  │ (audit/)     │  │ (registry/) │ │
│  └─────────────────┘  └──────────────┘  └─────────────┘ │
└──────────────────────────────────────────────────────────┘
           │                               │
           ▼                               ▼
┌──────────────────────────────────────────────────────────┐
│                 Reliability Layer                         │
│                                                          │
│  ┌─────────────────┐  ┌──────────────┐  ┌─────────────┐ │
│  │ Connection Pool │  │ Circuit      │  │ Job Queue   │ │
│  │ (HTTP shared)   │  │ Breaker      │  │ (SQLite)    │ │
│  │ (connection/)   │  │ (3-state)    │  │ (jobs/)     │ │
│  └─────────────────┘  └──────────────┘  └─────────────┘ │
│                                                          │
│  ┌─────────────────┐                                    │
│  │ Health Checker  │                                    │
│  │ (Background)    │                                    │
│  │ (health/)       │                                    │
│  └─────────────────┘                                    │
└──────────────────────────────────────────────────────────┘
```

---

## Components

### 1. System Name Validation

**Path:** `pkg/agentos/security/validation/system.go`

Prevents path traversal, null byte injection, and reserved word attacks on system names.

**Rules:**
- Regex: `^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`
- Rejects: path separators (`/`, `\`), `..`, `~`, null bytes
- Blocks reserved words: `aux`, `con`, `nul`, `prn`, `com1-9`, `lpt1-9`, `.`, `..`
- Auto-sanitizes invalid names (e.g., `"My System!"` → `"my_system"`)

```go
validator := validation.NewSystemNameValidator()
err := validator.Validate("../../../etc/passwd")  // Error!
name, err := validator.ValidateAndSanitize("My System!")  // "my_system"
```

### 2. Hash-Based Directory Isolation

**Path:** `pkg/agentos/storage/paths.go`

Each system gets its own isolated directory using a SHA-256 hash prefix to prevent name collisions and path traversal.

**Structure:**
```
~/.picoclaw/agentos/sys/<5-char-hash>/<system-name>/
├── system.yaml
├── .serving
├── config/
│   └── llm/
│       └── llm.yaml
├── data/
│   └── data.db
└── ...
```

- Hash: first 5 hex chars of SHA-256(system name) → ~1M unique buckets
- Auto-migration: `FindSystem()` detects legacy flat dirs and moves them
- `EnsureDirectories()` creates the full tree

```go
paths := storage.NewSystemPaths(dataDir, "my-system")
paths.EnsureDirectories()
paths.Root()   // /data/sys/a1b2c/my-system
paths.DB()     // /data/sys/a1b2c/my-system/data/data.db
```

### 3. Encrypted Keystore

**Path:** `pkg/agentos/security/keystore.go`, `encryption.go`

AES-256-GCM encrypted key storage backed by SQLite. Replaces plaintext environment variables.

**Properties:**
- AES-256-GCM: authenticated encryption (anti-tampering)
- Random nonce per operation (never reused)
- Master key at `~/.picoclaw/agentos/.master.key` (0600 permissions)
- Key rotation with history tracking
- Metadata per key entry

```go
ks, _ := security.NewKeyStore("keystore.db", masterKey)
ks.Store("api.openai", []byte("sk-..."), "Production key")
value, metadata, _ := ks.Retrieve("api.openai")
ks.Rotate("api.openai", []byte("new-key"), "Rotated 2026-04")
```

### 4. SQLite Registry

**Path:** `pkg/agentos/registry/db.go`, `migrations/001_initial.sql`

Replaces YAML registry with SQLite for atomicity, concurrency, and queryability.

**Schema (6 tables):**

| Table | Purpose |
|-------|---------|
| `systems` | System records with status tracking |
| `providers` | LLM provider configs per system |
| `system_metadata` | Key-value metadata per system |
| `audit_log` | Immutable operation trail |
| `jobs` | Async job queue entries |
| `health_checks` | Health check history |

- WAL mode for concurrent reads
- Soft delete (status `deleted` instead of row removal)
- Thread-safe with `sync.RWMutex`

```go
reg, _ := registry.NewDBRegistry("registry.db")
reg.RegisterSystem(&registry.System{
    Name:   "my-system",
    Status: registry.StatusInitialized,
})
systems, _ := reg.ListSystems()
```

### 5. Connection Pool + Circuit Breaker

**Path:** `pkg/agentos/llm/connection/pool.go`, `circuit_breaker.go`

Shared HTTP connection pool with per-provider rate limiting and circuit breaker protection.

**Connection Pool:**
- Shared `http.Client` across all providers
- Per-provider rate limiting (token bucket)
- Configurable max idle connections and timeouts

**Circuit Breaker (3 states):**
```
         ┌──────────┐
    5    │          │ timeout
  failures │  CLOSED  │──────────┐
         │          │          ▼
         └──────────┘    ┌──────────┐
                         │          │
                    ┌───│   OPEN    │───┐
                    │   │          │   │
                    │   └──────────┘   │
                    │                  │ 1 success
                    │   ┌──────────┐   │
                    └──│ HALF-OPEN │───┘
                       │          │
                       └──────────┘
```

- **Closed**: Normal operation, counts failures
- **Open**: Fast-fails all requests, waits for timeout
- **Half-Open**: Allows one probe request; success → Closed, failure → Open

```go
pool := connection.NewPool(connection.DefaultPoolConfig())
resp, err := pool.Execute(ctx, req, "openai")
```

### 6. Async Job Queue

**Path:** `pkg/agentos/jobs/queue.go`

SQLite-backed persistent job queue for long-running operations (bootstrap, migration, backups).

- Worker pool with configurable concurrency
- Progress reporting (0-100%)
- Job cancellation
- Auto-cleanup of old completed jobs
- States: `pending` → `running` → `completed`/`failed`/`cancelled`

```go
queue, _ := jobs.NewQueue("jobs.db")
queue.RegisterHandler("bootstrap", bootstrapHandler)
queue.Start(ctx)

jobID, _ := queue.Submit(ctx, "bootstrap", "system-id", params)
job, _ := queue.Get(ctx, jobID)
```

### 7. Audit Logging

**Path:** `pkg/agentos/audit/logger.go`

Immutable, append-only log of all security-relevant operations.

**Tracked operations:**
- `system_created`, `system_deleted`, `system_bootstrapped`
- `system_started`, `system_stopped`
- `config_changed`, `provider_configured`
- `query_executed`, `job_completed`, `job_failed`
- `health_check`

**Features:**
- Filter by system, user, operation, time range
- Export to JSON
- Cleanup of old entries (configurable retention)
- Helper methods for common operations

```go
logger, _ := audit.NewLogger("audit.db")
logger.LogSystemCreated(ctx, systemID, userID, details)
events, _ := logger.Query(ctx, audit.Filter{SystemID: systemID, Limit: 100})
```

### 8. Health Checks

**Path:** `pkg/agentos/health/checker.go`

Background health monitoring with status aggregation.

**Statuses:** `healthy`, `degraded`, `unhealthy`, `unknown`

**Built-in checks:**
- `DatabaseHealthCheck` — verifies SQLite connectivity
- `LLMProviderHealthCheck` — probes provider endpoint
- `CompositeHealthCheck` — aggregates multiple checks

```go
checker := health.NewChecker(30*time.Second, db)
checker.Register("database", health.DatabaseHealthCheck(db))
checker.Start(ctx)

status := checker.GetOverallStatus()
```

### 9. SecureAgentOSManager

**Path:** `pkg/tools/agentos_secure.go`

Unified API that integrates all security components into a single entry point.

```go
manager, _ := tools.NewSecureAgentOSManager(dataDir)
defer manager.Close()

system, _ := manager.CreateSystem(ctx, "my-system", manifestPath, userID)
manager.BootstrapSystem(ctx, "my-system", userID)
manager.StoreAPIKey("llm.openai", apiKey, "Production")
```

---

## Data Flow

### Creating a System

```
User → "Create a system called my-app"
       │
       ▼
  ExecAgentOSTool.Execute()
       │
       ├── 1. Validate system name (security/validation)
       │      └── Reject if: traversal, reserved, invalid chars
       │
       ├── 2. Create isolated directories (storage/paths)
       │      └── ~/.picoclaw/agentos/sys/a1b2c/my-app/
       │
       ├── 3. Register in SQLite (registry/db)
       │      └── INSERT INTO systems (name, hash_prefix, path, status)
       │
       └── 4. Log to audit trail (audit/logger)
              └── INSERT INTO audit_log (operation='system_created', ...)
```

### Querying an LLM Provider

```
User → "Summarize this text"
       │
       ▼
  LLM Service.ExecuteFunction()
       │
       ├── 1. Route to provider (llm/router)
       │
       ├── 2. Check circuit breaker (connection/circuit_breaker)
       │      └── Open? → Fast-fail, try fallback
       │
       ├── 3. Acquire rate limit slot (connection/pool)
       │
       ├── 4. Execute HTTP request (shared pool)
       │      └── Retry with exponential backoff on failure
       │
       └── 5. Return response or fallback to next provider
```

---

## Directory Structure

```
pkg/agentos/
├── security/
│   ├── encryption.go              # AES-256-GCM encryptor
│   ├── encryption_test.go
│   ├── keystore.go                # Encrypted key storage
│   ├── keystore_test.go
│   └── validation/
│       ├── system.go              # System name validator
│       └── system_test.go
├── storage/
│   ├── paths.go                   # Hash-based directory paths
│   └── paths_test.go
├── registry/
│   ├── db.go                      # SQLite registry
│   ├── db_test.go
│   └── migrations/
│       └── 001_initial.sql        # Schema: 6 tables
├── llm/connection/
│   ├── pool.go                    # Shared HTTP connection pool
│   ├── circuit_breaker.go         # 3-state circuit breaker
│   └── circuit_breaker_test.go
├── jobs/
│   ├── queue.go                   # Async job queue
│   └── queue_test.go
├── audit/
│   ├── logger.go                  # Immutable audit logging
│   └── logger_test.go
├── health/
│   ├── checker.go                 # Background health checks
│   └── checker_test.go
├── auth/                          # Auth providers (basic, LDAP, OIDC, SAML, social)
├── channels/                      # Channels (telegram, email, webhook)
├── evolution/                     # Manifest diff, plan, executor
├── stateful/                      # Workflow engine, guards, side effects, timeouts
├── bootstrap.go                   # System bootstrapper
├── operations.go                  # Operation catalog
├── types.go                       # Core types
└── ...

pkg/tools/
├── agentos.go                     # Tool implementations (security-integrated)
├── agentos_secure.go              # SecureAgentOSManager
├── agentos_test.go
├── agentos_integration_test.go
└── agentos_integration_secure_test.go

cmd/picoclaw/internal/agentos/commands/
├── root.go                        # Command dispatcher
└── configure.go                   # Provider config, key management
```

---

## Security Vulnerabilities Mitigated

| Vulnerability | CWE | Before | After |
|--------------|-----|--------|-------|
| Path Traversal | CWE-22 | Accepted `../../../etc/passwd` | Rejected by validator |
| SQL Injection | CWE-89 | YAML parseable | SQLite parameterized queries |
| API Key Exposure | CWE-312 | Plaintext env vars | AES-256-GCM encrypted keystore |
| Race Condition | CWE-362 | Basic mutex | Atomic operations + WAL mode |
| Missing Audit | CWE-778 | No logging | Immutable audit trail |
| No Input Validation | CWE-20 | Simple regex | Full validation + sanitization |
| No Isolation | CWE-276 | Flat directories | Hash-based isolation |
| No Rate Limiting | CWE-770 | Unlimited requests | Per-provider token bucket |
| No Fault Tolerance | N/A | Timeouts cascade | Circuit breaker pattern |

### OWASP Top 10 Coverage

| OWASP | Mitigation |
|-------|-----------|
| A01 - Broken Access Control | Audit logging |
| A02 - Cryptographic Failures | AES-256-GCM |
| A03 - Injection | Validation + parameterized queries |
| A05 - Security Misconfiguration | Secure defaults |
| A06 - Vulnerable Components | Circuit breaker |
| A09 - Security Logging | Full audit trail |
| A10 - SSRF | Input validation |

---

## CLI Commands

```bash
# Provider configuration (interactive)
picoclaw agentos configure-provider openai
picoclaw agentos configure-provider groq

# Key management
picoclaw agentos show-keys
picoclaw agentos delete-key llm.provider.openai.api_key
picoclaw agentos rotate-key llm.provider.openai.api_key

# System management
picoclaw agentos init --name my-system
picoclaw agentos bootstrap --system my-system
picoclaw agentos status
```

---

## Technical Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Encryption | AES-256-GCM | Authenticated encryption prevents tampering; random nonce per op |
| Registry | SQLite WAL | Better concurrency than YAML; crash-resistant; queryable |
| Hash prefix | 5 chars SHA-256 | ~1M unique buckets; balances collision resistance with readability |
| Circuit breaker | 3-state (Closed/Open/HalfOpen) | Classic pattern; avoids thundering herd on recovery |
| Job queue | SQLite-backed | Persistence across restarts; simple deployment |
| Master key | File at 0600 | OS-level protection; no network exposure |

---

## Testing

```bash
# Security components
go test ./pkg/agentos/security/... -v
go test ./pkg/agentos/storage/... -v
go test ./pkg/agentos/registry/... -v
go test ./pkg/agentos/audit/... -v

# Reliability components
go test ./pkg/agentos/llm/connection/... -v
go test ./pkg/agentos/jobs/... -v
go test ./pkg/agentos/health/... -v

# All with race detector
go test ./pkg/agentos/... -race -v

# Tools integration
go test ./pkg/tools/... -v -run AgentOS
```
