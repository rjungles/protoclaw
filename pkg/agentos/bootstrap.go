package agentos

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sipeed/picoclaw/pkg/api"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/infra/db"
	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/workflow"

	_ "github.com/mattn/go-sqlite3"
)

type BootstrapConfig struct {
	ManifestPath string
	Manifest     *manifest.Manifest
	DBDriver     string
	DBConnection string
	DataDir      string

	SkipMigrate bool
	SkipAudit   bool
	SkipMCP     bool
}

type SystemInstance struct {
	Manifest     *manifest.Manifest
	DB           *sql.DB
	Catalog      *OperationCatalog
	ActorStore   ActorStore
	PolicyEngine *policy.Engine
	RuleExecutor *api.RuleExecutor
	APIGenerator *api.Generator
	Migrator     *db.Migrator

	WorkflowEngines map[string]*workflow.FSM

	HTTPMux       *http.ServeMux
	ShutdownFuncs []func(context.Context) error
}

type Bootstrapper struct {
	config BootstrapConfig
}

func NewBootstrapper(cfg BootstrapConfig) *Bootstrapper {
	return &Bootstrapper{config: cfg}
}

func (b *Bootstrapper) Bootstrap(ctx context.Context) (*SystemInstance, error) {
	instance := &SystemInstance{
		ShutdownFuncs:   make([]func(context.Context) error, 0),
		WorkflowEngines: make(map[string]*workflow.FSM),
	}

	if err := b.loadManifest(instance); err != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", err)
	}

	if err := b.validateManifest(instance); err != nil {
		return nil, fmt.Errorf("failed to validate manifest: %w", err)
	}

	if err := b.openDatabase(ctx, instance); err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if !b.config.SkipMigrate {
		if err := b.runMigrations(ctx, instance); err != nil {
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	if err := b.provisionActors(instance); err != nil {
		return nil, fmt.Errorf("failed to provision actors: %w", err)
	}

	if err := b.buildOperationCatalog(instance); err != nil {
		return nil, fmt.Errorf("failed to build operation catalog: %w", err)
	}

	if err := b.createPolicyEngine(instance); err != nil {
		return nil, fmt.Errorf("failed to create policy engine: %w", err)
	}

	if err := b.createRuleExecutor(instance); err != nil {
		return nil, fmt.Errorf("failed to create rule executor: %w", err)
	}

	if err := b.createWorkflowEngines(instance); err != nil {
		return nil, fmt.Errorf("failed to create workflow engines: %w", err)
	}

	if err := b.createAPIGenerator(instance); err != nil {
		return nil, fmt.Errorf("failed to create API generator: %w", err)
	}

	b.mountHTTPMux(instance)

	instance.ShutdownFuncs = append(instance.ShutdownFuncs, func(ctx context.Context) error {
		if instance.DB != nil {
			return instance.DB.Close()
		}
		return nil
	})

	return instance, nil
}

func (b *Bootstrapper) loadManifest(instance *SystemInstance) error {
	if b.config.Manifest != nil {
		instance.Manifest = b.config.Manifest
		return nil
	}

	if b.config.ManifestPath == "" {
		return fmt.Errorf("no manifest provided: set ManifestPath or Manifest")
	}

	m, err := manifest.ParseFile(b.config.ManifestPath)
	if err != nil {
		return err
	}

	instance.Manifest = m
	return nil
}

func (b *Bootstrapper) validateManifest(instance *SystemInstance) error {
	parser := &manifest.Parser{}
	if err := parser.Validate(instance.Manifest); err != nil {
		return err
	}

	if len(instance.Manifest.Actors) == 0 {
		return fmt.Errorf("manifest must have at least one actor defined")
	}

	return nil
}

func (b *Bootstrapper) openDatabase(ctx context.Context, instance *SystemInstance) error {
	driver := b.config.DBDriver
	connection := b.config.DBConnection

	if driver == "" {
		driver = "sqlite"
	}

	if connection == "" {
		dataDir := b.config.DataDir
		if dataDir == "" {
			dataDir = "."
		}
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return fmt.Errorf("failed to create data directory: %w", err)
		}
		dbName := fmt.Sprintf("%s.db", instance.Manifest.Metadata.Name)
		connection = filepath.Join(dataDir, dbName)
	}

	var db *sql.DB
	var err error

	switch driver {
	case "sqlite":
		db, err = sql.Open("sqlite3", connection)
	case "postgres":
		db, err = sql.Open("postgres", connection)
	case "mysql":
		db, err = sql.Open("mysql", connection)
	default:
		db, err = sql.Open(driver, connection)
	}

	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	instance.DB = db

	return nil
}

