package security

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewKeyStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, err := NewKeyStore(dbPath, masterKey)
	if err != nil {
		t.Fatalf("NewKeyStore() error = %v", err)
	}
	defer ks.Close()

	if ks == nil {
		t.Fatal("NewKeyStore() returned nil")
	}
}

func TestKeyStore_StoreAndRetrieve(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	tests := []struct {
		name     string
		keyName  string
		value    []byte
		metadata string
	}{
		{"simple", "api-key", []byte("secret123"), "test key"},
		{"binary", "binary-key", []byte{0x00, 0x01, 0x02, 0xff}, "binary data"},
		{"long value", "long-key", []byte("very long value " + string(make([]byte, 1000))), "long key"},
		{"unicode", "unicode-key", []byte("Hello, 世界! 🌍"), "unicode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ks.Store(tt.keyName, tt.value, tt.metadata)
			if err != nil {
				t.Fatalf("Store() error = %v", err)
			}

			retrieved, metadata, err := ks.Retrieve(tt.keyName)
			if err != nil {
				t.Fatalf("Retrieve() error = %v", err)
			}

			if string(retrieved) != string(tt.value) {
				t.Errorf("Retrieved value mismatch: got %v, want %v", retrieved, tt.value)
			}

			if metadata != tt.metadata {
				t.Errorf("Metadata mismatch: got %q, want %q", metadata, tt.metadata)
			}
		})
	}
}

func TestKeyStore_RetrieveNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	_, _, err := ks.Retrieve("non-existent")
	if err == nil {
		t.Error("Retrieve() should error for non-existent key")
	}
}

func TestKeyStore_Update(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	// Store initial value
	err := ks.Store("my-key", []byte("initial"), "initial metadata")
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	// Update value
	err = ks.Store("my-key", []byte("updated"), "updated metadata")
	if err != nil {
		t.Fatalf("Store() update error = %v", err)
	}

	// Retrieve and verify
	retrieved, metadata, _ := ks.Retrieve("my-key")
	if string(retrieved) != "updated" {
		t.Errorf("Value not updated: got %q, want %q", string(retrieved), "updated")
	}
	if metadata != "updated metadata" {
		t.Errorf("Metadata not updated: got %q, want %q", metadata, "updated metadata")
	}
}

func TestKeyStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	// Store and delete
	ks.Store("to-delete", []byte("value"), "")
	err := ks.Delete("to-delete")
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify deletion
	_, _, err = ks.Retrieve("to-delete")
	if err == nil {
		t.Error("Retrieve() should error after deletion")
	}
}

func TestKeyStore_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	if ks.Exists("test-key") {
		t.Error("Exists() should be false for non-existent key")
	}

	ks.Store("test-key", []byte("value"), "")

	if !ks.Exists("test-key") {
		t.Error("Exists() should be true for existing key")
	}
}

func TestKeyStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	// Empty list
	names, _ := ks.List()
	if len(names) != 0 {
		t.Errorf("List() returned %d items, want 0", len(names))
	}

	// Add keys
	ks.Store("key-a", []byte("a"), "")
	ks.Store("key-b", []byte("b"), "")
	ks.Store("key-c", []byte("c"), "")

	names, err := ks.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(names) != 3 {
		t.Errorf("List() returned %d items, want 3", len(names))
	}

	// Verify sorted order
	expected := []string{"key-a", "key-b", "key-c"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("List()[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestKeyStore_Rotate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	// Store initial
	ks.Store("rotating-key", []byte("old-value"), "")

	// Rotate
	err := ks.Rotate("rotating-key", []byte("new-value"), "rotated")
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	// Verify new value
	retrieved, metadata, _ := ks.Retrieve("rotating-key")
	if string(retrieved) != "new-value" {
		t.Errorf("Value not rotated: got %q, want %q", string(retrieved), "new-value")
	}
	if metadata != "rotated" {
		t.Errorf("Metadata not updated: got %q, want %q", metadata, "rotated")
	}

	// Check rotation count
	count, _ := ks.GetRotationCount("rotating-key")
	if count != 1 {
		t.Errorf("Rotation count = %d, want 1", count)
	}

	// Check last rotation time
	lastRotated, _ := ks.GetLastRotation("rotating-key")
	if lastRotated == nil {
		t.Error("Last rotation time should be set")
	}
	if time.Since(*lastRotated) > time.Second {
		t.Error("Last rotation time should be recent")
	}
}

func TestKeyStore_RotateNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	err := ks.Rotate("non-existent", []byte("value"), "")
	if err == nil {
		t.Error("Rotate() should error for non-existent key")
	}
}

