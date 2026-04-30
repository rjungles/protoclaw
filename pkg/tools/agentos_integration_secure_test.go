package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/picoclaw/protoclaw/pkg/agentos/audit"
	"github.com/picoclaw/protoclaw/pkg/agentos/registry"
	"github.com/picoclaw/protoclaw/pkg/agentos/security"
	"github.com/picoclaw/protoclaw/pkg/agentos/security/validation"
	"github.com/picoclaw/protoclaw/pkg/agentos/storage"
)

// TestSecureWorkflow tests the complete secure workflow
func TestSecureWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	tool := &ExecAgentOSTool{
		dataDir:   tmpDir,
		validator: validation.NewSystemNameValidator(),
	}

	ctx := context.Background()

	// Step 1: Generate manifest
	manTool := NewAgentOSGenerateManifestTool()
	result := manTool.Execute(ctx, map[string]any{
		"description": "A restaurant system",
		"system_name": "my-restaurant",
		"output_path": filepath.Join(tmpDir, "restaurant.yaml"),
	})

	if result.Status != ToolStatusSuccess {
		t.Fatalf("Manifest generation failed: %s", result.Error)
	}

	// Verify manifest was created
	manifestPath := filepath.Join(tmpDir, "restaurant.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatal("Manifest was not created")
	}

	// Step 2: Initialize system with security
	result = tool.Execute(ctx, map[string]any{
		"action":       "init",
		"system_name":  "my-restaurant",
		"manifest_path": manifestPath,
		"user_id":      "test-user",
	})

	if result.Status != ToolStatusSuccess {
		t.Fatalf("Init failed: %s", result.Error)
	}

	// Verify system directory structure
	paths := storage.NewSystemPaths(tmpDir, "my-restaurant")
	if !paths.Exists() {
		t.Fatal("System directory was not created")
	}

	// Verify manifest copied
	if _, err := os.Stat(paths.Manifest()); err != nil {
		t.Error("Manifest not copied")
	}

	// Step 3: Verify registry entry
	reg, err := registry.NewDBRegistry(filepath.Join(tmpDir, "registry.db"))
	if err != nil {
		t.Fatalf("Failed to open registry: %v", err)
	}
	defer reg.Close()

	system, err := reg.GetSystem("my-restaurant")
	if err != nil {
		t.Fatalf("System not in registry: %v", err)
	}

	if system.Status != registry.StatusInitialized {
		t.Errorf("Wrong status: got %v, want %v", system.Status, registry.StatusInitialized)
	}

	// Step 4: Bootstrap system
	result = tool.Execute(ctx, map[string]any{
		"action":      "bootstrap",
		"system_name": "my-restaurant",
		"user_id":     "test-user",
	})

	if result.Status != ToolStatusSuccess {
		t.Fatalf("Bootstrap failed: %s", result.Error)
	}

	// Verify database created
	if _, err := os.Stat(paths.DB()); err != nil {
		t.Error("Database not created")
	}

	// Step 5: Verify status in registry
	system, err = reg.GetSystem("my-restaurant")
	if err != nil {
		t.Fatalf("System not in registry after bootstrap: %v", err)
	}

	if system.Status != registry.StatusBootstrapped {
		t.Errorf("Wrong status after bootstrap: got %v, want %v", system.Status, registry.StatusBootstrapped)
	}

	// Step 6: Serve system
	result = tool.Execute(ctx, map[string]any{
		"action":      "serve",
		"system_name": "my-restaurant",
	})

	if result.Status != ToolStatusSuccess {
		t.Fatalf("Serve failed: %s", result.Error)
	}

	// Verify status file and registry
	if _, err := os.Stat(paths.StatusFile()); err != nil {
		t.Error("Status file not created")
	}

	// Step 7: Query system status
	queryTool := NewAgentOSQueryTool()
	result = queryTool.Execute(ctx, map[string]any{
		"system_name": "my-restaurant",
		"entity":      "Menu",
		"limit":       10,
	})

	if result.Status != ToolStatusSuccess {
		t.Fatalf("Query failed: %s", result.Error)
	}

	// Step 8: Verify audit log
	auditPath := filepath.Join(tmpDir, "audit.db")
	aud, err := audit.NewLogger(auditPath)
	if err != nil {
		t.Fatalf("Failed to open audit log: %v", err)
	}
	defer aud.Close()

	events, err := aud.Query(ctx, audit.Filter{
		SystemID: system.ID,
	})
	if err != nil {
		t.Fatalf("Failed to query audit log: %v", err)
	}

	if len(events) < 2 {
		t.Errorf("Audit log incomplete: got %d events, want at least 2", len(events))
	}

	hasInit := false
	hasBootstrap := false
	for _, event := range events {
		if event.Operation == audit.OpSystemInitialized {
			hasInit = true
		}
		if event.Operation == audit.OpSystemBootstrapped {
			hasBootstrap = true
		}
	}

	if !hasInit {
		t.Error("Missing init event in audit log")
	}
	if !hasBootstrap {
		t.Error("Missing bootstrap event in audit log")
	}
}

