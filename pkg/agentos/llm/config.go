package llm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Settings represents the global settings
type Settings struct {
	HotReload      bool           `yaml:"hot_reload"`
	ReloadInterval int            `yaml:"reload_interval"` // seconds
	ProviderChain  ProviderChain  `yaml:"provider_chain"`
	DefaultRouting RoutingRule    `yaml:"default_routing"`
}

// ProviderChain represents the provider chain configuration
type ProviderChain struct {
	Timeout    int  `yaml:"timeout"`    // seconds
	MaxRetries int  `yaml:"max_retries"`
	Fallback   bool `yaml:"fallback"`
}

// RoutingConfig represents the routing configuration
type RoutingConfig struct {
	Functions   map[string]RoutingRule `yaml:"functions,omitempty"`
	Intents     map[string]RoutingRule `yaml:"intents,omitempty"`
	CostBased   CostBasedConfig        `yaml:"cost_based,omitempty"`
	ABTesting   ABTestingConfig        `yaml:"ab_testing,omitempty"`
}

// CostBasedConfig represents cost-based routing
type CostBasedConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Strategy string `yaml:"strategy,omitempty"`
}

// ABTestingConfig represents A/B testing configuration
type ABTestingConfig struct {
	Enabled        bool       `yaml:"enabled"`
	CurrentVariant string     `yaml:"current_variant,omitempty"`
	VariantA       RoutingRule `yaml:"variant_a,omitempty"`
	VariantB       RoutingRule `yaml:"variant_b,omitempty"`
}

// Costs represents provider pricing
type Costs struct {
	InputPer1K  float64 `yaml:"input_per_1k" json:"input_per_1k"`
	OutputPer1K float64 `yaml:"output_per_1k" json:"output_per_1k"`
}

// Config represents the complete LLM configuration for a system
type Config struct {
	Version      string                 `yaml:"version"`
	GeneratedAt  time.Time              `yaml:"generated_at"`
	System       string                 `yaml:"system"`
	Settings     Settings               `yaml:"settings"`
	Defaults     DefaultConfig          `yaml:"defaults"`
	Providers    []ProviderConfig       `yaml:"providers"`
	Agents       map[string]AgentConfig `yaml:"agents"`
	Routing      RoutingConfig          `yaml:"routing"`
	RoutingRules []RoutingRule          `yaml:"routing_rules,omitempty"`
	Alerts       []AlertConfig          `yaml:"alerts,omitempty"`
	Media        MediaConfig            `yaml:"media,omitempty"`
	AutoSelect   AutoSelectConfig       `yaml:"auto_select,omitempty"`
	HotReload    HotReloadConfig        `yaml:"hot_reload,omitempty"`
	EnvFile      string                 `yaml:"env_file,omitempty"`
}

// DefaultConfig represents default settings
type DefaultConfig struct {
	Temperature float64       `yaml:"temperature"`
	MaxTokens   int           `yaml:"max_tokens"`
	Timeout     time.Duration `yaml:"timeout"`
	Retry       RetryConfig   `yaml:"retry"`
}

// RetryConfig represents retry configuration
type RetryConfig struct {
	MaxAttempts  int           `yaml:"max_attempts"`
	Backoff      string        `yaml:"backoff"`
	InitialDelay time.Duration `yaml:"initial_delay"`
	MaxDelay     time.Duration `yaml:"max_delay"`
}

// ProviderConfig represents a provider configuration
type ProviderConfig struct {
	Name    string                 `yaml:"name"`
	Type    ProviderType           `yaml:"type"`
	Enabled bool                   `yaml:"enabled"`
	Priority int                  `yaml:"priority"`
	Models  []ModelConfig          `yaml:"models"`
	Config  map[string]interface{} `yaml:"config"`
	Costs   Costs                  `yaml:"costs,omitempty"`
}

// ModelConfig represents a model configuration
type ModelConfig struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	MaxTokens   int    `yaml:"max_tokens"`
	Description string `yaml:"description,omitempty"`
}

// GetAPIKey retrieves the API key from config or environment
func (pc *ProviderConfig) GetAPIKey() (string, error) {
	if apiKey, ok := pc.Config["api_key"]; ok {
		if key, ok := apiKey.(string); ok {
			return key, nil
		}
	}
	// Try environment variable based on provider name
	envVar := fmt.Sprintf("%s_API_KEY", pc.Name)
	if value := os.Getenv(envVar); value != "" {
		return value, nil
	}
	// Try generic env vars
	if value := os.Getenv("OPENAI_API_KEY"); value != "" && pc.Type == ProviderTypeOpenAI {
		return value, nil
	}
	if value := os.Getenv("ANTHROPIC_API_KEY"); value != "" && pc.Type == ProviderTypeAnthropic {
		return value, nil
	}
	return "", fmt.Errorf("API key not found for provider %s", pc.Name)
}

