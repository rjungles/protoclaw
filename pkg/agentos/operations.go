package agentos

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type Operation struct {
	Name           string
	Entity         string
	Action         string
	Method         string
	Path           string
	Description    string
	InputSchema    map[string]interface{}
	OutputSchema   map[string]interface{}
	Permissions    []string
	WorkflowAction string
}

type OperationCatalog struct {
	operations []Operation
	byEntity   map[string][]Operation
	byName     map[string]*Operation
}

func NewCatalog(manifest *manifest.Manifest) *OperationCatalog {
	catalog := &OperationCatalog{
		operations: make([]Operation, 0),
		byEntity:   make(map[string][]Operation),
		byName:     make(map[string]*Operation),
	}

	catalog.buildCRUDOperations(manifest)
	catalog.buildWorkflowOperations(manifest)
	catalog.buildCustomOperations(manifest)

	return catalog
}

func (c *OperationCatalog) buildCRUDOperations(m *manifest.Manifest) {
	entityNames := make(map[string]bool)

	for _, entity := range m.DataModel.Entities {
		entityNames[entity.Name] = true
		entityNames[toPlural(entity.Name)] = true
	}

	for _, entity := range m.DataModel.Entities {
		plural := toPlural(entity.Name)
		basePath := "/api/v1/" + toSnakeCase(plural)

		inputSchema := c.buildInputSchema(entity)
		outputSchema := c.buildOutputSchema(entity)

		listOp := Operation{
			Name:         fmt.Sprintf("%s.list", entity.Name),
			Entity:       entity.Name,
			Action:       "list",
			Method:       "GET",
			Path:         basePath,
			Description:  fmt.Sprintf("List all %s", plural),
			InputSchema:  map[string]interface{}{},
			OutputSchema: outputSchema,
			Permissions:  c.inferPermissions(m, entity.Name, "read"),
		}
		c.register(listOp)

		getOp := Operation{
			Name:        fmt.Sprintf("%s.get", entity.Name),
			Entity:      entity.Name,
			Action:      "get",
			Method:      "GET",
			Path:        basePath + "/{id}",
			Description: fmt.Sprintf("Get a %s by ID", entity.Name),
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "The unique identifier",
					},
				},
				"required": []string{"id"},
			},
			OutputSchema: outputSchema,
			Permissions:  c.inferPermissions(m, entity.Name, "read"),
		}
		c.register(getOp)

		createOp := Operation{
			Name:         fmt.Sprintf("%s.create", entity.Name),
			Entity:       entity.Name,
			Action:       "create",
			Method:       "POST",
			Path:         basePath,
			Description:  fmt.Sprintf("Create a new %s", entity.Name),
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
			Permissions:  c.inferPermissions(m, entity.Name, "write"),
		}
		c.register(createOp)

		updateOp := Operation{
			Name:         fmt.Sprintf("%s.update", entity.Name),
			Entity:       entity.Name,
			Action:       "update",
			Method:       "PUT",
			Path:         basePath + "/{id}",
			Description:  fmt.Sprintf("Update a %s", entity.Name),
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
			Permissions:  c.inferPermissions(m, entity.Name, "write"),
		}
		c.register(updateOp)

		deleteOp := Operation{
			Name:        fmt.Sprintf("%s.delete", entity.Name),
			Entity:      entity.Name,
			Action:      "delete",
			Method:      "DELETE",
			Path:        basePath + "/{id}",
			Description: fmt.Sprintf("Delete a %s", entity.Name),
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "The unique identifier",
					},
				},
				"required": []string{"id"},
			},
			OutputSchema: map[string]interface{}{"type": "object"},
			Permissions:  c.inferPermissions(m, entity.Name, "delete"),
		}
		c.register(deleteOp)
	}
}

