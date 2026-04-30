package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewSystemPaths(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")

	if paths.Name != "my-system" {
		t.Errorf("Name = %q, want %q", paths.Name, "my-system")
	}
	if paths.DataDir != "/data" {
		t.Errorf("DataDir = %q, want %q", paths.DataDir, "/data")
	}
	if len(paths.Hash) != HashPrefixLength {
		t.Errorf("Hash length = %d, want %d", len(paths.Hash), HashPrefixLength)
	}

	// Verify hash is deterministic
	paths2 := NewSystemPaths("/data", "my-system")
	if paths.Hash != paths2.Hash {
		t.Error("Hash should be deterministic")
	}

	// Verify different names produce different hashes
	paths3 := NewSystemPaths("/data", "other-system")
	if paths.Hash == paths3.Hash {
		t.Error("Different names should produce different hashes")
	}
}

func TestSystemPaths_Root(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join("/data", SystemsSubdir, paths.Hash, "my-system")
	if paths.Root() != expected {
		t.Errorf("Root() = %q, want %q", paths.Root(), expected)
	}
}

func TestSystemPaths_Config(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join(paths.Root(), "config")
	if paths.Config() != expected {
		t.Errorf("Config() = %q, want %q", paths.Config(), expected)
	}
}

func TestSystemPaths_LLMConfig(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join(paths.Root(), "config", "llm")
	if paths.LLMConfig() != expected {
		t.Errorf("LLMConfig() = %q, want %q", paths.LLMConfig(), expected)
	}
}

func TestSystemPaths_Data(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join(paths.Root(), "data")
	if paths.Data() != expected {
		t.Errorf("Data() = %q, want %q", paths.Data(), expected)
	}
}

func TestSystemPaths_DB(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join(paths.Root(), "data", "data.db")
	if paths.DB() != expected {
		t.Errorf("DB() = %q, want %q", paths.DB(), expected)
	}
}

func TestSystemPaths_Manifest(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join(paths.Root(), "system.yaml")
	if paths.Manifest() != expected {
		t.Errorf("Manifest() = %q, want %q", paths.Manifest(), expected)
	}
}

func TestSystemPaths_StatusFile(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join(paths.Root(), ".serving")
	if paths.StatusFile() != expected {
		t.Errorf("StatusFile() = %q, want %q", paths.StatusFile(), expected)
	}
}

func TestSystemPaths_LLMConfigFile(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join(paths.Root(), "config", "llm", "llm.yaml")
	if paths.LLMConfigFile() != expected {
		t.Errorf("LLMConfigFile() = %q, want %q", paths.LLMConfigFile(), expected)
	}
}

func TestSystemPaths_RegistryDB(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join("/data", "registry.db")
	if paths.RegistryDB() != expected {
		t.Errorf("RegistryDB() = %q, want %q", paths.RegistryDB(), expected)
	}
}

func TestSystemPaths_KeyStore(t *testing.T) {
	paths := NewSystemPaths("/data", "my-system")
	expected := filepath.Join("/data", ".keys.db")
	if paths.KeyStore() != expected {
		t.Errorf("KeyStore() = %q, want %q", paths.KeyStore(), expected)
	}
}

func TestSystemPaths_EnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewSystemPaths(tmpDir, "test-system")

	err := paths.EnsureDirectories()
	if err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}

	// Verify directories exist
	dirs := []string{
		paths.Root(),
		paths.Config(),
		paths.LLMConfig(),
		paths.Data(),
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("Directory %s does not exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
}

func TestSystemPaths_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewSystemPaths(tmpDir, "test-system")

	if paths.Exists() {
		t.Error("Exists() should be false before creation")
	}

	os.MkdirAll(paths.Root(), 0750)

	if !paths.Exists() {
		t.Error("Exists() should be true after creation")
	}
}

func TestSystemPaths_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewSystemPaths(tmpDir, "test-system")

	// Should error for non-existent system
	err := paths.Remove()
	if err == nil {
		t.Error("Remove() should error for non-existent system")
	}

	// Create and then remove
	os.MkdirAll(paths.Root(), 0750)
	err = paths.Remove()
	if err != nil {
		t.Errorf("Remove() error = %v", err)
	}

	if paths.Exists() {
		t.Error("System should not exist after removal")
	}
}

