package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

// MigrationRecord representa um registro de migração
type MigrationRecord struct {
	ID          int
	Version     string
	Checksum    string
	Description string
	AppliedAt   time.Time
	Success     bool
}

// SchemaChange representa uma mudança no schema
type SchemaChange struct {
	Type        string // CREATE_TABLE, ALTER_TABLE, ADD_COLUMN, DROP_COLUMN, CREATE_INDEX
	Table       string
	Column      string
	ColumnType  string
	Constraints string
	SQL         string
	EntityName  string
	FieldName   string
}

// DBInterface define a interface para banco de dados
type DBInterface interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (RowsInterface, error)
	QueryRow(query string, args ...interface{}) RowInterface
}

// RowsInterface define interface para rows
type RowsInterface interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close() error
	Err() error
}

// RowInterface define interface para row
type RowInterface interface {
	Scan(dest ...interface{}) error
}

// Migrator gerencia migrações automáticas de banco de dados baseadas no manifesto
type Migrator struct {
	db           DBInterface
	manifest     *manifest.Manifest
	mu           sync.Mutex
	migrationLog []MigrationRecord
}

// NewMigrator cria uma nova instância do migrator
func NewMigrator(db DBInterface, m *manifest.Manifest) *Migrator {
	return &Migrator{
		db:       db,
		manifest: m,
	}
}

// Migrate executa todas as migrações necessárias
func (m *Migrator) Migrate(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.createMigrationsTable(); err != nil {
		return fmt.Errorf("falha ao criar tabela de migrações: %w", err)
	}

	changes, err := m.generateSchemaChanges()
	if err != nil {
		return fmt.Errorf("falha ao gerar mudanças de schema: %w", err)
	}

	for _, change := range changes {
		if err := m.applyChange(ctx, change); err != nil {
			return fmt.Errorf("falha ao aplicar mudança %s: %w", change.Type, err)
		}
	}

	return nil
}

