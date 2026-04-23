package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

func TestExportPythonSvelte(t *testing.T) {
	// Create a simple test manifest
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:        "test-system",
			Version:     "1.0.0",
			Description: "Test system for export",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name:        "Customer",
					Description: "Test customer entity",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true, Unique: true},
						{Name: "name", Type: "string", Required: true, MaxLength: ptr(100)},
						{Name: "email", Type: "string", Required: true, Unique: true},
						{Name: "active", Type: "bool", Required: true, Default: true},
					},
					Indexes: []manifest.Index{
						{Name: "idx_customer_email", Fields: []string{"email"}, Unique: true},
					},
				},
			},
		},
	}

	// Create temp output directory
	outputDir, err := os.MkdirTemp("", "export-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(outputDir)

	// Run export
	if err := exportPythonSvelte(m, nil, outputDir, false); err != nil {
		t.Fatalf("exportPythonSvelte failed: %v", err)
	}

	// Check that expected files exist
	expectedFiles := []string{
		"backend/app/models.py",
		"backend/app/main.py",
		"backend/requirements.txt",
		"backend/Dockerfile",
		"frontend/package.json",
		"frontend/svelte.config.js",
		"frontend/src/routes/+layout.svelte",
		"frontend/src/routes/+page.svelte",
		"frontend/src/routes/customer/+page.svelte",
		"frontend/src/routes/customer/[id]/+page.svelte",
		"docker-compose.yml",
		"database/init.sql",
		".env",
	}

	for _, file := range expectedFiles {
		path := filepath.Join(outputDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file not found: %s", file)
		}
	}

	// Check content of models.py
	modelsPath := filepath.Join(outputDir, "backend/app/models.py")
	modelsContent, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatalf("failed to read models.py: %v", err)
	}

	expectedStrings := []string{
		"class Customer(Base):",
		"__tablename__ = 'customer'",
		"id = Column(String)",
		"name = Column(String)",
		"email = Column(String)",
		"active = Column(Boolean)",
		"def to_dict(self):",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(string(modelsContent), s) {
			t.Errorf("models.py missing: %s", s)
		}
	}

	// Check PostgreSQL init script
	initSQLPath := filepath.Join(outputDir, "database/init.sql")
	initSQLContent, err := os.ReadFile(initSQLPath)
	if err != nil {
		t.Fatalf("failed to read init.sql: %v", err)
	}

	expectedSQLStrings := []string{
		"CREATE TABLE customer",
		"id TEXT NOT NULL UNIQUE",
		"name VARCHAR(100) NOT NULL",
		"email TEXT NOT NULL UNIQUE",
		"active BOOLEAN NOT NULL",
		"PRIMARY KEY (id)",
		"CREATE UNIQUE INDEX idx_customer_email",
	}

	for _, s := range expectedSQLStrings {
		if !strings.Contains(string(initSQLContent), s) {
			t.Errorf("init.sql missing: %s", s)
		}
	}

	// Check docker-compose.yml
	dockerComposePath := filepath.Join(outputDir, "docker-compose.yml")
	dockerComposeContent, err := os.ReadFile(dockerComposePath)
	if err != nil {
		t.Fatalf("failed to read docker-compose.yml: %v", err)
	}

	if !strings.Contains(string(dockerComposeContent), "test_system") {
		t.Error("docker-compose.yml should contain database name")
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Customer", "customer"},
		{"LoyaltyAccount", "loyalty_account"},
		{"TierBenefit", "tier_benefit"},
		{"lowercase", "lowercase"},
		{"MixedCaseName", "mixed_case_name"},
	}

	for _, tt := range tests {
		result := toSnakeCase(tt.input)
		if result != tt.expected {
			t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestPythonTypeForField(t *testing.T) {
	tests := []struct {
		fieldType string
		expected  string
	}{
		{"string", "String"},
		{"integer", "Integer"},
		{"float", "Float"},
		{"boolean", "Boolean"},
		{"datetime", "DateTime"},
		{"json", "JSON"},
		{"unknown", "String"},
	}

	for _, tt := range tests {
		field := manifest.Field{Type: tt.fieldType}
		result := pythonTypeForField(field)
		if result != tt.expected {
			t.Errorf("pythonTypeForField(%q) = %q, want %q", tt.fieldType, result, tt.expected)
		}
	}
}

func TestTypeScriptTypeForField(t *testing.T) {
	tests := []struct {
		fieldType string
		expected  string
	}{
		{"string", "string"},
		{"integer", "number"},
		{"float", "number"},
		{"boolean", "boolean"},
		{"datetime", "string"},
		{"json", "any"},
		{"unknown", "string"},
	}

	for _, tt := range tests {
		field := manifest.Field{Type: tt.fieldType}
		result := tsTypeForField(field)
		if result != tt.expected {
			t.Errorf("tsTypeForField(%q) = %q, want %q", tt.fieldType, result, tt.expected)
		}
	}
}

func TestPostgresTypeForField(t *testing.T) {
	tests := []struct {
		fieldType  string
		maxLength  *int
		expected   string
	}{
		{"string", nil, "TEXT"},
		{"string", ptr(50), "VARCHAR(50)"},
		{"text", nil, "TEXT"},
		{"integer", nil, "INTEGER"},
		{"float", nil, "REAL"},
		{"boolean", nil, "BOOLEAN"},
		{"datetime", nil, "TIMESTAMP"},
		{"json", nil, "JSONB"},
		{"unknown", nil, "TEXT"},
	}

	for _, tt := range tests {
		field := manifest.Field{Type: tt.fieldType, MaxLength: tt.maxLength}
		result := postgresTypeForField(field)
		if result != tt.expected {
			t.Errorf("postgresTypeForField(%q) = %q, want %q", tt.fieldType, result, tt.expected)
		}
	}
}

func ptr[T any](v T) *T {
	return &v
}
