package db

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	tmpFile := "/tmp/test_migrator_" + time.Now().Format("20060102150405") + ".db"
	
	db, err := sql.Open("sqlite3", tmpFile)
	if err != nil {
		t.Fatalf("Falha ao abrir banco de dados: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(tmpFile)
	}

	return db, cleanup
}

func loadManifest(t *testing.T, path string) *manifest.Manifest {
	m, err := manifest.LoadFromFile(path)
	if err != nil {
		t.Fatalf("Falha ao carregar manifesto: %v", err)
	}
	return m
}

func TestMigrator_CafeteriaLoyalty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := loadManifest(t, "../../examples/manifests/cafeteria-loyalty.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	// Verificar se tabelas foram criadas
	tables := []string{"customer", "loyalty_account", "transaction", "reward", "promotion", "partner"}
	for _, table := range tables {
		if !migrator.tableExists(table) {
			t.Errorf("Tabela %s não foi criada", table)
		}
	}

	// Verificar log de migrações
	log := migrator.GetMigrationLog()
	if len(log) == 0 {
		t.Error("Nenhuma migração foi registrada")
	}

	t.Logf("Migrações aplicadas: %d", len(log))
	for _, record := range log {
		t.Logf("  - %s: %s", record.Type, record.Description)
	}
}

func TestMigrator_ParkingTicket(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := loadManifest(t, "../../examples/manifests/parking-ticket.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	// Verificar tabelas principais
	tables := []string{"vehicle", "ticket", "payment", "zone", "device", "operator", "shift"}
	for _, table := range tables {
		if !migrator.tableExists(table) {
			t.Errorf("Tabela %s não foi criada", table)
		}
	}

	log := migrator.GetMigrationLog()
	t.Logf("Migrações aplicadas: %d", len(log))
}

func TestMigrator_ExperiencePlatform(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := loadManifest(t, "../../examples/manifests/experience-platform.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	// Verificar tabela experience
	if !migrator.tableExists("experience") {
		t.Error("Tabela experience não foi criada")
	}

	// Verificar colunas da tabela experience
	cols, err := migrator.getTableColumns("experience")
	if err != nil {
		t.Fatalf("Falha ao obter colunas: %v", err)
	}

	expectedCols := []string{"id", "title", "raw_content", "final_narrative", "status", "author_id", "editor_id", "created_at", "updated_at"}
	colSet := make(map[string]bool)
	for _, c := range cols {
		colSet[c] = true
	}

	for _, expected := range expectedCols {
		if !colSet[expected] {
			t.Errorf("Coluna esperada %s não encontrada na tabela experience", expected)
		}
	}

	t.Logf("Colunas da tabela experience: %v", cols)
}

func TestMigrator_TaskManagement(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := loadManifest(t, "../../examples/manifests/task-management.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	// Verificar tabelas
	tables := []string{"task", "project", "user", "comment", "attachment"}
	for _, table := range tables {
		if !migrator.tableExists(table) {
			t.Errorf("Tabela %s não foi criada", table)
		}
	}
}

func TestMigrator_Idempotency(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := loadManifest(t, "../../examples/manifests/task-management.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	
	// Primeira migração
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Primeira migração falhou: %v", err)
	}

	firstLogLen := len(migrator.GetMigrationLog())

	// Segunda migração (deve ser idempotente)
	err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Segunda migração falhou: %v", err)
	}

	secondLogLen := len(migrator.GetMigrationLog())

	if firstLogLen != secondLogLen {
		t.Errorf("Migração não é idempotente: primeira=%d, segunda=%d", firstLogLen, secondLogLen)
	}

	t.Logf("Migração idempotente confirmada: %d migrações em ambas as execuções", firstLogLen)
}

func TestMigrator_ValidateSchema(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := loadManifest(t, "../../examples/manifests/task-management.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	
	// Migrar primeiro
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	// Validar schema (deve passar)
	err = migrator.ValidateSchema()
	if err != nil {
		t.Errorf("Validação de schema falhou após migração: %v", err)
	}

	t.Log("Schema validado com sucesso")
}

func TestMigrator_IndexCreation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := loadManifest(t, "../../examples/manifests/cafeteria-loyalty.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	// Verificar se índices foram criados (query no sqlite_master)
	query := "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'"
	var count int
	err = db.QueryRow(query).Scan(&count)
	if err != nil {
		t.Fatalf("Falha ao verificar índices: %v", err)
	}

	if count == 0 {
		t.Error("Nenhum índice foi criado")
	}

	t.Logf("Índices criados: %d", count)
}

func TestMigrator_FieldTypeMapping(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := &manifest.Manifest{
		Meta: manifest.Metadata{
			Name:    "Test Types",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "test_types",
					Fields: []manifest.Field{
						{Name: "text_field", Type: "string", Required: true},
						{Name: "int_field", Type: "integer"},
						{Name: "float_field", Type: "float"},
						{Name: "bool_field", Type: "boolean"},
						{Name: "date_field", Type: "datetime"},
						{Name: "json_field", Type: "json"},
					},
				},
			},
		},
	}

	migrator := NewMigrator(db, m)
	ctx := context.Background()
	
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	// Verificar tipos das colunas
	query := "PRAGMA table_info(test_types)"
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("Falha ao obter info das colunas: %v", err)
	}
	defer rows.Close()

	type colInfo struct {
		name string
		typ  string
	}

	var columns []colInfo
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt interface{}
		var pk int
		
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("Falha ao scan: %v", err)
		}
		columns = append(columns, colInfo{name, typ})
	}

	expectedTypes := map[string]string{
		"text_field":   "TEXT",
		"int_field":    "INTEGER",
		"float_field":  "REAL",
		"bool_field":   "BOOLEAN",
		"date_field":   "DATETIME",
		"json_field":   "JSON",
	}

	for _, col := range columns {
		if col.name == "id" || col.name == "created_at" || col.name == "updated_at" {
			continue
		}
		
		expected, exists := expectedTypes[col.name]
		if !exists {
			t.Errorf("Coluna inesperada: %s", col.name)
			continue
		}
		
		if col.typ != expected {
			t.Errorf("Tipo incorreto para %s: esperado=%s, obtido=%s", col.name, expected, col.typ)
		}
	}

	t.Logf("Tipos de colunas validados: %v", columns)
}

