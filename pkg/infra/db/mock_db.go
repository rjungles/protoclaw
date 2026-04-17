package db

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ResultInterface define interface para resultado
type ResultInterface interface {
	LastInsertId() (int64, error)
	RowsAffected() (int64, error)
}

// mockResultWrapper adapta ResultInterface para sql.Result
type mockResultWrapper struct {
	ResultInterface
}

func (w *mockResultWrapper) LastInsertId() (int64, error) {
	return w.ResultInterface.LastInsertId()
}

func (w *mockResultWrapper) RowsAffected() (int64, error) {
	return w.ResultInterface.RowsAffected()
}

// MockDB é um banco de dados em memória para testes
type MockDB struct {
	mu         sync.Mutex
	tables     map[string]*MockTable
	migrations []*MigrationRecord
}

// MockTable representa uma tabela em memória
type MockTable struct {
	name    string
	columns []string
	rows    [][]interface{}
	indexes []string
}

// NewMockDB cria um novo banco de dados mock
func NewMockDB() *MockDB {
	return &MockDB{
		tables: make(map[string]*MockTable),
		migrations: make([]*MigrationRecord, 0),
	}
}

// Exec executa uma query SQL no mock
func (m *MockDB) Exec(query string, args ...interface{}) (ResultInterface, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	query = strings.TrimSpace(query)
	
	// CREATE TABLE
	if strings.HasPrefix(strings.ToUpper(query), "CREATE TABLE") {
		return m.handleCreateTable(query)
	}
	
	// ALTER TABLE
	if strings.HasPrefix(strings.ToUpper(query), "ALTER TABLE") {
		return m.handleAlterTable(query)
	}
	
	// CREATE INDEX
	if strings.HasPrefix(strings.ToUpper(query), "CREATE INDEX") {
		return m.handleCreateIndex(query)
	}
	
	// INSERT INTO _migrations
	if strings.Contains(strings.ToUpper(query), "INSERT INTO") && strings.Contains(query, "_migrations") {
		return m.handleInsertMigration(query, args)
	}
	
	// SELECT FROM sqlite_master
	if strings.Contains(query, "sqlite_master") {
		return m.handleSqliteMaster(query)
	}
	
	// PRAGMA table_info
	if strings.HasPrefix(strings.ToUpper(query), "PRAGMA TABLE_INFO") {
		return m.handlePragmaTableInfo(query)
	}
	
	// SELECT FROM _migrations
	if strings.Contains(query, "_migrations") && strings.HasPrefix(strings.ToUpper(query), "SELECT") {
		return m.handleSelectMigrations(query, args)
	}

	return &mockResult{}, nil
}

// Query executa uma query SELECT no mock
func (m *MockDB) Query(query string, args ...interface{}) (RowsInterface, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// SELECT FROM sqlite_master
	if strings.Contains(query, "sqlite_master") {
		return m.handleSqliteMasterQuery(query)
	}
	
	// PRAGMA table_info
	if strings.HasPrefix(strings.ToUpper(query), "PRAGMA TABLE_INFO") {
		return m.handlePragmaTableInfoQuery(query)
	}
	
	// SELECT FROM _migrations
	if strings.Contains(query, "_migrations") {
		return m.handleSelectMigrationsQuery(query, args)
	}

	return &mockRows{closed: true}, nil
}

// QueryRow executa uma query que retorna uma linha
func (m *MockDB) QueryRow(query string, args ...interface{}) RowInterface {
	m.mu.Lock()
	defer m.mu.Unlock()

	// SELECT COUNT FROM sqlite_master
	if strings.Contains(query, "sqlite_master") && strings.Contains(query, "COUNT") {
		tableName := extractTableName(query)
		exists := 0
		if _, ok := m.tables[tableName]; ok {
			exists = 1
		}
		return &mockRow{value: exists}
	}
	
	// SELECT COUNT FROM _migrations
	if strings.Contains(query, "_migrations") && strings.Contains(query, "COUNT") {
		count := len(m.migrations)
		return &mockRow{value: count}
	}

	return &mockRow{}
}

// ExecContext executa uma query com contexto (wrapper para Exec)
func (m *MockDB) ExecContext(ctx context.Context, query string, args ...interface{}) (ResultInterface, error) {
	return m.Exec(query, args...)
}

