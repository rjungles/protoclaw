// Package jobs provides asynchronous job queue functionality for AgentOS
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// Status represents job status
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Job represents an asynchronous job
type Job struct {
	ID          string
	Type        string
	SystemID    string
	Status      Status
	Progress    int
	Error       string
	Params      map[string]interface{}
	Result      map[string]interface{}
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// Queue manages asynchronous jobs
type Queue struct {
	db       *sql.DB
	handlers map[string]Handler
	workers  int
	mu       sync.RWMutex
	stop     chan bool
	wg       sync.WaitGroup
}

// Handler processes a job
type Handler func(ctx context.Context, job *Job) error

// NewQueue creates a new job queue
func NewQueue(dbPath string) (*Queue, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open jobs database: %w", err)
	}

	if err := createJobsSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create jobs schema: %w", err)
	}

	return &Queue{
		db:       db,
		handlers: make(map[string]Handler),
		workers:  3,
		stop:     make(chan bool),
	}, nil
}

func createJobsSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			system_id TEXT,
			status TEXT DEFAULT 'pending',
			progress INTEGER DEFAULT 0,
			error TEXT,
			params TEXT,
			result TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			started_at DATETIME,
			completed_at DATETIME
		);

		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_system ON jobs(system_id);
		CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at);
	`)
	return err
}

// RegisterHandler registers a handler for a job type
func (q *Queue) RegisterHandler(jobType string, handler Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[jobType] = handler
}

// Submit submits a new job
func (q *Queue) Submit(ctx context.Context, jobType, systemID string, params map[string]interface{}) (string, error) {
	id := uuid.New().String()

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal params: %w", err)
	}

	_, err = q.db.ExecContext(ctx, `
		INSERT INTO jobs (id, type, system_id, status, params, created_at)
		VALUES (?, ?, ?, 'pending', ?, datetime('now'))
	`, id, jobType, systemID, string(paramsJSON))

	if err != nil {
		return "", fmt.Errorf("failed to submit job: %w", err)
	}

	return id, nil
}

// Get retrieves a job by ID
func (q *Queue) Get(ctx context.Context, id string) (*Job, error) {
	var j Job
	var paramsJSON, resultJSON sql.NullString
	var startedAt, completedAt sql.NullTime

	err := q.db.QueryRowContext(ctx, `
		SELECT id, type, system_id, status, progress, error, params, result,
			created_at, started_at, completed_at
		FROM jobs WHERE id = ?
	`, id).Scan(
		&j.ID, &j.Type, &j.SystemID, &j.Status, &j.Progress, &j.Error,
		&paramsJSON, &resultJSON, &j.CreatedAt, &startedAt, &completedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		j.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}

	if paramsJSON.Valid {
		json.Unmarshal([]byte(paramsJSON.String), &j.Params)
	}
	if resultJSON.Valid {
		json.Unmarshal([]byte(resultJSON.String), &j.Result)
	}

	return &j, nil
}

// UpdateProgress updates job progress
func (q *Queue) UpdateProgress(ctx context.Context, id string, progress int) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	_, err := q.db.ExecContext(ctx, `
		UPDATE jobs SET progress = ? WHERE id = ?
	`, progress, id)

	return err
}

// UpdateStatus updates job status
func (q *Queue) UpdateStatus(ctx context.Context, id string, status Status, result map[string]interface{}, errMsg string) error {
	resultJSON, _ := json.Marshal(result)

	var completedAt interface{}
	if status == StatusCompleted || status == StatusFailed || status == StatusCancelled {
		completedAt = time.Now()
	}

	_, err := q.db.ExecContext(ctx, `
		UPDATE jobs 
		SET status = ?, result = ?, error = ?, completed_at = ?
		WHERE id = ?
	`, string(status), string(resultJSON), errMsg, completedAt, id)

	return err
}

// Cancel cancels a pending job
func (q *Queue) Cancel(ctx context.Context, id string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE jobs 
		SET status = 'cancelled', completed_at = datetime('now')
		WHERE id = ? AND status = 'pending'
	`, id)

	if err != nil {
		return err
	}

	// Check if any rows were updated
	var count int
	q.db.QueryRowContext(ctx, `SELECT changes()`).Scan(&count)
	if count == 0 {
		return fmt.Errorf("job not found or not in pending state")
	}

	return nil
}

