package agentos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SystemRegistry represents the registry of all systems in a data directory
type SystemRegistry struct {
	Systems map[string]*SystemInfo `json:"systems"`
	Default string                 `json:"default"`
	Version string                 `json:"version"`
}

// SystemInfo holds information about a bootstrapped system
type SystemInfo struct {
	Name         string    `json:"name"`
	ManifestPath string    `json:"manifest_path"`
	DBConnection string    `json:"db_connection"`
	DataDir      string    `json:"data_dir"`
	ServerURL    string    `json:"server_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

const registryFile = ".agentos_registry.json"
const currentVersion = "1.0"

// GetRegistryPath returns the path to the registry file
func GetRegistryPath(dataDir string) string {
	return filepath.Join(dataDir, registryFile)
}

// LoadRegistry loads or creates the system registry
func LoadRegistry(dataDir string) (*SystemRegistry, error) {
	registryPath := GetRegistryPath(dataDir)

	// Check if registry exists
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		// Create new registry
		return &SystemRegistry{
			Systems: make(map[string]*SystemInfo),
			Version: currentVersion,
		}, nil
	}

	// Load existing registry
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry: %w", err)
	}

	var registry SystemRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry: %w", err)
	}

	// Ensure Systems map exists
	if registry.Systems == nil {
		registry.Systems = make(map[string]*SystemInfo)
	}

	return &registry, nil
}

// Save persists the registry to disk
func (r *SystemRegistry) Save(dataDir string) error {
	registryPath := GetRegistryPath(dataDir)

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	if err := os.WriteFile(registryPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write registry: %w", err)
	}

	return nil
}

// RegisterSystem adds or updates a system in the registry
func (r *SystemRegistry) RegisterSystem(name, manifestPath, dbConnection, dataDir string) *SystemInfo {
	info := &SystemInfo{
		Name:         name,
		ManifestPath: manifestPath,
		DBConnection: dbConnection,
		DataDir:      dataDir,
		ServerURL:    "http://localhost:8080",
		UpdatedAt:    time.Now(),
	}

	// Preserve creation time if system already exists
	if existing, ok := r.Systems[name]; ok {
		info.CreatedAt = existing.CreatedAt
	} else {
		info.CreatedAt = time.Now()
	}

	r.Systems[name] = info

	// Set as default if it's the first system
	if r.Default == "" {
		r.Default = name
	}

	return info
}

// GetSystem retrieves a system by name, or the default if name is empty
func (r *SystemRegistry) GetSystem(name string) (*SystemInfo, error) {
	if name == "" {
		name = r.Default
	}

	if name == "" {
		return nil, fmt.Errorf("no default system configured")
	}

	system, ok := r.Systems[name]
	if !ok {
		return nil, fmt.Errorf("system not found: %s", name)
	}

	return system, nil
}

// ListSystems returns all registered systems
func (r *SystemRegistry) ListSystems() []*SystemInfo {
	var systems []*SystemInfo
	for _, info := range r.Systems {
		systems = append(systems, info)
	}
	return systems
}

// SetDefault sets the default system
func (r *SystemRegistry) SetDefault(name string) error {
	if _, ok := r.Systems[name]; !ok {
		return fmt.Errorf("system not found: %s", name)
	}
	r.Default = name
	return nil
}

// RemoveSystem removes a system from the registry
func (r *SystemRegistry) RemoveSystem(name string) error {
	if _, ok := r.Systems[name]; !ok {
		return fmt.Errorf("system not found: %s", name)
	}

	delete(r.Systems, name)

	// Clear default if it was removed
	if r.Default == name {
		r.Default = ""
		// Set new default if any systems remain
		for newDefault := range r.Systems {
			r.Default = newDefault
			break
		}
	}

	return nil
}

// GetDefaultSystem returns the default system info
func (r *SystemRegistry) GetDefaultSystem() (*SystemInfo, error) {
	return r.GetSystem("")
}

// GetSystemNames returns a list of all system names
func (r *SystemRegistry) GetSystemNames() []string {
	names := make([]string, 0, len(r.Systems))
	for name := range r.Systems {
		names = append(names, name)
	}
	return names
}

// HasMultipleSystems returns true if there are multiple systems registered
func (r *SystemRegistry) HasMultipleSystems() bool {
	return len(r.Systems) > 1
}

// GetSystemCount returns the number of registered systems
func (r *SystemRegistry) GetSystemCount() int {
	return len(r.Systems)
}
