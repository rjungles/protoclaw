// Package audit provides audit logging functionality for AgentOS
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Operation represents the type of audited operation
type Operation string

const (
	OpSystemCreated       Operation = "system_created"
	OpSystemDeleted       Operation = "system_deleted"
	OpSystemBootstrapped  Operation = "system_bootstrapped"
	OpSystemStarted       Operation = "system_started"
	OpSystemStopped       Operation = "system_stopped"
	OpConfigChanged       Operation = "config_changed"
	OpProviderConfigured  Operation = "provider_configured"
	OpProviderEnabled     Operation = "provider_enabled"
	OpProviderDisabled    Operation = "provider_disabled"
	OpQueryExecuted       Operation = "query_executed"
	OpManifestGenerated   Operation = "manifest_generated"
	OpSystemInitialized   Operation = "system_initialized"
	OpSystemValidated     Operation = "system_validated"
	OpKeyStored           Operation = "key_stored"
	OpKeyRotated          Operation = "key_rotated"
	OpKeyDeleted          Operation = "key_deleted"
	OpJobSubmitted        Operation = "job_submitted"
	OpJobCompleted        Operation = "job_completed"
	OpJobFailed           Operation = "job_failed"
	OpHealthCheck         Operation = "health_check"
)

// Event represents an audit event
type Event struct {
	ID        int64
	Operation Operation
	SystemID  string
	UserID    string
	Details   map[string]interface{}
	IPAddress string
	UserAgent string
	Timestamp time.Time
}

// Logger provides audit logging
type Logger struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewLogger creates a new audit logger
func NewLogger(dbPath string) (*Logger, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create audit directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}

	if err := createAuditSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create audit schema: %w", err)
	}

	return &Logger{db: db}, nil
}

func createAuditSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			operation TEXT NOT NULL,
			system_id TEXT,
			user_id TEXT,
			details TEXT,
			ip_address TEXT,
			user_agent TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_audit_system ON audit_log(system_id);
		CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
		CREATE INDEX IF NOT EXISTS idx_audit_operation ON audit_log(operation);
		CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_log(user_id);

		-- Partition by date for easier archiving
		CREATE INDEX IF NOT EXISTS idx_audit_date ON audit_log(date(timestamp));
	`)
	return err
}

// Log records an audit event
func (l *Logger) Log(ctx context.Context, op Operation, systemID, userID string, details map[string]interface{}) error {
	return l.LogWithMetadata(ctx, op, systemID, userID, details, "", "")
}

// LogWithMetadata records an audit event with additional metadata
func (l *Logger) LogWithMetadata(ctx context.Context, op Operation, systemID, userID string, details map[string]interface{}, ipAddress, userAgent string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	detailsJSON, err := json.Marshal(details)
	if err != nil {
		detailsJSON = []byte("{}")
	}

	_, err = l.db.ExecContext(ctx, `
		INSERT INTO audit_log (operation, system_id, user_id, details, ip_address, user_agent, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
	`, string(op), systemID, userID, string(detailsJSON), ipAddress, userAgent)

	if err != nil {
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	return nil
}

// Query retrieves audit events
func (l *Logger) Query(ctx context.Context, filter Filter) ([]*Event, error) {
	query := `SELECT id, operation, system_id, user_id, details, ip_address, user_agent, timestamp
		FROM audit_log WHERE 1=1`
	args := []interface{}{}

	if filter.SystemID != "" {
		query += " AND system_id = ?"
		args = append(args, filter.SystemID)
	}

	if filter.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.UserID)
	}

	if filter.Operation != "" {
		query += " AND operation = ?"
		args = append(args, string(filter.Operation))
	}

	if !filter.Since.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, filter.Since)
	}

	if !filter.Until.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, filter.Until)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var e Event
		var detailsJSON string

		err := rows.Scan(
			&e.ID, &e.Operation, &e.SystemID, &e.UserID,
			&detailsJSON, &e.IPAddress, &e.UserAgent, &e.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		if detailsJSON != "" {
			json.Unmarshal([]byte(detailsJSON), &e.Details)
		}

		events = append(events, &e)
	}

	return events, rows.Err()
}

// Filter provides filtering options for audit queries
type Filter struct {
	SystemID  string
	UserID    string
	Operation Operation
	Since     time.Time
	Until     time.Time
	Limit     int
}

// GetSystemHistory retrieves all events for a system
func (l *Logger) GetSystemHistory(ctx context.Context, systemID string, limit int) ([]*Event, error) {
	return l.Query(ctx, Filter{
		SystemID: systemID,
		Limit:    limit,
	})
}

// GetUserActivity retrieves all events for a user
func (l *Logger) GetUserActivity(ctx context.Context, userID string, limit int) ([]*Event, error) {
	return l.Query(ctx, Filter{
		UserID: userID,
		Limit:  limit,
	})
}

// Count returns the number of events matching the filter
func (l *Logger) Count(ctx context.Context, filter Filter) (int, error) {
	query := `SELECT COUNT(*) FROM audit_log WHERE 1=1`
	args := []interface{}{}

	if filter.SystemID != "" {
		query += " AND system_id = ?"
		args = append(args, filter.SystemID)
	}

	if filter.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.UserID)
	}

	if filter.Operation != "" {
		query += " AND operation = ?"
		args = append(args, string(filter.Operation))
	}

	if !filter.Since.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, filter.Since)
	}

	if !filter.Until.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, filter.Until)
	}

	var count int
	err := l.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// Cleanup removes old events
func (l *Logger) Cleanup(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	_, err := l.db.ExecContext(ctx, `
		DELETE FROM audit_log WHERE timestamp < ?
	`, cutoff)
	return err
}

// Export exports events to JSON
func (l *Logger) Export(ctx context.Context, filter Filter) ([]byte, error) {
	events, err := l.Query(ctx, filter)
	if err != nil {
		return nil, err
	}

	return json.Marshal(events)
}

// Close closes the logger
func (l *Logger) Close() error {
	return l.db.Close()
}

// Helper methods for common operations

// LogSystemCreated logs system creation
func (l *Logger) LogSystemCreated(ctx context.Context, systemID, userID string, details map[string]interface{}) error {
	return l.Log(ctx, OpSystemCreated, systemID, userID, details)
}

// LogSystemDeleted logs system deletion
func (l *Logger) LogSystemDeleted(ctx context.Context, systemID, userID string, details map[string]interface{}) error {
	return l.Log(ctx, OpSystemDeleted, systemID, userID, details)
}

// LogSystemBootstrapped logs system bootstrap
func (l *Logger) LogSystemBootstrapped(ctx context.Context, systemID, userID string, details map[string]interface{}) error {
	return l.Log(ctx, OpSystemBootstrapped, systemID, userID, details)
}

// LogQuery logs a query execution
func (l *Logger) LogQuery(ctx context.Context, systemID, userID, entity string, filters map[string]interface{}) error {
	return l.Log(ctx, OpQueryExecuted, systemID, userID, map[string]interface{}{
		"entity":  entity,
		"filters": filters,
	})
}

// LogJobCompleted logs job completion
func (l *Logger) LogJobCompleted(ctx context.Context, systemID, jobID string, duration time.Duration) error {
	return l.Log(ctx, OpJobCompleted, systemID, "system", map[string]interface{}{
		"job_id":   jobID,
		"duration": duration.String(),
	})
}

// LogJobFailed logs job failure
func (l *Logger) LogJobFailed(ctx context.Context, systemID, jobID string, err error) error {
	return l.Log(ctx, OpJobFailed, systemID, "system", map[string]interface{}{
		"job_id": jobID,
		"error":  err.Error(),
	})
}

// LogHealthCheck logs a health check
func (l *Logger) LogHealthCheck(ctx context.Context, systemID, component, status string, latency time.Duration) error {
	return l.Log(ctx, OpHealthCheck, systemID, "system", map[string]interface{}{
		"component": component,
		"status":    status,
		"latency":   latency.String(),
	})
}
