package evolution

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type EvolutionResult struct {
	Success      bool
	AppliedSteps []MigrationStep
	FailedStep   *MigrationStep
	Error        error
	Warnings     []string
}

type ManifestVersion struct {
	ID               int64
	Version          string
	ManifestYAML     string
	CreatedAt        time.Time
	CreatedBy        string
	Description      string
	DiffFromPrevious string
}

type ManifestVersionStore interface {
	SaveVersion(version *ManifestVersion) error
	GetVersion(version string) (*ManifestVersion, error)
	GetLatestVersion() (*ManifestVersion, error)
	ListVersions() ([]ManifestVersion, error)
	DeleteVersion(version string) error
}

type DBManifestVersionStore struct {
	db *sql.DB
}

func NewDBManifestVersionStore(db *sql.DB) *DBManifestVersionStore {
	store := &DBManifestVersionStore{db: db}
	store.createTable()
	return store
}

func (s *DBManifestVersionStore) createTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS _manifest_versions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version TEXT NOT NULL UNIQUE,
		manifest_yaml TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_by TEXT,
		description TEXT,
		diff_from_previous TEXT
	)`

	_, err := s.db.Exec(query)
	return err
}

func (s *DBManifestVersionStore) SaveVersion(version *ManifestVersion) error {
	query := `INSERT INTO _manifest_versions (version, manifest_yaml, created_at, created_by, description, diff_from_previous)
			  VALUES (?, ?, ?, ?, ?, ?)`

	result, err := s.db.Exec(query,
		version.Version,
		version.ManifestYAML,
		version.CreatedAt,
		version.CreatedBy,
		version.Description,
		version.DiffFromPrevious,
	)
	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()
	version.ID = id
	return nil
}

func (s *DBManifestVersionStore) GetVersion(version string) (*ManifestVersion, error) {
	query := `SELECT id, version, manifest_yaml, created_at, created_by, description, diff_from_previous
			  FROM _manifest_versions WHERE version = ?`

	var v ManifestVersion
	err := s.db.QueryRow(query, version).Scan(
		&v.ID, &v.Version, &v.ManifestYAML, &v.CreatedAt, &v.CreatedBy, &v.Description, &v.DiffFromPrevious,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *DBManifestVersionStore) GetLatestVersion() (*ManifestVersion, error) {
	query := `SELECT id, version, manifest_yaml, created_at, created_by, description, diff_from_previous
			  FROM _manifest_versions ORDER BY created_at DESC LIMIT 1`

	var v ManifestVersion
	err := s.db.QueryRow(query).Scan(
		&v.ID, &v.Version, &v.ManifestYAML, &v.CreatedAt, &v.CreatedBy, &v.Description, &v.DiffFromPrevious,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *DBManifestVersionStore) ListVersions() ([]ManifestVersion, error) {
	query := `SELECT id, version, manifest_yaml, created_at, created_by, description, diff_from_previous
			  FROM _manifest_versions ORDER BY created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []ManifestVersion
	for rows.Next() {
		var v ManifestVersion
		if err := rows.Scan(&v.ID, &v.Version, &v.ManifestYAML, &v.CreatedAt, &v.CreatedBy, &v.Description, &v.DiffFromPrevious); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (s *DBManifestVersionStore) DeleteVersion(version string) error {
	query := `DELETE FROM _manifest_versions WHERE version = ?`
	_, err := s.db.Exec(query, version)
	return err
}

type EvolutionExecutor struct {
	manifest   *manifest.Manifest
	db         *sql.DB
	versioning ManifestVersionStore
}

func NewEvolutionExecutor(manifest *manifest.Manifest, db *sql.DB) *EvolutionExecutor {
	var versioning ManifestVersionStore
	if db != nil {
		versioning = NewDBManifestVersionStore(db)
	}

	return &EvolutionExecutor{
		manifest:   manifest,
		db:         db,
		versioning: versioning,
	}
}

func (e *EvolutionExecutor) Evolve(ctx context.Context, newManifest *manifest.Manifest) (*EvolutionResult, error) {
	result := &EvolutionResult{
		AppliedSteps: make([]MigrationStep, 0),
		Warnings:     make([]string, 0),
	}

	diff := DiffManifests(e.manifest, newManifest)

	if !diff.HasChanges() {
		result.Success = true
		return result, nil
	}

	plan := CreateMigrationPlan(diff, e.db, newManifest)

	applyResult, err := e.ApplyPlan(ctx, plan)
	if err != nil {
		return nil, err
	}

	if e.versioning != nil {
		yamlBytes, _ := newManifest.ToYAML()
		diffJSON, _ := json.Marshal(diff)

		version := &ManifestVersion{
			Version:          newManifest.Metadata.Version,
			ManifestYAML:     string(yamlBytes),
			CreatedAt:        time.Now(),
			CreatedBy:        "system",
			Description:      diff.Summary(),
			DiffFromPrevious: string(diffJSON),
		}

		if err := e.versioning.SaveVersion(version); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to save version: %v", err))
		}
	}

	e.manifest = newManifest

	return applyResult, nil
}

