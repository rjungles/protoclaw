// Package security provides secure storage for sensitive data
package security

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// KeyStore provides secure storage for API keys and other sensitive data
type KeyStore struct {
	db        *sql.DB
	encryptor *Encryptor
	mu        sync.RWMutex
}

// NewKeyStore creates a new keystore at the specified path
func NewKeyStore(dbPath string, masterKey []byte) (*KeyStore, error) {
	// Ensure directory exists with restricted permissions
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create keystore directory: %w", err)
	}

	// Open database with WAL mode for better concurrency
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open keystore: %w", err)
	}

	// Create tables
	if err := createKeyStoreSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create keystore schema: %w", err)
	}

	return &KeyStore{
		db:        db,
		encryptor: NewEncryptor(masterKey),
	}, nil
}

func createKeyStoreSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS keys (
			name TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_keys_name ON keys(name);

		CREATE TABLE IF NOT EXISTS key_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			rotated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (name) REFERENCES keys(name) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_key_history_name ON key_history(name);
	`)
	return err
}

// Store saves an encrypted key
func (ks *KeyStore) Store(name string, value []byte, metadata string) error {
	if name == "" {
		return fmt.Errorf("key name cannot be empty")
	}

	if len(value) == 0 {
		return fmt.Errorf("key value cannot be empty")
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()

	encrypted, err := ks.encryptor.Encrypt(value)
	if err != nil {
		return fmt.Errorf("failed to encrypt key: %w", err)
	}

	_, err = ks.db.Exec(
		`INSERT OR REPLACE INTO keys (name, value, updated_at, metadata)
		VALUES (?, ?, datetime('now'), ?)`,
		name, encrypted, metadata,
	)
	if err != nil {
		return fmt.Errorf("failed to store key: %w", err)
	}

	return nil
}

// Retrieve gets and decrypts a key
func (ks *KeyStore) Retrieve(name string) ([]byte, string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	var encrypted, metadata string
	err := ks.db.QueryRow(
		`SELECT value, metadata FROM keys WHERE name = ?`,
		name,
	).Scan(&encrypted, &metadata)

	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("key not found: %s", name)
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to retrieve key: %w", err)
	}

	decrypted, err := ks.encryptor.Decrypt(encrypted)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decrypt key: %w", err)
	}

	return decrypted, metadata, nil
}

// Delete removes a key
func (ks *KeyStore) Delete(name string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	_, err := ks.db.Exec(`DELETE FROM keys WHERE name = ?`, name)
	return err
}

// Exists checks if a key exists
func (ks *KeyStore) Exists(name string) bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	var count int
	err := ks.db.QueryRow(`SELECT COUNT(*) FROM keys WHERE name = ?`, name).Scan(&count)
	return err == nil && count > 0
}

// List returns all key names
func (ks *KeyStore) List() ([]string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	rows, err := ks.db.Query(`SELECT name FROM keys ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}

	return names, rows.Err()
}

// Rotate creates a new key and records the rotation
func (ks *KeyStore) Rotate(name string, newValue []byte, metadata string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Check if key exists
	var exists bool
	err := ks.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM keys WHERE name = ?)`, name).Scan(&exists)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("key not found: %s", name)
	}

	// Record rotation
	_, err = ks.db.Exec(`INSERT INTO key_history (name) VALUES (?)`, name)
	if err != nil {
		return fmt.Errorf("failed to record rotation: %w", err)
	}

	// Store new value
	encrypted, err := ks.encryptor.Encrypt(newValue)
	if err != nil {
		return fmt.Errorf("failed to encrypt new key: %w", err)
	}

	_, err = ks.db.Exec(
		`UPDATE keys SET value = ?, updated_at = datetime('now'), metadata = ? WHERE name = ?`,
		encrypted, metadata, name,
	)
	if err != nil {
		return fmt.Errorf("failed to update key: %w", err)
	}

	return nil
}

// GetRotationCount returns how many times a key has been rotated
func (ks *KeyStore) GetRotationCount(name string) (int, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	var count int
	err := ks.db.QueryRow(
		`SELECT COUNT(*) FROM key_history WHERE name = ?`,
		name,
	).Scan(&count)

	return count, err
}

// GetLastRotation returns when a key was last rotated
func (ks *KeyStore) GetLastRotation(name string) (*time.Time, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	var rotatedAt time.Time
	err := ks.db.QueryRow(
		`SELECT MAX(rotated_at) FROM key_history WHERE name = ?`,
		name,
	).Scan(&rotatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &rotatedAt, nil
}

// Close closes the keystore
func (ks *KeyStore) Close() error {
	return ks.db.Close()
}

// KeyInfo contains metadata about a stored key
type KeyInfo struct {
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Metadata  string
}

// GetInfo returns metadata for a key
func (ks *KeyStore) GetInfo(name string) (*KeyInfo, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	var info KeyInfo
	err := ks.db.QueryRow(
		`SELECT name, created_at, updated_at, metadata FROM keys WHERE name = ?`,
		name,
	).Scan(&info.Name, &info.CreatedAt, &info.UpdatedAt, &info.Metadata)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("key not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	return &info, nil
}

// MigrateFromEnv migrates an API key from environment variable to keystore
func (ks *KeyStore) MigrateFromEnv(keyName, envVarName string) error {
	value := os.Getenv(envVarName)
	if value == "" {
		return fmt.Errorf("environment variable %s not set", envVarName)
	}

	metadata := fmt.Sprintf("Migrated from env var: %s at %s", envVarName, time.Now().Format(time.RFC3339))

	if err := ks.Store(keyName, []byte(value), metadata); err != nil {
		return fmt.Errorf("failed to migrate key: %w", err)
	}

	return nil
}
