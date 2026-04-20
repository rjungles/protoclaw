package stateful

import (
	"database/sql"
	"fmt"
	"time"
)

// HistoryStore interface para gerenciamento de histórico de transições
type HistoryStore interface {
	Record(record *TransitionRecord) error
	GetHistory(entityType, entityID string) ([]TransitionRecord, error)
	GetLastTransition(entityType, entityID string) (*TransitionRecord, error)
	GetHistoryByActor(actorID string, limit int) ([]TransitionRecord, error)
	GetHistoryByState(entityType, state string, limit int) ([]TransitionRecord, error)
	CleanupHistory(olderThan time.Duration) (int64, error)
}

// DBHistoryStore implementação SQL do HistoryStore
type DBHistoryStore struct {
	db *sql.DB
}

// NewDBHistoryStore cria uma nova instância do DBHistoryStore
func NewDBHistoryStore(db *sql.DB) *DBHistoryStore {
	store := &DBHistoryStore{db: db}
	store.createTables()
	return store
}

// createTables cria as tabelas necessárias para o histórico
func (s *DBHistoryStore) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS _workflow_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			from_state TEXT,
			to_state TEXT NOT NULL,
			action TEXT NOT NULL,
			actor_id TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_history_entity ON _workflow_history(entity_type, entity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_history_actor ON _workflow_history(actor_id)`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_history_state ON _workflow_history(to_state)`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_history_timestamp ON _workflow_history(timestamp)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

// Record registra uma transição no histórico
func (s *DBHistoryStore) Record(record *TransitionRecord) error {
	query := `INSERT INTO _workflow_history 
		(entity_type, entity_id, from_state, to_state, action, actor_id, timestamp, metadata) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	metadata := ""
	if record.Metadata != nil {
		metadata = formatMetadata(record.Metadata)
	}

	result, err := s.db.Exec(query,
		record.EntityType, record.EntityID, record.FromState, record.ToState,
		record.Action, record.ActorID, record.Timestamp, metadata,
	)
	if err != nil {
		return err
	}

	// Atualizar o ID do registro
	if id, err := result.LastInsertId(); err == nil {
		record.ID = fmt.Sprintf("%d", id)
	}

	return nil
}

// GetHistory retorna o histórico completo de transições para uma entidade
func (s *DBHistoryStore) GetHistory(entityType, entityID string) ([]TransitionRecord, error) {
	query := `SELECT id, entity_type, entity_id, from_state, to_state, action, actor_id, timestamp, metadata 
		FROM _workflow_history 
		WHERE entity_type = ? AND entity_id = ? 
		ORDER BY timestamp ASC`

	rows, err := s.db.Query(query, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TransitionRecord
	for rows.Next() {
		var rec TransitionRecord
		var actorID sql.NullString
		var metadata sql.NullString

		if err := rows.Scan(&rec.ID, &rec.EntityType, &rec.EntityID, &rec.FromState, &rec.ToState,
			&rec.Action, &actorID, &rec.Timestamp, &metadata); err != nil {
			return nil, err
		}

		if actorID.Valid {
			rec.ActorID = actorID.String
		}
		if metadata.Valid {
			rec.Metadata = parseMetadata(metadata.String)
		}

		result = append(result, rec)
	}

	return result, rows.Err()
}

// GetLastTransition retorna a última transição de uma entidade
func (s *DBHistoryStore) GetLastTransition(entityType, entityID string) (*TransitionRecord, error) {
	query := `SELECT id, entity_type, entity_id, from_state, to_state, action, actor_id, timestamp, metadata 
		FROM _workflow_history 
		WHERE entity_type = ? AND entity_id = ? 
		ORDER BY timestamp DESC 
		LIMIT 1`

	var rec TransitionRecord
	var actorID sql.NullString
	var metadata sql.NullString

	err := s.db.QueryRow(query, entityType, entityID).Scan(
		&rec.ID, &rec.EntityType, &rec.EntityID, &rec.FromState, &rec.ToState,
		&rec.Action, &actorID, &rec.Timestamp, &metadata,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if actorID.Valid {
		rec.ActorID = actorID.String
	}
	if metadata.Valid {
		rec.Metadata = parseMetadata(metadata.String)
	}

	return &rec, nil
}

// GetHistoryByActor retorna o histórico de transições por ator
func (s *DBHistoryStore) GetHistoryByActor(actorID string, limit int) ([]TransitionRecord, error) {
	query := `SELECT id, entity_type, entity_id, from_state, to_state, action, actor_id, timestamp, metadata 
		FROM _workflow_history 
		WHERE actor_id = ? 
		ORDER BY timestamp DESC 
		LIMIT ?`

	rows, err := s.db.Query(query, actorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TransitionRecord
	for rows.Next() {
		var rec TransitionRecord
		var actorID sql.NullString
		var metadata sql.NullString

		if err := rows.Scan(&rec.ID, &rec.EntityType, &rec.EntityID, &rec.FromState, &rec.ToState,
			&rec.Action, &actorID, &rec.Timestamp, &metadata); err != nil {
			return nil, err
		}

		if actorID.Valid {
			rec.ActorID = actorID.String
		}
		if metadata.Valid {
			rec.Metadata = parseMetadata(metadata.String)
		}

		result = append(result, rec)
	}

	return result, rows.Err()
}

// GetHistoryByState retorna o histórico de transições para um estado específico
func (s *DBHistoryStore) GetHistoryByState(entityType, state string, limit int) ([]TransitionRecord, error) {
	query := `SELECT id, entity_type, entity_id, from_state, to_state, action, actor_id, timestamp, metadata 
		FROM _workflow_history 
		WHERE entity_type = ? AND to_state = ? 
		ORDER BY timestamp DESC 
		LIMIT ?`

	rows, err := s.db.Query(query, entityType, state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TransitionRecord
	for rows.Next() {
		var rec TransitionRecord
		var actorID sql.NullString
		var metadata sql.NullString

		if err := rows.Scan(&rec.ID, &rec.EntityType, &rec.EntityID, &rec.FromState, &rec.ToState,
			&rec.Action, &actorID, &rec.Timestamp, &metadata); err != nil {
			return nil, err
		}

		if actorID.Valid {
			rec.ActorID = actorID.String
		}
		if metadata.Valid {
			rec.Metadata = parseMetadata(metadata.String)
		}

		result = append(result, rec)
	}

	return result, rows.Err()
}

// CleanupHistory remove registros antigos do histórico
func (s *DBHistoryStore) CleanupHistory(olderThan time.Duration) (int64, error) {
	query := `DELETE FROM _workflow_history 
		WHERE timestamp < datetime('now', '-' || ? || ' seconds')`

	result, err := s.db.Exec(query, int(olderThan.Seconds()))
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
