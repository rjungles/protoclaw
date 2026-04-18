package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type Registry struct {
	mu      sync.RWMutex
	clients map[string]*Client
	configs map[string]manifest.MCPConfig
}

func NewRegistry() *Registry {
	return &Registry{
		clients: make(map[string]*Client),
		configs: make(map[string]manifest.MCPConfig),
	}
}

func NewRegistryFromManifest(m *manifest.Manifest) *Registry {
	r := NewRegistry()
	for _, mcp := range m.Integrations.MCPs {
		r.RegisterConfig(mcp.Name, mcp)
	}
	return r
}

func (r *Registry) RegisterConfig(name string, cfg manifest.MCPConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[name] = cfg
}

func (r *Registry) GetClient(ctx context.Context, name string) (*Client, error) {
	r.mu.RLock()
	if client, ok := r.clients[name]; ok {
		r.mu.RUnlock()
		return client, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	if client, ok := r.clients[name]; ok {
		return client, nil
	}

	cfg, ok := r.configs[name]
	if !ok {
		return nil, fmt.Errorf("MCP server %q not registered", name)
	}

	client, err := NewClientFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for %q: %w", name, err)
	}

	if _, err := client.Initialize(ctx, "agentos-mcp-client", "1.0.0"); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to initialize client for %q: %w", name, err)
	}

	r.clients[name] = client
	return client, nil
}

func (r *Registry) InitializeAll(ctx context.Context) error {
	r.mu.RLock()
	configs := make([]manifest.MCPConfig, 0, len(r.configs))
	for _, cfg := range r.configs {
		configs = append(configs, cfg)
	}
	r.mu.RUnlock()

	var errs []error
	for _, cfg := range configs {
		if _, err := r.GetClient(ctx, cfg.Name); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", cfg.Name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to initialize some MCP servers: %v", errs)
	}
	return nil
}

func (r *Registry) CallTool(ctx context.Context, serverName, toolName string, args map[string]interface{}) (*CallToolResult, error) {
	client, err := r.GetClient(ctx, serverName)
	if err != nil {
		return nil, err
	}
	return client.CallTool(ctx, toolName, args)
}

func (r *Registry) ListAllTools(ctx context.Context) (map[string][]Tool, error) {
	r.mu.RLock()
	configs := make([]manifest.MCPConfig, 0, len(r.configs))
	for _, cfg := range r.configs {
		configs = append(configs, cfg)
	}
	r.mu.RUnlock()

	result := make(map[string][]Tool)
	for _, cfg := range configs {
		client, err := r.GetClient(ctx, cfg.Name)
		if err != nil {
			continue
		}
		tools, err := client.ListTools(ctx, "")
		if err != nil {
			continue
		}
		result[cfg.Name] = tools.Tools
	}
	return result, nil
}

func (r *Registry) FindTool(ctx context.Context, toolName string) (string, *Tool, error) {
	r.mu.RLock()
	configs := make([]manifest.MCPConfig, 0, len(r.configs))
	for _, cfg := range r.configs {
		configs = append(configs, cfg)
	}
	r.mu.RUnlock()

	for _, cfg := range configs {
		client, err := r.GetClient(ctx, cfg.Name)
		if err != nil {
			continue
		}
		if tool := client.FindTool(toolName); tool != nil {
			return cfg.Name, tool, nil
		}
	}
	return "", nil, fmt.Errorf("tool %q not found in any MCP server", toolName)
}

func (r *Registry) ReadResource(ctx context.Context, serverName, uri string) (*ReadResourceResult, error) {
	client, err := r.GetClient(ctx, serverName)
	if err != nil {
		return nil, err
	}
	return client.ReadResource(ctx, uri)
}

func (r *Registry) GetPrompt(ctx context.Context, serverName, promptName string, args map[string]interface{}) (*GetPromptResult, error) {
	client, err := r.GetClient(ctx, serverName)
	if err != nil {
		return nil, err
	}
	return client.GetPrompt(ctx, promptName, args)
}

func (r *Registry) CloseAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, client := range r.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	r.clients = make(map[string]*Client)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing MCP clients: %v", errs)
	}
	return nil
}

func (r *Registry) ListServers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	servers := make([]string, 0, len(r.configs))
	for name := range r.configs {
		servers = append(servers, name)
	}
	return servers
}

func (r *Registry) GetConfig(name string) (manifest.MCPConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.configs[name]
	return cfg, ok
}

func (r *Registry) GetServerInfo(ctx context.Context, name string) (*ServerInfo, error) {
	client, err := r.GetClient(ctx, name)
	if err != nil {
		return nil, err
	}
	return client.ServerInfo(), nil
}

func (r *Registry) GetServerCapabilities(ctx context.Context, name string) (*ServerCapabilities, error) {
	client, err := r.GetClient(ctx, name)
	if err != nil {
		return nil, err
	}
	return client.Capabilities(), nil
}