// GetBaseURL retrieves the base URL from config
func (pc *ProviderConfig) GetBaseURL() string {
	if baseURL, ok := pc.Config["base_url"]; ok {
		if url, ok := baseURL.(string); ok {
			return url
		}
	}
	return ""
}

// AutoSelectConfig represents auto-selection configuration
type AutoSelectConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Strategy string `yaml:"strategy"`
}

// HotReloadConfig represents hot-reload configuration
type HotReloadConfig struct {
	Enabled            bool          `yaml:"enabled"`
	CheckInterval      time.Duration `yaml:"check_interval"`
	GracefulTransition bool          `yaml:"graceful_transition"`
}

// LoadConfig loads the LLM configuration from a file
func LoadConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if config.HotReload.CheckInterval == 0 {
		config.HotReload.CheckInterval = 5 * time.Second
	}
	if config.Defaults.Temperature == 0 {
		config.Defaults.Temperature = 0.7
	}
	if config.Defaults.MaxTokens == 0 {
		config.Defaults.MaxTokens = 2000
	}
	if config.Defaults.Timeout == 0 {
		config.Defaults.Timeout = 30 * time.Second
	}
	if config.Defaults.Retry.MaxAttempts == 0 {
		config.Defaults.Retry.MaxAttempts = 3
	}
	if config.Defaults.Retry.Backoff == "" {
		config.Defaults.Retry.Backoff = "exponential"
	}

	// Load environment variables from .env file if specified
	if config.EnvFile != "" {
		envPath := filepath.Join(filepath.Dir(path), config.EnvFile)
		if err := loadEnvFile(envPath); err != nil {
			return nil, fmt.Errorf("failed to load env file: %w", err)
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// SaveConfig saves the configuration to a file
func SaveConfig(config *Config, path string) error {
	if config.Version == "" {
		config.Version = generateVersion()
	}
	config.GeneratedAt = time.Now()

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.System == "" {
		return fmt.Errorf("system name is required")
	}

	if len(c.Providers) == 0 {
		return fmt.Errorf("at least one provider is required")
	}

	providerNames := make(map[string]bool)
	for _, p := range c.Providers {
		if p.Name == "" {
			return fmt.Errorf("provider name is required")
		}
		if providerNames[p.Name] {
			return fmt.Errorf("duplicate provider name: %s", p.Name)
		}
		providerNames[p.Name] = true

		if p.Type == "" {
			return fmt.Errorf("provider type is required for %s", p.Name)
		}

		if len(p.Models) == 0 {
			return fmt.Errorf("at least one model is required for provider %s", p.Name)
		}
	}

	// Validate agent configurations
	for agentName, agent := range c.Agents {
		if agent.Provider == "" && len(agent.ProviderChain) == 0 {
			return fmt.Errorf("agent %s must have either provider or provider_chain", agentName)
		}

		if agent.Provider != "" {
			if !providerNames[agent.Provider] {
				return fmt.Errorf("agent %s references unknown provider: %s", agentName, agent.Provider)
			}
		}

		for _, entry := range agent.ProviderChain {
			if !providerNames[entry.Provider] {
				return fmt.Errorf("agent %s provider chain references unknown provider: %s", agentName, entry.Provider)
			}
		}
	}

	return nil
}

// GetProvider returns a provider by name
func (c *Config) GetProvider(name string) *ProviderConfig {
	for _, p := range c.Providers {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// GetEnabledProviders returns all enabled providers
func (c *Config) GetEnabledProviders() []ProviderConfig {
	var enabled []ProviderConfig
	for _, p := range c.Providers {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetAgent returns an agent configuration by name
func (c *Config) GetAgent(name string) *AgentConfig {
	if agent, ok := c.Agents[name]; ok {
		return &agent
	}
	return nil
}

func generateVersion() string {
	return time.Now().Format("1.0.20060102-150405")
}

func loadEnvFile(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := string(data)
	for _, line := range splitLines(lines) {
		if line == "" || line[0] == '#' {
			continue
		}
		parts := splitEnvLine(line)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
		}
	}
	return nil
}

func splitLines(s string) []string {
	var lines []string
	var current string
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func splitEnvLine(line string) []string {
	for i, r := range line {
		if r == '=' {
			return []string{line[:i], line[i+1:]}
		}
	}
	return []string{line}
}
