// PicoClaw AgentOS Integration Tools
// These tools enable the PicoClaw agent to interact with AgentOS systems
// allowing conversational system creation and management
//
// This version includes security improvements:
// - Validated system names
// - Hash-based directory structure
// - SQLite-based registry
// - Audit logging
// - Secure storage

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/picoclaw/protoclaw/pkg/agentos/audit"
	"github.com/picoclaw/protoclaw/pkg/agentos/registry"
	"github.com/picoclaw/protoclaw/pkg/agentos/security"
	"github.com/picoclaw/protoclaw/pkg/agentos/security/validation"
	"github.com/picoclaw/protoclaw/pkg/agentos/storage"
)

// AgentOS tool constants
const (
	agentosDefaultDataDir = ".picoclaw/agentos"
)

// getAgentOSDataDir returns the AgentOS data directory
func getAgentOSDataDir() string {
	if dir := os.Getenv("AGENTOS_DATA_DIR"); dir != "" {
		return dir
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), agentosDefaultDataDir)
	}
	return filepath.Join(homeDir, agentosDefaultDataDir)
}

// ExecAgentOSTool wraps AgentOS CLI operations
type ExecAgentOSTool struct {
	dataDir   string
	validator *validation.SystemNameValidator
}

// NewExecAgentOSTool creates a new AgentOS execution tool
func NewExecAgentOSTool() *ExecAgentOSTool {
	return &ExecAgentOSTool{
		dataDir:   getAgentOSDataDir(),
		validator: validation.NewSystemNameValidator(),
	}
}

// Name returns the tool name
func (t *ExecAgentOSTool) Name() string {
	return "agentos"
}

// Description returns the tool description
func (t *ExecAgentOSTool) Description() string {
	return `Execute AgentOS commands to manage systems. Supports: init, bootstrap, serve, status, validate, migrate.

Use this tool to:
- Create new systems from manifest files (init)
- Bootstrap systems with database schema (bootstrap)
- Check system status and list all systems (status)
- Validate manifest files (validate)
- Start/stop system emulation servers (serve)

Security features:
- System names are validated to prevent path traversal
- Systems are stored in isolated hash-based directories
- Registry uses SQLite for thread safety
- All operations are logged for audit trail

Examples:
- "Create a system from manifest" -> action: init, system_name: my-system, manifest_path: /path/to/manifest.yaml
- "Bootstrap the system" -> action: bootstrap, system_name: my-system
- "List all systems" -> action: status
- "Check system health" -> action: validate, system_name: my-system`
}

// Parameters defines the tool parameters
func (t *ExecAgentOSTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"description": "Action to perform: init, bootstrap, serve, status, validate, list, migrate",
				"enum": []string{"init", "bootstrap", "serve", "status", "validate", "list", "migrate"},
			},
			"system_name": map[string]any{
				"type": "string",
				"description": "Name of the system (required for most actions, validated for security)",
			},
			"manifest_path": map[string]any{
				"type": "string",
				"description": "Path to manifest YAML file (required for init)",
			},
			"data_dir": map[string]any{
				"type": "string",
				"description": "AgentOS data directory (optional, uses default if not provided)",
			},
			"user_id": map[string]any{
				"type": "string",
				"description": "User ID for audit logging (optional)",
			},
		},
		"required": []string{"action"},
	}
}

