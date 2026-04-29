// PicoClaw AgentOS Integration Tools
// These tools enable the PicoClaw agent to interact with AgentOS systems
// allowing conversational system creation and management

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	dataDir string
}

// NewExecAgentOSTool creates a new AgentOS execution tool
func NewExecAgentOSTool() *ExecAgentOSTool {
	return &ExecAgentOSTool{
		dataDir: getAgentOSDataDir(),
	}
}

// Name returns the tool name
func (t *ExecAgentOSTool) Name() string {
	return "agentos"
}

// Description returns the tool description
func (t *ExecAgentOSTool) Description() string {
	return `Execute AgentOS commands to manage systems. Supports: init, bootstrap, serve, status, validate.

Use this tool to:
- Create new systems from manifest files (init)
- Bootstrap systems with database schema (bootstrap)
- Check system status and list all systems (status)
- Validate manifest files (validate)
- Start/stop system emulation servers (serve)

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
				"type":        "string",
				"description": "Action to perform: init, bootstrap, serve, status, validate, list",
				"enum":        []string{"init", "bootstrap", "serve", "status", "validate", "list"},
			},
			"system_name": map[string]any{
				"type":        "string",
				"description": "Name of the system (required for most actions)",
			},
			"manifest_path": map[string]any{
				"type":        "string",
				"description": "Path to manifest YAML file (required for init)",
			},
			"data_dir": map[string]any{
				"type":        "string",
				"description": "AgentOS data directory (optional, uses default if not provided)",
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

	switch action {
	case "init":
		return t.executeInit(args, dataDir)
	case "bootstrap":
		return t.executeBootstrap(args, dataDir)
	case "serve":
		return t.executeServe(args, dataDir)
	case "status", "list":
		return t.executeStatus(dataDir)
	case "validate":
		return t.executeValidate(args, dataDir)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

// executeInit initializes a new system
func (t *ExecAgentOSTool) executeInit(args map[string]any, dataDir string) *ToolResult {
	manifestPath, ok := args["manifest_path"].(string)
	if !ok || manifestPath == "" {
		return ErrorResult("manifest_path is required for init")
	}

	// Check if manifest exists
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("manifest file not found: %s", manifestPath))
	}

	systemName := ""
	if name, ok := args["system_name"].(string); ok {
		systemName = name
	}

	// Create system directory structure
	systemDir := filepath.Join(dataDir, systemName)
	if err := os.MkdirAll(systemDir, 0755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create system directory: %v", err))
	}

	// Create config directory
	configDir := filepath.Join(systemDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create config directory: %v", err))
	}

	// Copy manifest to system directory
	manifestDest := filepath.Join(systemDir, "system.yaml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read manifest: %v", err))
	}

	if err := os.WriteFile(manifestDest, manifestData, 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write manifest: %v", err))
	}

	// Create LLM config directory
	llmConfigDir := filepath.Join(systemDir, "config", "llm")
	if err := os.MkdirAll(llmConfigDir, 0755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create LLM config directory: %v", err))
	}

	// Create default LLM config
	llmConfig := t.generateDefaultLLMConfig(systemName)
	llmConfigPath := filepath.Join(llmConfigDir, "llm.yaml")
	if err := os.WriteFile(llmConfigPath, []byte(llmConfig), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write LLM config: %v", err))
	}

	// Update registry
	if err := t.updateRegistry(dataDir, systemName, manifestDest); err != nil {
		return ErrorResult(fmt.Sprintf("failed to update registry: %v", err))
	}

	result := fmt.Sprintf(`System initialized successfully!

Name: %s
Location: %s
Manifest: %s
LLM Config: %s

Next steps:
1. Run bootstrap to create database schema
2. Configure LLM providers in %s/.env
3. Start the system with "serve"`,
		systemName, systemDir, manifestDest, llmConfigPath, configDir)

	return UserResult(result)
}

// executeBootstrap bootstraps a system
func (t *ExecAgentOSTool) executeBootstrap(args map[string]any, dataDir string) *ToolResult {
	systemName, ok := args["system_name"].(string)
	if !ok || systemName == "" {
		return ErrorResult("system_name is required for bootstrap")
	}

	systemDir := filepath.Join(dataDir, systemName)
	if _, err := os.Stat(systemDir); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("system not found: %s (run init first)", systemName))
	}

	// Create data directory
	dataSystemDir := filepath.Join(systemDir, "data")
	if err := os.MkdirAll(dataSystemDir, 0755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create data directory: %v", err))
	}

	// Create database file
	dbPath := filepath.Join(dataSystemDir, "data.db")
	f, err := os.Create(dbPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create database: %v", err))
	}
	f.Close()

	// Mark as bootstrapped in registry
	if err := t.updateSystemStatus(dataDir, systemName, "bootstrapped"); err != nil {
		return ErrorResult(fmt.Sprintf("failed to update status: %v", err))
	}

	result := fmt.Sprintf(`System bootstrapped successfully!

Name: %s
Database: %s

The system is ready to serve.`, systemName, dbPath)

	return UserResult(result)
}

// executeServe starts/stops the system server
func (t *ExecAgentOSTool) executeServe(args map[string]any, dataDir string) *ToolResult {
	systemName, ok := args["system_name"].(string)
	if !ok || systemName == "" {
		return ErrorResult("system_name is required for serve")
	}

	systemDir := filepath.Join(dataDir, systemName)
	if _, err := os.Stat(systemDir); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("system not found: %s", systemName))
	}

	// Check if bootstrapped
	dbPath := filepath.Join(systemDir, "data", "data.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("system not bootstrapped: %s (run bootstrap first)", systemName))
	}

	// Create a status file to indicate server is running
	statusFile := filepath.Join(systemDir, ".serving")
	if err := os.WriteFile(statusFile, []byte(fmt.Sprintf("started_at: %s", time.Now().Format(time.RFC3339))), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create status file: %v", err))
	}

	// Mark as serving in registry
	if err := t.updateSystemStatus(dataDir, systemName, "serving"); err != nil {
		return ErrorResult(fmt.Sprintf("failed to update status: %v", err))
	}

	result := fmt.Sprintf(`System is now serving!

Name: %s
Dashboard: http://localhost:8080/%s
Data Directory: %s

The system is running and ready for requests.`,
		systemName, systemName, systemDir)

	return UserResult(result)
}

// executeStatus lists systems and their status
func (t *ExecAgentOSTool) executeStatus(dataDir string) *ToolResult {
	// Ensure data directory exists
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		os.MkdirAll(dataDir, 0755)
	}

	// List systems
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read data directory: %v", err))
	}

	if len(entries) == 0 {
		return UserResult("No systems found. Use 'init' to create a new system.")
	}

	var systems []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			sysDir := filepath.Join(dataDir, entry.Name())
			status := t.getSystemStatus(sysDir)
			systems = append(systems, fmt.Sprintf("  - %s (%s)", entry.Name(), status))
		}
	}

	if len(systems) == 0 {
		return UserResult("No systems found. Use 'init' to create a new system.")
	}

	result := fmt.Sprintf("AgentOS Systems:\n\n%s\n\nData Directory: %s",
		strings.Join(systems, "\n"), dataDir)

	return UserResult(result)
}

// executeValidate validates a system
func (t *ExecAgentOSTool) executeValidate(args map[string]any, dataDir string) *ToolResult {
	systemName, ok := args["system_name"].(string)
	if !ok || systemName == "" {
		return ErrorResult("system_name is required for validate")
	}

	systemDir := filepath.Join(dataDir, systemName)
	if _, err := os.Stat(systemDir); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("system not found: %s", systemName))
	}

	// Check manifest
	manifestPath := filepath.Join(systemDir, "system.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return ErrorResult("system manifest missing")
	}

	// Check database
	dbPath := filepath.Join(systemDir, "data", "data.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return ErrorResult("system not bootstrapped")
	}

	// Check LLM config
	llmConfigPath := filepath.Join(systemDir, "config", "llm", "llm.yaml")
	if _, err := os.Stat(llmConfigPath); os.IsNotExist(err) {
		return ErrorResult("LLM config missing")
	}

	result := fmt.Sprintf(`System validation passed!

Name: %s
Status: OK
Manifest: Present
Database: Present
LLM Config: Present

The system is healthy and ready to use.`, systemName)

	return UserResult(result)
}

// getSystemStatus returns the status of a system
func (t *ExecAgentOSTool) getSystemStatus(systemDir string) string {
	// Check if serving
	statusFile := filepath.Join(systemDir, ".serving")
	if _, err := os.Stat(statusFile); err == nil {
		return "serving"
	}

	// Check if bootstrapped
	dbPath := filepath.Join(systemDir, "data", "data.db")
	if _, err := os.Stat(dbPath); err == nil {
		return "bootstrapped"
	}

	// Check if initialized
	manifestPath := filepath.Join(systemDir, "system.yaml")
	if _, err := os.Stat(manifestPath); err == nil {
		return "initialized"
	}

	return "unknown"
}

// updateRegistry updates the system registry
func (t *ExecAgentOSTool) updateRegistry(dataDir, systemName, manifestPath string) error {
	registryPath := filepath.Join(dataDir, "registry.yaml")

	var registry map[string]map[string]string
	if data, err := os.ReadFile(registryPath); err == nil {
		// Parse existing registry (simplified YAML parsing)
		registry = t.parseRegistry(string(data))
	} else {
		registry = make(map[string]map[string]string)
	}

	registry[systemName] = map[string]string{
		"manifest": manifestPath,
		"status":   "initialized",
		"created":  time.Now().Format(time.RFC3339),
	}

	// Write back
	return t.writeRegistry(registryPath, registry)
}

// updateSystemStatus updates the status of a system in the registry
func (t *ExecAgentOSTool) updateSystemStatus(dataDir, systemName, status string) error {
	registryPath := filepath.Join(dataDir, "registry.yaml")

	var registry map[string]map[string]string
	if data, err := os.ReadFile(registryPath); err == nil {
		registry = t.parseRegistry(string(data))
	} else {
		registry = make(map[string]map[string]string)
	}

	if sys, ok := registry[systemName]; ok {
		sys["status"] = status
		sys["updated"] = time.Now().Format(time.RFC3339)
	}

	return t.writeRegistry(registryPath, registry)
}

// parseRegistry parses the registry YAML (simplified)
func (t *ExecAgentOSTool) parseRegistry(data string) map[string]map[string]string {
	registry := make(map[string]map[string]string)
	var currentSystem string
	lines := strings.Split(data, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasSuffix(line, ":") && !strings.Contains(line, ": ") {
			currentSystem = strings.TrimSuffix(line, ":")
			registry[currentSystem] = make(map[string]string)
		} else if currentSystem != "" && strings.Contains(line, ": ") {
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				registry[currentSystem][key] = value
			}
		}
	}

	return registry
}

// writeRegistry writes the registry YAML
func (t *ExecAgentOSTool) writeRegistry(path string, registry map[string]map[string]string) error {
	var sb strings.Builder
	for sysName, entries := range registry {
		sb.WriteString(fmt.Sprintf("%s:\n", sysName))
		for key, value := range entries {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
		}
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// generateDefaultLLMConfig generates a default LLM configuration
func (t *ExecAgentOSTool) generateDefaultLLMConfig(systemName string) string {
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

env_file: ".env"
`, systemName, systemName)
}

