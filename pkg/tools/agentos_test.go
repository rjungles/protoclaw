package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewExecAgentOSTool(t *testing.T) {
	tool := NewExecAgentOSTool()
	if tool == nil {
		t.Fatal("Expected tool to be created")
	}

	if tool.Name() != "agentos" {
		t.Errorf("Expected name 'agentos', got '%s'", tool.Name())
	}

	if tool.dataDir == "" {
		t.Error("Expected dataDir to be set")
	}
}

func TestExecAgentOSToolParameters(t *testing.T) {
	tool := NewExecAgentOSTool()
	params := tool.Parameters()

	if params["type"] != "object" {
		t.Errorf("Expected type 'object', got '%v'", params["type"])
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("Expected properties to be a map")
	}

	required := params["required"].([]string)
	if len(required) != 1 || required[0] != "action" {
		t.Errorf("Expected required ['action'], got %v", required)
	}

	// Check action enum
	actionProps, ok := props["action"].(map[string]any)
	if !ok {
		t.Fatal("Expected action properties")
	}

	enum, ok := actionProps["enum"].([]string)
	if !ok {
		t.Fatal("Expected action enum")
	}

	expectedActions := []string{"init", "bootstrap", "serve", "status", "validate", "list"}
	if len(enum) != len(expectedActions) {
		t.Errorf("Expected %d actions, got %d", len(expectedActions), len(enum))
	}
}

func TestExecAgentOSToolExecuteStatus(t *testing.T) {
	tmpDir := t.TempDir()
	tool := &ExecAgentOSTool{dataDir: tmpDir}

	ctx := context.Background()
	args := map[string]any{
		"action": "status",
	}

	result := tool.Execute(ctx, args)

	if result == nil {
		t.Fatal("Expected result")
	}

	if result.IsError {
		t.Errorf("Expected no error, got: %s", result.ForLLM)
	}

	if !strings.Contains(result.ForLLM, "No systems found") {
		t.Errorf("Expected 'No systems found' message, got: %s", result.ForLLM)
	}
}

func TestExecAgentOSToolExecuteStatusWithSystems(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock system
	systemDir := filepath.Join(tmpDir, "test-system")
	if err := os.MkdirAll(systemDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create system.yaml
	manifestContent := `apiVersion: v1
kind: System
metadata:
  name: Test System
  version: 1.0.0
data_model:
  entities: []
`
	os.WriteFile(filepath.Join(systemDir, "system.yaml"), []byte(manifestContent), 0644)

	tool := &ExecAgentOSTool{dataDir: tmpDir}
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{"action": "status"})

	if result == nil {
		t.Fatal("Expected result")
	}

	if !strings.Contains(result.ForLLM, "test-system") {
		t.Errorf("Expected system name in result, got: %s", result.ForLLM)
	}
}

func TestExecAgentOSToolExecuteInit(t *testing.T) {
	tmpDir := t.TempDir()
	manifestDir := t.TempDir()

	// Create a manifest file
	manifestPath := filepath.Join(manifestDir, "test.yaml")
	manifestContent := `apiVersion: v1
kind: System
metadata:
  name: Test System
  version: 1.0.0
data_model:
  entities: []
`
	os.WriteFile(manifestPath, []byte(manifestContent), 0644)

	tool := &ExecAgentOSTool{dataDir: tmpDir}
	ctx := context.Background()

	args := map[string]any{
		"action":        "init",
		"manifest_path": manifestPath,
		"system_name":   "test-system",
	}

	result := tool.Execute(ctx, args)

	if result == nil {
		t.Fatal("Expected result")
	}

	if result.IsError {
		t.Errorf("Expected no error, got: %s", result.ForLLM)
	}

	// Verify system directory was created
	systemDir := filepath.Join(tmpDir, "test-system")
	if _, err := os.Stat(systemDir); os.IsNotExist(err) {
		t.Error("Expected system directory to be created")
	}

	// Verify manifest was copied
	manifestDest := filepath.Join(systemDir, "system.yaml")
	if _, err := os.Stat(manifestDest); os.IsNotExist(err) {
		t.Error("Expected manifest to be copied")
	}

	// Verify LLM config was created
	llmConfig := filepath.Join(systemDir, "config", "llm", "llm.yaml")
	if _, err := os.Stat(llmConfig); os.IsNotExist(err) {
		t.Error("Expected LLM config to be created")
	}
}

