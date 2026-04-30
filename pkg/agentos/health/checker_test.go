package health

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestNewChecker(t *testing.T) {
	checker := NewChecker(30*time.Second, nil)

	if checker == nil {
		t.Fatal("NewChecker() returned nil")
	}

	if checker.interval != 30*time.Second {
		t.Errorf("interval = %v, want %v", checker.interval, 30*time.Second)
	}

	if len(checker.checks) != 0 {
		t.Errorf("checks count = %d, want 0", len(checker.checks))
	}
}

func TestChecker_Register(t *testing.T) {
	checker := NewChecker(30*time.Second, nil)

	check := func(ctx context.Context) CheckResult {
		return CheckResult{Status: StatusHealthy}
	}

	checker.Register("test-check", check)

	if len(checker.checks) != 1 {
		t.Errorf("checks count = %d, want 1", len(checker.checks))
	}

	if _, exists := checker.checks["test-check"]; !exists {
		t.Error("check should be registered")
	}
}

func TestChecker_Unregister(t *testing.T) {
	checker := NewChecker(30*time.Second, nil)

	check := func(ctx context.Context) CheckResult {
		return CheckResult{Status: StatusHealthy}
	}

	checker.Register("test-check", check)
	checker.Unregister("test-check")

	if len(checker.checks) != 0 {
		t.Errorf("checks count = %d, want 0", len(checker.checks))
	}
}

func TestChecker_Check(t *testing.T) {
	checker := NewChecker(30*time.Second, nil)

	check := func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:  StatusHealthy,
			Message: "all good",
			Latency: 10 * time.Millisecond,
		}
	}

	checker.Register("healthy-check", check)

	ctx := context.Background()
	result, err := checker.Check(ctx, "healthy-check")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusHealthy)
	}

	if result.Message != "all good" {
		t.Errorf("Message = %q, want %q", result.Message, "all good")
	}

	if result.Component != "healthy-check" {
		t.Errorf("Component = %q, want %q", result.Component, "healthy-check")
	}
}

func TestChecker_CheckNotFound(t *testing.T) {
	checker := NewChecker(30*time.Second, nil)

	ctx := context.Background()
	_, err := checker.Check(ctx, "non-existent")
	if err == nil {
		t.Error("Check() should error for non-existent check")
	}
}

func TestChecker_CheckAll(t *testing.T) {
	checker := NewChecker(30*time.Second, nil)

	checker.Register("check-1", func(ctx context.Context) CheckResult {
		return CheckResult{Status: StatusHealthy}
	})

	checker.Register("check-2", func(ctx context.Context) CheckResult {
		return CheckResult{Status: StatusDegraded}
	})

	ctx := context.Background()
	results := checker.CheckAll(ctx)

	if len(results) != 2 {
		t.Errorf("results count = %d, want 2", len(results))
	}

	if results["check-1"].Status != StatusHealthy {
		t.Errorf("check-1 status = %v, want %v", results["check-1"].Status, StatusHealthy)
	}

	if results["check-2"].Status != StatusDegraded {
		t.Errorf("check-2 status = %v, want %v", results["check-2"].Status, StatusDegraded)
	}
}

func TestChecker_GetResult(t *testing.T) {
	checker := NewChecker(30*time.Second, nil)

	checker.Register("test-check", func(ctx context.Context) CheckResult {
		return CheckResult{Status: StatusHealthy}
	})

	ctx := context.Background()
	checker.Check(ctx, "test-check")

	result, err := checker.GetResult("test-check")
	if err != nil {
		t.Fatalf("GetResult() error = %v", err)
	}

	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusHealthy)
	}
}

func TestChecker_GetResultNotFound(t *testing.T) {
	checker := NewChecker(30*time.Second, nil)

	_, err := checker.GetResult("non-existent")
	if err == nil {
		t.Error("GetResult() should error for non-existent check")
	}
}

func TestChecker_GetComponentHealth(t *testing.T) {
	checker := NewChecker(30*time.Second, nil)

	checker.Register("test-check", func(ctx context.Context) CheckResult {
		return CheckResult{Status: StatusHealthy}
	})

	ctx := context.Background()
	checker.Check(ctx, "test-check")

	health, err := checker.GetComponentHealth("test-check")
	if err != nil {
		t.Fatalf("GetComponentHealth() error = %v", err)
	}

	if health.Name != "test-check" {
		t.Errorf("Name = %q, want %q", health.Name, "test-check")
	}

	if health.Status != StatusHealthy {
		t.Errorf("Status = %v, want %v", health.Status, StatusHealthy)
	}

	if health.Successes != 1 {
		t.Errorf("Successes = %d, want 1", health.Successes)
	}
}

