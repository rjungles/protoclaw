package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
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
	Exec(query string, args ...interface{}) (ResultInterface, error)
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

	changes = SortChanges(changes)
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

func (m *Migrator) indexExists(name string) bool {
	query := "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?"
	var count int
	err := m.db.QueryRow(query, name).Scan(&count)
	return err == nil && count > 0
}

// reservedKeywords contains SQL reserved keywords that need quoting
var reservedKeywords = map[string]bool{
	"transaction": true, "select": true, "insert": true, "update": true, "delete": true,
	"create": true, "drop": true, "alter": true, "table": true, "index": true,
	"view": true, "trigger": true, "pragma": true, "begin": true, "commit": true,
	"rollback": true, "savepoint": true, "release": true, "where": true, "from": true,
	"join": true, "inner": true, "outer": true, "left": true, "right": true,
	"group": true, "order": true, "by": true, "having": true, "limit": true,
	"offset": true, "union": true, "intersect": true, "except": true, "distinct": true,
	"all": true, "values": true, "set": true, "into": true, "as": true, "on": true,
	"and": true, "or": true, "not": true, "null": true, "is": true, "in": true,
	"between": true, "like": true, "exists": true, "case": true, "when": true,
	"then": true, "else": true, "end": true, "cast": true, "collate": true,
}

// isReservedKeyword checks if a word is a SQL reserved keyword
func isReservedKeyword(word string) bool {
	return reservedKeywords[strings.ToLower(word)]
}

// quoteIdentifier quotes an identifier if it's a reserved keyword
func quoteIdentifier(name string) string {
	if isReservedKeyword(name) {
		return fmt.Sprintf(`"%s"`, name)
	}
	return name
}

// generateCreateTable gera SQL para criar uma tabela
func (m *Migrator) generateCreateTable(entity manifest.Entity) (SchemaChange, error) {
	tableName := m.formatTableName(entity.Name)
	quotedTableName := quoteIdentifier(tableName)

	var columns []string
	var tableConstraints []string

	pkCol := m.detectPrimaryKeyColumn(entity)

	for _, field := range entity.Fields {
		colDef, err := m.fieldToColumn(entity, field, pkCol)
		if err != nil {
			return SchemaChange{}, err
		}
		columns = append(columns, colDef)
	}

	for _, c := range entity.Constraints {
		sql, err := m.constraintToSQL(entity, c)
		if err != nil {
			return SchemaChange{}, err
		}
		if sql != "" {
			tableConstraints = append(tableConstraints, sql)
		}
	}

	defs := make([]string, 0, len(columns)+len(tableConstraints))
	defs = append(defs, columns...)
	defs = append(defs, tableConstraints...)
	sql := fmt.Sprintf("CREATE TABLE %s (%s)", quotedTableName, strings.Join(defs, ", "))

	return SchemaChange{
		Type:       "CREATE_TABLE",
		Table:      tableName,
		EntityName: entity.Name,
		SQL:        sql,
	}, nil
}

// fieldToColumn converte campo do manifesto para definição de coluna SQL
func (m *Migrator) fieldToColumn(entity manifest.Entity, field manifest.Field, pkCol string) (string, error) {
	colName := m.formatColumnName(field.Name)
	colType, err := m.mapFieldType(field)
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

	if colName == pkCol {
		constraints += " PRIMARY KEY"
	}

	if field.Default != nil {
		lit, ok := sqlLiteral(field.Default)
		if !ok {
			return "", fmt.Errorf("default inválido para %s.%s", entity.Name, field.Name)
		}
		constraints += " DEFAULT " + lit
	}

	if field.Reference != nil {
		refEntity := m.formatTableName(field.Reference.Entity)
		quotedRefEntity := quoteIdentifier(refEntity)
		refField := m.formatColumnName(field.Reference.Field)
		constraints += fmt.Sprintf(" REFERENCES %s(%s)%s", quotedRefEntity, refField, onDeleteClause(field.Reference.OnDelete))
	}

	return fmt.Sprintf("%s %s%s", colName, colType, constraints), nil
}