func (c *OperationCatalog) buildWorkflowOperations(m *manifest.Manifest) {
	_ = m.NonFunctional

	if m.Workflows != nil {
		for _, wf := range m.Workflows {
			entityName := wf.Entity
			plural := toPlural(entityName)
			basePath := "/api/v1/" + toSnakeCase(plural)

			actionPath := basePath + "/{id}/actions/{action}"

			actionNames := make(map[string]bool)
			for _, state := range wf.States {
				for _, trans := range state.Transitions {
					actionNames[string(trans.Action)] = true
				}
			}

			for actionName := range actionNames {
				op := Operation{
					Name:           fmt.Sprintf("%s.transition.%s", entityName, actionName),
					Entity:         entityName,
					Action:         "transition",
					Method:         "POST",
					Path:           actionPath,
					Description:    fmt.Sprintf("Execute workflow action '%s' on a %s", actionName, entityName),
					InputSchema:    c.buildTransitionInputSchema(actionName),
					OutputSchema:   c.buildTransitionOutputSchema(actionName),
					Permissions:    c.inferTransitionPermissions(m, entityName, actionName),
					WorkflowAction: actionName,
				}
				c.register(op)
			}
		}
	}
}

func (c *OperationCatalog) buildCustomOperations(m *manifest.Manifest) {
	for _, api := range m.Integrations.APIs {
		for _, ep := range api.Endpoints {
			if ep.Handler != "" && !isCRUDHandler(ep.Handler) {
				op := Operation{
					Name:         fmt.Sprintf("%s.%s", api.Name, ep.Handler),
					Entity:       c.extractEntityFromPath(ep.Path),
					Action:       ep.Handler,
					Method:       ep.Method,
					Path:         api.BasePath + ep.Path,
					Description:  ep.Description,
					InputSchema:  c.buildCustomInputSchema(ep),
					OutputSchema: map[string]interface{}{"type": "object"},
					Permissions:  ep.Permissions,
				}
				c.register(op)
			}
		}
	}
}

func (c *OperationCatalog) buildInputSchema(entity manifest.Entity) map[string]interface{} {
	properties := make(map[string]interface{})
	required := make([]string, 0)

	for _, field := range entity.Fields {
		if field.Name == "id" || field.Name == "created_at" || field.Name == "updated_at" {
			continue
		}

		fieldType := mapManifestTypeToJSON(field.Type)
		prop := map[string]interface{}{
			"type": fieldType,
		}

		if field.Description != "" {
			prop["description"] = field.Description
		}

		if field.Default != nil {
			prop["default"] = field.Default
		}

		if field.MaxLength != nil {
			prop["maxLength"] = *field.MaxLength
		}

		properties[field.Name] = prop

		if field.Required {
			required = append(required, field.Name)
		}
	}

	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

func (c *OperationCatalog) buildOutputSchema(entity manifest.Entity) map[string]interface{} {
	properties := make(map[string]interface{})

	for _, field := range entity.Fields {
		fieldType := mapManifestTypeToJSON(field.Type)
		prop := map[string]interface{}{
			"type": fieldType,
		}

		if field.Description != "" {
			prop["description"] = field.Description
		}

		properties[field.Name] = prop
	}

	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
}

func (c *OperationCatalog) buildTransitionInputSchema(actionName string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The unique identifier of the entity",
			},
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The workflow action to execute",
				"enum":        []string{actionName},
			},
			"metadata": map[string]interface{}{
				"type":        "object",
				"description": "Optional metadata for the transition",
			},
		},
		"required": []string{"id", "action"},
	}
}

func (c *OperationCatalog) buildTransitionOutputSchema(actionName string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"success": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether the transition was successful",
			},
			"from_state": map[string]interface{}{
				"type":        "string",
				"description": "The previous state",
			},
			"to_state": map[string]interface{}{
				"type":        "string",
				"description": "The new state after transition",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable message about the transition",
			},
		},
	}
}

