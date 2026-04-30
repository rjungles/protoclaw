// Package registry provides a thread-safe SQLite-based system registry
package registry

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// Status represents system status
type Status string

const (
	StatusInitialized  Status = "initialized"
	StatusBootstrapped Status = "bootstrapped"
	StatusServing      Status = "serving"
	StatusError        Status = "error"
	StatusDeleted      Status = "deleted"
)

// System represents an AgentOS system
type System struct {
	ID            string
	Name          string
	HashPrefix    string
	Path          string
	Status        Status
	ManifestPath  string
	LLMConfigPath string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Metadata      map[string]string
}

// DBRegistry provides thread-safe system registry using SQLite
type DBRegistry struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

// NewDBRegistry creates a new database-backed registry
func NewDBRegistry(dbPath string) (*DBRegistry, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create registry directory: %w", err)
	}

	// Open database with WAL mode for better concurrency
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open registry: %w", err)
	}

	// Set connection limits
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// Run migrations
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &DBRegistry{
		db:   db,
		path: dbPath,
	}, nil
}

// runMigrations executes database migrations
func runMigrations(db *sql.DB) error {
	// Read migration file
	// In production, embed the SQL file
	migrationSQL := `-- Migration 001: Initial schema
CREATE TABLE IF NOT EXISTS systems (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    hash_prefix TEXT NOT NULL,
    path TEXT NOT NULL,
    status TEXT DEFAULT 'initialized',
    manifest_path TEXT,
    llm_config_path TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_systems_name ON systems(name);

CREATE TABLE IF NOT EXISTS system_metadata (
    system_id TEXT REFERENCES systems(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT,
    PRIMARY KEY (system_id, key)
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO schema_migrations (version) VALUES (1);`

	_, err := db.Exec(migrationSQL)
	return err
}

// RegisterSystem adds a new system to the registry
func (r *DBRegistry) RegisterSystem(system *System) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate ID if not provided
	if system.ID == "" {
		system.ID = uuid.New().String()
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert system
	_, err = tx.Exec(`
		INSERT INTO systems (id, name, hash_prefix, path, status, manifest_path, llm_config_path)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, system.ID, system.Name, system.HashPrefix, system.Path,
		string(system.Status), system.ManifestPath, system.LLMConfigPath)

	if err != nil {
		return fmt.Errorf("failed to register system: %w", err)
	}

	// Insert metadata
	if len(system.Metadata) > 0 {
		for key, value := range system.Metadata {
			_, err = tx.Exec(`
				INSERT INTO system_metadata (system_id, key, value)
				VALUES (?, ?, ?)
			`, system.ID, key, value)
			if err != nil {
				return fmt.Errorf("failed to store metadata: %w", err)
			}
		}
	}

	return tx.Commit()
}

// UpdateStatus updates system status atomically
func (r *DBRegistry) UpdateStatus(systemID string, status Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	result, err := r.db.Exec(`
		UPDATE systems 
		SET status = ?, updated_at = datetime('now') 
		WHERE id = ?
	`, string(status), systemID)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("system not found: %s", systemID)
	}

	return nil
}

// GetSystem retrieves a system by name
func (r *DBRegistry) GetSystem(name string) (*System, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var s System
	var statusStr string

	err := r.db.QueryRow(`
		SELECT id, name, hash_prefix, path, status, manifest_path, llm_config_path, created_at, updated_at
		FROM systems WHERE name = ? AND status != 'deleted'
	`, name).Scan(&s.ID, &s.Name, &s.HashPrefix, &s.Path, &statusStr,
		&s.ManifestPath, &s.LLMConfigPath, &s.CreatedAt, &s.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("system not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	s.Status = Status(statusStr)

	// Load metadata
	metadata, err := r.getMetadata(s.ID)
	if err != nil {
		return nil, err
	}
	s.Metadata = metadata

	return &s, nil
}

// GetSystemByID retrieves a system by ID
func (r *DBRegistry) GetSystemByID(id string) (*System, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var s System
	var statusStr string

	err := r.db.QueryRow(`
		SELECT id, name, hash_prefix, path, status, manifest_path, llm_config_path, created_at, updated_at
		FROM systems WHERE id = ? AND status != 'deleted'
	`, id).Scan(&s.ID, &s.Name, &s.HashPrefix, &s.Path, &statusStr,
		&s.ManifestPath, &s.LLMConfigPath, &s.CreatedAt, &s.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("system not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	s.Status = Status(statusStr)

	return &s, nil
}

// ListSystems returns all non-deleted systems
func (r *DBRegistry) ListSystems() ([]*System, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, err := r.db.Query(`
		SELECT id, name, hash_prefix, path, status, manifest_path, llm_config_path, created_at, updated_at
		FROM systems WHERE status != 'deleted'
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var systems []*System
	for rows.Next() {
		var s System
		var statusStr string

		err := rows.Scan(&s.ID, &s.Name, &s.HashPrefix, &s.Path, &statusStr,
			&s.ManifestPath, &s.LLMConfigPath, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}

		s.Status = Status(statusStr)
		systems = append(systems, &s)
	}

	return systems, rows.Err()
}

// DeleteSystem marks a system as deleted (soft delete)
func (r *DBRegistry) DeleteSystem(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	result, err := r.db.Exec(`
		UPDATE systems SET status = 'deleted', updated_at = datetime('now')
		WHERE name = ?
	`, name)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("system not found: %s", name)
	}

	return nil
}