func TestExecAgentOSToolExecuteInitNoManifest(t *testing.T) {
	tmpDir := t.TempDir()
	tool := &ExecAgentOSTool{dataDir: tmpDir}
	ctx := context.Background()

	args := map[string]any{
		"action":        "init",
		"manifest_path": "",
	}

	result := tool.Execute(ctx, args)

	if result == nil {
		t.Fatal("Expected result")
	}

	if !result.IsError {
		t.Error("Expected error for missing manifest_path")
	}

	if !strings.Contains(result.ForLLM, "manifest_path is required") {
		t.Errorf("Expected error message about manifest_path, got: %s", result.ForLLM)
	}
}

func TestExecAgentOSToolExecuteBootstrap(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initialized system
	systemDir := filepath.Join(tmpDir, "test-system")
	os.MkdirAll(systemDir, 0755)
	os.WriteFile(filepath.Join(systemDir, "system.yaml"), []byte(""), 0644)

	tool := &ExecAgentOSTool{dataDir: tmpDir}
	ctx := context.Background()

	args := map[string]any{
		"action":      "bootstrap",
		"system_name": "test-system",
	}

	result := tool.Execute(ctx, args)

	if result == nil {
		t.Fatal("Expected result")
	}

	if result.IsError {
		t.Errorf("Expected no error, got: %s", result.ForLLM)
	}

	// Verify database was created
	dbPath := filepath.Join(systemDir, "data", "data.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected database to be created")
	}
}

func TestExecAgentOSToolExecuteBootstrapNotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	tool := &ExecAgentOSTool{dataDir: tmpDir}
	ctx := context.Background()

	args := map[string]any{
		"action":      "bootstrap",
		"system_name": "nonexistent",
	}

	result := tool.Execute(ctx, args)

	if result == nil {
		t.Fatal("Expected result")
	}

	if !result.IsError {
		t.Error("Expected error for non-initialized system")
	}

	if !strings.Contains(result.ForLLM, "not found") {
		t.Errorf("Expected error about system not found, got: %s", result.ForLLM)
	}
}

func TestExecAgentOSToolExecuteValidate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create complete system
	systemDir := filepath.Join(tmpDir, "test-system")
	os.MkdirAll(filepath.Join(systemDir, "config", "llm"), 0755)
	os.MkdirAll(filepath.Join(systemDir, "data"), 0755)
	os.WriteFile(filepath.Join(systemDir, "system.yaml"), []byte(""), 0644)
	os.WriteFile(filepath.Join(systemDir, "data", "data.db"), []byte(""), 0644)
	os.WriteFile(filepath.Join(systemDir, "config", "llm", "llm.yaml"), []byte(""), 0644)

	tool := &ExecAgentOSTool{dataDir: tmpDir}
	ctx := context.Background()

	args := map[string]any{
		"action":      "validate",
		"system_name": "test-system",
	}

	result := tool.Execute(ctx, args)

	if result == nil {
		t.Fatal("Expected result")
	}

	if result.IsError {
		t.Errorf("Expected no error, got: %s", result.ForLLM)
	}

	if !strings.Contains(result.ForLLM, "validation passed") {
		t.Errorf("Expected validation passed message, got: %s", result.ForLLM)
	}
}

func TestAgentOSGenerateManifestTool(t *testing.T) {
	tool := NewAgentOSGenerateManifestTool()
	if tool == nil {
		t.Fatal("Expected tool to be created")
	}

	if tool.Name() != "agentos_generate_manifest" {
		t.Errorf("Expected name 'agentos_generate_manifest', got '%s'", tool.Name())
	}

	params := tool.Parameters()
	required := params["required"].([]string)
	if len(required) != 2 {
		t.Errorf("Expected 2 required params, got %d", len(required))
	}
}