func (e *EvolutionExecutor) ApplyPlan(ctx context.Context, plan *MigrationPlan) (*EvolutionResult, error) {
	result := &EvolutionResult{
		AppliedSteps: make([]MigrationStep, 0),
		Warnings:     make([]string, 0),
	}

	if e.db == nil {
		result.Success = true
		result.Warnings = append(result.Warnings, "No database configured, skipping SQL execution")
		return result, nil
	}

	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	for _, step := range plan.Safe {
		if err := e.executeStepInTx(ctx, tx, &step); err != nil {
			tx.Rollback()
			result.Success = false
			result.FailedStep = &step
			result.Error = err
			return result, nil
		}
		result.AppliedSteps = append(result.AppliedSteps, step)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	if len(plan.Review) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d changes require manual review", len(plan.Review)))
	}

	if len(plan.Breaking) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d breaking changes detected (data will be deprecated, not deleted)", len(plan.Breaking)))
	}

	result.Success = true
	return result, nil
}

func (e *EvolutionExecutor) executeStep(ctx context.Context, step *MigrationStep) error {
	if e.db == nil || step.SQL == "" || strings.HasPrefix(step.SQL, "--") {
		return nil
	}

	_, err := e.db.ExecContext(ctx, step.SQL)
	return err
}

func (e *EvolutionExecutor) executeStepInTx(ctx context.Context, tx *sql.Tx, step *MigrationStep) error {
	if step.SQL == "" || strings.HasPrefix(step.SQL, "--") {
		return nil
	}

	_, err := tx.ExecContext(ctx, step.SQL)
	return err
}

func (e *EvolutionExecutor) Rollback(ctx context.Context, version string) error {
	if e.versioning == nil {
		return fmt.Errorf("versioning not configured")
	}

	v, err := e.versioning.GetVersion(version)
	if err != nil {
		return err
	}
	if v == nil {
		return fmt.Errorf("version not found: %s", version)
	}

	oldManifest, err := manifest.ParseYAML([]byte(v.ManifestYAML))
	if err != nil {
		return fmt.Errorf("failed to parse old manifest: %w", err)
	}

	_, err = e.Evolve(ctx, oldManifest)
	return err
}

func (e *EvolutionExecutor) GetCurrentVersion() string {
	if e.manifest == nil {
		return "unknown"
	}
	return e.manifest.Metadata.Version
}

func (e *EvolutionExecutor) GetVersionHistory() ([]ManifestVersion, error) {
	if e.versioning == nil {
		return nil, nil
	}
	return e.versioning.ListVersions()
}

func (e *EvolutionExecutor) GetCurrentManifest() *manifest.Manifest {
	return e.manifest
}

func (e *EvolutionExecutor) DiffWith(newManifest *manifest.Manifest) *ManifestDiff {
	return DiffManifests(e.manifest, newManifest)
}

func (e *EvolutionExecutor) CreatePlanFor(newManifest *manifest.Manifest) *MigrationPlan {
	diff := DiffManifests(e.manifest, newManifest)
	return CreateMigrationPlan(diff, e.db, newManifest)
}