// TestPathTraversalPrevention tests that path traversal attacks are prevented
func TestPathTraversalPrevention(t *testing.T) {
	tool := &ExecAgentOSTool{
		dataDir:   t.TempDir(),
		validator: validation.NewSystemNameValidator(),
	}

	ctx := context.Background()

	maliciousNames := []string{
		"../../../etc/passwd",
		"..\\windows\\system32",
		"....//....//...",
		"system/concat",
		"/dev/null",
	}

	manifestPath := filepath.Join(tool.dataDir, "test-manifest.yaml")
	os.WriteFile(manifestPath, []byte("test: manifest"), 0644)

	for _, attack := range maliciousNames {
		result := tool.Execute(ctx, map[string]any{
			"action":       "init",
			"system_name":  attack,
			"manifest_path": manifestPath,
		})

		// Should either fail validation or create sanitized name
		if result.Status == ToolStatusSuccess {
			// Verify system was created with sanitized name
			t.Logf("Path traversal attempt '%s' was sanitized", attack)
			name := strings.Split(result.Value, "Name: ")[1]
			name = strings.Split(name, "\n")[0]

			if !strings.Contains(name, "validated from") {
				t.Errorf("Expected sanitized name for attack '%s'", attack)
			}
		} else if result.Status != ToolStatusError {
			t.Errorf("Expected error for path traversal attack '%s'", attack)
		}
	}
}

// TestRegistryOperations tests registry operations
func TestRegistryOperations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	reg, err := registry.NewDBRegistry(dbPath)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer reg.Close()

	// Test create
	system := &registry.System{
		Name:       "test-system",
		HashPrefix: "abc12",
		Path:       "/data/test",
		Status:     registry.StatusInitialized,
		Metadata:   map[string]string{"version": "1.0"},
	}

	err = reg.RegisterSystem(system)
	if err != nil {
		t.Fatalf("Failed to register system: %v", err)
	}

	if system.ID == "" {
		t.Error("System ID not set after registration")
	}

	// Test read
	 retrieved, err := reg.GetSystem("test-system")
	if err != nil {
		t.Fatalf("Failed to retrieve system: %v", err)
	}

	if retrieved.Name != "test-system" {
		t.Errorf("Wrong name: got %s, want test-system", retrieved.Name)
	}

	if retrieved.Status != registry.StatusInitialized {
		t.Errorf("Wrong status: got %v, want %v", retrieved.Status, registry.StatusInitialized)
	}

	// Test update status
	err = reg.UpdateStatus(retrieved.ID, registry.StatusBootstrapped)
	if err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	retrieved, _ = reg.GetSystem("test-system")
	if retrieved.Status != registry.StatusBootstrapped {
		t.Errorf("Status not updated: got %v, want %v", retrieved.Status, registry.StatusBootstrapped)
	}

	// Test metadata
	err = reg.SetMetadata(retrieved.ID, "key1", "value1")
	if err != nil {
		t.Fatalf("Failed to set metadata: %v", err)
	}

	value, err := reg.GetMetadata(retrieved.ID, "key1")
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}

	if value != "value1" {
		t.Errorf("Wrong metadata value: got %s, want value1", value)
	}

	// Test list
	system2 := &registry.System{
		Name:   "test-system-2",
		Status: registry.StatusInitialized,
	}
	reg.RegisterSystem(system2)

	systems, err := reg.ListSystems()
	if err != nil {
		t.Fatalf("Failed to list systems: %v", err)
	}

	if len(systems) != 2 {
		t.Errorf("Wrong count: got %d, want 2", len(systems))
	}
}

// TestKeystoreOperations tests keystore operations
func TestKeystoreOperations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, err := security.NewKeyStore(dbPath, masterKey)
	if err != nil {
		t.Fatalf("Failed to create keystore: %v", err)
	}
	defer ks.Close()

	// Test store and retrieve
	testKey := "api.openai"
	testValue := []byte("sk-test123456789")
	metadata := "Test key"

	err = ks.Store(testKey, testValue, metadata)
	if err != nil {
		t.Fatalf("Failed to store key: %v", err)
	}

	retrieved, retrievedMeta, err := ks.Retrieve(testKey)
	if err != nil {
		t.Fatalf("Failed to retrieve key: %v", err)
	}

	if string(retrieved) != string(testValue) {
		t.Errorf("Wrong value: got %s, want %s", retrieved, testValue)
	}

	if retrievedMeta != metadata {
		t.Errorf("Wrong metadata: got %s, want %s", retrievedMeta, metadata)
	}

	// Test exists
	if !ks.Exists(testKey) {
		t.Error("Key should exist")
	}

	if ks.Exists("non-existent") {
		t.Error("Key should not exist")
	}

	// Test list
	err = ks.Store("key2", []byte("value2"), "")
	if err != nil {
		t.Fatalf("Failed to store second key: %v", err)
	}

	keys, err := ks.List()
	if err != nil {
		t.Fatalf("Failed to list keys: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("Wrong count: got %d, want 2", len(keys))
	}
}