func TestAgentOSGenerateManifestToolExecute(t *testing.T) {
	tmpDir := t.TempDir()
	tool := NewAgentOSGenerateManifestTool()
	ctx := context.Background()

	args := map[string]any{
		"description": "A car dealership system with customers and vehicles",
		"system_name": "dealership",
		"output_path": filepath.Join(tmpDir, "manifest.yaml"),
	}

	result := tool.Execute(ctx, args)

	if result == nil {
		t.Fatal("Expected result")
	}

	if result.IsError {
		t.Errorf("Expected no error, got: %s", result.ForLLM)
	}

	// Verify file was created
	if _, err := os.Stat(filepath.Join(tmpDir, "manifest.yaml")); os.IsNotExist(err) {
		t.Error("Expected manifest file to be created")
	}

	// Verify entities were extracted
	if !strings.Contains(result.ForLLM, "Customer") {
		t.Error("Expected Customer entity in manifest")
	}

	if !strings.Contains(result.ForLLM, "Vehicle") {
		t.Error("Expected Vehicle entity in manifest")
	}
}

func TestAgentOSQueryTool(t *testing.T) {
	tmpDir := t.TempDir()

	// Create system
	systemDir := filepath.Join(tmpDir, "test-system")
	os.MkdirAll(systemDir, 0755)

	// Set AGENTOS_DATA_DIR env var
	oldDataDir := os.Getenv("AGENTOS_DATA_DIR")
	os.Setenv("AGENTOS_DATA_DIR", tmpDir)
	defer os.Setenv("AGENTOS_DATA_DIR", oldDataDir)

	tool := NewAgentOSQueryTool()
	ctx := context.Background()

	args := map[string]any{
		"system_name": "test-system",
		"entity":      "Customer",
		"limit":       float64(5),
	}

	result := tool.Execute(ctx, args)

	if result == nil {
		t.Fatal("Expected result")
	}

	if result.IsError {
		t.Errorf("Expected no error, got: %s", result.ForLLM)
	}

	if !strings.Contains(result.ForLLM, "Customer") {
		t.Error("Expected entity name in result")
	}
}

func TestAgentOSQueryToolMissingParams(t *testing.T) {
	tool := NewAgentOSQueryTool()
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"system_name": "",
		"entity":      "",
	})

	if result == nil {
		t.Fatal("Expected result")
	}

	if !result.IsError {
		t.Error("Expected error for missing params")
	}
}

func TestGetAgentOSDataDir(t *testing.T) {
	// Test with env var
	os.Setenv("AGENTOS_DATA_DIR", "/custom/path")
	defer os.Unsetenv("AGENTOS_DATA_DIR")

	dir := getAgentOSDataDir()
	if dir != "/custom/path" {
		t.Errorf("Expected '/custom/path', got '%s'", dir)
	}

	// Test without env var
	os.Unsetenv("AGENTOS_DATA_DIR")
	dir = getAgentOSDataDir()
	if dir == "" {
		t.Error("Expected non-empty path")
	}
	if !strings.Contains(dir, "agentos") {
		t.Error("Expected path to contain 'agentos'")
	}
}

func TestExecAgentOSToolUnknownAction(t *testing.T) {
	tmpDir := t.TempDir()
	tool := &ExecAgentOSTool{dataDir: tmpDir}
	ctx := context.Background()

	args := map[string]any{
		"action": "unknown",
	}

	result := tool.Execute(ctx, args)

	if result == nil {
		t.Fatal("Expected result")
	}

	if !result.IsError {
		t.Error("Expected error for unknown action")
	}

	if !strings.Contains(result.ForLLM, "unknown action") {
		t.Errorf("Expected error about unknown action, got: %s", result.ForLLM)
	}
}
