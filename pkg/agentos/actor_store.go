package agentos

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

var (
	ErrActorNotFound   = errors.New("actor not found")
	ErrActorExists     = errors.New("actor already exists")
	ErrInvalidAPIKey   = errors.New("invalid API key")
)

type ActorCredential struct {
	ActorID   string
	ActorType string
	APIKey    string
	APIKeyHash string
	Roles     []string
	CreatedAt string
	IsActive  bool
}

type ActorStore interface {
	Provision(actor manifest.Actor) (*ActorCredential, error)
	GetByID(actorID string) (*ActorCredential, error)
	GetByAPIKey(apiKey string) (*ActorCredential, error)
	ListAll() ([]*ActorCredential, error)
	Deactivate(actorID string) error
}

type MemoryActorStore struct {
	mu      sync.RWMutex
	actors  map[string]*ActorCredential
	byAPIKey map[string]string
}

func NewMemoryActorStore() *MemoryActorStore {
	return &MemoryActorStore{
		actors:   make(map[string]*ActorCredential),
		byAPIKey: make(map[string]string),
	}
}

func (s *MemoryActorStore) Provision(actor manifest.Actor) (*ActorCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.actors[actor.ID]; exists {
		return nil, fmt.Errorf("actor %s already provisioned: %w", actor.ID, ErrActorExists)
	}

	apiKey, apiKeyHash := generateAPIKey()

	cred := &ActorCredential{
		ActorID:    actor.ID,
		ActorType:  actor.Name,
		APIKey:     apiKey,
		APIKeyHash: apiKeyHash,
		Roles:      actor.Roles,
		CreatedAt:  "",
		IsActive:  true,
	}

	s.actors[actor.ID] = cred
	s.byAPIKey[apiKeyHash] = actor.ID

	return cred, nil
}

func (s *MemoryActorStore) GetByID(actorID string) (*ActorCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cred, exists := s.actors[actorID]
	if !exists {
		return nil, ErrActorNotFound
	}
	return cred, nil
}

func (s *MemoryActorStore) GetByAPIKey(apiKey string) (*ActorCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hash := hashAPIKey(apiKey)
	actorID, exists := s.byAPIKey[hash]
	if !exists {
		return nil, ErrInvalidAPIKey
	}

	cred, exists := s.actors[actorID]
	if !exists {
		return nil, ErrActorNotFound
	}

	if !cred.IsActive {
		return nil, ErrActorNotFound
	}

	return cred, nil
}

func (s *MemoryActorStore) ListAll() ([]*ActorCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*ActorCredential, 0, len(s.actors))
	for _, cred := range s.actors {
		if cred.IsActive {
			result = append(result, cred)
		}
	}
	return result, nil
}

func (s *MemoryActorStore) Deactivate(actorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cred, exists := s.actors[actorID]
	if !exists {
		return ErrActorNotFound
	}

	cred.IsActive = false
	return nil
}

type DBActorStore struct {
	db *sql.DB
	mu sync.Mutex
}

func NewDBActorStore(db *sql.DB) *DBActorStore {
	store := &DBActorStore{db: db}
	store.createTable()
	return store
}

func (s *DBActorStore) createTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS _actors (
		actor_id TEXT PRIMARY KEY,
		actor_type TEXT NOT NULL,
		api_key_hash TEXT NOT NULL,
		roles TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		is_active BOOLEAN DEFAULT TRUE
	)`

	_, err := s.db.Exec(query)
	return err
}

func (s *DBActorStore) Provision(actor manifest.Actor) (*ActorCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var existing int
	err := s.db.QueryRow("SELECT COUNT(*) FROM _actors WHERE actor_id = ?", actor.ID).Scan(&existing)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing actor: %w", err)
	}
	if existing > 0 {
		return nil, fmt.Errorf("actor %s already provisioned: %w", actor.ID, ErrActorExists)
	}

	apiKey, apiKeyHash := generateAPIKey()

	rolesJSON, err := json.Marshal(actor.Roles)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal roles: %w", err)
	}

	query := `INSERT INTO _actors (actor_id, actor_type, api_key_hash, roles, is_active) VALUES (?, ?, ?, ?, TRUE)`
	_, err = s.db.Exec(query, actor.ID, actor.Name, apiKeyHash, string(rolesJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to insert actor: %w", err)
	}

	return &ActorCredential{
		ActorID:    actor.ID,
		ActorType:  actor.Name,
		APIKey:     apiKey,
		APIKeyHash: apiKeyHash,
		Roles:      actor.Roles,
		CreatedAt:  "",
		IsActive:  true,
	}, nil
}

func (s *DBActorStore) GetByID(actorID string) (*ActorCredential, error) {
	var cred ActorCredential
	var rolesJSON string

	query := `SELECT actor_id, actor_type, api_key_hash, roles, created_at, is_active FROM _actors WHERE actor_id = ?`
	err := s.db.QueryRow(query, actorID).Scan(
		&cred.ActorID, &cred.ActorType, &cred.APIKeyHash, &rolesJSON, &cred.CreatedAt, &cred.IsActive,
	)
	if err == sql.ErrNoRows {
		return nil, ErrActorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query actor: %w", err)
	}

	if err := json.Unmarshal([]byte(rolesJSON), &cred.Roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}

	return &cred, nil
}

func (s *DBActorStore) GetByAPIKey(apiKey string) (*ActorCredential, error) {
	hash := hashAPIKey(apiKey)

	var cred ActorCredential
	var rolesJSON string

	query := `SELECT actor_id, actor_type, api_key_hash, roles, created_at, is_active FROM _actors WHERE api_key_hash = ? AND is_active = TRUE`
	err := s.db.QueryRow(query, hash).Scan(
		&cred.ActorID, &cred.ActorType, &cred.APIKeyHash, &rolesJSON, &cred.CreatedAt, &cred.IsActive,
	)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidAPIKey
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query actor by API key: %w", err)
	}

	if err := json.Unmarshal([]byte(rolesJSON), &cred.Roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}

	return &cred, nil
}

func (s *DBActorStore) ListAll() ([]*ActorCredential, error) {
	query := `SELECT actor_id, actor_type, api_key_hash, roles, created_at, is_active FROM _actors WHERE is_active = TRUE`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query actors: %w", err)
	}
	defer rows.Close()

	var result []*ActorCredential
	for rows.Next() {
		var cred ActorCredential
		var rolesJSON string

		if err := rows.Scan(&cred.ActorID, &cred.ActorType, &cred.APIKeyHash, &rolesJSON, &cred.CreatedAt, &cred.IsActive); err != nil {
			return nil, fmt.Errorf("failed to scan actor: %w", err)
		}

		if err := json.Unmarshal([]byte(rolesJSON), &cred.Roles); err != nil {
			return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
		}

		result = append(result, &cred)
	}

	return result, rows.Err()
}

func (s *DBActorStore) Deactivate(actorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `UPDATE _actors SET is_active = FALSE WHERE actor_id = ?`
	result, err := s.db.Exec(query, actorID)
	if err != nil {
		return fmt.Errorf("failed to deactivate actor: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrActorNotFound
	}

	return nil
}

func generateAPIKey() (string, string) {
	b := make([]byte, 32)
	rand.Read(b)
	key := base64.URLEncoding.EncodeToString(b)
	return key, hashAPIKey(key)
}

func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
