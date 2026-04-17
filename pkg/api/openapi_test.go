package api

import (
	"strings"
	"testing"
)

func TestOpenAPI_Generate(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")
	gen, err := NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	spec := gen.GenerateOpenAPI()

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("OpenAPI version: want 3.0.3, got %s", spec.OpenAPI)
	}

	if spec.Info.Title != m.Metadata.Name {
		t.Errorf("Info.Title: want %s, got %s", m.Metadata.Name, spec.Info.Title)
	}

	if spec.Info.Version != m.Metadata.Version {
		t.Errorf("Info.Version: want %s, got %s", m.Metadata.Version, spec.Info.Version)
	}

	if len(spec.Components.Schemas) == 0 {
		t.Error("Components.Schemas is empty")
	}

	for _, entity := range m.DataModel.Entities {
		if _, ok := spec.Components.Schemas[entity.Name]; !ok {
			t.Errorf("Schema for entity %s not found", entity.Name)
		}
	}

	if len(spec.Paths) == 0 {
		t.Error("Paths is empty")
	}
}

func TestOpenAPI_EntitySchema(t *testing.T) {
	m := loadTestManifest(t, "task-management.yaml")
	gen, err := NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	spec := gen.GenerateOpenAPI()

	userSchema, ok := spec.Components.Schemas["User"]
	if !ok {
		t.Fatal("User schema not found")
	}

	if userSchema.Type != "object" {
		t.Errorf("User schema type: want object, got %s", userSchema.Type)
	}

	if len(userSchema.Properties) == 0 {
		t.Error("User schema has no properties")
	}

	idProp, ok := userSchema.Properties["id"]
	if !ok {
		t.Error("User schema missing id property")
	}
	if idProp.Type != "string" {
		t.Errorf("User.id type: want string, got %s", idProp.Type)
	}
}

func TestOpenAPI_EndpointPaths(t *testing.T) {
	m := loadTestManifest(t, "parking-ticket.yaml")
	gen, err := NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	spec := gen.GenerateOpenAPI()

	for _, api := range m.Integrations.APIs {
		for _, ep := range api.Endpoints {
			path := strings.TrimSuffix(api.BasePath, "/") + "/" + strings.TrimPrefix(ep.Path, "/")
			path = strings.ReplaceAll(path, "{id}", "{id}")

			pathObj, ok := spec.Paths[path]
			if !ok {
				t.Errorf("Path %s not found in spec", path)
				continue
			}

			method := strings.ToLower(ep.Method)
			if _, ok := pathObj[method]; !ok {
				t.Errorf("Method %s not found for path %s", method, path)
			}
		}
	}
}

func TestOpenAPI_SecuritySchemes(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")
	gen, err := NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	spec := gen.GenerateOpenAPI()

	hasJWT := false
	hasAPIKey := false
	for _, method := range m.Security.Authentication.Methods {
		if method == "jwt" {
			hasJWT = true
		}
		if method == "api_key" {
			hasAPIKey = true
		}
	}

	if hasJWT {
		if _, ok := spec.Components.SecuritySchemes["bearerAuth"]; !ok {
			t.Error("bearerAuth security scheme not found")
		}
	}

	if hasAPIKey {
		if _, ok := spec.Components.SecuritySchemes["apiKeyAuth"]; !ok {
			t.Error("apiKeyAuth security scheme not found")
		}
	}
}

func TestOpenAPI_Parameters(t *testing.T) {
	m := loadTestManifest(t, "task-management.yaml")
	gen, err := NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	spec := gen.GenerateOpenAPI()

	for path, methods := range spec.Paths {
		for method, pathObj := range methods {
			if strings.Contains(path, "{id}") {
				hasIDParam := false
				for _, param := range pathObj.Parameters {
					if param.Name == "id" && param.In == "path" {
						hasIDParam = true
						if !param.Required {
							t.Errorf("Path parameter 'id' should be required for %s %s", method, path)
						}
					}
				}
				if !hasIDParam {
					t.Errorf("Missing 'id' path parameter for %s %s", method, path)
				}
			}
		}
	}
}

func TestOpenAPI_Responses(t *testing.T) {
	m := loadTestManifest(t, "cafeteria-loyalty.yaml")
	gen, err := NewGenerator(m)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	spec := gen.GenerateOpenAPI()

	for path, methods := range spec.Paths {
		for method, pathObj := range methods {
			if _, ok := pathObj.Responses["200"]; !ok {
				t.Errorf("Missing 200 response for %s %s", method, path)
			}
			if _, ok := pathObj.Responses["400"]; !ok {
				t.Errorf("Missing 400 response for %s %s", method, path)
			}
			if _, ok := pathObj.Responses["404"]; !ok {
				t.Errorf("Missing 404 response for %s %s", method, path)
			}
		}
	}
}
