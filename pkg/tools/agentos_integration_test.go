package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentOSWorkflow tests a complete workflow
func TestAgentOSWorkflow(t *testing.T) {
	tmpDir := t.TempDir()

	// Step 1: Generate manifest
	t.Run("Generate Manifest", func(t *testing.T) {
		genTool := NewAgentOSGenerateManifestTool()
		ctx := context.Background()

		args := map[string]any{
			"description": "A car dealership system with customers, vehicles, and sales",
			"system_name": "dealership",
			"output_path": filepath.Join(tmpDir, "dealership.yaml"),
		}

		result := genTool.Execute(ctx, args)
		if result.IsError {
			t.Fatalf("Failed to generate manifest: %s", result.ForLLM)
		}

		// Verify file exists
		if _, err := os.Stat(filepath.Join(tmpDir, "dealership.yaml")); os.IsNotExist(err) {
			t.Error("Manifest file was not created")
		}

		// Verify entities
		if !strings.Contains(result.ForLLM, "Customer") {
			t.Error("Expected Customer entity")
		}
		if !strings.Contains(result.ForLLM, "Vehicle") {
			t.Error("Expected Vehicle entity")
		}
		if !strings.Contains(result.ForLLM, "Sale") {
			t.Error("Expected Sale entity")
		}
	})

	// Step 2: Initialize system
	t.Run("Initialize System", func(t *testing.T) {
		execTool := &ExecAgentOSTool{dataDir: tmpDir}
		ctx := context.Background()

		args := map[string]any{
			"action":        "init",
			"manifest_path": filepath.Join(tmpDir, "dealership.yaml"),
			"system_name":   "dealership",
		}

		result := execTool.Execute(ctx, args)
		if result.IsError {
			t.Fatalf("Failed to initialize system: %s", result.ForLLM)
		}

		// Verify system directory
		systemDir := filepath.Join(tmpDir, "dealership")
		if _, err := os.Stat(systemDir); os.IsNotExist(err) {
			t.Fatal("System directory was not created")
		}

		// Verify manifest
		if _, err := os.Stat(filepath.Join(systemDir, "system.yaml")); os.IsNotExist(err) {
			t.Error("Manifest was not copied")
		}

		// Verify LLM config
		if _, err := os.Stat(filepath.Join(systemDir, "config", "llm", "llm.yaml")); os.IsNotExist(err) {
			t.Error("LLM config was not created")
		}
	})

	// Step 3: Bootstrap system
	t.Run("Bootstrap System", func(t *testing.T) {
		execTool := &ExecAgentOSTool{dataDir: tmpDir}
		ctx := context.Background()

		args := map[string]any{
			"action":      "bootstrap",
			"system_name": "dealership",
		}

		result := execTool.Execute(ctx, args)
		if result.IsError {
			t.Fatalf("Failed to bootstrap system: %s", result.ForLLM)
		}

		// Verify database
		dbPath := filepath.Join(tmpDir, "dealership", "data", "data.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Error("Database was not created")
		}
	})

	// Step 4: Validate system
	t.Run("Validate System", func(t *testing.T) {
		execTool := &ExecAgentOSTool{dataDir: tmpDir}
		ctx := context.Background()

		args := map[string]any{
			"action":      "validate",
			"system_name": "dealership",
		}

		result := execTool.Execute(ctx, args)
		if result.IsError {
			t.Fatalf("Failed to validate system: %s", result.ForLLM)
		}

		if !strings.Contains(result.ForLLM, "validation passed") {
			t.Error("Expected validation passed message")
		}
	})

	// Step 5: Query entities
	t.Run("Query Entities", func(t *testing.T) {
		queryTool := NewAgentOSQueryTool()
		ctx := context.Background()

		// Set AGENTOS_DATA_DIR
		oldDataDir := os.Getenv("AGENTOS_DATA_DIR")
		os.Setenv("AGENTOS_DATA_DIR", tmpDir)
		defer os.Setenv("AGENTOS_DATA_DIR", oldDataDir)

		args := map[string]any{
			"system_name": "dealership",
			"entity":      "Customer",
			"limit":       float64(5),
		}

		result := queryTool.Execute(ctx, args)
		if result.IsError {
			t.Fatalf("Failed to query: %s", result.ForLLM)
		}

		if !strings.Contains(result.ForLLM, "Customer") {
			t.Error("Expected Customer in results")
		}
	})

	// Step 6: Check status
	t.Run("Check Status", func(t *testing.T) {
		execTool := &ExecAgentOSTool{dataDir: tmpDir}
		ctx := context.Background()

		args := map[string]any{
			"action": "status",
		}

		result := execTool.Execute(ctx, args)
		if result.IsError {
			t.Fatalf("Failed to get status: %s", result.ForLLM)
		}

		if !strings.Contains(result.ForLLM, "dealership") {
			t.Error("Expected dealership in status")
		}
	})
}

