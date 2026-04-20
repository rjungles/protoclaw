package agentos

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/sipeed/picoclaw/pkg/api"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/manifest"
)

// EngineConfig configuração da AgentOS Engine
type EngineConfig struct {
	ManifestPath    string
	DataDir         string
	DBDriver        string
	DBConnection    string
	AutoEvolve      bool
	EnableMCP       bool
	EnableAPI       bool
	EnableWorkflows bool
}

// AgentOSEngine é o núcleo do sistema que orquestra todos os subsistemas
type AgentOSEngine struct {
	config           *EngineConfig
	instance         *SystemInstance
	manifestStore    ManifestStore
	evolutionManager *EvolutionManager
}

// NewEngine cria uma nova instância da AgentOS Engine
func NewEngine(config *EngineConfig) *AgentOSEngine {
	return &AgentOSEngine{
		config: config,
	}
}

// Bootstrap inicializa o sistema completo a partir do manifesto
func (e *AgentOSEngine) Bootstrap(ctx context.Context) (*SystemInstance, error) {
	log.Printf("Starting AgentOS Bootstrap...")

	// 1. Carregar e validar manifesto atual
	manifest, err := e.loadCurrentManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", err)
	}
	log.Printf("Loaded manifest version: %s", manifest.Metadata.Version)

	// 2. Detectar evolução se houver manifesto anterior
	if e.hasPreviousManifest() && e.config.AutoEvolve {
		log.Printf("Detected previous manifest, checking for evolution...")
		if err := e.evolveSystem(ctx, manifest); err != nil {
			return nil, fmt.Errorf("evolution failed: %w", err)
		}
	}

	// 3. Criar nova instância do sistema
	instance := e.createSystemInstance(manifest)

	// 4. Inicializar subsistemas
	if err := e.initializeSubsystems(ctx, instance); err != nil {
		return nil, fmt.Errorf("subsystem initialization failed: %w", err)
	}

	// 5. Registrar shutdown handlers
	e.setupShutdownHandlers(instance)

	log.Printf("AgentOS Bootstrap completed successfully")
	return instance, nil
}

// loadCurrentManifest carrega o manifesto atual
func (e *AgentOSEngine) loadCurrentManifest() (*manifest.Manifest, error) {
	if e.config.ManifestPath == "" {
		return nil, fmt.Errorf("manifest path not configured")
	}

	data, err := os.ReadFile(e.config.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	m, err := manifest.ParseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return m, nil
}

// hasPreviousManifest verifica se existe um manifesto anterior
func (e *AgentOSEngine) hasPreviousManifest() bool {
	if e.manifestStore == nil {
		return false
	}

	latest, err := e.manifestStore.GetLatestVersion()
	return err == nil && latest != nil
}

// evolveSystem detecta e aplica evoluções do sistema
func (e *AgentOSEngine) evolveSystem(ctx context.Context, newManifest *manifest.Manifest) error {
	if e.evolutionManager == nil {
		return fmt.Errorf("evolution manager not configured")
	}

	// Obter manifesto anterior
	previous, err := e.manifestStore.GetLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to get previous manifest: %w", err)
	}

	if previous == nil {
		return nil // Não há manifesto anterior
	}

	// Comparar e evoluir
	result, err := e.evolutionManager.Evolve(ctx, newManifest)
	if err != nil {
		return fmt.Errorf("evolution failed: %w", err)
	}

	if result.Success {
		log.Printf("System evolved successfully: %d steps applied", len(result.AppliedSteps))
	} else {
		return fmt.Errorf("evolution failed at step: %s", result.FailedStep.Description)
	}

	return nil
}

// createSystemInstance cria uma nova instância do sistema
func (e *AgentOSEngine) createSystemInstance(manifest *manifest.Manifest) *SystemInstance {
	return &SystemInstance{
		Manifest:      manifest,
		CreatedAt:     time.Now(),
		ShutdownFuncs: make([]func(context.Context) error, 0),
	}
}

// initializeSubsystems inicializa todos os subsistemas
func (e *AgentOSEngine) initializeSubsystems(ctx context.Context, instance *SystemInstance) error {
	log.Printf("Initializing subsystems...")

	// 1. Inicializar banco de dados
	if err := e.initializeDatabase(ctx, instance); err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
	}

	// 2. Configurar atores e permissões
	if err := e.initializeActors(ctx, instance); err != nil {
		return fmt.Errorf("actor initialization failed: %w", err)
	}

	// 3. Criar engines de workflow
	if e.config.EnableWorkflows {
		if err := e.initializeWorkflows(ctx, instance); err != nil {
			return fmt.Errorf("workflow initialization failed: %w", err)
		}
	}

	// 4. Configurar políticas e regras
	if err := e.initializePolicies(ctx, instance); err != nil {
		return fmt.Errorf("policy initialization failed: %w", err)
	}

	// 5. Inicializar gerador de APIs
	if e.config.EnableAPI {
		if err := e.initializeAPI(ctx, instance); err != nil {
			return fmt.Errorf("API initialization failed: %w", err)
		}
	}

	// 6. Configurar evolução
	if err := e.initializeEvolution(ctx, instance); err != nil {
		return fmt.Errorf("evolution initialization failed: %w", err)
	}

	log.Printf("All subsystems initialized successfully")
	return nil
}