func (b *Bootstrapper) runMigrations(ctx context.Context, instance *SystemInstance) error {
	migrator := db.NewMigrator(db.NewSQLDB(instance.DB), instance.Manifest)
	instance.Migrator = migrator

	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

func (b *Bootstrapper) provisionActors(instance *SystemInstance) error {
	if instance.DB != nil {
		instance.ActorStore = NewDBActorStore(instance.DB)
	} else {
		instance.ActorStore = NewMemoryActorStore()
	}

	for _, actor := range instance.Manifest.Actors {
		cred, err := instance.ActorStore.Provision(actor)
		if err != nil {
			if err == ErrActorExists {
				continue
			}
			return fmt.Errorf("failed to provision actor %s: %w", actor.ID, err)
		}
		_ = cred
	}

	return nil
}

func (b *Bootstrapper) buildOperationCatalog(instance *SystemInstance) error {
	instance.Catalog = NewCatalog(instance.Manifest)
	return nil
}

func (b *Bootstrapper) createPolicyEngine(instance *SystemInstance) error {
	engine, err := policy.NewEngine(instance.Manifest)
	if err != nil {
		return err
	}
	instance.PolicyEngine = engine
	return nil
}

func (b *Bootstrapper) createRuleExecutor(instance *SystemInstance) error {
	instance.RuleExecutor = api.NewRuleExecutor(instance.Manifest)
	return nil
}

func (b *Bootstrapper) createWorkflowEngines(instance *SystemInstance) error {
	if len(instance.Manifest.Workflows) == 0 {
		instance.WorkflowEngines = make(map[string]*workflow.FSM)
		return nil
	}

	for _, wf := range instance.Manifest.Workflows {
		states := make(map[workflow.State]workflow.StateConfig)
		for _, s := range wf.States {
			transitions := make([]workflow.Transition, len(s.Transitions))
			for i, t := range s.Transitions {
				transitions[i] = workflow.Transition{
					To:           workflow.State(t.To),
					Action:       workflow.Action(t.Action),
					AllowedRoles: t.AllowedRoles,
				}
			}
			states[workflow.State(s.ID)] = workflow.StateConfig{
				ID:          workflow.State(s.ID),
				Transitions: transitions,
			}
		}

		fsmConfig := workflow.FSMConfig{
			EntityName:   wf.Entity,
			InitialState: workflow.State(wf.InitialState),
			States:       states,
		}

		fsm, err := workflow.NewFSM(fsmConfig)
		if err != nil {
			return fmt.Errorf("failed to create FSM for %s: %w", wf.Entity, err)
		}

		instance.WorkflowEngines[wf.Entity] = fsm
	}

	return nil
}

func (b *Bootstrapper) createAPIGenerator(instance *SystemInstance) error {
	var gen *api.Generator
	var err error

	if instance.DB != nil {
		gen, err = api.NewGeneratorWithDB(instance.Manifest, instance.DB)
	} else {
		gen, err = api.NewGenerator(instance.Manifest)
	}

	if err != nil {
		return err
	}

	instance.APIGenerator = gen
	return nil
}

func (b *Bootstrapper) mountHTTPMux(instance *SystemInstance) {
	apiMux, _ := instance.APIGenerator.BuildMux()

	authMiddleware := NewAuthMiddleware(instance.ActorStore)

	systemMux := http.NewServeMux()
	systemMux.HandleFunc("GET /_system/info", instance.serveSystemInfo)
	systemMux.HandleFunc("GET /_system/actors", instance.serveListActors)
	systemMux.HandleFunc("GET /_system/operations", instance.serveListOperations)
	systemMux.HandleFunc("GET /_health", instance.serveHealth)

	compositeMux := http.NewServeMux()
	compositeMux.Handle("/", authMiddleware.Wrap(apiMux))
	compositeMux.Handle("/_system/", systemMux)

	instance.HTTPMux = compositeMux
}

func (si *SystemInstance) serveSystemInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"name":             si.Manifest.Metadata.Name,
		"version":          si.Manifest.Metadata.Version,
		"description":      si.Manifest.Metadata.Description,
		"actors_count":     len(si.Manifest.Actors),
		"entities_count":   len(si.Manifest.DataModel.Entities),
		"rules_count":      len(si.Manifest.BusinessRules),
		"workflows_count":  len(si.Manifest.Workflows),
		"operations_count": len(si.Catalog.ListAll()),
		"has_database":     si.DB != nil,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (si *SystemInstance) serveListActors(w http.ResponseWriter, r *http.Request) {
	actors, err := si.ActorStore.ListAll()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list actors: %v", err), http.StatusInternalServerError)
		return
	}

	result := make([]map[string]interface{}, 0, len(actors))
	for _, actor := range actors {
		result = append(result, map[string]interface{}{
			"actor_id":   actor.ActorID,
			"actor_type": actor.ActorType,
			"roles":      actor.Roles,
			"is_active":  actor.IsActive,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"actors": result,
		"count":  len(result),
	})
}

func (si *SystemInstance) serveListOperations(w http.ResponseWriter, r *http.Request) {
	operations := si.Catalog.ListAll()

	result := make([]map[string]interface{}, 0, len(operations))
	for _, op := range operations {
		result = append(result, map[string]interface{}{
			"name":        op.Name,
			"entity":      op.Entity,
			"action":      op.Action,
			"method":      op.Method,
			"path":        op.Path,
			"permissions": op.Permissions,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"operations": result,
		"count":      len(result),
	})
}

func (si *SystemInstance) serveHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]string{"status": "ok"}

	if si.DB != nil {
		if err := si.DB.Ping(); err != nil {
			health["status"] = "degraded"
			health["database"] = "unreachable"
		} else {
			health["database"] = "ok"
		}
	} else {
		health["database"] = "not_configured"
	}

	status := http.StatusOK
	if health["status"] != "ok" {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(health)
}

func (si *SystemInstance) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	si.HTTPMux.ServeHTTP(w, r)
}

func (si *SystemInstance) Shutdown(ctx context.Context) error {
	for _, fn := range si.ShutdownFuncs {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (si *SystemInstance) GetOperationCatalog() *OperationCatalog {
	return si.Catalog
}

func (si *SystemInstance) GetActorStore() ActorStore {
	return si.ActorStore
}

func (si *SystemInstance) GetPolicyEngine() *policy.Engine {
	return si.PolicyEngine
}

func (si *SystemInstance) GetWorkflowEngine(entityType string) *workflow.FSM {
	return si.WorkflowEngines[entityType]
}