func (c *OperationCatalog) buildCustomInputSchema(ep manifest.Endpoint) map[string]interface{} {
	if ep.Input != nil && len(ep.Input.Fields) > 0 {
		properties := make(map[string]interface{})
		for _, field := range ep.Input.Fields {
			properties[field] = map[string]interface{}{"type": "string"}
		}
		return map[string]interface{}{
			"type":       "object",
			"properties": properties,
		}
	}
	return map[string]interface{}{"type": "object"}
}

func (c *OperationCatalog) inferPermissions(m *manifest.Manifest, entityName, action string) []string {
	var permissions []string

	for _, actor := range m.Actors {
		for _, perm := range actor.Permissions {
			if perm.Resource == entityName || perm.Resource == toPlural(entityName) {
				for _, permAction := range perm.Actions {
					if permAction == action || permAction == "*" {
						permStr := fmt.Sprintf("%s:%s", perm.Resource, permAction)
						if !slices.Contains(permissions, permStr) {
							permissions = append(permissions, permStr)
						}
					}
				}
			}
		}
	}

	return permissions
}

func (c *OperationCatalog) inferTransitionPermissions(m *manifest.Manifest, entityName, actionName string) []string {
	var permissions []string

	if m.Workflows != nil {
		for _, wf := range m.Workflows {
			if wf.Entity != entityName {
				continue
			}
			for _, state := range wf.States {
				for _, trans := range state.Transitions {
					if string(trans.Action) == actionName {
						for _, role := range trans.AllowedRoles {
							permStr := fmt.Sprintf("workflow:%s:%s", entityName, actionName)
							if !slices.Contains(permissions, permStr) {
								permissions = append(permissions, permStr)
							}
							rolePerm := fmt.Sprintf("role:%s", role)
							if !slices.Contains(permissions, rolePerm) {
								permissions = append(permissions, rolePerm)
							}
						}
					}
				}
			}
		}
	}

	return permissions
}

func (c *OperationCatalog) extractEntityFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 {
		singular := toSingular(parts[0])
		return toPascalCase(singular)
	}
	return ""
}

func (c *OperationCatalog) register(op Operation) {
	c.operations = append(c.operations, op)

	c.byEntity[op.Entity] = append(c.byEntity[op.Entity], op)

	opCopy := op
	c.byName[op.Name] = &opCopy
}

func (c *OperationCatalog) Register(op Operation) {
	c.register(op)
}

func (c *OperationCatalog) Get(name string) *Operation {
	return c.byName[name]
}

func (c *OperationCatalog) ListByEntity(entity string) []Operation {
	return c.byEntity[entity]
}

func (c *OperationCatalog) ListAll() []Operation {
	return c.operations
}

func mapManifestTypeToJSON(mtype string) string {
	switch strings.ToLower(mtype) {
	case "string", "text":
		return "string"
	case "integer", "int":
		return "integer"
	case "float", "number", "decimal":
		return "number"
	case "boolean", "bool":
		return "boolean"
	case "datetime", "timestamp", "date":
		return "string"
	case "array", "array<string>", "array<int>":
		return "array"
	case "json":
		return "object"
	default:
		return "string"
	}
}

func toPlural(s string) string {
	if strings.HasSuffix(s, "y") {
		return strings.TrimSuffix(s, "y") + "ies"
	}
	if strings.HasSuffix(s, "s") {
		return s + "es"
	}
	return s + "s"
}

func toSingular(s string) string {
	if strings.HasSuffix(s, "ies") {
		return strings.TrimSuffix(s, "ies") + "y"
	}
	if strings.HasSuffix(s, "es") {
		return strings.TrimSuffix(s, "es")
	}
	if strings.HasSuffix(s, "s") && len(s) > 1 {
		return strings.TrimSuffix(s, "s")
	}
	return s
}

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, []rune(strings.ToLower(string(r)))...)
	}
	return string(result)
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

func isCRUDHandler(handler string) bool {
	switch handler {
	case "list", "get", "create", "update", "delete":
		return true
	default:
		return false
	}
}