func TestMigrator_ConstraintHandling(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	m := &manifest.Manifest{
		Meta: manifest.Metadata{
			Name:    "Test Constraints",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "constrained_entity",
					Fields: []manifest.Field{
						{Name: "required_field", Type: "string", Required: true},
						{Name: "unique_field", Type: "string", Unique: true},
						{Name: "optional_field", Type: "string"},
					},
				},
			},
		},
	}

	migrator := NewMigrator(db, m)
	ctx := context.Background()
	
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	// Testar constraint NOT NULL
	_, err = db.Exec("INSERT INTO constrained_entity (optional_field) VALUES ('test')")
	if err == nil {
		t.Error("Constraint NOT NULL não funcionou - deveria falhar sem required_field")
	}

	// Testar inserção válida
	_, err = db.Exec("INSERT INTO constrained_entity (required_field, unique_field, optional_field) VALUES ('req', 'uniq1', 'opt')")
	if err != nil {
		t.Errorf("Inserção válida falhou: %v", err)
	}

	// Testar constraint UNIQUE
	_, err = db.Exec("INSERT INTO constrained_entity (required_field, unique_field) VALUES ('req2', 'uniq1')")
	if err == nil {
		t.Error("Constraint UNIQUE não funcionou - deveria falhar com valor duplicado")
	}

	t.Log("Constraints validadas com sucesso")
}

func TestSortChanges(t *testing.T) {
	changes := []SchemaChange{
		{Type: "CREATE_INDEX", Table: "t1", SQL: "CREATE INDEX..."},
		{Type: "ADD_COLUMN", Table: "t1", SQL: "ALTER TABLE..."},
		{Type: "CREATE_TABLE", Table: "t1", SQL: "CREATE TABLE..."},
		{Type: "CREATE_INDEX", Table: "t2", SQL: "CREATE INDEX..."},
		{Type: "CREATE_TABLE", Table: "t2", SQL: "CREATE TABLE..."},
	}

	sorted := SortChanges(changes)

	expectedOrder := []string{"CREATE_TABLE", "CREATE_TABLE", "ADD_COLUMN", "CREATE_INDEX", "CREATE_INDEX"}
	
	for i, exp := range expectedOrder {
		if sorted[i].Type != exp {
			t.Errorf("Posição %d: esperado=%s, obtido=%s", i, exp, sorted[i].Type)
		}
	}

	t.Logf("Ordenação correta: %v", expectedOrder)
}

func TestMigrator_MultipleManifests(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	manifests := []string{
		"../../examples/manifests/task-management.yaml",
		"../../examples/manifests/cafeteria-loyalty.yaml",
		"../../examples/manifests/parking-ticket.yaml",
		"../../examples/manifests/experience-platform.yaml",
	}

	ctx := context.Background()
	
	for _, path := range manifests {
		m := loadManifest(t, path)
		migrator := NewMigrator(db, m)
		
		err := migrator.Migrate(ctx)
		if err != nil {
			t.Errorf("Migração de %s falhou: %v", path, err)
		} else {
			t.Logf("%s: %d migrações aplicadas", path, len(migrator.GetMigrationLog()))
		}
	}

	// Verificar total de tabelas
	query := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != '_migrations'"
	var count int
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		t.Fatalf("Falha ao contar tabelas: %v", err)
	}

	t.Logf("Total de tabelas criadas: %d", count)
	
	if count < 15 {
		t.Errorf("Número de tabelas menor que o esperado: %d", count)
	}
}