// createMigrationsTable cria a tabela de controle de migrações
func (m *Migrator) createMigrationsTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS _migrations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version TEXT NOT NULL,
		checksum TEXT NOT NULL,
		description TEXT,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		success BOOLEAN DEFAULT TRUE
	)`

	_, err := m.db.Exec(query)
	return err
}

// generateSchemaChanges gera todas as mudanças necessárias baseadas no manifesto
func (m *Migrator) generateSchemaChanges() ([]SchemaChange, error) {
	var changes []SchemaChange

	existingTables, err := m.getExistingTables()
	if err != nil {
		return nil, err
	}

	tableSet := make(map[string]bool)
	for _, t := range existingTables {
		tableSet[t] = true
	}

	// Processar entidades do manifesto
	for _, entity := range m.manifest.DataModel.Entities {
		tableName := m.formatTableName(entity.Name)
		tableSet[tableName] = true

		if !m.tableExists(tableName) {
			// Criar tabela nova
			change, err := m.generateCreateTable(entity)
			if err != nil {
				return nil, err
			}
			changes = append(changes, change)
		} else {
			// Verificar colunas existentes
			columnChanges, err := m.generateAlterTable(entity, tableName)
			if err != nil {
				return nil, err
			}
			changes = append(changes, columnChanges...)
		}

		// Gerar índices
		indexChanges := m.generateIndexes(entity, tableName)
		changes = append(changes, indexChanges...)
	}

	return changes, nil
}

// getExistingTables retorna lista de tabelas existentes
func (m *Migrator) getExistingTables() ([]string, error) {
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'"
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}

	return tables, rows.Err()
}

// tableExists verifica se uma tabela existe
func (m *Migrator) tableExists(name string) bool {
	query := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?"
	var count int
	err := m.db.QueryRow(query, name).Scan(&count)
	return err == nil && count > 0
}

// generateCreateTable gera SQL para criar uma tabela
func (m *Migrator) generateCreateTable(entity manifest.Entity) (SchemaChange, error) {
	tableName := m.formatTableName(entity.Name)
	
	var columns []string
	
	// Adicionar ID padrão
	columns = append(columns, "id INTEGER PRIMARY KEY AUTOINCREMENT")
	
	// Adicionar campos definidos
	for _, field := range entity.Fields {
		colDef, err := m.fieldToColumn(field)
		if err != nil {
			return SchemaChange{}, err
		}
		columns = append(columns, colDef)
	}

	// Adicionar timestamps padrão
	columns = append(columns, "created_at DATETIME DEFAULT CURRENT_TIMESTAMP")
	columns = append(columns, "updated_at DATETIME DEFAULT CURRENT_TIMESTAMP")

	sql := fmt.Sprintf("CREATE TABLE %s (%s)", tableName, strings.Join(columns, ", "))

	return SchemaChange{
		Type:       "CREATE_TABLE",
		Table:      tableName,
		EntityName: entity.Name,
		SQL:        sql,
	}, nil
}

// fieldToColumn converte campo do manifesto para definição de coluna SQL
func (m *Migrator) fieldToColumn(field manifest.Field) (string, error) {
	colName := m.formatColumnName(field.Name)
	colType, err := m.mapFieldType(field.Type)
	if err != nil {
		return "", err
	}

	constraints := ""
	if field.Required {
		constraints += " NOT NULL"
	}
	if field.Unique {
		constraints += " UNIQUE"
	}

	return fmt.Sprintf("%s %s%s", colName, colType, constraints), nil
}

// mapFieldType mapeia tipos do manifesto para tipos SQL
func (m *Migrator) mapFieldType(fieldType string) (string, error) {
	switch strings.ToLower(fieldType) {
	case "string", "text":
		return "TEXT", nil
	case "integer", "int":
		return "INTEGER", nil
	case "float", "number", "decimal":
		return "REAL", nil
	case "boolean", "bool":
		return "BOOLEAN", nil
	case "datetime", "timestamp", "date":
		return "DATETIME", nil
	case "json":
		return "JSON", nil
	default:
		return "TEXT", nil // Default para TEXT
	}
}

// generateAlterTable gera alterações em tabela existente
func (m *Migrator) generateAlterTable(entity manifest.Entity, tableName string) ([]SchemaChange, error) {
	var changes []SchemaChange

	existingColumns, err := m.getTableColumns(tableName)
	if err != nil {
		return nil, err
	}

	columnSet := make(map[string]bool)
	for _, col := range existingColumns {
		columnSet[col] = true
	}

	// Adicionar novas colunas
	for _, field := range entity.Fields {
		colName := m.formatColumnName(field.Name)
		if !columnSet[colName] {
			colDef, err := m.fieldToColumn(field)
			if err != nil {
				return nil, err
			}
			
			sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", tableName, colDef)
			changes = append(changes, SchemaChange{
				Type:       "ADD_COLUMN",
				Table:      tableName,
				Column:     colName,
				EntityName: entity.Name,
				FieldName:  field.Name,
				SQL:        sql,
			})
		}
	}

	return changes, nil
}

// getTableColumns retorna colunas existentes de uma tabela
func (m *Migrator) getTableColumns(tableName string) ([]string, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notnull int
		var dflt interface{}
		var pk int
		
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}

	return columns, rows.Err()
}

// generateIndexes gera índices para a entidade
func (m *Migrator) generateIndexes(entity manifest.Entity, tableName string) []SchemaChange {
	var changes []SchemaChange

	// Verificar índices definidos na entidade
	for _, idx := range entity.Indexes {
		idxName := idx.Name
		if idxName == "" {
			idxName = fmt.Sprintf("idx_%s_%s", tableName, strings.Join(idx.Fields, "_"))
		}
		
		fields := make([]string, len(idx.Fields))
		for i, f := range idx.Fields {
			fields[i] = m.formatColumnName(f)
		}
		
		sql := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)", idxName, tableName, strings.Join(fields, ", "))
		changes = append(changes, SchemaChange{
			Type:       "CREATE_INDEX",
			Table:      tableName,
			EntityName: entity.Name,
			SQL:        sql,
		})
	}

	return changes
}

// applyChange aplica uma mudança de schema
func (m *Migrator) applyChange(ctx context.Context, change SchemaChange) error {
	// Calcular checksum
	checksum := m.calculateChecksum(change.SQL)

	// Verificar se já foi aplicada
	if m.migrationExists(checksum) {
		return nil
	}

	// DBInterface com ExecContext
	type DBWithContext interface {
		DBInterface
		ExecContext(ctx context.Context, query string, args ...interface{}) (ResultInterface, error)
	}

	// Verificar se db suporta ExecContext
	dbWithCtx, ok := m.db.(DBWithContext)
	if !ok {
		// Fallback para Exec normal
		_, err := m.db.Exec(change.SQL)
		success := err == nil
		description := m.generateChangeDescription(change)
		record := MigrationRecord{
			Checksum:    checksum,
			Version:     time.Now().Format("20060102150405"),
			Description: description,
			AppliedAt:   time.Now(),
			Success:     success,
		}
		if err := m.recordMigration(record); err != nil {
			return fmt.Errorf("falha ao registrar migração: %w", err)
		}
		if !success {
			return fmt.Errorf("migração falhou: %s", change.SQL)
		}
		m.migrationLog = append(m.migrationLog, record)
		return nil
	}

	// Executar SQL com contexto
	_, err := dbWithCtx.ExecContext(ctx, change.SQL)
	success := err == nil

	// Gerar descrição da mudança
	description := m.generateChangeDescription(change)

	// Registrar migração
	record := MigrationRecord{
		Checksum:    checksum,
		Version:     time.Now().Format("20060102150405"),
		Description: description,
		AppliedAt:   time.Now(),
		Success:     success,
	}

	if err := m.recordMigration(record); err != nil {
		return fmt.Errorf("falha ao registrar migração: %w", err)
	}

	if !success {
		return fmt.Errorf("migração falhou: %s", change.SQL)
	}

	m.migrationLog = append(m.migrationLog, record)
	return nil
}

// calculateChecksum calcula checksum de uma migration
func (m *Migrator) calculateChecksum(sql string) string {
	hash := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(hash[:])
}

// migrationExists verifica se migration já foi aplicada
func (m *Migrator) migrationExists(checksum string) bool {
	query := "SELECT COUNT(*) FROM _migrations WHERE checksum=? AND success=TRUE"
	var count int
	err := m.db.QueryRow(query, checksum).Scan(&count)
	return err == nil && count > 0
}

// recordMigration registra uma migração no banco
func (m *Migrator) recordMigration(record MigrationRecord) error {
	query := `INSERT INTO _migrations (version, checksum, description, applied_at, success) 
			  VALUES (?, ?, ?, ?, ?)`
	_, err := m.db.Exec(query, record.Version, record.Checksum, record.Description, record.AppliedAt, record.Success)
	return err
}

// formatTableName formata nome de tabela para padrão snake_case
func (m *Migrator) formatTableName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
}

// formatColumnName formata nome de coluna para padrão snake_case
func (m *Migrator) formatColumnName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
}

// GetMigrationLog retorna log de migrações aplicadas
func (m *Migrator) GetMigrationLog() []MigrationRecord {
	return m.migrationLog
}

// ValidateSchema valida se schema atual corresponde ao manifesto
func (m *Migrator) ValidateSchema() error {
	changes, err := m.generateSchemaChanges()
	if err != nil {
		return err
	}

	if len(changes) > 0 {
		var descriptions []string
		for _, c := range changes {
			descriptions = append(descriptions, m.generateChangeDescription(c))
		}
		return fmt.Errorf("schema desatualizado: %d mudanças pendentes: %s", len(changes), strings.Join(descriptions, "; "))
	}

	return nil
}

// Rollback reverte última migração (implementação simplificada)
func (m *Migrator) Rollback(ctx context.Context) error {
	if len(m.migrationLog) == 0 {
		return fmt.Errorf("nenhuma migração para reverter")
	}

	last := m.migrationLog[len(m.migrationLog)-1]
	
	// Implementação básica - em produção seria necessário mapear rollback de cada operação
	return fmt.Errorf("rollback não implementado para migração: %s", last.Description)
}

// generateChangeDescription gera descrição legível da mudança
func (m *Migrator) generateChangeDescription(change SchemaChange) string {
	switch change.Type {
	case "CREATE_TABLE":
		return fmt.Sprintf("Criar tabela %s para entidade %s", change.Table, change.EntityName)
	case "ADD_COLUMN":
		return fmt.Sprintf("Adicionar coluna %s à tabela %s", change.Column, change.Table)
	case "CREATE_INDEX":
		return fmt.Sprintf("Criar índice na tabela %s", change.Table)
	default:
		return fmt.Sprintf("%s na tabela %s", change.Type, change.Table)
	}
}

// SortChanges ordena mudanças para execução correta
func SortChanges(changes []SchemaChange) []SchemaChange {
	// Ordenar: CREATE_TABLE primeiro, depois ADD_COLUMN, depois CREATE_INDEX
	sort.Slice(changes, func(i, j int) bool {
		order := map[string]int{"CREATE_TABLE": 1, "ADD_COLUMN": 2, "CREATE_INDEX": 3}
		return order[changes[i].Type] < order[changes[j].Type]
	})
	return changes
}
