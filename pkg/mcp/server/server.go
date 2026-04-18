package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/sipeed/picoclaw/pkg/agentos"
	"github.com/sipeed/picoclaw/pkg/api"
	"github.com/sipeed/picoclaw/pkg/governance/policy"
	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/mcp"
)

type ServerConfig struct {
	Name        string
	Version     string
	Description string
}

type MCPServer struct {
	config       ServerConfig
	manifest     *manifest.Manifest
	catalog      *agentos.OperationCatalog
	actorStore   agentos.ActorStore
	policyEngine *policy.Engine
	ruleExecutor *api.RuleExecutor
	db           *sql.DB

	toolGenerator     *ToolGenerator
	resourceGenerator *ResourceGenerator
	handler           *OperationHandler

	mu          sync.RWMutex
	initialized bool
}

func NewMCPServer(cfg ServerConfig, si *agentos.SystemInstance) *MCPServer {
	s := &MCPServer{
		config:       cfg,
		manifest:     si.Manifest,
		catalog:      si.Catalog,
		actorStore:   si.ActorStore,
		policyEngine: si.PolicyEngine,
		ruleExecutor: si.RuleExecutor,
		db:           si.DB,
	}

	s.toolGenerator = NewToolGenerator(s.catalog)
	s.resourceGenerator = NewResourceGenerator(s.manifest, s.db)
	s.handler = NewOperationHandler(s.manifest, s.catalog, s.actorStore, s.policyEngine, s.ruleExecutor, s.db)

	return s
}

func (s *MCPServer) HandleRequest(ctx context.Context, req *mcp.Request) (*mcp.Response, error) {
	resp := &mcp.Response{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      req.ID,
	}

	var handler func(context.Context, json.RawMessage) (interface{}, error)

	switch req.Method {
	case "initialize":
		handler = s.handleInitialize
	case "tools/list":
		handler = s.handleListTools
	case "tools/call":
		handler = s.handleCallTool
	case "resources/list":
		handler = s.handleListResources
	case "resources/read":
		handler = s.handleReadResource
	case "prompts/list":
		handler = s.handleListPrompts
	default:
		resp.Error = &mcp.Error{
			Code:    mcp.MethodNotFound,
			Message: fmt.Sprintf("method not found: %s", req.Method),
		}
		return resp, nil
	}

	result, err := handler(ctx, req.Params)
	if err != nil {
		resp.Error = &mcp.Error{
			Code:    mcp.InternalError,
			Message: err.Error(),
		}
		return resp, nil
	}

	resultData, err := json.Marshal(result)
	if err != nil {
		resp.Error = &mcp.Error{
			Code:    mcp.InternalError,
			Message: fmt.Sprintf("failed to marshal result: %v", err),
		}
		return resp, nil
	}

	resp.Result = resultData
	return resp, nil
}

func (s *MCPServer) handleInitialize(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var initParams mcp.InitializeParams
	if err := json.Unmarshal(params, &initParams); err != nil {
		return nil, fmt.Errorf("failed to parse initialize params: %w", err)
	}

	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()

	return &mcp.InitializeResult{
		ProtocolVersion: mcp.ProtocolVersion,
		ServerInfo: mcp.ServerInfo{
			Name:    s.config.Name,
			Version: s.config.Version,
		},
		Capabilities: mcp.ServerCapabilities{
			Tools:     &mcp.ToolsCapability{ListChanged: true},
			Resources: &mcp.ResourcesCapability{Subscribe: true, ListChanged: true},
			Prompts:   &mcp.PromptsCapability{ListChanged: false},
			Logging:   &mcp.LoggingCapability{},
		},
		Instructions: s.config.Description,
	}, nil
}

func (s *MCPServer) handleListTools(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var listParams mcp.ListToolsParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &listParams); err != nil {
			return nil, fmt.Errorf("failed to parse list tools params: %w", err)
		}
	}

	tools := s.toolGenerator.GenerateTools()

	return &mcp.ListToolsResult{
		Tools:      tools,
		NextCursor: "",
	}, nil
}

func (s *MCPServer) handleCallTool(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var callParams mcp.CallToolParams
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, fmt.Errorf("failed to parse call tool params: %w", err)
	}

	actorID := resolveActorFromContext(ctx)
	if actorID == "" {
		actorID = "anonymous"
	}

	result, err := s.handler.ExecuteTool(ctx, callParams.Name, callParams.Arguments, actorID)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				{Type: "text", Text: fmt.Sprintf("error: %v", err)},
			},
			IsError: true,
		}, nil
	}

	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				{Type: "text", Text: fmt.Sprintf("error marshaling result: %v", err)},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			{Type: "text", Text: string(resultJSON)},
		},
		IsError: false,
	}, nil
}

func (s *MCPServer) handleListResources(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var listParams mcp.ListResourcesParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &listParams); err != nil {
			return nil, fmt.Errorf("failed to parse list resources params: %w", err)
		}
	}

	resources := s.resourceGenerator.GenerateResources()

	return &mcp.ListResourcesResult{
		Resources:  resources,
		NextCursor: "",
	}, nil
}

func (s *MCPServer) handleReadResource(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var readParams mcp.ReadResourceParams
	if err := json.Unmarshal(params, &readParams); err != nil {
		return nil, fmt.Errorf("failed to parse read resource params: %w", err)
	}

	result, err := s.resourceGenerator.ReadResourceData(ctx, readParams.URI)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *MCPServer) handleListPrompts(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return &mcp.ListPromptsResult{
		Prompts: []mcp.Prompt{},
	}, nil
}

func (s *MCPServer) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

func (s *MCPServer) GetCatalog() *agentos.OperationCatalog {
	return s.catalog
}

func (s *MCPServer) GetManifest() *manifest.Manifest {
	return s.manifest
}

func resolveActorFromContext(ctx context.Context) string {
	if actorID := ctx.Value(agentos.ActorIDContextKey); actorID != nil {
		if id, ok := actorID.(string); ok {
			return id
		}
	}
	return ""
}
