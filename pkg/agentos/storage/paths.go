// Package storage provides secure path management for AgentOS systems
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// SystemsSubdir is the subdirectory for systems
	SystemsSubdir = "sys"
	// HashPrefixLength is the length of the hash prefix used in paths
	HashPrefixLength = 5
)

// SystemPaths manages paths for an AgentOS system
type SystemPaths struct {
	Name    string
	DataDir string
	Hash    string
}

// NewSystemPaths creates paths for a system using hash-based directory structure
// Structure: ~/.picoclaw/agentos/sys/<hash-prefix>/<system-name>/
func NewSystemPaths(dataDir, systemName string) *SystemPaths {
	// Calculate hash of the system name
	hash := sha256.Sum256([]byte(systemName))
	hashStr := hex.EncodeToString(hash[:])[:HashPrefixLength]

	return &SystemPaths{
		Name:    systemName,
		DataDir: dataDir,
		Hash:    hashStr,
	}
}

// Root returns the system root directory
func (p *SystemPaths) Root() string {
	return filepath.Join(p.DataDir, SystemsSubdir, p.Hash, p.Name)
}

// Config returns the configuration directory
func (p *SystemPaths) Config() string {
	return filepath.Join(p.Root(), "config")
}

// LLMConfig returns the LLM configuration directory
func (p *SystemPaths) LLMConfig() string {
	return filepath.Join(p.Config(), "llm")
}

// Data returns the data directory
func (p *SystemPaths) Data() string {
	return filepath.Join(p.Root(), "data")
}

// DB returns the database file path
func (p *SystemPaths) DB() string {
	return filepath.Join(p.Data(), "data.db")
}

// Manifest returns the manifest file path
func (p *SystemPaths) Manifest() string {
	return filepath.Join(p.Root(), "system.yaml")
}

// StatusFile returns the status file path
func (p *SystemPaths) StatusFile() string {
	return filepath.Join(p.Root(), ".serving")
}

// LLMConfigFile returns the LLM config file path
func (p *SystemPaths) LLMConfigFile() string {
	return filepath.Join(p.LLMConfig(), "llm.yaml")
}

// RegistryDB returns the registry database path
func (p *SystemPaths) RegistryDB() string {
	return filepath.Join(p.DataDir, "registry.db")
}

// KeyStore returns the keystore path
func (p *SystemPaths) KeyStore() string {
	return filepath.Join(p.DataDir, ".keys.db")
}

// EnsureDirectories creates all necessary directories for the system
func (p *SystemPaths) EnsureDirectories() error {
	dirs := []string{
		p.Root(),
		p.Config(),
		p.LLMConfig(),
		p.Data(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// Exists checks if the system exists
func (p *SystemPaths) Exists() bool {
	_, err := os.Stat(p.Root())
	return !os.IsNotExist(err)
}

// Remove removes the entire system directory
func (p *SystemPaths) Remove() error {
	if !p.Exists() {
		return fmt.Errorf("system does not exist: %s", p.Name)
	}
	return os.RemoveAll(p.Root())
}

// LegacySystemPaths manages old-style paths (backward compatibility)
type LegacySystemPaths struct {
	Name    string
	DataDir string
}

// Root returns the legacy system root directory
func (p *LegacySystemPaths) Root() string {
	return filepath.Join(p.DataDir, p.Name)
}

// Exists checks if the legacy system exists
func (p *LegacySystemPaths) Exists() bool {
	_, err := os.Stat(p.Root())
	return !os.IsNotExist(err)
}

// MigrateToNewStructure migrates from legacy to new structure
// Returns the new paths and any error
func MigrateToNewStructure(dataDir, systemName string) (*SystemPaths, error) {
	legacy := &LegacySystemPaths{Name: systemName, DataDir: dataDir}
	newPaths := NewSystemPaths(dataDir, systemName)

	// Check if exists in legacy structure
	if !legacy.Exists() {
		// Check if already in new structure
		if newPaths.Exists() {
			return newPaths, nil // Already migrated
		}
		return nil, fmt.Errorf("system not found in legacy or new structure: %s", systemName)
	}

	// Check if already exists in new structure
	if newPaths.Exists() {
		return nil, fmt.Errorf("system exists in both old and new locations, manual intervention required")
	}

	// Create new directories
	if err := newPaths.EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create new directories: %w", err)
	}

	// Move content from legacy to new
	// We use rename which is atomic when possible
	// For directories across filesystems, we need to copy
	if err := moveDirectory(legacy.Root(), newPaths.Root()); err != nil {
		// Clean up on error
		os.RemoveAll(newPaths.Root())
		return nil, fmt.Errorf("failed to migrate system: %w", err)
	}

	return newPaths, nil
}

// FindSystem finds a system in either new or legacy structure
// Automatically migrates if found in legacy structure
func FindSystem(dataDir, systemName string) (*SystemPaths, error) {
	// Try new structure first
	paths := NewSystemPaths(dataDir, systemName)
	if paths.Exists() {
		return paths, nil
	}

	// Try legacy structure
	legacy := &LegacySystemPaths{Name: systemName, DataDir: dataDir}
	if legacy.Exists() {
		// Auto-migrate
		return MigrateToNewStructure(dataDir, systemName)
	}

	return nil, fmt.Errorf("system not found: %s", systemName)
}

// GetAllSystems returns paths for all systems in the data directory
func GetAllSystems(dataDir string) ([]*SystemPaths, error) {
	sysDir := filepath.Join(dataDir, SystemsSubdir)

	entries, err := os.ReadDir(sysDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*SystemPaths{}, nil
		}
		return nil, fmt.Errorf("failed to read systems directory: %w", err)
	}

	var systems []*SystemPaths
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Each entry is a hash prefix directory
		hashPrefix := entry.Name()
		hashDir := filepath.Join(sysDir, hashPrefix)

		systemEntries, err := os.ReadDir(hashDir)
		if err != nil {
			continue // Skip unreadable directories
		}

		for _, sysEntry := range systemEntries {
			if !sysEntry.IsDir() {
				continue
			}

			systemName := sysEntry.Name()
			paths := &SystemPaths{
				Name:    systemName,
				DataDir: dataDir,
				Hash:    hashPrefix,
			}
			systems = append(systems, paths)
		}
	}

	return systems, nil
}

// moveDirectory moves a directory (uses rename if possible, otherwise copy+delete)
func moveDirectory(src, dst string) error {
	// Try atomic rename first
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fall back to copy + delete
	return copyAndDeleteDirectory(src, dst)
}

// copyAndDeleteDirectory copies a directory and then deletes the source
func copyAndDeleteDirectory(src, dst string) error {
	// Walk the source directory
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		return copyFile(path, dstPath)
	})
}

// copyFile copies a file
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}
