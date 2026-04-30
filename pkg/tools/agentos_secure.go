// AgentOS Secure Integration
// This file extends AgentOS tools with security components

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/picoclaw/protoclaw/pkg/agentos/audit"
	"github.com/picoclaw/protoclaw/pkg/agentos/registry"
	"github.com/picoclaw/protoclaw/pkg/agentos/security"
	"github.com/picoclaw/protoclaw/pkg/agentos/security/validation"
	"github.com/picoclaw/protoclaw/pkg/agentos/storage"
)

// SecureAgentOSManager wraps AgentOS operations with security features
type SecureAgentOSManager struct {
	dataDir       string
	registry      *registry.DBRegistry
	validator     *validation.SystemNameValidator
	logger        *audit.Logger
	keystore      *security.KeyStore
	keystoreKey   []byte
}

// NewSecureAgentOSManager creates a new secure manager
func NewSecureAgentOSManager(dataDir string) (*SecureAgentOSManager, error) {
	// Create data directory if needed
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Initialize registry
	registryPath := filepath.Join(dataDir, "registry.db")
	reg, err := registry.NewDBRegistry(registryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize registry: %w", err)
	}

	// Initialize validator
	validator := validation.NewSystemNameValidator()

	// Initialize audit logger
	auditPath := filepath.Join(dataDir, "audit.db")
	logger, err := audit.NewLogger(auditPath)
	if err != nil {
		reg.Close()
		return nil, fmt.Errorf("failed to initialize audit logger: %w", err)
	}

	// Initialize keystore (without master key for now)
	keystorePath := filepath.Join(dataDir, ".keys.db")
	keystoreKey := []byte("default-key-change-me-in-production")
	ks, err := security.NewKeyStore(keystorePath, keystoreKey)
	if err != nil {
		reg.Close()
		logger.Close()
		return nil, fmt.Errorf("failed to initialize keystore: %w", err)
	}

	return &SecureAgentOSManager{
		dataDir:     dataDir,
		registry:    reg,
		validator:   validator,
		logger:      logger,
		keystore:    ks,
		keystoreKey: keystoreKey,
	}, nil
}

// Close closes all resources
func (m *SecureAgentOSManager) Close() {
	if m.registry != nil {
		m.registry.Close()
	}
	if m.logger != nil {
		m.logger.Close()
	}
	if m.keystore != nil {
		m.keystore.Close()
	}
}

// ValidateSystemName validates and sanitizes a system name
func (m *SecureAgentOSManager) ValidateSystemName(name string) (string, error) {
	return m.validator.ValidateAndSanitize(name)
}

// SystemExists checks if a system exists
func (m *SecureAgentOSManager) SystemExists(name string) bool {
	return m.registry.SystemExists(name)
}

// GetSystem retrieves a system by name
func (m *SecureAgentOSManager) GetSystem(name string) (*registry.System, error) {
	return m.registry.GetSystem(name)
}

// ListSystems returns all systems
func (m *SecureAgentOSManager) ListSystems() ([]*registry.System, error) {
	return m.registry.ListSystems()
}

// GetSystemPaths returns the storage paths for a system
func (m *SecureAgentOSManager) GetSystemPaths(systemName string) *storage.SystemPaths {
	return storage.NewSystemPaths(m.dataDir, systemName)
}

