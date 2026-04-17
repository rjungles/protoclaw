package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sipeed/picoclaw/pkg/manifest"
	_ "modernc.org/sqlite"
)

func loadManifest(t *testing.T, path string) *manifest.Manifest {
	m, err := manifest.ParseFile("../../../examples/manifests/" + path[strings.LastIndex(path, "/")+1:])
	if err != nil {
		t.Fatalf("Falha ao carregar manifesto: %v", err)
	}
	return m
}

var testDBCounter uint64

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	n := atomic.AddUint64(&testDBCounter, 1)
	testName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:agentos_etapa2_%s_%d?mode=memory&cache=shared", testName, n)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrator_CafeteriaLoyalty_Mock(t *testing.T) {
	db := NewMockDB()
	m := loadManifest(t, "../../examples/manifests/cafeteria-loyalty.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	tables := db.ListTables()
	if len(tables) == 0 {
		t.Error("Nenhuma tabela criada")
	}

	t.Logf("Tabelas criadas: %v", tables)
}

func TestMigrator_ExperiencePlatform_Mock(t *testing.T) {
	db := NewMockDB()
	m := loadManifest(t, "../../examples/manifests/experience-platform.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	tables := db.ListTables()
	if len(tables) == 0 {
		t.Error("Nenhuma tabela criada")
	}

	t.Logf("Tabelas criadas: %v", tables)
}

func TestMigrator_ParkingTicket_Mock(t *testing.T) {
	db := NewMockDB()
	m := loadManifest(t, "../../examples/manifests/parking-ticket.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	tables := db.ListTables()
	if len(tables) == 0 {
		t.Error("Nenhuma tabela criada")
	}

	t.Logf("Tabelas criadas: %v", tables)
}

func TestMigrator_TaskManagement_Mock(t *testing.T) {
	db := NewMockDB()
	m := loadManifest(t, "../../examples/manifests/task-management.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	tables := db.ListTables()
	if len(tables) == 0 {
		t.Error("Nenhuma tabela criada")
	}

	t.Logf("Tabelas criadas: %v", tables)
}

func TestMigrator_Idempotency_Mock(t *testing.T) {
	db := NewMockDB()
	m := loadManifest(t, "../../examples/manifests/cafeteria-loyalty.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()

	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Primeira migração falhou: %v", err)
	}

	firstTables := db.ListTables()
	firstMigrations := len(db.GetMigrations())

	err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Segunda migração falhou: %v", err)
	}

	secondTables := db.ListTables()
	secondMigrations := len(db.GetMigrations())

	if len(firstTables) != len(secondTables) {
		t.Errorf("Número de tabelas mudou: %d -> %d", len(firstTables), len(secondTables))
	}

	if firstMigrations != secondMigrations {
		t.Errorf("Novas migrações aplicadas indevidamente: %d -> %d", firstMigrations, secondMigrations)
	}

	t.Logf("Idempotência verificada: %d tabelas, %d migrações", len(firstTables), firstMigrations)
}

func TestMigrator_ValidateSchema_Mock(t *testing.T) {
	db := NewMockDB()
	m := loadManifest(t, "../../examples/manifests/cafeteria-loyalty.yaml")
	migrator := NewMigrator(db, m)

	ctx := context.Background()

	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}

	err = migrator.ValidateSchema()
	if err != nil {
		t.Errorf("Schema validation falhou após migração: %v", err)
	}

	t.Log("Schema validado com sucesso")
}

func TestMigrator_TaskManagement_SQLite(t *testing.T) {
	db := openTestDB(t)
	sqlDB := NewSQLDB(db)

	m, err := manifest.ParseFile(filepath.Join("..", "..", "..", "examples", "manifests", "task-management.yaml"))
	if err != nil {
		t.Fatalf("Falha ao carregar manifesto: %v", err)
	}

	migrator := NewMigrator(sqlDB, m)
	ctx := context.Background()

	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migração falhou: %v", err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migração (idempotente) falhou: %v", err)
	}

	for _, entity := range m.DataModel.Entities {
		tableName := toSnakeCase(entity.Name)
		var name string
		if err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableName,
		).Scan(&name); err != nil {
			t.Fatalf("tabela %q não encontrada: %v", tableName, err)
		}

		rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			t.Fatalf("PRAGMA table_info(%s): %v", tableName, err)
		}

		colSet := make(map[string]int)
		for rows.Next() {
			var cid int
			var cname string
			var ctype string
			var notnull int
			var dflt interface{}
			var pk int
			if err := rows.Scan(&cid, &cname, &ctype, &notnull, &dflt, &pk); err != nil {
				_ = rows.Close()
				t.Fatalf("scan table_info(%s): %v", tableName, err)
			}
			colSet[cname]++
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("rows err table_info(%s): %v", tableName, err)
		}
		_ = rows.Close()

		for _, field := range entity.Fields {
			col := toSnakeCase(field.Name)
			if colSet[col] != 1 {
				t.Fatalf("coluna %q em %q esperada 1 vez, got=%d", col, tableName, colSet[col])
			}
		}
	}
}
