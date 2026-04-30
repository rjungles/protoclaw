-- Migration 001: Initial schema for AgentOS registry
-- Creates tables for systems, providers, and metadata

-- Systems table: stores information about each AgentOS system
CREATE TABLE IF NOT EXISTS systems (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    hash_prefix TEXT NOT NULL,
    path TEXT NOT NULL,
    status TEXT CHECK (status IN ('initialized', 'bootstrapped', 'serving', 'error', 'deleted')) DEFAULT 'initialized',
    manifest_path TEXT,
    llm_config_path TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookups by name
CREATE INDEX IF NOT EXISTS idx_systems_name ON systems(name);

-- Index for status filtering
CREATE INDEX IF NOT EXISTS idx_systems_status ON systems(status);

-- Providers table: stores LLM provider configurations per system
CREATE TABLE IF NOT EXISTS providers (
    id TEXT PRIMARY KEY,
    system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    enabled BOOLEAN DEFAULT 0,
    priority INTEGER DEFAULT 0,
    config JSON,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(system_id, name)
);

-- Index for provider lookups
CREATE INDEX IF NOT EXISTS idx_providers_system ON providers(system_id);
CREATE INDEX IF NOT EXISTS idx_providers_enabled ON providers(enabled);

-- System metadata table: key-value store for system-specific metadata
CREATE TABLE IF NOT EXISTS system_metadata (
    system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (system_id, key)
);

-- Index for metadata queries
CREATE INDEX IF NOT EXISTS idx_metadata_system ON system_metadata(system_id);

-- Audit log table: immutable record of all operations
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation TEXT NOT NULL,
    system_id TEXT REFERENCES systems(id) ON DELETE SET NULL,
    user_id TEXT,
    details TEXT, -- JSON
    ip_address TEXT,
    user_agent TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for audit queries
CREATE INDEX IF NOT EXISTS idx_audit_system ON audit_log(system_id);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_operation ON audit_log(operation);

-- Jobs table: async job queue
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    system_id TEXT REFERENCES systems(id) ON DELETE CASCADE,
    status TEXT CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')) DEFAULT 'pending',
    progress INTEGER DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
    error TEXT,
    params TEXT, -- JSON
    result TEXT, -- JSON
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    completed_at DATETIME
);

-- Indexes for job queries
CREATE INDEX IF NOT EXISTS idx_jobs_system ON jobs(system_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at);

-- Health checks table: stores health check results
CREATE TABLE IF NOT EXISTS health_checks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    system_id TEXT REFERENCES systems(id) ON DELETE CASCADE,
    component TEXT NOT NULL,
    status TEXT CHECK (status IN ('healthy', 'degraded', 'unhealthy', 'unknown')),
    latency_ms INTEGER,
    message TEXT,
    checked_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for health queries
CREATE INDEX IF NOT EXISTS idx_health_system ON health_checks(system_id);
CREATE INDEX IF NOT EXISTS idx_health_checked ON health_checks(checked_at);

-- Migration tracking table
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    description TEXT
);

-- Insert initial migration record
INSERT OR IGNORE INTO schema_migrations (version, description) VALUES (1, 'Initial schema: systems, providers, metadata, audit_log, jobs, health_checks');
