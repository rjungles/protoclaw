package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/picoclaw/protoclaw/pkg/agentos/security"
	"github.com/picoclaw/protoclaw/pkg/agentos/storage"
)

// ConfigureProvider handles interactive provider configuration
func ConfigureProvider(dataDir, providerName string) error {
	if dataDir == "" {
		dataDir = getDefaultDataDir()
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Initialize keystore
	dbPath := storage.NewSystemPaths(dataDir, "").KeyStore()
	masterKey := getOrCreateMasterKey(dataDir)

	keystore, err := security.NewKeyStore(dbPath, masterKey)
	if err != nil {
		return fmt.Errorf("failed to initialize keystore: %w", err)
	}
	defer keystore.Close()

	// Map common providers to their env var names
	providerEnvVars := map[string]string{
		"openai":     "OPENAI_API_KEY",
		"groq":       "GROQ_API_KEY",
		"together":   "TOGETHER_API_KEY",
		"fireworks":  "FIREWORKS_API_KEY",
		"openrouter": "OPENROUTER_API_KEY",
		"anthropic":  "ANTHROPIC_API_KEY",
		"google":     "GOOGLE_API_KEY",
	}

	envVarName, exists := providerEnvVars[providerName]
	if !exists {
		envVarName = strings.ToUpper(providerName) + "_API_KEY"
	}

	fmt.Printf("Configuring %s provider\n", providerName)
	fmt.Printf("Key name in keystore: llm.provider.%s.api_key\n", providerName)
	fmt.Printf("Environment variable: %s\n\n", envVarName)

	// Check if key already exists
	keyName := fmt.Sprintf("llm.provider.%s.api_key", providerName)
	if keystore.Exists(keyName) {
		fmt.Println("⚠️  A key for this provider already exists.")
		fmt.Print("Do you want to overwrite it? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(response), "y") {
			fmt.Println("Configuration cancelled.")
			return nil
		}
	}

	// Prompt for API key
	fmt.Print("Enter API key: ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	if apiKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	// Confirm key (show first and last few chars)
	if len(apiKey) > 10 {
		masked := fmt.Sprintf("%s...%s", apiKey[:3], apiKey[len(apiKey)-3:])
		fmt.Printf("Key: %s\n", masked)
	}

	fmt.Print("Save this key? (Y/n): ")
	response, _ := reader.ReadString('\n')
	if strings.HasPrefix(strings.ToLower(response), "n") {
		fmt.Println("Configuration cancelled.")
		return nil
	}

	// Store in keystore
	metadata := fmt.Sprintf("Provider: %s, Configured: %s", providerName, time.Now().Format("2006-01-02 15:04:05"))
	if err := keystore.Store(keyName, []byte(apiKey), metadata); err != nil {
		return fmt.Errorf("failed to store key: %w", err)
	}

	fmt.Printf("\n✅ API key for %s stored securely in keystore!\n", providerName)
	fmt.Printf("   Key: %s\n", keyName)
	fmt.Printf("   Location: %s\n", dbPath)
	fmt.Println("\nSecurity features:")
	fmt.Println("  - Encrypted with AES-256-GCM")
	fmt.Println("  - Stored in SQLite with WAL mode")
	fmt.Println("  - Protected by master key")
	fmt.Println("  - Audit trail enabled")

	// Offer to migrate from environment
	if envValue := os.Getenv(envVarName); envValue != "" && envValue != apiKey {
		fmt.Printf("\n⚠️  Environment variable %s is set but different.\n", envVarName)
		fmt.Print("Do you want to migrate the environment variable? (y/N): ")
		response, _ := reader.ReadString('\n')
		if strings.HasPrefix(strings.ToLower(response), "y") {
			envKeyName := fmt.Sprintf("%s.migrated", keyName)
			keystore.Store(envKeyName, []byte(envValue), fmt.Sprintf("Migrated from %s", envVarName))
			fmt.Println("Environment variable migrated (you can now unset it)")
		}
	}

	return nil
}

// ShowKeys displays all keys in the keystore
func ShowKeys(dataDir string) error {
	if dataDir == "" {
		dataDir = getDefaultDataDir()
	}

	dbPath := storage.NewSystemPaths(dataDir, "").KeyStore()
	masterKey := getOrCreateMasterKey(dataDir)

	keystore, err := security.NewKeyStore(dbPath, masterKey)
	if err != nil {
		return fmt.Errorf("failed to open keystore: %w", err)
	}
	defer keystore.Close()

	keys, err := keystore.List()
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	if len(keys) == 0 {
		fmt.Println("No keys found in keystore.")
		return nil
	}

	fmt.Printf("\nKeys in keystore (%s):\n\n", dbPath)
	for _, keyName := range keys {
		info, err := keystore.GetInfo(keyName)
		if err != nil {
			continue
		}

		// Show last 4 chars of key
		keyValue, _, _ := keystore.Retrieve(keyName)
		masked := "***"
		if len(keyValue) >= 4 {
			masked = fmt.Sprintf("***%s", string(keyValue[len(keyValue)-4:]))
		}

		fmt.Printf("  %s\n", keyName)
		fmt.Printf("    Value: %s\n", masked)
		fmt.Printf("    Created: %s\n", info.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("    Updated: %s\n", info.UpdatedAt.Format("2006-01-02 15:04:05"))
		if info.Metadata != "" {
			fmt.Printf("    Metadata: %s\n", info.Metadata)
		}

		// Show rotation count
		count, _ := keystore.GetRotationCount(keyName)
		if count > 0 {
			fmt.Printf("    Rotations: %d\n", count)
			if last, err := keystore.GetLastRotation(keyName); err == nil && last != nil {
				fmt.Printf("    Last rotated: %s\n", last.Format("2006-01-02 15:04:05"))
			}
		}
		fmt.Println()
	}

	fmt.Printf("Total: %d keys\n", len(keys))
	return nil
}

// DeleteKey removes a key from the keystore
func DeleteKey(dataDir string, keyName string) error {
	if dataDir == "" {
		dataDir = getDefaultDataDir()
	}

	dbPath := storage.NewSystemPaths(dataDir, "").KeyStore()
	masterKey := getOrCreateMasterKey(dataDir)

	keystore, err := security.NewKeyStore(dbPath, masterKey)
	if err != nil {
		return fmt.Errorf("failed to open keystore: %w", err)
	}
	defer keystore.Close()

	if !keystore.Exists(keyName) {
		return fmt.Errorf("key not found: %s", keyName)
	}

	err = keystore.Delete(keyName)
	if err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}

	fmt.Printf("✅ Key deleted: %s\n", keyName)
	return nil
}

// RotateKey rotates a key in the keystore
func RotateKey(dataDir string, keyName string, newValue string) error {
	if dataDir == "" {
		dataDir = getDefaultDataDir()
	}

	dbPath := storage.NewSystemPaths(dataDir, "").KeyStore()
	masterKey := getOrCreateMasterKey(dataDir)

	keystore, err := security.NewKeyStore(dbPath, masterKey)
	if err != nil {
		return fmt.Errorf("failed to open keystore: %w", err)
	}
	defer keystore.Close()

	if !keystore.Exists(keyName) {
		return fmt.Errorf("key not found: %s", keyName)
	}

	metadata := fmt.Sprintf("Rotated: %s", time.Now().Format("2006-01-02 15:04:05"))
	err = keystore.Rotate(keyName, []byte(newValue), metadata)
	if err != nil {
		return fmt.Errorf("failed to rotate key: %w", err)
	}

	fmt.Printf("✅ Key rotated: %s\n", keyName)
	return nil
}

// Helper functions

func getDefaultDataDir() string {
	if dir := os.Getenv("AGENTOS_DATA_DIR"); dir != "" {
		return dir
	}

	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".picoclaw", "agentos")
}

// getOrCreateMasterKey returns the master key, creating one if needed
func getOrCreateMasterKey(dataDir string) []byte {
	keyFile := filepath.Join(dataDir, ".master.key")

	// Try to read existing key
	if keyData, err := os.ReadFile(keyFile); err == nil {
		return keyData
	}

	// Generate new key (32 bytes for AES-256)
	key, _ := security.GenerateKey(32)

	// Save key with restricted permissions
	os.MkdirAll(dataDir, 0700)
	os.WriteFile(keyFile, key, 0600)

	return key
}