func (m *MockDB) handleCreateTable(query string) (*mockResult, error) {
	// Extrair nome da tabela
	parts := strings.Fields(query)
	if len(parts) < 3 {
		return nil, fmt.Errorf("CREATE TABLE inválido")
	}
	
	tableName := parts[2]
	
	// Remover parênteses e conteúdo
	start := strings.Index(query, "(")
	end := strings.LastIndex(query, ")")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("sintaxe CREATE TABLE inválida")
	}
	
	columnsStr := query[start+1 : end]
	columns := parseColumns(columnsStr)
	
	m.tables[tableName] = &MockTable{
		name:    tableName,
		columns: columns,
		rows:    make([][]interface{}, 0),
		indexes: make([]string, 0),
	}
	
	return &mockResult{}, nil
}

func (m *MockDB) handleAlterTable(query string) (*mockResult, error) {
	// ALTER TABLE name ADD COLUMN col_def
	parts := strings.Fields(query)
	if len(parts) < 5 {
		return nil, fmt.Errorf("ALTER TABLE inválido")
	}
	
	tableName := parts[2]
	table, ok := m.tables[tableName]
	if !ok {
		return nil, fmt.Errorf("tabela %s não existe", tableName)
	}
	
	// Extrair definição da coluna
	addIdx := findKeyword(parts, "ADD")
	if addIdx == -1 {
		return nil, fmt.Errorf("ADD não encontrado")
	}
	
	colDef := strings.Join(parts[addIdx+1:], " ")
	colName := strings.Fields(colDef)[0]
	
	// Adicionar coluna
	table.columns = append(table.columns, colName)
	
	return &mockResult{}, nil
}

func (m *MockDB) handleCreateIndex(query string) (*mockResult, error) {
	// CREATE INDEX IF NOT EXISTS name ON table (cols)
	parts := strings.Fields(query)
	if len(parts) < 6 {
		return nil, fmt.Errorf("CREATE INDEX inválido")
	}
	
	idxName := parts[2]
	if strings.ToUpper(idxName) == "IF" {
		idxName = parts[4]
	}
	
	// Encontrar nome da tabela
	onIdx := findKeyword(parts, "ON")
	if onIdx == -1 {
		return nil, fmt.Errorf("ON não encontrado em CREATE INDEX")
	}
	
	tableName := strings.Trim(parts[onIdx+1], "()")
	table, ok := m.tables[tableName]
	if !ok {
		// Tabela pode não existir ainda no mock, ignorar
		return &mockResult{}, nil
	}
	
	table.indexes = append(table.indexes, idxName)
	
	return &mockResult{}, nil
}

func (m *MockDB) handleInsertMigration(query string, args []interface{}) (*mockResult, error) {
	record := &MigrationRecord{
		ID:          len(m.migrations) + 1,
		Version:     getStringArg(args, 0),
		Checksum:    getStringArg(args, 1),
		Description: getStringArg(args, 2),
		AppliedAt:   time.Now(),
		Success:     getBoolArg(args, 4, true),
	}
	
	m.migrations = append(m.migrations, record)
	
	return &mockResult{}, nil
}

func (m *MockDB) handleSqliteMaster(query string) (*mockResult, error) {
	return &mockResult{}, nil
}

func (m *MockDB) handleSqliteMasterQuery(query string) (*mockRows, error) {
	var values []string
	for name := range m.tables {
		values = append(values, name)
	}
	return &mockRows{values: values, pos: 0}, nil
}

func (m *MockDB) handlePragmaTableInfo(query string) (*mockResult, error) {
	return &mockResult{}, nil
}

func (m *MockDB) handlePragmaTableInfoQuery(query string) (*mockRows, error) {
	// PRAGMA table_info(table_name)
	start := strings.Index(query, "(")
	end := strings.Index(query, ")")
	if start == -1 || end == -1 {
		return &mockRows{closed: true}, nil
	}
	
	tableName := query[start+1 : end]
	table, ok := m.tables[tableName]
	if !ok {
		return &mockRows{closed: true}, nil
	}
	
	// Retornar informações das colunas
	var values []interface{}
	for i, col := range table.columns {
		// cid, name, type, notnull, dflt_value, pk
		values = append(values, []interface{}{i, col, "TEXT", 0, nil, 0})
	}
	
	return &mockRows{interfaceValues: values, pos: 0}, nil
}

