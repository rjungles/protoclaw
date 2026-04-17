package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

type OpenAPISpec struct {
	OpenAPI    string                            `json:"openapi"`
	Info       OpenAPIInfo                       `json:"info"`
	Servers    []OpenAPIServer                   `json:"servers,omitempty"`
	Paths      map[string]map[string]OpenAPIPath `json:"paths"`
	Components OpenAPIComponents                 `json:"components"`
}

type OpenAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type OpenAPIPath struct {
	Summary     string                     `json:"summary,omitempty"`
	Description string                     `json:"description,omitempty"`
	OperationID string                     `json:"operationId,omitempty"`
	Tags        []string                   `json:"tags,omitempty"`
	Parameters  []OpenAPIParameter         `json:"parameters,omitempty"`
	RequestBody *OpenAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
	Security    []map[string][]string      `json:"security,omitempty"`
}

type OpenAPIParameter struct {
	Name        string        `json:"name"`
	In          string        `json:"in"`
	Description string        `json:"description,omitempty"`
	Required    bool          `json:"required"`
	Schema      OpenAPISchema `json:"schema"`
}

type OpenAPIRequestBody struct {
	Description string                      `json:"description,omitempty"`
	Required    bool                        `json:"required"`
	Content     map[string]OpenAPIMediaType `json:"content"`
}

type OpenAPIMediaType struct {
	Schema OpenAPISchema `json:"schema"`
}

type OpenAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]OpenAPIMediaType `json:"content,omitempty"`
}

type OpenAPISchema struct {
	Type                 string                   `json:"type,omitempty"`
	Format               string                   `json:"format,omitempty"`
	Description          string                   `json:"description,omitempty"`
	Properties           map[string]OpenAPISchema `json:"properties,omitempty"`
	Required             []string                 `json:"required,omitempty"`
	Items                *OpenAPISchema           `json:"items,omitempty"`
	Ref                  string                   `json:"$ref,omitempty"`
	AdditionalProperties *OpenAPISchema           `json:"additionalProperties,omitempty"`
}

type OpenAPIComponents struct {
	Schemas         map[string]OpenAPISchema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]OpenAPISecurityScheme `json:"securitySchemes,omitempty"`
}