// initializeDatabase configura o banco de dados
func (e *AgentOSEngine) initializeDatabase(ctx context.Context, instance *SystemInstance) error {
	if e.config.DBConnection == "" {
		return fmt.Errorf("database connection not configured")
	}

	db, err := sql.Open(e.config.DBDriver, e.config.DBConnection)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Testar conexão
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database connection test failed: %w", err)
	}

	instance.DB = db

	// Adicionar função de shutdown
	instance.AddShutdownFunc(func(ctx context.Context) error {
		return db.Close()
	})

	return nil
}

// initializeActors configura sistema de atores
func (e *AgentOSEngine) initializeActors(ctx context.Context, instance *SystemInstance) error {
	// Criar ActorStore
	actorStore := NewMemoryActorStore()
	if instance.DB != nil {
		actorStore = NewDBActorStore(instance.DB)
	}
	instance.ActorStore = actorStore

	// Provisionar atores do manifesto
	for _, actor := range instance.Manifest.Actors {
		if err := actorStore.CreateActor(&Actor{
			ID:          actor.ID,
			Name:        actor.Name,
			Type:        actor.Type,
			Permissions: actor.Permissions,
		}); err != nil {
			return fmt.Errorf("failed to create actor %s: %w", actor.ID, err)
		}
	}

	return nil
}

// initializeWorkflows configura sistema de workflows
func (e *AgentOSEngine) initializeWorkflows(ctx context.Context, instance *SystemInstance) error {
	if len(instance.Manifest.Workflows) == 0 {
		return nil
	}

	// Criar WorkflowEngine
	workflowEngine := NewWorkflowEngine(instance.Manifest, instance.DB)
	instance.WorkflowEngine = workflowEngine

	return nil
}

// initializePolicies configura sistema de políticas
func (e *AgentOSEngine) initializePolicies(ctx context.Context, instance *SystemInstance) error {
	// Criar PolicyEngine
	policyEngine := policy.NewEngine(instance.Manifest)
	instance.PolicyEngine = policyEngine

	// Criar RuleExecutor
	ruleExecutor := api.NewRuleExecutor(instance.Manifest, instance.DB)
	instance.RuleExecutor = ruleExecutor

	return nil
}

// initializeAPI configura sistema de APIs
func (e *AgentOSEngine) initializeAPI(ctx context.Context, instance *SystemInstance) error {
	// Criar OperationCatalog
	catalog := NewCatalog(instance.Manifest)
	instance.Catalog = catalog

	// Criar APIGenerator
	apiGenerator := api.NewGenerator(instance.Manifest, catalog, instance.DB)
	instance.APIGenerator = apiGenerator

	return nil
}

// initializeEvolution configura sistema de evolução
func (e *AgentOSEngine) initializeEvolution(ctx context.Context, instance *SystemInstance) error {
	if instance.DB == nil {
		return nil // Evolução requer banco de dados
	}

	// Criar ManifestStore
	manifestStore := NewDBManifestStore(instance.DB)
	e.manifestStore = manifestStore
	instance.ManifestStore = manifestStore

	// Criar EvolutionManager
	evolutionManager := NewEvolutionManager(instance)
	e.evolutionManager = evolutionManager
	instance.EvolutionManager = evolutionManager

	return nil
}

// setupShutdownHandlers configura handlers de shutdown
func (e *AgentOSEngine) setupShutdownHandlers(instance *SystemInstance) {
	// Adicionar handler para salvar estado atual
	instance.AddShutdownFunc(func(ctx context.Context) error {
		if e.manifestStore != nil {
			// Salvar versão atual do manifesto
			return e.manifestStore.SaveVersion(&ManifestVersion{
				Version:   instance.Manifest.Metadata.Version,
				CreatedAt: time.Now(),
				CreatedBy: "system",
			})
		}
		return nil
	})
}

// Shutdown desliga o sistema gracefulmente
func (e *AgentOSEngine) Shutdown(ctx context.Context) error {
	if e.instance != nil {
		return e.instance.Shutdown(ctx)
	}
	return nil
}

// GetInstance retorna a instância atual do sistema
func (e *AgentOSEngine) GetInstance() *SystemInstance {
	return e.instance
}

// GetManifestStore retorna o store de manifestos
func (e *AgentOSEngine) GetManifestStore() ManifestStore {
	return e.manifestStore
}

// GetEvolutionManager retorna o gerenciador de evolução
func (e *AgentOSEngine) GetEvolutionManager() *EvolutionManager {
	return e.evolutionManager
}
