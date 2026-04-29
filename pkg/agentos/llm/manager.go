package llm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/fsnotify.v1"
	"gopkg.in/yaml.v3"
)

// Manager manages LLM configuration and providers
type Manager struct {
	configPath      string
	config          *Config
	providers       map[string]Provider
	router          *Router
	factory         *ProviderFactory
	mu              sync.RWMutex
	hotReload       bool
	watcher         *fsnotify.Watcher
	stopWatcher     chan bool
	configTimestamp time.Time
}

// NewManager creates a new LLM manager
func NewManager(configPath string) *Manager {
	factory := NewProviderFactory()
	
	return &Manager{
		configPath:  configPath,
		providers:   make(map[string]Provider),
		factory:     factory,
		router:      NewRouter(),
		stopWatcher: make(chan bool),
	}
}

// Initialize loads configuration and sets up providers
func (m *Manager) Initialize() error {
	if err := m.loadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := m.setupProviders(); err != nil {
		return fmt.Errorf("failed to setup providers: %w", err)
	}

	if m.config.Settings.HotReload {
		m.hotReload = true
		if err := m.startWatcher(); err != nil {
			return fmt.Errorf("failed to start config watcher: %w", err)
		}
	}

	return nil
}

// loadConfig loads configuration from YAML file
func (m *Manager) loadConfig() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default config if not exists
			return m.createDefaultConfig()
		}
		return err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Settings.ProviderChain.Timeout == 0 {
		config.Settings.ProviderChain.Timeout = 30
	}
	if config.Settings.ProviderChain.MaxRetries == 0 {
		config.Settings.ProviderChain.MaxRetries = 2
	}
	if config.Settings.DefaultRouting.Timeout == 0 {
		config.Settings.DefaultRouting.Timeout = 30
	}
	if config.Settings.DefaultRouting.MaxTokens == 0 {
		config.Settings.DefaultRouting.MaxTokens = 4096
	}

	m.config = &config
	m.configTimestamp = time.Now()
	return nil
}

// createDefaultConfig creates a default configuration
func (m *Manager) createDefaultConfig() error {
	defaultConfig := &Config{
		Version: "1.0",
		Settings: Settings{
			HotReload:   true,
			ReloadInterval: 5,
			ProviderChain: ProviderChain{
				Timeout:    30,
				MaxRetries: 2,
				Fallback:   true,
			},
			DefaultRouting: RoutingRule{
				Provider: "default",
				Model:    "gpt-4o-mini",
				Timeout:  30,
				MaxTokens: 4096,
				Temperature: 0.7,
			},
		},
		Providers: []ProviderConfig{},
		Routing:   RoutingConfig{},
	}

	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write default config: %w", err)
	}

	m.config = defaultConfig
	return nil
}

// setupProviders initializes all configured providers
func (m *Manager) setupProviders() error {
	for _, providerConfig := range m.config.Providers {
		if !providerConfig.Enabled {
			continue
		}

		provider, err := m.factory.Create(providerConfig.Type, providerConfig.Name, &providerConfig)
		if err != nil {
			return fmt.Errorf("failed to create provider %s: %w", providerConfig.Name, err)
		}

		m.providers[providerConfig.Name] = provider
	}

	return nil
}

// startWatcher starts the file watcher for hot-reload
func (m *Manager) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	m.watcher = watcher

	// Watch the config directory
	dir := filepath.Dir(m.configPath)
	if err := watcher.Add(dir); err != nil {
		return err
	}

	go m.watchLoop()
	return nil
}

// watchLoop handles file change events
func (m *Manager) watchLoop() {
	ticker := time.NewTicker(time.Duration(m.config.Settings.ReloadInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Periodic reload check
			if err := m.reloadIfChanged(); err != nil {
				fmt.Printf("Error reloading config: %v\\n", err)
			}
		case <-m.stopWatcher:
			return
		}
	}
}

// reloadIfChanged reloads config if file has changed
func (m *Manager) reloadIfChanged() error {
	info, err := os.Stat(m.configPath)
	if err != nil {
		return err
	}

	if info.ModTime().After(m.configTimestamp) {
		fmt.Println("Reloading LLM configuration...")
		
		m.mu.Lock()
		defer m.mu.Unlock()

		// Load new config
		if err := m.loadConfig(); err != nil {
			return err
		}

		// Update providers
		m.providers = make(map[string]Provider)
		if err := m.setupProviders(); err != nil {
			return err
		}

		fmt.Println("LLM configuration reloaded successfully")
	}

	return nil
}

// Complete generates a completion using the appropriate provider
func (m *Manager) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Select provider based on routing rules
	provider, err := m.selectProvider(req)
	if err != nil {
		return nil, err
	}

	// Try primary provider
	resp, err := provider.Complete(ctx, req)
	if err == nil {
		return resp, nil
	}

	// Fallback if enabled
	if m.config.Settings.ProviderChain.Fallback {
		return m.tryFallback(ctx, req, err)
	}

	return nil, err
}

// selectProvider selects the appropriate provider based on routing rules
func (m *Manager) selectProvider(req CompletionRequest) (Provider, error) {
	// Check function-specific routing
	if req.Function != "" {
		if rule, ok := m.config.Routing.Functions[req.Function]; ok {
			if provider, ok := m.providers[rule.Provider]; ok {
				return provider, nil
			}
		}
	}

	// Check intent-based routing
	if req.Intent != "" {
		if rule, ok := m.config.Routing.Intents[req.Intent]; ok {
			if provider, ok := m.providers[rule.Provider]; ok {
				return provider, nil
			}
		}
	}

	// Check cost-based routing
	if m.config.Routing.CostBased.Enabled {
		// Simple implementation: use cheapest enabled provider
		for _, providerConfig := range m.config.Providers {
			if providerConfig.Enabled && providerConfig.Costs.InputPer1K > 0 {
				if provider, ok := m.providers[providerConfig.Name]; ok {
					return provider, nil
				}
			}
		}
	}

	// Check A/B testing
	if m.config.Routing.ABTesting.Enabled {
		if m.config.Routing.ABTesting.CurrentVariant == "b" {
			if provider, ok := m.providers[m.config.Routing.ABTesting.VariantB.Provider]; ok {
				return provider, nil
			}
		}
		if provider, ok := m.providers[m.config.Routing.ABTesting.VariantA.Provider]; ok {
			return provider, nil
		}
	}

	// Use default provider
	if provider, ok := m.providers[m.config.Settings.DefaultRouting.Provider]; ok {
		return provider, nil
	}

	// Fallback to any available provider
	for _, provider := range m.providers {
		return provider, nil
	}

	return nil, fmt.Errorf("no providers available")
}

// tryFallback attempts to complete with fallback providers
func (m *Manager) tryFallback(ctx context.Context, req CompletionRequest, originalErr error) (*CompletionResponse, error) {
	// Try each provider in order
	for _, providerConfig := range m.config.Providers {
		if !providerConfig.Enabled {
			continue
		}

		provider, ok := m.providers[providerConfig.Name]
		if !ok {
			continue
		}

		resp, err := provider.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
	}

	return nil, fmt.Errorf("all providers failed, original error: %w", originalErr)
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// GetProvider returns a provider by name
func (m *Manager) GetProvider(name string) (Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	provider, ok := m.providers[name]
	return provider, ok
}

// ListProviders returns all provider names
func (m *Manager) ListProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// Shutdown stops the manager and cleans up resources
func (m *Manager) Shutdown() {
	if m.hotReload && m.watcher != nil {
		close(m.stopWatcher)
		m.watcher.Close()
	}
}
