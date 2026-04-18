package server

import (
	"strings"

	"github.com/sipeed/picoclaw/pkg/agentos"
	"github.com/sipeed/picoclaw/pkg/mcp"
)

type ToolGenerator struct {
	catalog *agentos.OperationCatalog
}

func NewToolGenerator(catalog *agentos.OperationCatalog) *ToolGenerator {
	return &ToolGenerator{catalog: catalog}
}

func (g *ToolGenerator) GenerateTools() []mcp.Tool {
	operations := g.catalog.ListAll()
	tools := make([]mcp.Tool, 0, len(operations))

	for _, op := range operations {
		tool := g.OperationToTool(&op)
		tools = append(tools, tool)
	}

	return tools
}

func (g *ToolGenerator) OperationToTool(op *agentos.Operation) mcp.Tool {
	return mcp.Tool{
		Name:        op.Name,
		Description: op.Description,
		InputSchema: g.BuildInputSchema(op),
	}
}

func (g *ToolGenerator) BuildInputSchema(op *agentos.Operation) map[string]interface{} {
	if op.InputSchema != nil {
		return op.InputSchema
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The unique identifier of the entity",
			},
		},
		"required": []string{},
	}

	switch op.Action {
	case "list":
		schema["properties"].(map[string]interface{})["limit"] = map[string]interface{}{
			"type":        "integer",
			"description": "Maximum number of items to return",
		}
		schema["properties"].(map[string]interface{})["offset"] = map[string]interface{}{
			"type":        "integer",
			"description": "Number of items to skip",
		}
	case "create", "update":
		schema["properties"].(map[string]interface{})["data"] = map[string]interface{}{
			"type":        "object",
			"description": "The data for the operation",
		}
	case "transition":
		schema["properties"].(map[string]interface{})["action"] = map[string]interface{}{
			"type":        "string",
			"description": "The workflow action to execute",
		}
	}

	return schema
}

func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	var result string
	for _, part := range parts {
		if len(part) > 0 {
			result += strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return result
}