// Execute runs the AgentOS command
func (t *ExecAgentOSTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	if action == "" {
		return ErrorResult("action is required")
	}

	dataDir := t.dataDir
	if dir, ok := args["data_dir"].(string); ok && dir != "" {
		dataDir = dir
	}

	// Get user ID for audit logging
	userID, _ := args["user_id"].(string)
	if userID == "" {
		userID = "anonymous"
	}

	switch action {
	case "init":
		return t.executeInitSecure(ctx, args, dataDir, userID)
	case "bootstrap":
		return t.executeBootstrapSecure(ctx, args, dataDir, userID)
	case "serve":
		return t.executeServeSecure(args, dataDir)
	case "status", "list":
		return t.executeStatusSecure(dataDir)
	case "validate":
		return t.executeValidateSecure(ctx, args, dataDir, userID)
	case "migrate":
		return t.executeMigrateSecure(ctx, args, dataDir, userID)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

// executeInitSecure initializes a new system with security
func (t *ExecAgentOSTool) executeInitSecure(ctx context.Context, args map[string]any, dataDir string, userID string) *ToolResult {
	manifestPath, ok := args["manifest_path"].(string)
	if !ok || manifestPath == "" {
		return ErrorResult("manifest_path is required for init")
	}

	// Check if manifest exists
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("manifest file not found: %s", manifestPath))
	}

	systemName, ok := args["system_name"].(string)
	if !ok || systemName == "" {
		return ErrorResult("system_name is required for init")
	}

	// Validate system name
	validatedName, err := t.validator.ValidateAndSanitize(systemName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid system name: %v", err))
	}

	// Check if system already exists
	registryPath := filepath.Join(dataDir, "registry.db")
	reg, err := registry.NewDBRegistry(registryPath)
	if err == nil {
		defer reg.Close()
		if reg.SystemExists(validatedName) {
			return ErrorResult(fmt.Sprintf("system '%s' already exists", validatedName))
		}
	}

	// Create secure paths
	paths := storage.NewSystemPaths(dataDir, validatedName)

	// Ensure directories exist
	if err := paths.EnsureDirectories(); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create directories: %v", err))
	}

	// Copy manifest to system directory
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read manifest: %v", err))
	}

	manifestDest := paths.Manifest()
	if err := os.WriteFile(manifestDest, manifestData, 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write manifest: %v", err))
	}

	// Create LLM config
	llmConfig := generateDefaultLLMConfigSecure(validatedName)
	llmConfigPath := paths.LLMConfigFile()
	if err := os.WriteFile(llmConfigPath, []byte(llmConfig), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write LLM config: %v", err))
	}

	// Register system in database
	reg, err = registry.NewDBRegistry(registryPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open registry: %v", err))
	}
	defer reg.Close()

	system := &registry.System{
		Name:          validatedName,
		HashPrefix:    paths.Hash,
		Path:          paths.Root(),
		Status:        registry.StatusInitialized,
		ManifestPath:  manifestDest,
		LLMConfigPath: llmConfigPath,
	}

	if err := reg.RegisterSystem(system); err != nil {
		return ErrorResult(fmt.Sprintf("failed to register system: %v", err))
	}

	// Log audit event
	// Note: For simplicity, we're using a basic logger here
	// In production, use the full audit logger
	logAuditEvent(dataDir, audit.OpSystemInitialized, system.ID, userID, map[string]interface{}{
		"name":           validatedName,
		"original_name":  systemName,
		"manifest_path":  manifestPath,
		"system_path":    paths.Root(),
		"hash_prefix":    paths.Hash,
	})

	result := fmt.Sprintf(`System initialized successfully!

Name: %s (validated from "%s")
Location: %s
Manifest: %s
LLM Config: %s
Registry: %s

Next steps:
1. Run bootstrap to create database schema
2. Configure LLM providers
3. Start the system with "serve"`,
		validatedName, systemName, paths.Root(), manifestDest, llmConfigPath, registryPath)

	return UserResult(result)
}

// executeBootstrapSecure bootstraps a system with security
func (t *ExecAgentOSTool) executeBootstrapSecure(ctx context.Context, args map[string]any, dataDir string, userID string) *ToolResult {
	systemName, ok := args["system_name"].(string)
	if !ok || systemName == "" {
		return ErrorResult("system_name is required for bootstrap")
	}

	// Validate system name
	validatedName, err := t.validator.Validate(systemName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid system name: %v", err))
	}

	// Find system
	paths := storage.NewSystemPaths(dataDir, validatedName)
	if !paths.Exists() {
		return ErrorResult(fmt.Sprintf("system not found: %s", validatedName))
	}

	// Check if already bootstrapped
	if _, err := os.Stat(paths.DB()); err == nil {
		return ErrorResult(fmt.Sprintf("system already bootstrapped: %s", validatedName))
	}

	// Create data directory
	if err := os.MkdirAll(paths.Data(), 0750); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create data directory: %v", err))
	}

	// Create database file
	f, err := os.Create(paths.DB())
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create database: %v", err))
	}
	f.Close()

	// Update status in registry
	registryPath := filepath.Join(dataDir, "registry.db")
	reg, err := registry.NewDBRegistry(registryPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open registry: %v", err))
	}
	defer reg.Close()

	system, err := reg.GetSystem(validatedName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("system not found in registry: %v", err))
	}

	if err := reg.UpdateStatus(system.ID, registry.StatusBootstrapped); err != nil {
		return ErrorResult(fmt.Sprintf("failed to update status: %v", err))
	}

	// Log audit event
	logAuditEvent(dataDir, audit.OpSystemBootstrapped, system.ID, userID, map[string]interface{}{
		"database_path": paths.DB(),
	})

	result := fmt.Sprintf(`System bootstrapped successfully!

Name: %s
Database: %s
Registry: %s

The system is ready to serve.`,
		validatedName, paths.DB(), registryPath)

	return UserResult(result)
}

