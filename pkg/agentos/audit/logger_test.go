package audit

import (
	"context"
	"testing"
	"time"
)

func setupTestLogger(t *testing.T) (*Logger, func()) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test-audit.db"

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	cleanup := func() {
		logger.Close()
	}

	return logger, cleanup
}

func TestNewLogger(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	if logger == nil {
		t.Fatal("NewLogger() returned nil")
	}
}

func TestLogger_Log(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	ctx := context.Background()
	details := map[string]interface{}{
		"user":   "test-user",
		"action": "create",
	}

	err := logger.Log(ctx, OpSystemCreated, "system-1", "user-123", details)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Query to verify
	events, err := logger.Query(ctx, Filter{SystemID: "system-1"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(events) != 1 {
		t.Errorf("Query() returned %d events, want 1", len(events))
	}

	if events[0].Operation != OpSystemCreated {
		t.Errorf("Event.Operation = %q, want %q", events[0].Operation, OpSystemCreated)
	}

	if events[0].SystemID != "system-1" {
		t.Errorf("Event.SystemID = %q, want %q", events[0].SystemID, "system-1")
	}

	if events[0].UserID != "user-123" {
		t.Errorf("Event.UserID = %q, want %q", events[0].UserID, "user-123")
	}
}

func TestLogger_LogWithMetadata(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	ctx := context.Background()
	details := map[string]interface{}{"action": "test"}

	err := logger.LogWithMetadata(ctx, OpQueryExecuted, "system-1", "user-1", details, "192.168.1.1", "Mozilla/5.0")
	if err != nil {
		t.Fatalf("LogWithMetadata() error = %v", err)
	}

	events, _ := logger.Query(ctx, Filter{})
	if len(events) != 1 {
		t.Fatal("Expected 1 event")
	}

	if events[0].IPAddress != "192.168.1.1" {
		t.Errorf("Event.IPAddress = %q, want %q", events[0].IPAddress, "192.168.1.1")
	}

	if events[0].UserAgent != "Mozilla/5.0" {
		t.Errorf("Event.UserAgent = %q, want %q", events[0].UserAgent, "Mozilla/5.0")
	}
}

func TestLogger_QueryFilters(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create events
	logger.Log(ctx, OpSystemCreated, "sys-a", "user-1", nil)
	logger.Log(ctx, OpSystemBootstrapped, "sys-a", "user-1", nil)
	logger.Log(ctx, OpSystemCreated, "sys-b", "user-2", nil)
	logger.Log(ctx, OpQueryExecuted, "sys-a", "user-1", nil)

	// Test system filter
	events, _ := logger.Query(ctx, Filter{SystemID: "sys-a"})
	if len(events) != 3 {
		t.Errorf("Query(system-a) returned %d events, want 3", len(events))
	}

	// Test user filter
	events, _ = logger.Query(ctx, Filter{UserID: "user-1"})
	if len(events) != 3 {
		t.Errorf("Query(user-1) returned %d events, want 3", len(events))
	}

	// Test operation filter
	events, _ = logger.Query(ctx, Filter{Operation: OpSystemCreated})
	if len(events) != 2 {
		t.Errorf("Query(SystemCreated) returned %d events, want 2", len(events))
	}

	// Test since filter
	events, _ = logger.Query(ctx, Filter{Since: now.Add(-time.Hour)})
	if len(events) != 4 {
		t.Errorf("Query(since) returned %d events, want 4", len(events))
	}

	events, _ = logger.Query(ctx, Filter{Since: now.Add(time.Hour)})
	if len(events) != 0 {
		t.Errorf("Query(future since) returned %d events, want 0", len(events))
	}

	// Test limit
	events, _ = logger.Query(ctx, Filter{Limit: 2})
	if len(events) != 2 {
		t.Errorf("Query(limit=2) returned %d events, want 2", len(events))
	}
}

func TestLogger_GetSystemHistory(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	ctx := context.Background()

	// Create events for system
	for i := 0; i < 10; i++ {
		logger.Log(ctx, OpSystemCreated, "test-system", "user", nil)
	}

	history, err := logger.GetSystemHistory(ctx, "test-system", 5)
	if err != nil {
		t.Fatalf("GetSystemHistory() error = %v", err)
	}

	if len(history) != 5 {
		t.Errorf("GetSystemHistory() returned %d events, want 5", len(history))
	}
}

func TestLogger_GetUserActivity(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	ctx := context.Background()

	// Create events for user
	for i := 0; i < 10; i++ {
		logger.Log(ctx, OpQueryExecuted, "system", "test-user", nil)
	}

	activity, err := logger.GetUserActivity(ctx, "test-user", 3)
	if err != nil {
		t.Fatalf("GetUserActivity() error = %v", err)
	}

	if len(activity) != 3 {
		t.Errorf("GetUserActivity() returned %d events, want 3", len(activity))
	}
}

func TestLogger_Count(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	ctx := context.Background()

	// Test empty
	count, _ := logger.Count(ctx, Filter{})
	if count != 0 {
		t.Errorf("Count() = %d, want 0", count)
	}

	// Create events
	logger.Log(ctx, OpSystemCreated, "sys-a", "user-1", nil)
	logger.Log(ctx, OpSystemCreated, "sys-b", "user-1", nil)
	logger.Log(ctx, OpQueryExecuted, "sys-a", "user-2", nil)

	count, _ = logger.Count(ctx, Filter{})
	if count != 3 {
		t.Errorf("Count() = %d, want 3", count)
	}

	count, _ = logger.Count(ctx, Filter{SystemID: "sys-a"})
	if count != 2 {
		t.Errorf("Count(system-a) = %d, want 2", count)
	}

	count, _ = logger.Count(ctx, Filter{UserID: "user-1"})
	if count != 2 {
		t.Errorf("Count(user-1) = %d, want 2", count)
	}
}

func TestLogger_Cleanup(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	ctx := context.Background()

	// Create old event
	logger.Log(ctx, OpSystemCreated, "system", "user", nil)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Cleanup old events
	err := logger.Cleanup(ctx, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Event should be deleted
	count, _ := logger.Count(ctx, Filter{})
	if count != 0 {
		t.Errorf("Count after cleanup = %d, want 0", count)
	}
}

func TestLogger_Export(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	ctx := context.Background()

	logger.Log(ctx, OpSystemCreated, "system", "user", map[string]interface{}{"version": "1.0"})

	data, err := logger.Export(ctx, Filter{})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Export() returned empty data")
	}

	// Should be valid JSON
	if data[0] != '[' {
		t.Error("Export() should return JSON array")
	}
}

func TestOperationString(t *testing.T) {
	tests := []struct {
		op   Operation
		want string
	}{
		{OpSystemCreated, "system_created"},
		{OpSystemDeleted, "system_deleted"},
		{OpSystemBootstrapped, "system_bootstrapped"},
		{OpQueryExecuted, "query_executed"},
		{OpHealthCheck, "health_check"},
	}

	for _, tt := range tests {
		if string(tt.op) != tt.want {
			t.Errorf("Operation(%q) = %q, want %q", tt.op, string(tt.op), tt.want)
		}
	}
}

func TestHelperMethods(t *testing.T) {
	logger, cleanup := setupTestLogger(t)
	defer cleanup()

	ctx := context.Background()

	// Test LogSystemCreated
	logger.LogSystemCreated(ctx, "system-1", "user-1", map[string]interface{}{"name": "test"})

	// Test LogSystemDeleted
	logger.LogSystemDeleted(ctx, "system-1", "user-1", nil)

	// Test LogSystemBootstrapped
	logger.LogSystemBootstrapped(ctx, "system-1", "user-1", nil)

	// Test LogQuery
	logger.LogQuery(ctx, "system-1", "user-1", "User", map[string]interface{}{"id": "123"})

	// Test LogJobCompleted
	logger.LogJobCompleted(ctx, "system-1", "job-123", 5*time.Second)

	// Test LogJobFailed
	logger.LogJobFailed(ctx, "system-1", "job-456", errors.New("test error"))

	// Test LogHealthCheck
	logger.LogHealthCheck(ctx, "system-1", "database", "healthy", 10*time.Millisecond)

	// Verify all logged
	count, _ := logger.Count(ctx, Filter{})
	if count != 7 {
		t.Errorf("Count() = %d, want 7", count)
	}
}

func BenchmarkLoggerLog(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := tmpDir + "/bench-audit.db"
	logger, _ := NewLogger(dbPath)
	defer logger.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Log(ctx, OpSystemCreated, "system", "user", map[string]interface{}{"key": i})
	}
}

func BenchmarkLoggerQuery(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := tmpDir + "/bench-audit.db"
	logger, _ := NewLogger(dbPath)
	defer logger.Close()

	ctx := context.Background()

	// Create events
	for i := 0; i < 100; i++ {
		logger.Log(ctx, OpSystemCreated, "system", "user", nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Query(ctx, Filter{SystemID: "system", Limit: 10})
	}
}
