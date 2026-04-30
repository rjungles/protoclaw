package jobs

import (
	"context"
	"testing"
	"time"
)

func setupTestQueue(t *testing.T) (*Queue, func()) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test-jobs.db"

	queue, err := NewQueue(dbPath)
	if err != nil {
		t.Fatalf("NewQueue() error = %v", err)
	}

	cleanup := func() {
		queue.Close()
	}

	return queue, cleanup
}

func TestNewQueue(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	if queue == nil {
		t.Fatal("NewQueue() returned nil")
	}
}

func TestQueue_RegisterHandler(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	handler := func(ctx context.Context, job *Job) error {
		return nil
	}

	queue.RegisterHandler("test", handler)

	// Verify handler is registered
	queue.mu.RLock()
	_, exists := queue.handlers["test"]
	queue.mu.RUnlock()

	if !exists {
		t.Error("Handler should be registered")
	}
}

func TestQueue_Submit(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	params := map[string]interface{}{
		"key": "value",
	}

	jobID, err := queue.Submit(ctx, "test", "system-1", params)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if jobID == "" {
		t.Error("Submit() returned empty job ID")
	}

	// Verify job exists
	job, err := queue.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if job.Type != "test" {
		t.Errorf("Job.Type = %q, want %q", job.Type, "test")
	}

	if job.SystemID != "system-1" {
		t.Errorf("Job.SystemID = %q, want %q", job.SystemID, "system-1")
	}

	if job.Status != StatusPending {
		t.Errorf("Job.Status = %q, want %q", job.Status, StatusPending)
	}

	if job.Params["key"] != "value" {
		t.Errorf("Job.Params['key'] = %v, want %v", job.Params["key"], "value")
	}
}

func TestQueue_GetNotFound(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	_, err := queue.Get(ctx, "non-existent")
	if err == nil {
		t.Error("Get() should error for non-existent job")
	}
}

func TestQueue_UpdateProgress(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	jobID, _ := queue.Submit(ctx, "test", "system-1", nil)

	err := queue.UpdateProgress(ctx, jobID, 50)
	if err != nil {
		t.Fatalf("UpdateProgress() error = %v", err)
	}

	job, _ := queue.Get(ctx, jobID)
	if job.Progress != 50 {
		t.Errorf("Job.Progress = %d, want 50", job.Progress)
	}
}

func TestQueue_UpdateProgressBounds(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	jobID, _ := queue.Submit(ctx, "test", "system-1", nil)

	// Test negative progress (should be clamped to 0)
	queue.UpdateProgress(ctx, jobID, -10)
	job, _ := queue.Get(ctx, jobID)
	if job.Progress != 0 {
		t.Errorf("Negative progress should be clamped to 0, got %d", job.Progress)
	}

	// Test > 100 progress (should be clamped to 100)
	queue.UpdateProgress(ctx, jobID, 150)
	job, _ = queue.Get(ctx, jobID)
	if job.Progress != 100 {
		t.Errorf("Progress > 100 should be clamped to 100, got %d", job.Progress)
	}
}

func TestQueue_Cancel(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	jobID, _ := queue.Submit(ctx, "test", "system-1", nil)

	err := queue.Cancel(ctx, jobID)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	job, _ := queue.Get(ctx, jobID)
	if job.Status != StatusCancelled {
		t.Errorf("Job.Status = %q, want %q", job.Status, StatusCancelled)
	}
}

func TestQueue_CancelRunning(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	jobID, _ := queue.Submit(ctx, "test", "system-1", nil)

	// Manually update to running
	queue.UpdateStatus(ctx, jobID, StatusRunning, nil, "")

	// Try to cancel running job
	err := queue.Cancel(ctx, jobID)
	if err == nil {
		t.Error("Cancel() should error for running job")
	}
}

func TestQueue_UpdateStatus(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()
	jobID, _ := queue.Submit(ctx, "test", "system-1", nil)

	result := map[string]interface{}{"output": "data"}
	err := queue.UpdateStatus(ctx, jobID, StatusCompleted, result, "")
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	job, _ := queue.Get(ctx, jobID)
	if job.Status != StatusCompleted {
		t.Errorf("Job.Status = %q, want %q", job.Status, StatusCompleted)
	}

	if job.Result["output"] != "data" {
		t.Errorf("Job.Result['output'] = %v, want %v", job.Result["output"], "data")
	}
}

func TestQueue_List(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// Create jobs
	for i := 0; i < 5; i++ {
		queue.Submit(ctx, "test", "system-1", nil)
	}
	for i := 0; i < 3; i++ {
		queue.Submit(ctx, "test", "system-2", nil)
	}

	// List all
	jobs, err := queue.List(ctx, "", "", 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(jobs) != 8 {
		t.Errorf("List() returned %d jobs, want 8", len(jobs))
	}

	// Filter by system
	jobs, _ = queue.List(ctx, "system-1", "", 0)
	if len(jobs) != 5 {
		t.Errorf("List(system-1) returned %d jobs, want 5", len(jobs))
	}

	// Filter by status
	jobs, _ = queue.List(ctx, "", StatusPending, 0)
	if len(jobs) != 8 {
		t.Errorf("List(pending) returned %d jobs, want 8", len(jobs))
	}

	// Limit
	jobs, _ = queue.List(ctx, "", "", 3)
	if len(jobs) != 3 {
		t.Errorf("List(limit=3) returned %d jobs, want 3", len(jobs))
	}
}

func TestQueue_GetPendingCount(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// No pending jobs initially
	count, _ := queue.GetPendingCount(ctx)
	if count != 0 {
		t.Errorf("Pending count = %d, want 0", count)
	}

	// Add jobs
	queue.Submit(ctx, "test", "system-1", nil)
	queue.Submit(ctx, "test", "system-1", nil)

	count, _ = queue.GetPendingCount(ctx)
	if count != 2 {
		t.Errorf("Pending count = %d, want 2", count)
	}
}

func TestQueue_GetRunningCount(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	jobID, _ := queue.Submit(ctx, "test", "system-1", nil)
	queue.UpdateStatus(ctx, jobID, StatusRunning, nil, "")

	count, _ := queue.GetRunningCount(ctx)
	if count != 1 {
		t.Errorf("Running count = %d, want 1", count)
	}
}

func TestQueue_Cleanup(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// Create old completed job
	jobID, _ := queue.Submit(ctx, "test", "system-1", nil)
	queue.UpdateStatus(ctx, jobID, StatusCompleted, nil, "")

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Cleanup old jobs
	err := queue.Cleanup(ctx, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Job should be deleted
	_, err = queue.Get(ctx, jobID)
	if err == nil {
		t.Error("Old job should be deleted after cleanup")
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("Status(%q) = %q, want %q", tt.status, string(tt.status), tt.want)
		}
	}
}

func BenchmarkQueueSubmit(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := tmpDir + "/bench-jobs.db"
	queue, _ := NewQueue(dbPath)
	defer queue.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queue.Submit(ctx, "test", "system-1", map[string]interface{}{"key": i})
	}
}

func BenchmarkQueueGet(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := tmpDir + "/bench-jobs.db"
	queue, _ := NewQueue(dbPath)
	defer queue.Close()

	ctx := context.Background()
	jobID, _ := queue.Submit(ctx, "test", "system-1", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queue.Get(ctx, jobID)
	}
}