type OpenAPISecurityScheme struct {
	Type         string `json:"type"`
	Description  string `json:"description,omitempty"`
	Name         string `json:"name,omitempty"`
	In           string `json:"in,omitempty"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
}

func (g *Generator) GenerateOpenAPI() *OpenAPISpec {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfo{
			Title:       g.manifest.Metadata.Name,
			Description: g.manifest.Metadata.Description,
			Version:     g.manifest.Metadata.Version,
		},
		Paths: make(map[string]map[string]OpenAPIPath),
		Components: OpenAPIComponents{
			Schemas:         make(map[string]OpenAPISchema),
			SecuritySchemes: make(map[string]OpenAPISecurityScheme),
		},
	}

	for _, entity := range g.manifest.DataModel.Entities {
		spec.Components.Schemas[entity.Name] = g.entityToSchema(entity)
	}

	for _, api := range g.manifest.Integrations.APIs {
		for _, ep := range api.Endpoints {
			path := strings.TrimSuffix(api.BasePath, "/") + "/" + strings.TrimPrefix(ep.Path, "/")
			path = strings.ReplaceAll(path, "{id}", "{id}")

			if spec.Paths[path] == nil {
				spec.Paths[path] = make(map[string]OpenAPIPath)
			}

			method := strings.ToLower(ep.Method)
			spec.Paths[path][method] = g.endpointToPath(api, ep)
		}
	}

	if len(g.manifest.Security.Authentication.Methods) > 0 {
		for _, method := range g.manifest.Security.Authentication.Methods {
			switch method {
			case "jwt":
				spec.Components.SecuritySchemes["bearerAuth"] = OpenAPISecurityScheme{
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "JWT",
					Description:  "JWT Authorization header using the Bearer scheme",
				}
			case "api_key":
				spec.Components.SecuritySchemes["apiKeyAuth"] = OpenAPISecurityScheme{
					Type:        "apiKey",
					In:          "header",
					Name:        "X-API-Key",
					Description: "API Key authentication",
				}
			}
		}
	}

	return spec
}

func (g *Generator) entityToSchema(entity manifest.Entity) OpenAPISchema {
	schema := OpenAPISchema{
		Type:       "object",
		Properties: make(map[string]OpenAPISchema),
		Required:   []string{},
	}

	for _, field := range entity.Fields {
		propSchema := g.fieldToSchema(field)
		schema.Properties[field.Name] = propSchema

		if field.Required {
			schema.Required = append(schema.Required, field.Name)
		}
	}

	return schema
}

func (g *Generator) fieldToSchema(field manifest.Field) OpenAPISchema {
	schema := OpenAPISchema{
		Description: field.Description,
	}

	switch strings.ToLower(field.Type) {
	case "string", "text":
		schema.Type = "string"
	case "int", "integer":
		schema.Type = "integer"
	case "float", "number", "decimal":
		schema.Type = "number"
	case "bool", "boolean":
		schema.Type = "boolean"
	case "datetime", "timestamp", "date":
		schema.Type = "string"
		schema.Format = "date-time"
	case "json":
		schema.Type = "object"
		schema.AdditionalProperties = &OpenAPISchema{}
	case "reference":
		schema.Type = "string"
		if field.Reference != nil {
			schema.Description = "Reference to " + field.Reference.Entity
		}
	default:
		schema.Type = "string"
	}

	return schema
}

func (g *Generator) endpointToPath(api manifest.APIConfig, ep manifest.Endpoint) OpenAPIPath {
	path := OpenAPIPath{
		Summary:     ep.Description,
		Description: ep.Description,
		OperationID: strings.ToLower(ep.Method) + "_" + strings.ReplaceAll(ep.Path, "/", "_"),
		Tags:        []string{api.Name},
		Responses:   make(map[string]OpenAPIResponse),
	}

	path.Parameters = g.extractParameters(ep.Path)

	if ep.Input != nil && ep.Input.Entity != "" {
		schemaRef := "#/components/schemas/" + ep.Input.Entity
		if len(ep.Input.Fields) > 0 {
			schemaRef = "#/components/schemas/" + ep.Input.Entity
		}

		path.RequestBody = &OpenAPIRequestBody{
			Required: true,
			Content: map[string]OpenAPIMediaType{
				"application/json": {
					Schema: OpenAPISchema{Ref: schemaRef},
				},
			},
		}
	} else if strings.ToUpper(ep.Method) == "POST" || strings.ToUpper(ep.Method) == "PUT" || strings.ToUpper(ep.Method) == "PATCH" {
		entityName := g.extractEntityFromPath(ep.Path)
		if entityName != "" {
			path.RequestBody = &OpenAPIRequestBody{
				Required: true,
				Content: map[string]OpenAPIMediaType{
					"application/json": {
						Schema: OpenAPISchema{Ref: "#/components/schemas/" + entityName},
					},
				},
			}
		}
	}

	path.Responses["200"] = OpenAPIResponse{
		Description: "Successful response",
		Content: map[string]OpenAPIMediaType{
			"application/json": {
				Schema: OpenAPISchema{Type: "object"},
			},
		},
	}

	path.Responses["400"] = OpenAPIResponse{
		Description: "Bad request",
		Content: map[string]OpenAPIMediaType{
			"application/json": {
				Schema: OpenAPISchema{
					Type: "object",
					Properties: map[string]OpenAPISchema{
						"error": {Type: "string"},
					},
				},
			},
		},
	}

	path.Responses["401"] = OpenAPIResponse{
		Description: "Unauthorized",
		Content: map[string]OpenAPIMediaType{
			"application/json": {
				Schema: OpenAPISchema{
					Type: "object",
					Properties: map[string]OpenAPISchema{
						"error": {Type: "string"},
					},
				},
			},
		},
	}

	path.Responses["403"] = OpenAPIResponse{
		Description: "Forbidden",
		Content: map[string]OpenAPIMediaType{
			"application/json": {
				Schema: OpenAPISchema{
					Type: "object",
					Properties: map[string]OpenAPISchema{
						"error": {Type: "string"},
					},
				},
			},
		},
	}

	path.Responses["404"] = OpenAPIResponse{
		Description: "Not found",
		Content: map[string]OpenAPIMediaType{
			"application/json": {
				Schema: OpenAPISchema{
					Type: "object",
					Properties: map[string]OpenAPISchema{
						"error": {Type: "string"},
					},
				},
			},
		},
	}

	if len(ep.Permissions) > 0 {
		if containsMethod(g.manifest.Security.Authentication.Methods, "jwt") {
			path.Security = []map[string][]string{
				{"bearerAuth": {}},
			}
		}
		if containsMethod(g.manifest.Security.Authentication.Methods, "api_key") {
			path.Security = []map[string][]string{
				{"apiKeyAuth": {}},
			}
		}
	}

	return path
}

func containsMethod(methods []string, method string) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

func (g *Generator) extractParameters(path string) []OpenAPIParameter {
	var params []OpenAPIParameter

	parts := strings.Split(strings.Trim(path, "/"), "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			paramName := strings.Trim(part, "{}")
			params = append(params, OpenAPIParameter{
				Name:        paramName,
				In:          "path",
				Required:    true,
				Description: paramName + " identifier",
				Schema:      OpenAPISchema{Type: "string"},
			})
		}
	}

	params = append(params, OpenAPIParameter{
		Name:        "limit",
		In:          "query",
		Required:    false,
		Description: "Maximum number of results to return",
		Schema:      OpenAPISchema{Type: "integer"},
	})

	params = append(params, OpenAPIParameter{
		Name:        "offset",
		In:          "query",
		Required:    false,
		Description: "Number of results to skip",
		Schema:      OpenAPISchema{Type: "integer"},
	})

	return params
}

func (g *Generator) serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	spec := g.GenerateOpenAPI()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(spec)
}

func (g *Generator) serveDocs(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>` + g.manifest.Metadata.Name + ` API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@4/swagger-ui.css">
    <style>
        body { margin: 0; padding: 0; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@4/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@4/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: "_openapi.json",
                dom_id: '#swagger-ui',
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                layout: "StandaloneLayout"
            })
        }
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
