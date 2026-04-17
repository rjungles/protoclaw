package db

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

func loadManifest(t *testing.T, path string) *manifest.Manifest {
	m, err := manifest.ParseFile("../../../examples/manifests/" + path[strings.LastIndex(path, "/")+1:])
	if err != nil {
		t.Fatalf("Falha ao carregar manifesto: %v", err)
	}
	return m
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