// UpdateSystem updates system fields
func (r *DBRegistry) UpdateSystem(system *System) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.Exec(`
		UPDATE systems 
		SET status = ?, manifest_path = ?, llm_config_path = ?, updated_at = datetime('now')
		WHERE id = ?
	`, string(system.Status), system.ManifestPath, system.LLMConfigPath, system.ID)

	return err
}

// SetMetadata sets a metadata key for a system
func (r *DBRegistry) SetMetadata(systemID, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO system_metadata (system_id, key, value)
		VALUES (?, ?, ?)
	`, systemID, key, value)

	return err
}

// GetMetadata retrieves a metadata value
func (r *DBRegistry) GetMetadata(systemID, key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var value string
	err := r.db.QueryRow(`
		SELECT value FROM system_metadata WHERE system_id = ? AND key = ?
	`, systemID, key).Scan(&value)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return value, nil
}

// getMetadata retrieves all metadata for a system
func (r *DBRegistry) getMetadata(systemID string) (map[string]string, error) {
	rows, err := r.db.Query(`
		SELECT key, value FROM system_metadata WHERE system_id = ?
	`, systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		metadata[key] = value
	}

	return metadata, rows.Err()
}

// SystemExists checks if a system exists
func (r *DBRegistry) SystemExists(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM systems WHERE name = ? AND status != 'deleted'
	`, name).Scan(&count)

	return err == nil && count > 0
}

// Count returns the total number of systems
func (r *DBRegistry) Count() (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM systems WHERE status != 'deleted'
	`).Scan(&count)

	return count, err
}

// Close closes the registry
func (r *DBRegistry) Close() error {
	return r.db.Close()
}

// BeginTx starts a transaction
func (r *DBRegistry) BeginTx() (*sql.Tx, error) {
	return r.db.Begin()
}

// Query executes a query
func (r *DBRegistry) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return r.db.Query(query, args...)
}

// Exec executes a statement
func (r *DBRegistry) Exec(query string, args ...interface{}) (sql.Result, error) {
	return r.db.Exec(query, args...)
}

// LogAudit logs an audit event
func (r *DBRegistry) LogAudit(operation, systemID, userID string, details map[string]interface{}) error {
	detailsJSON, _ := json.Marshal(details)

	_, err := r.db.Exec(`
		INSERT INTO audit_log (operation, system_id, user_id, details, timestamp)
		VALUES (?, ?, ?, ?, datetime('now'))
	`, operation, systemID, userID, string(detailsJSON))

	return err
}