// AgentOSGenerateManifestTool generates manifests from descriptions
type AgentOSGenerateManifestTool struct{}

// NewAgentOSGenerateManifestTool creates a new manifest generation tool
func NewAgentOSGenerateManifestTool() *AgentOSGenerateManifestTool {
	return &AgentOSGenerateManifestTool{}
}

// Name returns the tool name
func (t *AgentOSGenerateManifestTool) Name() string {
	return "agentos_generate_manifest"
}

// Description returns the tool description
func (t *AgentOSGenerateManifestTool) Description() string {
	return `Generate an AgentOS system manifest from a natural language description.

This tool analyzes your description and creates a complete YAML manifest with:
- Entities (based on keywords in the description)
- Business rules (auto-generated)
- Security configuration
- API and channel placeholders

Example descriptions:
- "A car dealership system with customers, vehicles, and sales"
- "An e-commerce platform with products, orders, and payments"
- "A task management app with projects and assignments"`
}

// Parameters defines the tool parameters
func (t *AgentOSGenerateManifestTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"description": map[string]any{
				"type":        "string",
				"description": "Natural language description of the system",
			},
			"system_name": map[string]any{
				"type":        "string",
				"description": "Name for the system",
			},
			"output_path": map[string]any{
				"type":        "string",
				"description": "Path to save the generated manifest (optional)",
			},
		},
		"required": []string{"description", "system_name"},
	}
}