// List returns jobs matching the filter
func (q *Queue) List(ctx context.Context, systemID string, status Status, limit int) ([]*Job, error) {
	query := `SELECT id, type, system_id, status, progress, error, params, result,
			created_at, started_at, completed_at
		FROM jobs WHERE 1=1`
	args := []interface{}{}

	if systemID != "" {
		query += " AND system_id = ?"
		args = append(args, systemID)
	}

	if status != "" {
		query += " AND status = ?"
		args = append(args, string(status))
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		var j Job
		var paramsJSON, resultJSON sql.NullString
		var startedAt, completedAt sql.NullTime

		err := rows.Scan(
			&j.ID, &j.Type, &j.SystemID, &j.Status, &j.Progress, &j.Error,
			&paramsJSON, &resultJSON, &j.CreatedAt, &startedAt, &completedAt,
		)
		if err != nil {
			return nil, err
		}

		if startedAt.Valid {
			j.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			j.CompletedAt = &completedAt.Time
		}

		if paramsJSON.Valid {
			json.Unmarshal([]byte(paramsJSON.String), &j.Params)
		}
		if resultJSON.Valid {
			json.Unmarshal([]byte(resultJSON.String), &j.Result)
		}

		jobs = append(jobs, &j)
	}

	return jobs, rows.Err()
}

// Start starts the worker pool
func (q *Queue) Start(ctx context.Context) {
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(ctx, i)
	}
}

// Stop stops the worker pool
func (q *Queue) Stop() {
	close(q.stop)
	q.wg.Wait()
}

// Close closes the queue
func (q *Queue) Close() error {
	q.Stop()
	return q.db.Close()
}

// worker processes jobs
func (q *Queue) worker(ctx context.Context, id int) {
	defer q.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.stop:
			return
		case <-ticker.C:
			q.processNextJob(ctx)
		}
	}
}

// processNextJob processes the next pending job
func (q *Queue) processNextJob(ctx context.Context) {
	// Get next pending job
	var jobID string
	err := q.db.QueryRowContext(ctx, `
		SELECT id FROM jobs 
		WHERE status = 'pending' 
		ORDER BY created_at ASC 
		LIMIT 1
	`).Scan(&jobID)

	if err == sql.ErrNoRows {
		return // No jobs
	}
	if err != nil {
		return // Error
	}

	// Get job details
	job, err := q.Get(ctx, jobID)
	if err != nil {
		return
	}

	// Get handler
	q.mu.RLock()
	handler, exists := q.handlers[job.Type]
	q.mu.RUnlock()

	if !exists {
		q.UpdateStatus(ctx, jobID, StatusFailed, nil, fmt.Sprintf("no handler for job type: %s", job.Type))
		return
	}

	// Mark as running
	q.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'running', started_at = datetime('now') WHERE id = ?
	`, jobID)

	// Execute handler
	err = handler(ctx, job)

	if err != nil {
		q.UpdateStatus(ctx, jobID, StatusFailed, nil, err.Error())
	} else {
		q.UpdateStatus(ctx, jobID, StatusCompleted, map[string]interface{}{"completed": true}, "")
	}
}

// GetPendingCount returns the number of pending jobs
func (q *Queue) GetPendingCount(ctx context.Context) (int, error) {
	var count int
	err := q.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs WHERE status = 'pending'
	`).Scan(&count)
	return count, err
}

// GetRunningCount returns the number of running jobs
func (q *Queue) GetRunningCount(ctx context.Context) (int, error) {
	var count int
	err := q.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs WHERE status = 'running'
	`).Scan(&count)
	return count, err
}

// Cleanup removes completed jobs older than the specified duration
func (q *Queue) Cleanup(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	_, err := q.db.ExecContext(ctx, `
		DELETE FROM jobs 
		WHERE status IN ('completed', 'failed', 'cancelled')
		AND completed_at < ?
	`, cutoff)
	return err
}