// TestAgentOSConversationFlow simulates a conversational workflow
func TestAgentOSConversationFlow(t *testing.T) {
	tmpDir := t.TempDir()

	scenarios := []struct {
		name    string
		intent  string
		tool    string
		args    map[string]any
		check   func(*ToolResult) bool
	}{
		{
			name:   "Create manifest",
			intent: "Create a restaurant system",
			tool:   "generate",
			args: map[string]any{
				"description": "A restaurant system with menus, orders, and reservations",
				"system_name": "restaurant",
				"output_path": filepath.Join(tmpDir, "restaurant.yaml"),
			},
			check: func(r *ToolResult) bool {
				return !r.IsError && strings.Contains(r.ForLLM, "Menu")
			},
		},
		{
			name:   "Initialize system",
			intent: "Initialize the restaurant system",
			tool:   "init",
			args: map[string]any{
				"action":        "init",
				"manifest_path": filepath.Join(tmpDir, "restaurant.yaml"),
				"system_name":   "restaurant",
			},
			check: func(r *ToolResult) bool {
				return !r.IsError && strings.Contains(r.ForLLM, "initialized")
			},
		},
		{
			name:   "Bootstrap system",
			intent: "Bootstrap the system",
			tool:   "bootstrap",
			args: map[string]any{
				"action":      "bootstrap",
				"system_name": "restaurant",
			},
			check: func(r *ToolResult) bool {
				return !r.IsError && strings.Contains(r.ForLLM, "bootstrapped")
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			var result *ToolResult
			ctx := context.Background()

			switch scenario.tool {
			case "generate":
				tool := NewAgentOSGenerateManifestTool()
				result = tool.Execute(ctx, scenario.args)
			case "init", "bootstrap", "status", "validate":
				tool := &ExecAgentOSTool{dataDir: tmpDir}
				scenario.args["data_dir"] = tmpDir
				result = tool.Execute(ctx, scenario.args)
			}

			if result == nil {
				t.Fatal("Expected result")
			}

			if !scenario.check(result) {
				t.Errorf("Check failed for %s. Result: %s", scenario.name, result.ForLLM)
			}
		})
	}
}

// TestAgentOSErrorHandling tests error scenarios
func TestAgentOSErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		tool      string
		args      map[string]any
		expectErr string
	}{
		{
			name:      "Init without manifest",
			tool:      "exec",
			args:      map[string]any{"action": "init"},
			expectErr: "manifest_path is required",
		},
		{
			name:      "Init with non-existent manifest",
			tool:      "exec",
			args:      map[string]any{"action": "init", "manifest_path": "/nonexistent"},
			expectErr: "not found",
		},
		{
			name:      "Bootstrap without system name",
			tool:      "exec",
			args:      map[string]any{"action": "bootstrap"},
			expectErr: "system_name is required",
		},
		{
			name:      "Bootstrap non-existent system",
			tool:      "exec",
			args:      map[string]any{"action": "bootstrap", "system_name": "nonexistent"},
			expectErr: "not found",
		},
		{
			name:      "Generate without description",
			tool:      "generate",
			args:      map[string]any{"system_name": "test"},
			expectErr: "description is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			var result *ToolResult

			if test.tool == "exec" {
				tool := &ExecAgentOSTool{dataDir: tmpDir}
				result = tool.Execute(ctx, test.args)
			} else {
				tool := NewAgentOSGenerateManifestTool()
				result = tool.Execute(ctx, test.args)
			}

			if result == nil {
				t.Fatal("Expected result")
			}

			if !result.IsError {
				t.Errorf("Expected error for %s", test.name)
			}

			if !strings.Contains(result.ForLLM, test.expectErr) {
				t.Errorf("Expected error containing '%s', got: %s", test.expectErr, result.ForLLM)
			}
		})
	}
}

// TestAgentOSMultipleSystems tests managing multiple systems
func TestAgentOSMultipleSystems(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Create two systems
	systems := []string{"system1", "system2"}

	for _, sysName := range systems {
		// Generate manifest
		genTool := NewAgentOSGenerateManifestTool()
		genTool.Execute(ctx, map[string]any{
			"description": "A " + sysName + " system",
			"system_name": sysName,
			"output_path": filepath.Join(tmpDir, sysName+".yaml"),
		})

		// Initialize
		execTool := &ExecAgentOSTool{dataDir: tmpDir}
		execTool.Execute(ctx, map[string]any{
			"action":        "init",
			"manifest_path": filepath.Join(tmpDir, sysName+".yaml"),
			"system_name":   sysName,
		})
	}

	// Check status shows both systems
	execTool := &ExecAgentOSTool{dataDir: tmpDir}
	result := execTool.Execute(ctx, map[string]any{"action": "status"})

	if result.IsError {
		t.Fatalf("Failed to get status: %s", result.ForLLM)
	}

	for _, sysName := range systems {
		if !strings.Contains(result.ForLLM, sysName) {
			t.Errorf("Expected %s in status", sysName)
		}
	}
}
