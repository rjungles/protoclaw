package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type Client struct {
	transport   Transport
	serverInfo  *ServerInfo
	capabilities *ServerCapabilities
	tools       []Tool
	resources   []Resource
	prompts     []Prompt
	mu          sync.RWMutex
	initialized bool
}

func NewClient(transport Transport) *Client {
	return &Client{
		transport: transport,
	}
}

func NewClientFromConfig(cfg manifest.MCPConfig) (*Client, error) {
	var transport Transport
	var err error

	switch cfg.Transport {
	case "stdio":
		args := []string{}
		transport, err = NewStdioTransport(cfg.Server, args, cfg.Config)
	case "sse":
		headers := make(map[string]string)
		if apiKey, ok := cfg.Config["api_key"]; ok {
			headers["Authorization"] = "Bearer " + apiKey
		}
		endpoint := cfg.Server
		if url, ok := cfg.Config["url"]; ok {
			endpoint = url
		}
		transport = NewSSETransport(endpoint, headers)
	case "http", "websocket":
		headers := make(map[string]string)
		if apiKey, ok := cfg.Config["api_key"]; ok {
			headers["Authorization"] = "Bearer " + apiKey
		}
		transport = NewHTTPTransport(cfg.Server, headers)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", cfg.Transport)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	return NewClient(transport), nil
}

func (c *Client) Initialize(ctx context.Context, clientName, clientVersion string) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientInfo: ClientInfo{
			Name:    clientName,
			Version: clientVersion,
		},
		Capabilities: ClientCapabilities{
			Roots:    &RootsCapability{ListChanged: true},
			Sampling: &SamplingCapability{},
		},
	}

	req, err := NewRequest("initialize", params)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("initialize request failed: %w", err)
	}

	result, err := DecodeResult[InitializeResult](resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode initialize result: %w", err)
	}

	c.mu.Lock()
	c.serverInfo = &result.ServerInfo
	c.capabilities = &result.Capabilities
	c.initialized = true
	c.mu.Unlock()

	return result, nil
}

func (c *Client) ListTools(ctx context.Context, cursor string) (*ListToolsResult, error) {
	params := ListToolsParams{Cursor: cursor}

	req, err := NewRequest("tools/list", params)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("tools/list request failed: %w", err)
	}

	result, err := DecodeResult[ListToolsResult](resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tools/list result: %w", err)
	}

	c.mu.Lock()
	c.tools = append(c.tools, result.Tools...)
	c.mu.Unlock()

	return result, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	params := CallToolParams{
		Name:      name,
		Arguments: args,
	}

	req, err := NewRequest("tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("tools/call request failed: %w", err)
	}

	result, err := DecodeResult[CallToolResult](resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tools/call result: %w", err)
	}

	return result, nil
}

func (c *Client) ListResources(ctx context.Context, cursor string) (*ListResourcesResult, error) {
	params := ListResourcesParams{Cursor: cursor}

	req, err := NewRequest("resources/list", params)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("resources/list request failed: %w", err)
	}

	result, err := DecodeResult[ListResourcesResult](resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode resources/list result: %w", err)
	}

	c.mu.Lock()
	c.resources = append(c.resources, result.Resources...)
	c.mu.Unlock()

	return result, nil
}

func (c *Client) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	params := ReadResourceParams{URI: uri}

	req, err := NewRequest("resources/read", params)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("resources/read request failed: %w", err)
	}

	result, err := DecodeResult[ReadResourceResult](resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode resources/read result: %w", err)
	}

	return result, nil
}

func (c *Client) ListPrompts(ctx context.Context, cursor string) (*ListPromptsResult, error) {
	params := ListPromptsParams{Cursor: cursor}

	req, err := NewRequest("prompts/list", params)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("prompts/list request failed: %w", err)
	}

	result, err := DecodeResult[ListPromptsResult](resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode prompts/list result: %w", err)
	}

	c.mu.Lock()
	c.prompts = append(c.prompts, result.Prompts...)
	c.mu.Unlock()

	return result, nil
}

func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]interface{}) (*GetPromptResult, error) {
	params := GetPromptParams{
		Name:      name,
		Arguments: args,
	}

	req, err := NewRequest("prompts/get", params)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("prompts/get request failed: %w", err)
	}

	result, err := DecodeResult[GetPromptResult](resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode prompts/get result: %w", err)
	}

	return result, nil
}

func (c *Client) Close() error {
	return c.transport.Close()
}

func (c *Client) ServerInfo() *ServerInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serverInfo
}

func (c *Client) Capabilities() *ServerCapabilities {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.capabilities
}

func (c *Client) Tools() []Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tools
}

func (c *Client) Resources() []Resource {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.resources
}

func (c *Client) Prompts() []Prompt {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.prompts
}

func (c *Client) IsInitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

func (c *Client) FindTool(name string) *Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.tools {
		if c.tools[i].Name == name {
			return &c.tools[i]
		}
	}
	return nil
}

func (c *Client) FindResource(uri string) *Resource {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.resources {
		if c.resources[i].URI == uri {
			return &c.resources[i]
		}
	}
	return nil
}

func (c *Client) FindPrompt(name string) *Prompt {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.prompts {
		if c.prompts[i].Name == name {
			return &c.prompts[i]
		}
	}
	return nil
}
