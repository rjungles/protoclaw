package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/sipeed/picoclaw/pkg/mcp"
)

type ResourceGenerator struct {
	manifest *manifest.Manifest
	db       *sql.DB
	systemName string
}

func NewResourceGenerator(manifest *manifest.Manifest, db *sql.DB) *ResourceGenerator {
	name := "system"
	if manifest != nil && manifest.Metadata.Name != "" {
		name = strings.ToLower(manifest.Metadata.Name)
	}
	return &ResourceGenerator{
		manifest:   manifest,
		db:         db,
		systemName: name,
	}
}

func (g *ResourceGenerator) GenerateResources() []mcp.Resource {
	if g.manifest == nil {
		return []mcp.Resource{}
	}

	resources := make([]mcp.Resource, 0)

	for _, entity := range g.manifest.DataModel.Entities {
		resource := g.EntityToResource(&entity)
		resources = append(resources, resource)
	}

	return resources
}

func (g *ResourceGenerator) EntityToResource(entity *manifest.Entity) mcp.Resource {
	return mcp.Resource{
		URI:         fmt.Sprintf("resource://%s/%s", g.systemName, strings.ToLower(entity.Name)),
		Name:        entity.Name,
		Description: entity.Description,
		MimeType:    "application/json",
	}
}

func (g *ResourceGenerator) ReadResourceData(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	if g.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	entityName, err := g.parseEntityFromURI(uri)
	if err != nil {
		return nil, err
	}

	entity := g.findEntity(entityName)
	if entity == nil {
		return nil, fmt.Errorf("entity not found: %s", entityName)
	}

	tableName := toSnakeCase(entityName)

	query := fmt.Sprintf("SELECT * FROM %s", tableName)
	rows, err := g.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContents{
			{
				URI:      uri,
				MimeType: "application/json",
				Text:     formatJSON(results),
			},
		},
	}, nil
}

func (g *ResourceGenerator) parseEntityFromURI(uri string) (string, error) {
	prefix := fmt.Sprintf("resource://%s/", g.systemName)
	if !strings.HasPrefix(uri, prefix) {
		return "", fmt.Errorf("invalid resource URI: %s", uri)
	}

	entity := strings.TrimPrefix(uri, prefix)
	return toPascalCase(entity), nil
}

func (g *ResourceGenerator) findEntity(entityName string) *manifest.Entity {
	if g.manifest == nil {
		return nil
	}

	for i := range g.manifest.DataModel.Entities {
		e := &g.manifest.DataModel.Entities[i]
		if e.Name == entityName {
			return e
		}
	}
	return nil
}

func formatJSON(v interface{}) string {
	return fmt.Sprintf("%v", v)
}
