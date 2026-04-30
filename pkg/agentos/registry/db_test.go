package registry

import (
	"testing"
	"time"
)

func setupTestRegistry(t *testing.T) (*DBRegistry, func()) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test-registry.db"

	reg, err := NewDBRegistry(dbPath)
	if err != nil {
		t.Fatalf("NewDBRegistry() error = %v", err)
	}

	cleanup := func() {
		reg.Close()
	}

	return reg, cleanup
}

func TestNewDBRegistry(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	if reg == nil {
		t.Fatal("NewDBRegistry() returned nil")
	}
}

func TestDBRegistry_RegisterSystem(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	system := &System{
		Name:          "test-system",
		HashPrefix:    "abc12",
		Path:          "/data/test-system",
		Status:        StatusInitialized,
		ManifestPath:  "/data/test-system/system.yaml",
		LLMConfigPath: "/data/test-system/llm.yaml",
		Metadata:      map[string]string{"key": "value"},
	}

	err := reg.RegisterSystem(system)
	if err != nil {
		t.Fatalf("RegisterSystem() error = %v", err)
	}

	if system.ID == "" {
		t.Error("System ID should be set after registration")
	}
}

func TestDBRegistry_RegisterSystemDuplicate(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	system := &System{
		Name:       "duplicate-system",
		HashPrefix: "abc12",
		Path:       "/data/duplicate",
		Status:     StatusInitialized,
	}

	// First registration should succeed
	err := reg.RegisterSystem(system)
	if err != nil {
		t.Fatalf("First RegisterSystem() error = %v", err)
	}

	// Second registration with same name should fail
	err = reg.RegisterSystem(system)
	if err == nil {
		t.Error("Second RegisterSystem() should fail for duplicate name")
	}
}

func TestDBRegistry_GetSystem(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Register system
	system := &System{
		Name:          "get-test",
		HashPrefix:    "def34",
		Path:          "/data/get-test",
		Status:        StatusBootstrapped,
		ManifestPath:  "/data/get-test/system.yaml",
		LLMConfigPath: "/data/get-test/llm.yaml",
		Metadata:      map[string]string{"version": "1.0"},
	}
	reg.RegisterSystem(system)

	// Get system
	retrieved, err := reg.GetSystem("get-test")
	if err != nil {
		t.Fatalf("GetSystem() error = %v", err)
	}

	if retrieved.Name != "get-test" {
		t.Errorf("Name = %q, want %q", retrieved.Name, "get-test")
	}
	if retrieved.HashPrefix != "def34" {
		t.Errorf("HashPrefix = %q, want %q", retrieved.HashPrefix, "def34")
	}
	if retrieved.Status != StatusBootstrapped {
		t.Errorf("Status = %q, want %q", retrieved.Status, StatusBootstrapped)
	}
	if retrieved.ManifestPath != "/data/get-test/system.yaml" {
		t.Errorf("ManifestPath = %q, want %q", retrieved.ManifestPath, "/data/get-test/system.yaml")
	}
	if retrieved.Metadata["version"] != "1.0" {
		t.Errorf("Metadata['version'] = %q, want %q", retrieved.Metadata["version"], "1.0")
	}
}

func TestDBRegistry_GetSystemNotFound(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	_, err := reg.GetSystem("non-existent")
	if err == nil {
		t.Error("GetSystem() should error for non-existent system")
	}
}

func TestDBRegistry_UpdateStatus(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Register system
	system := &System{
		Name:       "status-test",
		HashPrefix: "ghi56",
		Path:       "/data/status-test",
		Status:     StatusInitialized,
	}
	reg.RegisterSystem(system)

	// Update status
	err := reg.UpdateStatus(system.ID, StatusServing)
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	// Verify
	retrieved, _ := reg.GetSystem("status-test")
	if retrieved.Status != StatusServing {
		t.Errorf("Status = %q, want %q", retrieved.Status, StatusServing)
	}
}

func TestDBRegistry_UpdateStatusNotFound(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	err := reg.UpdateStatus("non-existent-id", StatusServing)
	if err == nil {
		t.Error("UpdateStatus() should error for non-existent system")
	}
}