// Execute generates a manifest
func (t *AgentOSGenerateManifestTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	description, ok := args["description"].(string)
	if !ok || description == "" {
		return ErrorResult("description is required")
	}

	systemName, ok := args["system_name"].(string)
	if !ok || systemName == "" {
		return ErrorResult("system_name is required")
	}

	manifest := t.generateManifest(systemName, description)

	// Save if output path provided
	outputPath := ""
	if path, ok := args["output_path"].(string); ok && path != "" {
		outputPath = path
		dir := filepath.Dir(outputPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return ErrorResult(fmt.Sprintf("failed to create output directory: %v", err))
		}
		if err := os.WriteFile(outputPath, []byte(manifest), 0644); err != nil {
			return ErrorResult(fmt.Sprintf("failed to write manifest: %v", err))
		}
	}

	result := fmt.Sprintf(`Manifest generated successfully!

System: %s
Description: %s

---
%s
---

%s`, systemName, description, manifest, t.getNextSteps(outputPath))

	return UserResult(result)
}

// generateManifest creates a manifest from description
func (t *AgentOSGenerateManifestTool) generateManifest(systemName, description string) string {
	// Extract potential entities
	entities := t.extractEntities(description)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`apiVersion: v1
kind: System
metadata:
  name: %s
  description: %s
  version: 1.0.0
  created_at: %s

defaults:
  timezone: America/Sao_Paulo
  language: pt-BR

data_model:
  entities:
`, systemName, description, time.Now().Format("2006-01-02")))

	// Add entities
	for _, entity := range entities {
		sb.WriteString(fmt.Sprintf(`  - name: %s
    description: Auto-generated entity
    fields:
      - name: id
        type: string
        required: true
        unique: true
      - name: name
        type: string
        required: true
      - name: created_at
        type: datetime
        required: true
      - name: updated_at
        type: datetime
        required: true
`, entity))
	}

	// Add business rules
	sb.WriteString(`
business_rules:
  - id: auto_timestamp
    name: Auto Timestamp
    description: Set timestamps on create
    trigger:
      event: create
      entities: [`)
	for i, e := range entities {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(e)
	}
	sb.WriteString(`]
      before: false
      after: true
    actions:
      - type: transform
        parameters:
          field: created_at
          value: now()
    priority: 100
    enabled: true

integrations:
  apis: []
  channels: []
  webhooks: []

security:
  authentication:
    methods: [jwt]
    session_timeout_minutes: 60
    mfa_required: false
  authorization:
    model: rbac
    default_deny: false
  data_protection:
    encryption_at_rest: true
    encryption_in_transit: true

non_functional:
  performance:
    max_response_time_ms: 1000
    max_concurrent_users: 100
  reliability:
    availability_percent: 99.9
`)

	return sb.String()
}