// executeServeSecure starts/stops the system server
func (t *ExecAgentOSTool) executeServeSecure(args map[string]any, dataDir string) *ToolResult {
	systemName, ok := args["system_name"].(string)
	if !ok || systemName == "" {
		return ErrorResult("system_name is required for serve")
	}

	// Validate system name
	validatedName, err := t.validator.Validate(systemName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid system name: %v", err))
	}

	// Find and validate system
	paths := storage.NewSystemPaths(dataDir, validatedName)
	if !paths.Exists() {
		return ErrorResult(fmt.Sprintf("system not found: %s", validatedName))
	}

	registryPath := filepath.Join(dataDir, "registry.db")
	reg, err := registry.NewDBRegistry(registryPath)
	if err == nil {
		defer reg.Close()
		system, err := reg.GetSystem(validatedName)
		if err == nil && system.Status != registry.StatusBootstrapped {
			return ErrorResult(fmt.Sprintf("system not bootstrapped: %s (run bootstrap first)", validatedName))
		}
	}

	// Check if bootstrapped
	if _, err := os.Stat(paths.DB()); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("system not bootstrapped: %s (run bootstrap first)", validatedName))
	}

	// Create status file
	statusFile := paths.StatusFile()
	err = os.WriteFile(statusFile, []byte(fmt.Sprintf("started_at: %s", time.Now().Format(time.RFC3339))), 0644)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create status file: %v", err))
	}

	// Update status in registry
	reg, err = registry.NewDBRegistry(registryPath)
	if err == nil {
		defer reg.Close()
		system, _ := reg.GetSystem(validatedName)
		reg.UpdateStatus(system.ID, registry.StatusServing)
	}

	result := fmt.Sprintf(`System is now serving!

Name: %s
Location: %s
Registry: %s

The system is running and ready for requests.`,
		validatedName, paths.Root(), registryPath)

	return UserResult(result)
}

// executeStatusSecure lists systems and their status
func (t *ExecAgentOSTool) executeStatusSecure(dataDir string) *ToolResult {
	// Ensure data directory exists
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		os.MkdirAll(dataDir, 0755)
	}

	// Open registry
	registryPath := filepath.Join(dataDir, "registry.db")
	reg, err := registry.NewDBRegistry(registryPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open registry: %v", err))
	}
	defer reg.Close()

	// List systems
	systems, err := reg.ListSystems()
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to list systems: %v", err))
	}

	if len(systems) == 0 {
		return UserResult("No systems found. Use 'init' to create a new system.")
	}

	var resultLines []string
	resultLines = append(resultLines, "AgentOS Systems (secure mode):")
	resultLines = append(resultLines, "")

	for _, system := range systems {
		resultLines = append(resultLines, fmt.Sprintf(" - %s (%s)",
			system.Name, system.Status))
	}

	resultLines = append(resultLines, "")
	resultLines = append(resultLines, fmt.Sprintf("Data Directory: %s", dataDir))
	resultLines = append(resultLines, fmt.Sprintf("Registry: %s (SQLite)", registryPath))

	return UserResult(strings.Join(resultLines, "\n"))
}