func TestKeyStore_GetInfo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	ks.Store("info-key", []byte("value"), "my metadata")

	info, err := ks.GetInfo("info-key")
	if err != nil {
		t.Fatalf("GetInfo() error = %v", err)
	}

	if info.Name != "info-key" {
		t.Errorf("Name = %q, want %q", info.Name, "info-key")
	}
	if info.Metadata != "my metadata" {
		t.Errorf("Metadata = %q, want %q", info.Metadata, "my metadata")
	}
	if info.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if info.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestKeyStore_MigrateFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	// Set env var
	os.Setenv("TEST_API_KEY", "secret-from-env")
	defer os.Unsetenv("TEST_API_KEY")

	err := ks.MigrateFromEnv("migrated-key", "TEST_API_KEY")
	if err != nil {
		t.Fatalf("MigrateFromEnv() error = %v", err)
	}

	// Verify
	value, _, _ := ks.Retrieve("migrated-key")
	if string(value) != "secret-from-env" {
		t.Errorf("Migrated value = %q, want %q", string(value), "secret-from-env")
	}
}

func TestKeyStore_MigrateFromEnvNotSet(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	err := ks.MigrateFromEnv("migrated-key", "NON_EXISTENT_VAR")
	if err == nil {
		t.Error("MigrateFromEnv() should error for unset env var")
	}
}

func TestKeyStore_DifferentMasterKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey1 := []byte("master-key-1-32-bytes-long!!")
	masterKey2 := []byte("master-key-2-32-bytes-long!!")

	// Create keystore with first key
	ks1, _ := NewKeyStore(dbPath, masterKey1)
	ks1.Store("test-key", []byte("secret"), "")
	ks1.Close()

	// Try to open with different key
	ks2, _ := NewKeyStore(dbPath, masterKey2)
	_, _, err := ks2.Retrieve("test-key")
	if err == nil {
		t.Error("Retrieve should fail with different master key")
	}
	ks2.Close()
}

func TestKeyStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	// Concurrent stores
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			keyName := fmt.Sprintf("key-%d", idx)
			value := []byte(fmt.Sprintf("value-%d", idx))
			ks.Store(keyName, value, "")
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all keys exist
	for i := 0; i < 10; i++ {
		keyName := fmt.Sprintf("key-%d", i)
		if !ks.Exists(keyName) {
			t.Errorf("Key %s should exist", keyName)
		}
	}
}

func TestKeyStore_EmptyKeyName(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	err := ks.Store("", []byte("value"), "")
	if err == nil {
		t.Error("Store() should error for empty key name")
	}
}

func TestKeyStore_EmptyValue(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-keys.db")
	masterKey := []byte("test-master-key-32-bytes-long!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()

	err := ks.Store("key", []byte{}, "")
	if err == nil {
		t.Error("Store() should error for empty value")
	}
}

func BenchmarkKeyStore_Store(b *testing.B) {
	tmpDir := os.TempDir()
	dbPath := filepath.Join(tmpDir, "bench-keys.db")
	masterKey := []byte("bench-master-key-32-bytes!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()
	defer os.Remove(dbPath)

	value := []byte("benchmark-value-12345")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		keyName := fmt.Sprintf("key-%d", i)
		ks.Store(keyName, value, "")
	}
}

func BenchmarkKeyStore_Retrieve(b *testing.B) {
	tmpDir := os.TempDir()
	dbPath := filepath.Join(tmpDir, "bench-keys.db")
	masterKey := []byte("bench-master-key-32-bytes!")

	ks, _ := NewKeyStore(dbPath, masterKey)
	defer ks.Close()
	defer os.Remove(dbPath)

	ks.Store("bench-key", []byte("benchmark-value"), "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ks.Retrieve("bench-key")
	}
}
