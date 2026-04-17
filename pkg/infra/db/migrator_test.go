package db

import (
m"context"
m"testing"

m"github.com/sipeed/picoclaw/pkg/manifest"
)

func loadManifest(t *testing.T, path string) *manifest.Manifest {
mm, err := manifest.LoadFromFile(path)
mif err != nil {
mt.Fatalf("Falha ao carregar manifesto: %v", err)
m}
mreturn m
}

func TestMigrator_CafeteriaLoyalty_Mock(t *testing.T) {
mdb := NewMockDB()
mm := loadManifest(t, "../../examples/manifests/cafeteria-loyalty.yaml")
mmigrator := NewMigrator(db, m)

mctx := context.Background()
merr := migrator.Migrate(ctx)
mif err != nil {
mt.Fatalf("Migração falhou: %v", err)
m}

mtables := db.GetTables()
mif len(tables) == 0 {
mt.Error("Nenhuma tabela criada")
m}

mt.Logf("Tabelas criadas: %v", tables)
}

func TestMigrator_ExperiencePlatform_Mock(t *testing.T) {
mdb := NewMockDB()
mm := loadManifest(t, "../../examples/manifests/experience-platform.yaml")
mmigrator := NewMigrator(db, m)

mctx := context.Background()
merr := migrator.Migrate(ctx)
mif err != nil {
mt.Fatalf("Migração falhou: %v", err)
m}

mtables := db.GetTables()
mif len(tables) == 0 {
mt.Error("Nenhuma tabela criada")
m}

mt.Logf("Tabelas criadas: %v", tables)
}

func TestMigrator_ParkingTicket_Mock(t *testing.T) {
mdb := NewMockDB()
mm := loadManifest(t, "../../examples/manifests/parking-ticket.yaml")
mmigrator := NewMigrator(db, m)

mctx := context.Background()
merr := migrator.Migrate(ctx)
mif err != nil {
mt.Fatalf("Migração falhou: %v", err)
m}

mtables := db.GetTables()
mif len(tables) == 0 {
mt.Error("Nenhuma tabela criada")
m}

mt.Logf("Tabelas criadas: %v", tables)
}

func TestMigrator_TaskManagement_Mock(t *testing.T) {
mdb := NewMockDB()
mm := loadManifest(t, "../../examples/manifests/task-management.yaml")
mmigrator := NewMigrator(db, m)

mctx := context.Background()
merr := migrator.Migrate(ctx)
mif err != nil {
mt.Fatalf("Migração falhou: %v", err)
m}

mtables := db.GetTables()
mif len(tables) == 0 {
mt.Error("Nenhuma tabela criada")
m}

mt.Logf("Tabelas criadas: %v", tables)
}

func TestMigrator_Idempotency_Mock(t *testing.T) {
mdb := NewMockDB()
mm := loadManifest(t, "../../examples/manifests/cafeteria-loyalty.yaml")
mmigrator := NewMigrator(db, m)

mctx := context.Background()
m
merr := migrator.Migrate(ctx)
mif err != nil {
mt.Fatalf("Primeira migração falhou: %v", err)
m}

mfirstTables := db.GetTables()
mfirstMigrations := len(db.GetMigrations())

merr = migrator.Migrate(ctx)
mif err != nil {
mt.Fatalf("Segunda migração falhou: %v", err)
m}

msecondTables := db.GetTables()
msecondMigrations := len(db.GetMigrations())

mif len(firstTables) != len(secondTables) {
mt.Errorf("Número de tabelas mudou: %d -> %d", len(firstTables), len(secondTables))
m}

mif firstMigrations != secondMigrations {
mt.Errorf("Novas migrações aplicadas indevidamente: %d -> %d", firstMigrations, secondMigrations)
m}

mt.Logf("Idempotência verificada: %d tabelas, %d migrações", len(firstTables), firstMigrations)
}

func TestMigrator_ValidateSchema_Mock(t *testing.T) {
mdb := NewMockDB()
mm := loadManifest(t, "../../examples/manifests/cafeteria-loyalty.yaml")
mmigrator := NewMigrator(db, m)

mctx := context.Background()
m
merr := migrator.Migrate(ctx)
mif err != nil {
mt.Fatalf("Migração falhou: %v", err)
m}

merr = migrator.ValidateSchema()
mif err != nil {
mt.Errorf("Schema validation falhou após migração: %v", err)
m}

mt.Log("Schema validado com sucesso")
}