// mapFieldType mapeia tipos do manifesto para tipos SQL
func (m *Migrator) mapFieldType(field manifest.Field) (string, error) {
	switch strings.ToLower(field.Type) {
	case "string", "text":
		if field.MaxLength != nil && *field.MaxLength > 0 {
			return fmt.Sprintf("VARCHAR(%d)", *field.MaxLength), nil
		}
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
	case "reference":
		if field.Reference == nil {
			return "TEXT", nil
		}
		refField, ok := m.findField(field.Reference.Entity, field.Reference.Field)
		if !ok {
			return "TEXT", nil
		}
		return m.mapFieldType(refField)
	default:
		return "TEXT", nil
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

	pkCol := m.detectPrimaryKeyColumn(entity)

	// Adicionar novas colunas
	for _, field := range entity.Fields {
		colName := m.formatColumnName(field.Name)
		if !columnSet[colName] {
			colDef, err := m.fieldToColumn(entity, field, pkCol)
			if err != nil {
				return nil, err
			}

		quotedTableName := quoteIdentifier(tableName)
		sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", quotedTableName, colDef)
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
		if m.indexExists(idxName) {
			continue
		}

		fields := make([]string, len(idx.Fields))
		for i, f := range idx.Fields {
			fields[i] = m.formatColumnName(f)
		}

		create := "CREATE INDEX"
		if idx.Unique {
			create = "CREATE UNIQUE INDEX"
		}
		quotedTableName := quoteIdentifier(tableName)
		sql := fmt.Sprintf("%s IF NOT EXISTS %s ON %s (%s)", create, idxName, quotedTableName, strings.Join(fields, ", "))
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
	return toSnakeCase(name)
}

// formatColumnName formata nome de coluna para padrão snake_case
func (m *Migrator) formatColumnName(name string) string {
	return toSnakeCase(name)
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
	// Separate CREATE_TABLE changes from others
	var createTableChanges []SchemaChange
	var otherChanges []SchemaChange

	for _, change := range changes {
		if change.Type == "CREATE_TABLE" {
			createTableChanges = append(createTableChanges, change)
		} else {
			otherChanges = append(otherChanges, change)
		}
	}

	// If no CREATE_TABLE changes, return original
	if len(createTableChanges) == 0 {
		return changes
	}

	// Build dependency graph using normalized (lowercase) table names
	// Table -> list of tables it depends on (references)
	dependsOn := make(map[string][]string)

	// Compile regex for finding REFERENCES (case insensitive)
	// Matches: REFERENCES table( or REFERENCES table ( or REFERENCES "table"(
	refRegex := regexp.MustCompile(`(?i)REFERENCES\s+(?:"([^"]+)"|(\w+))\s*\(`)

	for _, change := range createTableChanges {
		tableNameLower := strings.ToLower(change.Table)
		// Find all references in the SQL
		matches := refRegex.FindAllStringSubmatch(change.SQL, -1)
		for _, match := range matches {
			if len(match) > 1 {
				// Group 1 is quoted name, group 2 is unquoted name
				var refTable string
				if match[1] != "" {
					refTable = strings.ToLower(match[1])
				} else if len(match) > 2 && match[2] != "" {
					refTable = strings.ToLower(match[2])
				}
				if refTable != "" && refTable != tableNameLower {
					dependsOn[tableNameLower] = append(dependsOn[tableNameLower], refTable)
				}
			}
		}
	}

	// Topological sort using Kahn's algorithm
	// Calculate in-degree for each table using normalized names
	inDegree := make(map[string]int)
	tableMap := make(map[string]SchemaChange) // normalized name -> original change
	for _, change := range createTableChanges {
		nameLower := strings.ToLower(change.Table)
		inDegree[nameLower] = 0
		tableMap[nameLower] = change
	}

	// Calculate in-degrees (number of dependencies not yet resolved)
	for table, deps := range dependsOn {
		for _, dep := range deps {
			// Only count if the dependency is also a table being created
			if _, exists := tableMap[dep]; exists {
				inDegree[table]++
			}
		}
	}

	// Find all tables with in-degree 0
	var queue []string
	for nameLower := range tableMap {
		if inDegree[nameLower] == 0 {
			queue = append(queue, nameLower)
		}
	}

	// Process tables in topological order
	var sortedTables []string
	for len(queue) > 0 {
		// Take first from queue
		table := queue[0]
		queue = queue[1:]
		sortedTables = append(sortedTables, table)

		// Reduce in-degree for tables that depend on this one
		for otherTable, deps := range dependsOn {
			for _, dep := range deps {
				if dep == table {
					inDegree[otherTable]--
					if inDegree[otherTable] == 0 {
						queue = append(queue, otherTable)
					}
				}
			}
		}
	}

	// Check for cycles
	if len(sortedTables) != len(createTableChanges) {
		// Cycle detected, return original order
		return changes
	}

	// Build result in sorted order - tables with no dependencies come first
	var result []SchemaChange
	for _, tableNameLower := range sortedTables {
		if change, exists := tableMap[tableNameLower]; exists {
			result = append(result, change)
		}
	}

	// Add other changes
	result = append(result, otherChanges...)

	return result
}

func (m *Migrator) detectPrimaryKeyColumn(entity manifest.Entity) string {
	for _, f := range entity.Fields {
		if strings.EqualFold(f.Name, "id") && f.Unique {
			return m.formatColumnName(f.Name)
		}
	}
	return ""
}

func (m *Migrator) constraintToSQL(entity manifest.Entity, c manifest.Constraint) (string, error) {
	switch strings.ToLower(c.Type) {
	case "unique":
		if len(c.Fields) == 0 {
			return "", fmt.Errorf("constraint unique sem fields em %s", entity.Name)
		}
		cols := make([]string, 0, len(c.Fields))
		for _, f := range c.Fields {
			cols = append(cols, m.formatColumnName(f))
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return fmt.Sprintf("UNIQUE (%s)", strings.Join(cols, ", ")), nil
		}
		return fmt.Sprintf("CONSTRAINT %s UNIQUE (%s)", name, strings.Join(cols, ", ")), nil
	case "check":
		expr := strings.TrimSpace(c.Expression)
		if expr == "" {
			return "", fmt.Errorf("constraint check sem expression em %s", entity.Name)
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return fmt.Sprintf("CHECK (%s)", expr), nil
		}
		return fmt.Sprintf("CONSTRAINT %s CHECK (%s)", name, expr), nil
	case "foreign_key":
		if len(c.Fields) == 0 || strings.TrimSpace(c.Expression) == "" {
			return "", fmt.Errorf("constraint foreign_key inválida em %s", entity.Name)
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return c.Expression, nil
		}
		return fmt.Sprintf("CONSTRAINT %s %s", name, c.Expression), nil
	default:
		return "", fmt.Errorf("constraint type inválido %q em %s", c.Type, entity.Name)
	}
}

func (m *Migrator) findField(entityName, fieldName string) (manifest.Field, bool) {
	for _, e := range m.manifest.DataModel.Entities {
		if e.Name == entityName {
			for _, f := range e.Fields {
				if f.Name == fieldName {
					return f, true
				}
			}
			return manifest.Field{}, false
		}
	}
	return manifest.Field{}, false
}

func onDeleteClause(onDelete string) string {
	switch strings.ToLower(strings.TrimSpace(onDelete)) {
	case "":
		return ""
	case "cascade":
		return " ON DELETE CASCADE"
	case "set_null":
		return " ON DELETE SET NULL"
	case "restrict":
		return " ON DELETE RESTRICT"
	default:
		return ""
	}
}

func sqlLiteral(v interface{}) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "NULL", true
	case string:
		lower := strings.ToLower(t)
		if lower == "now()" || lower == "now" || lower == "current_timestamp" {
			return "CURRENT_TIMESTAMP", true
		}
		if lower == "null" {
			return "NULL", true
		}
		if lower == "true" {
			return "TRUE", true
		}
		if lower == "false" {
			return "FALSE", true
		}
		return "'" + strings.ReplaceAll(t, "'", "''") + "'", true
	case bool:
		if t {
			return "TRUE", true
		}
		return "FALSE", true
	case int:
		return strconv.FormatInt(int64(t), 10), true
	case int8:
		return strconv.FormatInt(int64(t), 10), true
	case int16:
		return strconv.FormatInt(int64(t), 10), true
	case int32:
		return strconv.FormatInt(int64(t), 10), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case uint:
		return strconv.FormatUint(uint64(t), 10), true
	case uint8:
		return strconv.FormatUint(uint64(t), 10), true
	case uint16:
		return strconv.FormatUint(uint64(t), 10), true
	case uint32:
		return strconv.FormatUint(uint64(t), 10), true
	case uint64:
		return strconv.FormatUint(t, 10), true
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	default:
		return "", false
	}
}

func toSnakeCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s) + 8)

	prevUnderscore := false
	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == ' ' || ch == '-' {
			if !prevUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				prevUnderscore = true
			}
			continue
		}

		isUpper := ch >= 'A' && ch <= 'Z'
		isLower := ch >= 'a' && ch <= 'z'
		isDigit := ch >= '0' && ch <= '9'

		if isUpper {
			if b.Len() > 0 && !prevUnderscore {
				prev := s[i-1]
				prevIsLower := prev >= 'a' && prev <= 'z'
				prevIsDigit := prev >= '0' && prev <= '9'
				nextIsLower := false
				if i+1 < len(s) {
					next := s[i+1]
					nextIsLower = next >= 'a' && next <= 'z'
				}
				if prevIsLower || prevIsDigit || nextIsLower {
					b.WriteByte('_')
				}
			}
			b.WriteByte(ch + ('a' - 'A'))
			prevUnderscore = false
			continue
		}

		if isLower || isDigit {
			b.WriteByte(ch)
			prevUnderscore = false
			continue
		}

		if !prevUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}

	out := b.String()
	out = strings.Trim(out, "_")
	return out
}
