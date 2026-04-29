package llm

import (
	"fmt"
)

// BaseProvider provides common functionality for all LLM providers
type BaseProvider struct {
	name         string
	providerType ProviderType
	config       *ProviderConfig
	isConfigured bool
}

// NewBaseProvider creates a new base provider
func NewBaseProvider(name string, providerType ProviderType, config *ProviderConfig) *BaseProvider {
	return &BaseProvider{
		name:         name,
		providerType: providerType,
		config:       config,
		isConfigured: config != nil && config.Enabled,
	}
}

// Type returns the provider type
func (p *BaseProvider) Type() ProviderType {
	return p.providerType
}

// Name returns the provider name
func (p *BaseProvider) Name() string {
	return p.name
}

// IsConfigured returns true if the provider is configured
func (p *BaseProvider) IsConfigured() bool {
	return p.isConfigured
}

// GetConfig returns the provider configuration
func (p *BaseProvider) GetConfig() *ProviderConfig {
	return p.config
}

// GetModelInfo returns information about a model
func (p *BaseProvider) GetModelInfo(modelID string) (*ModelInfo, error) {
	if p.config == nil {
		return nil, fmt.Errorf("provider not configured")
	}

	for _, model := range p.config.Models {
		if model.ID == modelID {
			return &ModelInfo{
				ID:          model.ID,
				Name:        model.Name,
				MaxTokens:   model.MaxTokens,
				Description: model.Description,
			}, nil
		}
	}

	return nil, fmt.Errorf("model not found: %s", modelID)
}

// GetModels returns all available models
func (p *BaseProvider) GetModels() []ModelInfo {
	if p.config == nil {
		return nil
	}

	var models []ModelInfo
	for _, model := range p.config.Models {
		models = append(models, ModelInfo{
			ID:          model.ID,
			Name:        model.Name,
			MaxTokens:   model.MaxTokens,
			Description: model.Description,
		})
	}
	return models
}

// Close is a no-op for base provider
func (p *BaseProvider) Close() error {
	return nil
}

// ProviderFactory creates providers based on configuration
type ProviderFactory struct {
	creators map[ProviderType]func(string, *ProviderConfig) (Provider, error)
}

// NewProviderFactory creates a new provider factory
func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{
		creators: make(map[ProviderType]func(string, *ProviderConfig) (Provider, error)),
	}
}

// Register registers a provider creator
func (f *ProviderFactory) Register(providerType ProviderType, creator func(string, *ProviderConfig) (Provider, error)) {
	f.creators[providerType] = creator
}

// Create creates a provider from configuration
func (f *ProviderFactory) Create(providerType ProviderType, name string, config *ProviderConfig) (Provider, error) {
	creator, ok := f.creators[providerType]
	if !ok {
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
	return creator(name, config)
}

// ProviderRegistry manages provider instances
type ProviderRegistry struct {
	providers map[string]Provider
	factory   *ProviderFactory
}

// NewProviderRegistry creates a new provider registry
func NewProviderRegistry(factory *ProviderFactory) *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
		factory:   factory,
	}
}

// Register registers a provider
func (r *ProviderRegistry) Register(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	if !provider.IsConfigured() {
		return fmt.Errorf("provider %s is not configured", provider.Name())
	}

	r.providers[provider.Name()] = provider
	return nil
}

// Get retrieves a provider by name
func (r *ProviderRegistry) Get(name string) (Provider, error) {
	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", name)
	}
	return provider, nil
}

// GetAll returns all registered providers
func (r *ProviderRegistry) GetAll() []Provider {
	providers := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}

// GetByType returns all providers of a specific type
func (r *ProviderRegistry) GetByType(providerType ProviderType) []Provider {
	var providers []Provider
	for _, p := range r.providers {
		if p.Type() == providerType {
			providers = append(providers, p)
		}
	}
	return providers
}

// Unregister removes a provider
func (r *ProviderRegistry) Unregister(name string) {
	if provider, ok := r.providers[name]; ok {
		provider.Close()
		delete(r.providers, name)
	}
}

// Close closes all providers
func (r *ProviderRegistry) Close() error {
	for name, provider := range r.providers {
		if err := provider.Close(); err != nil {
			return fmt.Errorf("failed to close provider %s: %w", name, err)
		}
	}
	r.providers = make(map[string]Provider)
	return nil
}

// Count returns the number of registered providers
func (r *ProviderRegistry) Count() int {
	return len(r.providers)
}