func (m *MockDB) handleSelectMigrations(query string, args []interface{}) (*mockResult, error) {
	return &mockResult{}, nil
}

func (m *MockDB) handleSelectMigrationsQuery(query string, args []interface{}) (*mockRows, error) {
	if len(m.migrations) == 0 {
		return &mockRows{closed: true}, nil
	}
	
	// Verificar se tem WHERE checksum=?
	if strings.Contains(query, "checksum=?") && len(args) > 0 {
		checksum := args[0].(string)
		for _, mig := range m.migrations {
			if mig.Checksum == checksum && mig.Success {
				return &mockRows{values: []string{"1"}, pos: 0}, nil
			}
		}
		return &mockRows{closed: true}, nil
	}
	
	var values []string
	for _, mig := range m.migrations {
		values = append(values, mig.Checksum)
	}
	return &mockRows{values: values, pos: 0}, nil
}

// Helper functions
func parseColumns(colsStr string) []string {
	var columns []string
	parts := strings.Split(colsStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) > 0 {
			columns = append(columns, fields[0])
		}
	}
	return columns
}

func findKeyword(parts []string, keyword string) int {
	for i, p := range parts {
		if strings.ToUpper(p) == keyword {
			return i
		}
	}
	return -1
}

func getStringArg(args []interface{}, idx int) string {
	if idx < len(args) {
		if s, ok := args[idx].(string); ok {
			return s
		}
	}
	return ""
}

func getBoolArg(args []interface{}, idx int, defaultVal bool) bool {
	if idx < len(args) {
		if b, ok := args[idx].(bool); ok {
			return b
		}
	}
	return defaultVal
}

func extractTableName(query string) string {
	// Simplificado: procurar por WHERE name=?
	parts := strings.Fields(query)
	for i, p := range parts {
		if p == "name=?" && i > 0 {
			return strings.Trim(parts[i-1], "'\"")
		}
	}
	return ""
}

// Mock types
type mockResult struct{}

func (r *mockResult) LastInsertId() (int64, error) { return 1, nil }
func (r *mockResult) RowsAffected() (int64, error) { return 1, nil }

type mockRows struct {
	values          []string
	interfaceValues []interface{}
	pos             int
	closed          bool
}

func (r *mockRows) Next() bool {
	if r.closed {
		return false
	}
	if len(r.values) > 0 {
		return r.pos < len(r.values)
	}
	if len(r.interfaceValues) > 0 {
		return r.pos < len(r.interfaceValues)
	}
	return false
}

func (r *mockRows) Scan(dest ...interface{}) error {
	if len(r.values) > 0 && r.pos < len(r.values) {
		if d, ok := dest[0].(*string); ok {
			*d = r.values[r.pos]
			r.pos++
			return nil
		}
	}
	
	if len(r.interfaceValues) > 0 && r.pos < len(r.interfaceValues) {
		if row, ok := r.interfaceValues[r.pos].([]interface{}); ok {
			for i, d := range dest {
				if i < len(row) {
					switch dv := d.(type) {
					case *int:
						if v, ok := row[i].(int); ok {
							*dv = v
						}
					case *string:
						if v, ok := row[i].(string); ok {
							*dv = v
						}
					case *interface{}:
						*dv = row[i]
					}
				}
			}
			r.pos++
			return nil
		}
	}
	
	return fmt.Errorf("no more rows")
}

func (r *mockRows) Close() error {
	r.closed = true
	return nil
}

func (r *mockRows) Err() error { return nil }

type mockRow struct {
	value interface{}
	err   error
}

func (r *mockRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) > 0 {
		switch d := dest[0].(type) {
		case *int:
			if v, ok := r.value.(int); ok {
				*d = v
			}
		case *string:
			if v, ok := r.value.(string); ok {
				*d = v
			}
		}
	}
	return nil
}

// Implement interfaces para sql.DB

// GetTableColumns retorna colunas de uma tabela
func (m *MockDB) GetTableColumns(tableName string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if table, ok := m.tables[tableName]; ok {
		return table.columns
	}
	return nil
}

// ListTables retorna lista de tabelas
func (m *MockDB) ListTables() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	var tables []string
	for name := range m.tables {
		tables = append(tables, name)
	}
	return tables
}

// GetMigrations retorna migrações aplicadas
func (m *MockDB) GetMigrations() []*MigrationRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.migrations
}
// Adapter para sql.DB será adicionado posteriormente