// CreateSystem creates a new system with security validations
func (m *SecureAgentOSManager) CreateSystem(ctx context.Context, systemName string, manifestPath string, userID string) (*registry.System, error) {
	// Validate system name
	validatedName, err := m.ValidateSystemName(systemName)
	if err != nil {
		return nil, fmt.Errorf("invalid system name: %w", err)
	}

	// Check if system already exists
	if m.SystemExists(validatedName) {
		return nil, fmt.Errorf("system already exists: %s", validatedName)
	}

	// Create secure paths
	paths := m.GetSystemPaths(validatedName)

	// Ensure directories exist
	if err := paths.EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	// Copy manifest
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	manifestDest := paths.Manifest()
	if err := os.WriteFile(manifestDest, manifestData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}

	// Create LLM config
	llmConfig := generateDefaultLLMConfigSecure(validatedName)
	llmConfigPath := paths.LLMConfigFile()
	if err := os.WriteFile(llmConfigPath, []byte(llmConfig), 0644); err != nil {
		return nil, fmt.Errorf("failed to write LLM config: %w", err)
	}

	// Register system in database
	system := &registry.System{
		Name:          validatedName,
		HashPrefix:    paths.Hash,
		Path:          paths.Root(),
		Status:        registry.StatusInitialized,
		ManifestPath:  manifestDest,
		LLMConfigPath: llmConfigPath,
	}

	if err := m.registry.RegisterSystem(system); err != nil {
		return nil, fmt.Errorf("failed to register system: %w", err)
	}

	// Log audit event
	m.logger.LogSystemCreated(ctx, system.ID, userID, map[string]interface{}{
		"name":           validatedName,
		"original_name":  systemName,
		"manifest_path":  manifestPath,
		"system_path":    paths.Root(),
		"hash_prefix":    paths.Hash,
	})

	return system, nil
}

// BootstrapSystem bootstraps a system with security logging
func (m *SecureAgentOSManager) BootstrapSystem(ctx context.Context, systemName string, userID string) error {
	// Get system
	system, err := m.registry.GetSystem(systemName)
	if err != nil {
		return fmt.Errorf("system not found: %w", err)
	}

	// Get paths
	paths := m.GetSystemPaths(systemName)

	// Create data directory
	if err := os.MkdirAll(paths.Data(), 0750); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create database file
	dbPath := paths.DB()
	f, err := os.Create(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	f.Close()

	// Update status
	if err := m.registry.UpdateStatus(system.ID, registry.StatusBootstrapped); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Log audit event
	m.logger.LogSystemBootstrapped(ctx, system.ID, userID, map[string]interface{}{
		"database_path": dbPath,
	})

	return nil
}

// DeleteSystem deletes a system with security logging
func (m *SecureAgentOSManager) DeleteSystem(ctx context.Context, systemName string, userID string) error {
	// Get system
	system, err := m.registry.GetSystem(systemName)
	if err != nil {
		return fmt.Errorf("system not found: %w", err)
	}

	// Get paths
	paths := m.GetSystemPaths(systemName)

	// Soft delete in registry
	if err := m.registry.DeleteSystem(systemName); err != nil {
		return fmt.Errorf("failed to delete from registry: %w", err)
	}

	// Remove directory
	if paths.Exists() {
		if err := paths.Remove(); err != nil {
			// Log but don't fail - already soft deleted
			fmt.Printf("Warning: failed to remove directory: %v\n", err)
		}
	}

	// Log audit event
	m.logger.LogSystemDeleted(ctx, system.ID, userID, map[string]interface{}{
		"name": systemName,
		"path": paths.Root(),
	})

	return nil
}

// StoreAPIKey stores an API key securely
func (m *SecureAgentOSManager) StoreAPIKey(keyName string, keyValue string, metadata string) error {
	return m.keystore.Store(keyName, []byte(keyValue), metadata)
}

// RetrieveAPIKey retrieves an API key
func (m *SecureAgentOSManager) RetrieveAPIKey(keyName string) (string, string, error) {
	value, metadata, err := m.keystore.Retrieve(keyName)
	if err != nil {
		return "", "", err
	}
	return string(value), metadata, nil
}

// LogAudit logs an audit event
func (m *SecureAgentOSManager) LogAudit(ctx context.Context, operation audit.Operation, systemID, userID string, details map[string]interface{}) error {
	return m.logger.Log(ctx, operation, systemID, userID, details)
}

// GetAuditHistory gets audit history for a system
func (m *SecureAgentOSManager) GetAuditHistory(ctx context.Context, systemID string, limit int) ([]*audit.Event, error) {
	return m.logger.GetSystemHistory(ctx, systemID, limit)
}

// generateDefaultLLMConfigSecure generates secure default LLM config
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

# Security settings
security:
  # API keys are stored in the secure keystore
  # Use: picoclaw agentos configure-provider to set keys
  key_storage: "keystore"
  encryption_at_rest: true
  audit_logging: true

env_file: ".env"
`, systemName, systemName)
}