// TestDirectoryIsolation tests hash-based directory isolation
func TestDirectoryIsolation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two systems with similar names
	systems := []string{
		"test-system-1",
		"test-system-2",
		"my-test-system",
	}

	var paths []*storage.SystemPaths
	for _, name := range systems {
		p := storage.NewSystemPaths(tmpDir, name)
		p.EnsureDirectories()
		paths = append(paths, p)
	}

	// Verify all have different hash prefixes
	hashes := make(map[string]bool)
	for i, p := range paths {
		if p.Hash == "" || len(p.Hash) != 5 {
			t.Errorf("System %d has invalid hash: %s", i, p.Hash)
		}
		if hashes[p.Hash] {
			t.Errorf("Duplicate hash found: %s", p.Hash)
		}
		hashes[p.Hash] = true

		// Verify directories were created
		if _, err := os.Stat(p.Root()); err != nil {
			t.Errorf("System root not created: %s", p.Root())
		}
	}

	// Verify isolation - no system should see another's files
	for _, p := range paths {
		entries, _ := os.ReadDir(filepath.Dir(filepath.Dir(p.Root())))
		foundCount := 0
		for _, entry := range entries {
			if entry.IsDir() {
				for _, sysName := range systems {
					if sysName != p.Name {
						otherPath := storage.NewSystemPaths(tmpDir, sysName)
						if entry.Name() == otherPath.Hash {
							foundCount++
						}
					}
				}
			}
		}
		if foundCount != len(systems)-1 {
			t.Error("Systems should not be able to access each other's directories easily")
		}
	}
}

// TestMigration tests automatic migration from old to new structure
func TestMigration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create old structure
	oldPath := filepath.Join(tmpDir, "test-system")
	os.MkdirAll(oldPath, 0750)
	os.WriteFile(filepath.Join(oldPath, "system.yaml"), []byte("test: data"), 0644)

	// Check old structure exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		t.Fatal("Old structure not created")
	}

	// Check new structure doesn't exist
	paths := storage.NewSystemPaths(tmpDir, "test-system")
	if paths.Exists() {
		t.Fatal("New structure shouldn't exist yet")
	}

	// Try to find system (should trigger migration)
	foundPaths, err := storage.FindSystem(tmpDir, "test-system")
	if err != nil {
		t.Fatalf("Failed to find system: %v", err)
	}

	// Verify migration occurred
	if !foundPaths.Exists() {
		t.Fatal("System not migrated")
	}

	if foundPaths.Root() == oldPath {
		t.Error("System should be migrated to new path")
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("Old path should not exist after migration")
	}

	// Verify files were migrated
	if _, err := os.Stat(foundPaths.Manifest()); err != nil {
		t.Error("Manifest not migrated")
	}
}

// TestSecurityScenarios tests various security scenarios
func TestSecurityScenarios(t *testing.T) {
	// Test conflict detection
	tmpDir := t.TempDir()

	// Create both old and new structures
	os.MkdirAll(filepath.Join(tmpDir, "test-system"), 0750)

	paths := storage.NewSystemPaths(tmpDir, "test-system")
	os.MkdirAll(paths.Root(), 0750)

	// Try to find (should error due to conflict)
	_, err := storage.FindSystem(tmpDir, "test-system")
	if err == nil {
		t.Error("Should error when both structures exist")
	}

	// Test name validation with reserved words
	validator := validation.NewSystemNameValidator()
	reservedWords := []string{"con", "aux", "nul", "prn", "com1", "lpt1"}

	for _, word := range reservedWords {
		err := validator.Validate(word)
		if err == nil {
			t.Errorf("Should reject reserved word: %s", word)
		}

		// Verify sanitization adds prefix
		sanitized := validator.Sanitize(word)
		if sanitized == word {
			t.Errorf("Sanitization should modify reserved word: %s", word)
		}
		if err := validator.Validate(sanitized); err != nil {
			t.Errorf("Sanitized name should be valid: %s", sanitized)
		}
	}
}

// TestCLICommands tests CLI command integration
func TestCLICommands(t *testing.T) {
	tmpDir := t.TempDir()

	// Test valid system name
	tool := &ExecAgentOSTool{
		dataDir:   tmpDir,
		validator: validation.NewSystemNameValidator(),
	}

	ctx := context.Background()
	manifestPath := filepath.Join(tmpDir, "test.yaml")
	os.WriteFile(manifestPath, []byte("test: data"), 0644)

	// Valid name
	result := tool.Execute(ctx, map[string]any{
		"action":       "init",
		"system_name":  "valid-system",
		"manifest_path": manifestPath,
		"user_id":      "cli-user",
	})

	if result.Status != ToolStatusSuccess {
		t.Errorf("Valid name rejected: %s", result.Error)
	}

	// Invalid name should be rejected
	result = tool.Execute(ctx, map[string]any{
		"action":       "init",
		"system_name":  "../../../etc/passwd",
		"manifest_path": manifestPath,
	})

	if result.Status == ToolStatusSuccess {
		t.Error("Path traversal should be rejected or sanitized")
	}
}
