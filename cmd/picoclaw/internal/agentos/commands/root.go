// Package commands contains AgentOS CLI commands
package commands

import (
	"fmt"
	"os"
	"path/filepath"
)

// Global configuration
var (
	dataDir string
)

func init() {
	dataDir = getDefaultDataDir()
}

// getDefaultDataDir returns the default AgentOS data directory
func getDefaultDataDir() string {
	if dir := os.Getenv("AGENTOS_DATA_DIR"); dir != "" {
		return dir
	}

	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".picoclaw", "agentos")
}

// ConfigureCommands defines available commands
var ConfigureCommands = []string{
	"configure-provider <provider>",
	"show-keys",
	"delete-key <key-name>",
	"rotate-key <key-name>",
	"health",
	"audit <system-id>",
}

// Execute runs the appropriate command
func Execute(command string, args []string) error {
	switch command {
	case "configure-provider":
		if len(args) < 1 {
			return fmt.Errorf("provider name required")
		}
		return ConfigureProvider(dataDir, args[0])

	case "show-keys":
		return ShowKeys(dataDir)

	case "delete-key":
		if len(args) < 1 {
			return fmt.Errorf("key name required")
		}
		return DeleteKey(dataDir, args[0])

	case "rotate-key":
		if len(args) < 1 {
			return fmt.Errorf("key name required")
		}
		return RotateKeyInteractive(dataDir, args[0])

	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

// RotateKeyInteractive rotates a key with interactive prompt
func RotateKeyInteractive(dataDir string, keyName string) error {
	fmt.Printf("Rotating key: %s\n", keyName)
	fmt.Print("Enter new key value: ")

	reader := bufio.NewReader(os.Stdin)
	newKey, _ := reader.ReadString('\n')
	newKey = strings.TrimSpace(newKey)

	if newKey == "" {
		return fmt.Errorf("key value cannot be empty")
	}

	return RotateKey(dataDir, keyName, newKey)
}