func TestDBRegistry_ListSystems(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Register multiple systems
	systems := []*System{
		{Name: "system-a", HashPrefix: "aaa", Path: "/data/a", Status: StatusInitialized},
		{Name: "system-b", HashPrefix: "bbb", Path: "/data/b", Status: StatusBootstrapped},
		{Name: "system-c", HashPrefix: "ccc", Path: "/data/c", Status: StatusServing},
	}

	for _, s := range systems {
		reg.RegisterSystem(s)
	}

	// List systems
	list, err := reg.ListSystems()
	if err != nil {
		t.Fatalf("ListSystems() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("ListSystems() returned %d systems, want 3", len(list))
	}

	// Verify order (should be by created_at DESC)
	if list[0].Name != "system-c" {
		t.Errorf("First system = %q, want %q", list[0].Name, "system-c")
	}
}

func TestDBRegistry_ListSystemsEmpty(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	list, err := reg.ListSystems()
	if err != nil {
		t.Fatalf("ListSystems() error = %v", err)
	}

	if len(list) != 0 {
		t.Errorf("ListSystems() returned %d systems, want 0", len(list))
	}
}

func TestDBRegistry_DeleteSystem(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Register and delete
	system := &System{
		Name:       "to-delete",
		HashPrefix: "del",
		Path:       "/data/to-delete",
		Status:     StatusInitialized,
	}
	reg.RegisterSystem(system)

	err := reg.DeleteSystem("to-delete")
	if err != nil {
		t.Fatalf("DeleteSystem() error = %v", err)
	}

	// Verify soft delete
	_, err = reg.GetSystem("to-delete")
	if err == nil {
		t.Error("GetSystem() should error after deletion")
	}

	// System should still be in list with deleted status
	// (depending on implementation, might be excluded from ListSystems)
}

func TestDBRegistry_DeleteSystemNotFound(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	err := reg.DeleteSystem("non-existent")
	if err == nil {
		t.Error("DeleteSystem() should error for non-existent system")
	}
}

func TestDBRegistry_SystemExists(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	if reg.SystemExists("test") {
		t.Error("SystemExists() should be false for non-existent")
	}

	system := &System{
		Name:       "test",
		HashPrefix: "tst",
		Path:       "/data/test",
		Status:     StatusInitialized,
	}
	reg.RegisterSystem(system)

	if !reg.SystemExists("test") {
		t.Error("SystemExists() should be true for existing")
	}
}

func TestDBRegistry_Count(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	// Empty
	count, _ := reg.Count()
	if count != 0 {
		t.Errorf("Count() = %d, want 0", count)
	}

	// Add systems
	for i := 0; i < 5; i++ {
		system := &System{
			Name:       "system-" + string('a'+byte(i)),
			HashPrefix: "tst",
			Path:       "/data/test",
			Status:     StatusInitialized,
		}
		reg.RegisterSystem(system)
	}

	count, _ = reg.Count()
	if count != 5 {
		t.Errorf("Count() = %d, want 5", count)
	}
}

func TestDBRegistry_SetMetadata(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	system := &System{
		Name:       "meta-test",
		HashPrefix: "meta",
		Path:       "/data/meta",
		Status:     StatusInitialized,
	}
	reg.RegisterSystem(system)

	// Set metadata
	err := reg.SetMetadata(system.ID, "key1", "value1")
	if err != nil {
		t.Fatalf("SetMetadata() error = %v", err)
	}

	// Update metadata
	err = reg.SetMetadata(system.ID, "key1", "value2")
	if err != nil {
		t.Fatalf("SetMetadata() update error = %v", err)
	}

	// Get metadata
	value, err := reg.GetMetadata(system.ID, "key1")
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}

	if value != "value2" {
		t.Errorf("GetMetadata() = %q, want %q", value, "value2")
	}
}

func TestDBRegistry_GetMetadataNotFound(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	system := &System{
		Name:       "meta-test",
		HashPrefix: "meta",
		Path:       "/data/meta",
		Status:     StatusInitialized,
	}
	reg.RegisterSystem(system)

	value, err := reg.GetMetadata(system.ID, "non-existent")
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}

	if value != "" {
		t.Errorf("GetMetadata() = %q, want empty string", value)
	}
}

func TestDBRegistry_LogAudit(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	system := &System{
		Name:       "audit-test",
		HashPrefix: "aud",
		Path:       "/data/audit",
		Status:     StatusInitialized,
	}
	reg.RegisterSystem(system)

	details := map[string]interface{}{
		"action": "test",
		"user":   "test-user",
	}

	err := reg.LogAudit("test_operation", system.ID, "user-123", details)
	if err != nil {
		t.Fatalf("LogAudit() error = %v", err)
	}
}

func TestDBRegistry_ConcurrentAccess(t *testing.T) {
	reg, cleanup := setupTestRegistry(t)
	defer cleanup()

	done := make(chan bool, 10)

	// Concurrent registrations
	for i := 0; i < 10; i++ {
		go func(idx int) {
			system := &System{
				Name:       "concurrent-" + string('a'+byte(idx)),
				HashPrefix: "con",
				Path:       "/data/concurrent",
				Status:     StatusInitialized,
			}
			reg.RegisterSystem(system)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all registered
	count, _ := reg.Count()
	if count != 10 {
		t.Errorf("Count() = %d, want 10", count)
	}
}

func BenchmarkRegisterSystem(b *testing.B) {
	reg, cleanup := setupTestRegistry(&testing.T{})
	defer cleanup()

	for i := 0; i < b.N; i++ {
		system := &System{
			Name:       "bench-" + string(rune(i)),
			HashPrefix: "b",
			Path:       "/data/bench",
			Status:     StatusInitialized,
		}
		reg.RegisterSystem(system)
	}
}

func BenchmarkGetSystem(b *testing.B) {
	reg, cleanup := setupTestRegistry(&testing.T{})
	defer cleanup()

	system := &System{
		Name:       "bench-get",
		HashPrefix: "b",
		Path:       "/data/bench",
		Status:     StatusInitialized,
	}
	reg.RegisterSystem(system)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reg.GetSystem("bench-get")
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusInitialized, "initialized"},
		{StatusBootstrapped, "bootstrapped"},
		{StatusServing, "serving"},
		{StatusError, "error"},
		{StatusDeleted, "deleted"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("Status(%q) = %q, want %q", tt.status, string(tt.status), tt.want)
		}
	}
}