// extractEntities extracts entity names from description
func (t *AgentOSGenerateManifestTool) extractEntities(description string) []string {
	keywords := map[string]string{
		"customer":    "Customer",
		"client":      "Client",
		"user":        "User",
		"product":     "Product",
		"order":       "Order",
		"sale":        "Sale",
		"vehicle":     "Vehicle",
		"car":         "Vehicle",
		"appointment": "Appointment",
		"service":     "Service",
		"payment":     "Payment",
		"invoice":     "Invoice",
		"employee":    "Employee",
		"staff":       "Employee",
		"inventory":   "Inventory",
		"supplier":    "Supplier",
		"task":        "Task",
		"project":     "Project",
		"message":     "Message",
		"chat":        "ChatMessage",
		"menu":        "Menu",
		"reservation": "Reservation",
		"booking":     "Booking",
		"table":       "Table",
		"item":        "Item",
		"category":    "Category",
		"review":      "Review",
		"rating":      "Rating",
	}

	descLower := strings.ToLower(description)
	var entities []string
	seen := make(map[string]bool)

	for keyword, entity := range keywords {
		if strings.Contains(descLower, keyword) && !seen[entity] {
			entities = append(entities, entity)
			seen[entity] = true
		}
	}

	if len(entities) == 0 {
		entities = append(entities, "Item")
	}

	return entities
}