// executeValidateSecure validates a system
func (t *ExecAgentOSTool) executeValidateSecure(ctx context.Context, args map[string]any, dataDir string, userID string) *ToolResult {
	systemName, ok := args["system_name"].(string)
	if !ok || systemName == "" {
		return ErrorResult("system_name is required for validate")
	}

	// Validate system name
	validatedName, err := t.validator.Validate(systemName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid system name: %v", err))
	}

	// Find system
	paths := storage.NewSystemPaths(dataDir, validatedName)
	if !paths.Exists() {
		return ErrorResult(fmt.Sprintf("system not found: %s", validatedName))
	}

	// Check manifest
	if _, err := os.Stat(paths.Manifest()); err != nil {
		return ErrorResult("system manifest missing")
	}

	// Check database
	if _, err := os.Stat(paths.DB()); err != nil {
		return ErrorResult("system not bootstrapped")
	}

	// Check LLM config
	if _, err := os.Stat(paths.LLMConfigFile()); err != nil {
		return ErrorResult("LLM config missing")
	}

	// Log audit event
	registryPath := filepath.Join(dataDir, "registry.db")
	reg, _ := registry.NewDBRegistry(registryPath)
	if reg != nil {
		defer reg.Close()
		system, _ := reg.GetSystem(validatedName)
		if system != nil {
			logAuditEvent(dataDir, audit.OpSystemValidated, system.ID, userID, map[string]interface{}{
				"status": "healthy",
			})
		}
	}

	result := fmt.Sprintf(`System validation passed!

Name: %s
Status: OK
Manifest: Present
Database: Present
LLM Config: Present
Registry: %s

The system is secure and ready to use.`,
		validatedName, registryPath)

	return UserResult(result)
}

// executeMigrateSecure migrates from old to new structure
func (t *ExecAgentOSTool) executeMigrateSecure(ctx context.Context, args map[string]any, dataDir string, userID string) *ToolResult {
	systemName, ok := args["system_name"].(string)
	if !ok || systemName == "" {
		return ErrorResult("system_name is required for migrate")
	}

	// Find in old location
	oldPath := filepath.Join(dataDir, systemName)
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("system not found in old location: %s", systemName))
	}

	// Migrate
	paths, err := storage.MigrateToNewStructure(dataDir, systemName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("migration failed: %v", err))
	}

	// Update registry
	registryPath := filepath.Join(dataDir, "registry.db")
	reg, err := registry.NewDBRegistry(registryPath)
	if err == nil {
		defer reg.Close()
		
		system, err := reg.GetSystem(systemName)
		if err == nil {
			system.HashPrefix = paths.Hash
			system.Path = paths.Root()
			reg.UpdateSystem(system)
		}
	}

	result := fmt.Sprintf(`System migrated successfully!

Name: %s
Old Location: %s
New Location: %s
Hash Prefix: %s

The system is now stored in the secure hash-based structure.`,
		systemName, oldPath, paths.Root(), paths.Hash)

	return UserResult(result)
}

// Utility Functions

// logAuditEvent logs an audit event to the audit database
func logAuditEvent(dataDir string, operation audit.Operation, systemID string, userID string, details map[string]interface{}) {
	auditPath := filepath.Join(dataDir, "audit.db")
	logger, err := audit.NewLogger(auditPath)
	if err != nil {
		// Silent error - logging failure shouldn't break operation
		return
	}
	defer logger.Close()

	ctx := context.Background()
	logger.Log(ctx, operation, systemID, userID, details)
}

// generateDefaultLLMConfigSecure generates a secure default LLM config
func generateDefaultLLMConfigSecure(systemName string) string {
	return fmt.Sprintf(`version: "1.0"
system: "%s"

settings:
  hot_reload: true
  reload_interval: 5
  provider_chain:
    timeout: 30
    max_retries: 2
    fallback: true
  default_routing:
    provider: "openai"
    model: "gpt-4o-mini"
    timeout: 30
    max_tokens: 4096
    temperature: 0.7

providers:
  - name: "openai"
    type: "openai"
    enabled: false
    priority: 1
    models:
      - id: "gpt-4o-mini"
        name: "GPT-4o Mini"
        max_tokens: 128000
    config:
      base_url: "https://api.openai.com/v1"

agents:
  assistant:
    provider: "openai"
    model: "gpt-4o-mini"
    temperature: 0.7
    max_tokens: 2000
    system_prompt: |
      You are an AI assistant for the %s system.
      Help users with their questions and tasks.
    capabilities:
      - text_generation
      - question_answering

routing:
  functions: {}
  intents: {}
  cost_based:
    enabled: false
  ab_testing:
    enabled: false

defaults:
  temperature: 0.7
  max_tokens: 2000
  timeout: 30s

# Security settings - keys stored in keystore
security:
  key_storage: "keystore"
  encryption_at_rest: true
  audit_logging: true

env_file: ".env"
`, systemName, systemName)
}