func TestChecker_GetOverallStatus(t *testing.T) {
	tests := []struct {
		name     string
		statuses []Status
		want     Status
	}{
		{"all healthy", []Status{StatusHealthy, StatusHealthy}, StatusHealthy},
		{"one degraded", []Status{StatusHealthy, StatusDegraded}, StatusDegraded},
		{"one unhealthy", []Status{StatusHealthy, StatusUnhealthy}, StatusUnhealthy},
		{"degraded and unhealthy", []Status{StatusDegraded, StatusUnhealthy}, StatusUnhealthy},
		{"empty", []Status{}, StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewChecker(30*time.Second, nil)

			for i, status := range tt.statuses {
				status := status // capture range variable
				checker.Register(string(rune('a'+i)), func(ctx context.Context) CheckResult {
					return CheckResult{Status: status}
				})
			}

			if tt.name != "empty" {
				ctx := context.Background()
				checker.CheckAll(ctx)
			}

			got := checker.GetOverallStatus()
			if got != tt.want {
				t.Errorf("GetOverallStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusHealthy, "healthy"},
		{StatusDegraded, "degraded"},
		{StatusUnhealthy, "unhealthy"},
		{StatusUnknown, "unknown"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("Status(%q) = %q, want %q", tt.status, string(tt.status), tt.want)
		}
	}
}

func TestDatabaseHealthCheck(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	check := DatabaseHealthCheck(db)
	ctx := context.Background()

	result := check(ctx)

	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusHealthy)
	}

	if result.Message != "database responsive" {
		t.Errorf("Message = %q, want %q", result.Message, "database responsive")
	}
}

func TestDatabaseHealthCheck_Failure(t *testing.T) {
	// Create closed database
	db, _ := sql.Open("sqlite3", ":memory:")
	db.Close() // Close immediately

	check := DatabaseHealthCheck(db)
	ctx := context.Background()

	result := check(ctx)

	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusUnhealthy)
	}
}

func TestLLMProviderHealthCheck(t *testing.T) {
	// Success case
	check := LLMProviderHealthCheck("test-provider", func() error {
		return nil
	})

	ctx := context.Background()
	result := check(ctx)

	if result.Status != StatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusHealthy)
	}

	// Failure case
	check = LLMProviderHealthCheck("test-provider", func() error {
		return errors.New("connection failed")
	})

	result = check(ctx)
	if result.Status != StatusUnhealthy {
		t.Errorf("Status = %v, want %v", result.Status, StatusUnhealthy)
	}
}

func TestCompositeCheck(t *testing.T) {
	tests := []struct {
		name    string
		results []CheckResult
		want    Status
	}{
		{
			name: "all healthy",
			results: []CheckResult{
				{Status: StatusHealthy},
				{Status: StatusHealthy},
			},
			want: StatusHealthy,
		},
		{
			name: "one degraded",
			results: []CheckResult{
				{Status: StatusHealthy},
				{Status: StatusDegraded},
			},
			want: StatusDegraded,
		},
		{
			name: "one unhealthy",
			results: []CheckResult{
				{Status: StatusHealthy},
				{Status: StatusUnhealthy},
			},
			want: StatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks := make([]Check, len(tt.results))
			for i, r := range tt.results {
				result := r // capture
				checks[i] = func(ctx context.Context) CheckResult {
					return result
				}
			}

			composite := CompositeCheck(checks...)
			ctx := context.Background()
			result := composite(ctx)

			if result.Status != tt.want {
				t.Errorf("Status = %v, want %v", result.Status, tt.want)
			}
		})
	}
}

func TestChecker_StartStop(t *testing.T) {
	checker := NewChecker(100*time.Millisecond, nil)

	checkRan := make(chan bool, 1)
	checker.Register("test-check", func(ctx context.Context) CheckResult {
		select {
		case checkRan <- true:
		default:
		}
		return CheckResult{Status: StatusHealthy}
	})

	ctx := context.Background()
	checker.Start(ctx)

	// Wait for at least one check
	select {
	case <-checkRan:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Check should have run")
	}

	checker.Stop()
}

func BenchmarkCheck(b *testing.B) {
	checker := NewChecker(30*time.Second, nil)

	checker.Register("bench-check", func(ctx context.Context) CheckResult {
		return CheckResult{Status: StatusHealthy}
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.Check(ctx, "bench-check")
	}
}

func BenchmarkCheckAll(b *testing.B) {
	checker := NewChecker(30*time.Second, nil)

	for i := 0; i < 10; i++ {
		checker.Register(string(rune('a'+i)), func(ctx context.Context) CheckResult {
			return CheckResult{Status: StatusHealthy}
		})
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		checker.CheckAll(ctx)
	}
}