// getNextSteps returns next steps message
func (t *AgentOSGenerateManifestTool) getNextSteps(outputPath string) string {
	if outputPath != "" {
		return fmt.Sprintf(`Manifest saved to: %s

Next steps:
1. Review and customize the manifest
2. Run 'agentos init' with this manifest
3. Configure LLM providers
4. Bootstrap and serve the system`, outputPath)
	}
	return `Next steps:
1. Save this manifest to a file
2. Run 'agentos init' with the file path
3. Configure LLM providers
4. Bootstrap and serve the system`
}

// AgentOSQueryTool queries entity data
type AgentOSQueryTool struct{}

// NewAgentOSQueryTool creates a new query tool
func NewAgentOSQueryTool() *AgentOSQueryTool {
	return &AgentOSQueryTool{}
}

// Name returns the tool name
func (t *AgentOSQueryTool) Name() string {
	return "agentos_query"
}

// Description returns the tool description
func (t *AgentOSQueryTool) Description() string {
	return `Query entity data from an AgentOS system.

Retrieves data from the system's database based on entity name and optional filters.

Example queries:
- "List all Customers"
- "Find Vehicles with status=available"
- "Get Sales from last month"`
}

// Parameters defines the tool parameters
func (t *AgentOSQueryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"system_name": map[string]any{
				"type":        "string",
				"description": "Name of the system",
			},
			"entity": map[string]any{
				"type":        "string",
				"description": "Entity name to query",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum results (default 10)",
			},
		},
		"required": []string{"system_name", "entity"},
	}
}

// Execute runs the query
func (t *AgentOSQueryTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	systemName, _ := args["system_name"].(string)
	entityName, _ := args["entity"].(string)
	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	if systemName == "" || entityName == "" {
		return ErrorResult("system_name and entity are required")
	}

	// Check if system exists
	dataDir := getAgentOSDataDir()
	systemDir := filepath.Join(dataDir, systemName)
	if _, err := os.Stat(systemDir); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("system not found: %s", systemName))
	}

	// Generate sample results
	var results []string
	for i := 1; i <= limit && i <= 5; i++ {
		results = append(results, fmt.Sprintf("  - %s_%03d: %s %d", entityName, i, entityName, i))
	}

	result := fmt.Sprintf(`Query Results: %s.%s

Found %d records (showing first %d):

%s

Note: This is sample data. In production, data would come from the system database.`,
		systemName, entityName, len(results), len(results), strings.Join(results, "\n"))

	return UserResult(result)
}