func TestMigrateToNewStructure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create legacy structure
	legacyDir := filepath.Join(tmpDir, "test-system")
	os.MkdirAll(legacyDir, 0750)
	os.WriteFile(filepath.Join(legacyDir, "file.txt"), []byte("test"), 0644)

	// Migrate
	newPaths, err := MigrateToNewStructure(tmpDir, "test-system")
	if err != nil {
		t.Fatalf("MigrateToNewStructure() error = %v", err)
	}

	// Verify new structure exists
	if !newPaths.Exists() {
		t.Error("New structure should exist after migration")
	}

	// Verify legacy structure no longer exists
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Error("Legacy structure should not exist after migration")
	}

	// Verify file was migrated
	migratedFile := filepath.Join(newPaths.Root(), "file.txt")
	if _, err := os.Stat(migratedFile); os.IsNotExist(err) {
		t.Error("File should be migrated to new structure")
	}
}

func TestMigrateToNewStructure_AlreadyMigrated(t *testing.T) {
	tmpDir := t.TempDir()

	// Create new structure
	paths := NewSystemPaths(tmpDir, "test-system")
	os.MkdirAll(paths.Root(), 0750)

	// Try to migrate
	newPaths, err := MigrateToNewStructure(tmpDir, "test-system")
	if err != nil {
		t.Errorf("MigrateToNewStructure() should not error for already migrated: %v", err)
	}
	if newPaths.Root() != paths.Root() {
		t.Error("Should return existing new paths")
	}
}

func TestMigrateToNewStructure_BothExist(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both structures
	legacyDir := filepath.Join(tmpDir, "test-system")
	os.MkdirAll(legacyDir, 0750)

	paths := NewSystemPaths(tmpDir, "test-system")
	os.MkdirAll(paths.Root(), 0750)

	// Try to migrate
	_, err := MigrateToNewStructure(tmpDir, "test-system")
	if err == nil {
		t.Error("Should error when both structures exist")
	}
}

func TestFindSystem_NewStructure(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewSystemPaths(tmpDir, "test-system")
	os.MkdirAll(paths.Root(), 0750)

	found, err := FindSystem(tmpDir, "test-system")
	if err != nil {
		t.Errorf("FindSystem() error = %v", err)
	}
	if found.Root() != paths.Root() {
		t.Errorf("FindSystem() returned wrong path: %q", found.Root())
	}
}

func TestFindSystem_LegacyStructure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create legacy structure
	legacyDir := filepath.Join(tmpDir, "test-system")
	os.MkdirAll(legacyDir, 0750)

	// Find should auto-migrate
	found, err := FindSystem(tmpDir, "test-system")
	if err != nil {
		t.Fatalf("FindSystem() error = %v", err)
	}

	// Should return new structure
	expectedPaths := NewSystemPaths(tmpDir, "test-system")
	if found.Hash != expectedPaths.Hash {
		t.Error("FindSystem() should return migrated paths")
	}
}

func TestFindSystem_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := FindSystem(tmpDir, "non-existent")
	if err == nil {
		t.Error("FindSystem() should error for non-existent system")
	}
}

func TestGetAllSystems(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple systems
	for _, name := range []string{"system-a", "system-b", "system-c"} {
		paths := NewSystemPaths(tmpDir, name)
		os.MkdirAll(paths.Root(), 0750)
	}

	systems, err := GetAllSystems(tmpDir)
	if err != nil {
		t.Fatalf("GetAllSystems() error = %v", err)
	}

	if len(systems) != 3 {
		t.Errorf("GetAllSystems() returned %d systems, want 3", len(systems))
	}

	// Verify all systems were found
	names := make(map[string]bool)
	for _, s := range systems {
		names[s.Name] = true
	}

	for _, expected := range []string{"system-a", "system-b", "system-c"} {
		if !names[expected] {
			t.Errorf("System %s not found", expected)
		}
	}
}

func TestGetAllSystems_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	systems, err := GetAllSystems(tmpDir)
	if err != nil {
		t.Errorf("GetAllSystems() error = %v", err)
	}

	if len(systems) != 0 {
		t.Errorf("GetAllSystems() returned %d systems, want 0", len(systems))
	}
}

func TestHashPrefixIsolation(t *testing.T) {
	// Verify that systems with similar names don't collide
	tmpDir := t.TempDir()

	paths1 := NewSystemPaths(tmpDir, "test")
	paths2 := NewSystemPaths(tmpDir, "test-system")
	paths3 := NewSystemPaths(tmpDir, "my-test")

	if paths1.Hash == paths2.Hash {
		t.Error("Different names should have different hashes")
	}
	if paths1.Hash == paths3.Hash {
		t.Error("Different names should have different hashes")
	}
	if paths2.Hash == paths3.Hash {
		t.Error("Different names should have different hashes")
	}
}

func BenchmarkNewSystemPaths(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewSystemPaths("/data", "my-system")
	}
}

func BenchmarkHashCalculation(b *testing.B) {
	names := []string{"a", "my-system", "very-long-system-name-that-exceeds-normal-length"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewSystemPaths("/data", names[i%len(names)])
	}
}
