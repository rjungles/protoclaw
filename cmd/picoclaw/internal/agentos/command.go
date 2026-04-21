package agentos

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/sipeed/picoclaw/pkg/agentos"
	"github.com/sipeed/picoclaw/pkg/manifest"
)

func NewAgentosCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agentos",
		Short: "AgentOS - Sistema Operacional de Agentes",
		Long: `AgentOS é um Sistema Operacional de Agentes que transforma o PicoClaw
em uma plataforma capaz de gerar automaticamente infraestrutura completa
a partir de um manifesto declarativo YAML/JSON.`,
	}

	cmd.AddCommand(
		newInitCommand(),
		newBootstrapCommand(),
		newServeCommand(),
		newValidateCommand(),
		newStatusCommand(),
		newLogsCommand(),
		newDiffCommand(),
		newVersionsCommand(),
		newConfigCommand(),
	)

	return cmd
}

// newInitCommand cria o comando para inicializar a estrutura do AgentOS
func newInitCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Inicializa a estrutura de dados do AgentOS",
		Long:  `Cria os diretórios necessários para o funcionamento do AgentOS.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dataDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				dataDir = filepath.Join(homeDir, "picoclaw-data")
			}

			dirs := []string{
				filepath.Join(dataDir, "manifests"),
				filepath.Join(dataDir, "db"),
				filepath.Join(dataDir, "logs"),
				filepath.Join(dataDir, "backups"),
			}

			for _, dir := range dirs {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", dir, err)
				}
				fmt.Printf("Created: %s\n", dir)
			}

			// Create sample config file
			configPath := filepath.Join(dataDir, "config.yaml")
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				configContent := `# Configuracao do PicoClaw com AgentOS
mode: agentos

agentos:
  manifest_path: ` + filepath.Join(dataDir, "manifests", "system.yaml") + `
  database:
    driver: sqlite
    connection: ` + filepath.Join(dataDir, "db", "agentos.db") + `
  data_dir: ` + dataDir + `
  auto_evolve: true
  enable_api: true
  enable_mcp: true
  api:
    host: 0.0.0.0
    port: 8080
`
				if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
					return fmt.Errorf("failed to create config file: %w", err)
				}
				fmt.Printf("Created: %s\n", configPath)
			}

			// Create sample manifest
			manifestPath := filepath.Join(dataDir, "manifests", "hello-world.yaml")
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				manifestContent := `metadata:
  name: "hello-world-system"
  version: "1.0.0"
  description: "Sistema de exemplo"

actors:
  - id: "admin"
    name: "Administrador"
    roles: ["admin"]
    permissions:
      - resource: "*"
        actions: ["*"]

data_model:
  entities:
    - name: "Message"
      fields:
        - name: "id"
          type: "string"
          required: true
          unique: true
        - name: "content"
          type: "string"
          required: true
        - name: "created_at"
          type: "datetime"
          required: true

integrations:
  apis:
    - name: "Hello API"
      base_path: "/api/v1"
      endpoints:
        - path: "/messages"
          method: "GET"
          handler: "Message.list"
        - path: "/messages"
          method: "POST"
          handler: "Message.create"
`
				if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
					return fmt.Errorf("failed to create manifest file: %w", err)
				}
				fmt.Printf("Created: %s\n", manifestPath)
			}

			fmt.Println("\nAgentOS initialized successfully!")
			fmt.Printf("Data directory: %s\n", dataDir)
			fmt.Println("\nNext steps:")
			fmt.Printf("  1. Edit your manifest: %s\n", filepath.Join(dataDir, "manifests", "system.yaml"))
			fmt.Printf("  2. Run bootstrap: picoclaw agentos bootstrap --manifest %s\n", filepath.Join(dataDir, "manifests", "system.yaml"))

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS (default: ~/picoclaw-data)")

	return cmd
}

// newBootstrapCommand cria o comando para fazer bootstrap do sistema
func newBootstrapCommand() *cobra.Command {
	var (
		manifestPath string
		dataDir      string
		dbDriver     string
		dbConnection string
		interactive  bool
	)

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Materializa o sistema descrito no manifesto",
		Long:  `Executa o pipeline de bootstrap que cria toda a infraestrutura do sistema.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interactive {
				reader := bufio.NewReader(os.Stdin)

				fmt.Print("Manifest path [manifests/system.yaml]: ")
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(input)
				if input != "" {
					manifestPath = input
				}

				fmt.Print("Data directory [~/picoclaw-data]: ")
				input, _ = reader.ReadString('\n')
				input = strings.TrimSpace(input)
				if input != "" {
					dataDir = input
				}

				fmt.Print("Database driver [sqlite]: ")
				input, _ = reader.ReadString('\n')
				input = strings.TrimSpace(input)
				if input != "" {
					dbDriver = input
				}
			}

			// Resolve paths
			if dataDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				dataDir = filepath.Join(homeDir, "picoclaw-data")
			}

			if manifestPath == "" {
				manifestPath = filepath.Join(dataDir, "manifests", "system.yaml")
			}

			// Make manifest path absolute if relative
			if !filepath.IsAbs(manifestPath) {
				manifestPath = filepath.Join(dataDir, manifestPath)
			}

			// Verify manifest exists
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				return fmt.Errorf("manifest not found: %s", manifestPath)
			}

			fmt.Println("=== Bootstrap Pipeline ===")
			fmt.Printf("Manifest: %s\n", manifestPath)
			fmt.Printf("Data Directory: %s\n", dataDir)
			fmt.Println()

			// Load and validate manifest first
			fmt.Println("[1/12] Load Manifest...")
			m, err := manifest.ParseFile(manifestPath)
			if err != nil {
				return fmt.Errorf("failed to parse manifest: %w", err)
			}
			fmt.Println("       Parse e validacao")

			fmt.Println("[2/12] Validate...")
			parser := &manifest.Parser{}
			if err := parser.Validate(m); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
			fmt.Println("       Consistencia verificada")

			// Configure database
			if dbDriver == "" {
				dbDriver = "sqlite"
			}
			if dbConnection == "" {
				dbConnection = filepath.Join(dataDir, "db", fmt.Sprintf("%s.db", m.Metadata.Name))
			}

			cfg := agentos.BootstrapConfig{
				ManifestPath: manifestPath,
				DBDriver:     dbDriver,
				DBConnection: dbConnection,
				DataDir:      dataDir,
			}

			fmt.Println("[3/12] Open Database...")
			fmt.Printf("       Driver: %s\n", dbDriver)

			bootstrapper := agentos.NewBootstrapper(cfg)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fmt.Println("[4/12] Run Migrations...")
			fmt.Println("[5/12] Provision Actors...")
			fmt.Println("[6/12] Build Catalog...")
			fmt.Println("[7/12] Create PolicyEng...")
			fmt.Println("[8/12] Create RuleExec...")
			fmt.Println("[9/12] Create FSMs...")
			fmt.Println("[10/12] Create APIGen...")
			fmt.Println("[11/12] Mount HTTP Mux...")

			instance, err := bootstrapper.Bootstrap(ctx)
			if err != nil {
				return fmt.Errorf("bootstrap failed: %w", err)
			}

			fmt.Println("[12/12] System Ready")
			fmt.Println()

			// Print summary
			fmt.Println("=== System Initialized ===")
			fmt.Printf("Name: %s v%s\n", instance.Manifest.Metadata.Name, instance.Manifest.Metadata.Version)
			if instance.Manifest.Metadata.Description != "" {
				fmt.Printf("Description: %s\n", instance.Manifest.Metadata.Description)
			}
			fmt.Printf("Actors: %d\n", len(instance.Manifest.Actors))
			fmt.Printf("Entities: %d\n", len(instance.Manifest.DataModel.Entities))
			fmt.Printf("Operations: %d\n", len(instance.Catalog.ListAll()))
			fmt.Println()

			// Store instance reference for potential serve command
			if err := saveInstanceState(dataDir, manifestPath, dbConnection); err != nil {
				fmt.Printf("Warning: failed to save instance state: %v\n", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to the manifest YAML file")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")
	cmd.Flags().StringVar(&dbDriver, "db-driver", "sqlite", "Database driver (sqlite, postgres, mysql)")
	cmd.Flags().StringVar(&dbConnection, "db-connection", "", "Database connection string")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Interactive mode")

	return cmd
}

// newServeCommand cria o comando para iniciar o servidor
func newServeCommand() *cobra.Command {
	var (
		dataDir      string
		manifestPath string
		port         int
		host         string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Inicia o servidor HTTP do sistema AgentOS",
		Long:  `Inicia o servidor HTTP com API REST e endpoints de sistema.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load saved state or use defaults
			if dataDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				dataDir = filepath.Join(homeDir, "picoclaw-data")
			}

			// Try to load saved state
			state, err := loadInstanceState(dataDir)
			if err == nil && manifestPath == "" {
				manifestPath = state.ManifestPath
			}

			if manifestPath == "" {
				manifestPath = filepath.Join(dataDir, "manifests", "system.yaml")
			}

			// Verify manifest exists
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				return fmt.Errorf("manifest not found: %s\nRun 'picoclaw agentos bootstrap' first", manifestPath)
			}

			// Bootstrap the system
			cfg := agentos.BootstrapConfig{
				ManifestPath: manifestPath,
				DBDriver:     "sqlite",
				DBConnection: state.DBConnection,
				DataDir:      dataDir,
			}

			if cfg.DBConnection == "" {
				m, _ := manifest.ParseFile(manifestPath)
				if m != nil {
					cfg.DBConnection = filepath.Join(dataDir, "db", fmt.Sprintf("%s.db", m.Metadata.Name))
				}
			}

			bootstrapper := agentos.NewBootstrapper(cfg)

			ctx := context.Background()
			instance, err := bootstrapper.Bootstrap(ctx)
			if err != nil {
				return fmt.Errorf("bootstrap failed: %w", err)
			}

			// Start HTTP server
			addr := fmt.Sprintf("%s:%d", host, port)
			server := &http.Server{
				Addr:    addr,
				Handler: instance,
			}

			fmt.Printf("=== Server Starting ===\n")
			fmt.Printf("Name: %s v%s\n", instance.Manifest.Metadata.Name, instance.Manifest.Metadata.Version)
			fmt.Printf("Address: http://%s\n", addr)
			fmt.Println()
			fmt.Println("=== API Endpoints ===")
			for _, op := range instance.Catalog.ListAll() {
				fmt.Printf("  %s %s - %s\n", op.Method, op.Path, op.Description)
			}
			fmt.Println()
			fmt.Println("=== System Endpoints ===")
			fmt.Println("  GET /_system/info - System information")
			fmt.Println("  GET /_system/actors - List actors")
			fmt.Println("  GET /_system/operations - List all operations")
			fmt.Println("  GET /_health - Health check")
			fmt.Println()
			fmt.Println("Press Ctrl+C to stop")

			// Handle graceful shutdown
			done := make(chan error, 1)
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					done <- err
				}
				close(done)
			}()

			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			select {
			case err := <-done:
				if err != nil {
					return fmt.Errorf("server error: %w", err)
				}
			case sig := <-sigChan:
				fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := instance.Shutdown(shutdownCtx); err != nil {
					fmt.Printf("Shutdown error: %v\n", err)
				}
				if err := server.Shutdown(shutdownCtx); err != nil {
					fmt.Printf("Server shutdown error: %v\n", err)
				}
			}

			fmt.Println("Server stopped gracefully")
			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to the manifest YAML file")
	cmd.Flags().IntVar(&port, "port", 8080, "Port to listen on")
	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "Host to bind to")

	return cmd
}

// newValidateCommand cria o comando para validar manifestos
func newValidateCommand() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Valida um manifesto YAML/JSON",
		Long:  `Valida a estrutura e semantica de um manifesto do AgentOS.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				manifestPath = args[0]
			}

			if manifestPath == "" {
				return fmt.Errorf("manifest path is required")
			}

			// Verify file exists
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				return fmt.Errorf("manifest not found: %s", manifestPath)
			}

			fmt.Printf("Validating manifest: %s\n", manifestPath)

			m, err := manifest.ParseFile(manifestPath)
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}

			parser := &manifest.Parser{}
			if err := parser.Validate(m); err != nil {
				return fmt.Errorf("validation error: %w", err)
			}

			fmt.Println("✓ Manifest is valid!")
			fmt.Printf("  Name: %s\n", m.Metadata.Name)
			fmt.Printf("  Version: %s\n", m.Metadata.Version)
			fmt.Printf("  Actors: %d\n", len(m.Actors))
			fmt.Printf("  Entities: %d\n", len(m.DataModel.Entities))
			fmt.Printf("  Rules: %d\n", len(m.BusinessRules))
			fmt.Printf("  Workflows: %d\n", len(m.Workflows))

			return nil
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", "", "Path to the manifest YAML file")

	return cmd
}

// newStatusCommand cria o comando para verificar status do sistema
func newStatusCommand() *cobra.Command {
	var (
		dataDir string
		host    string
		port    int
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Verifica o status do sistema AgentOS",
		Long:  `Conecta ao servidor e exibe informacoes de saude e status do sistema.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dataDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				dataDir = filepath.Join(homeDir, "picoclaw-data")
			}

			// Try to get server URL from saved state or use default
			state, err := loadInstanceState(dataDir)
			if err != nil {
				// Use command line flags
				_ = state
			}

			serverURL := fmt.Sprintf("http://%s:%d", host, port)
			if state.ServerURL != "" {
				serverURL = state.ServerURL
			}

			// Check health endpoint
			resp, err := http.Get(serverURL + "/_health")
			if err != nil {
				return fmt.Errorf("server unreachable: %w\nIs the server running? (picoclaw agentos serve)", err)
			}
			defer resp.Body.Close()

			body := make([]byte, 1024)
			n, _ := resp.Body.Read(body)

			fmt.Println("=== System Status ===")
			fmt.Printf("Server: %s\n", serverURL)
			fmt.Printf("Status: %d %s\n", resp.StatusCode, resp.Status)
			if n > 0 {
				fmt.Printf("Response: %s\n", string(body[:n]))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")
	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")
	cmd.Flags().IntVar(&port, "port", 8080, "Server port")

	return cmd
}

// newLogsCommand cria o comando para visualizar logs
func newLogsCommand() *cobra.Command {
	var (
		dataDir string
		follow  bool
		tail    int
	)

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Exibe os logs do sistema AgentOS",
		Long:  `Exibe os logs de operacao e auditoria do sistema.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dataDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				dataDir = filepath.Join(homeDir, "picoclaw-data")
			}

			logPath := filepath.Join(dataDir, "logs", "picoclaw.log")

			if _, err := os.Stat(logPath); os.IsNotExist(err) {
				return fmt.Errorf("log file not found: %s", logPath)
			}

			file, err := os.Open(logPath)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
			defer file.Close()

			if tail > 0 {
				// Read last N lines
				lines := make([]string, 0, tail)
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
					if len(lines) > tail {
						lines = lines[1:]
					}
				}
				for _, line := range lines {
					fmt.Println(line)
				}
			} else {
				// Read all
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					fmt.Println(scanner.Text())
				}
			}

			_ = follow

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVarP(&tail, "tail", "n", 0, "Output the last N lines")

	return cmd
}

// newDiffCommand cria o comando para comparar manifestos
func newDiffCommand() *cobra.Command {
	var (
		from string
		to   string
	)

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compara dois manifestos e detecta mudancas",
		Long:  `Analisa as diferencas entre duas versoes de manifesto.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" || to == "" {
				return fmt.Errorf("both --from and --to flags are required")
			}

			fmt.Println("Detectando mudancas...")
			fmt.Println("============================")

			// Parse manifests
			fromManifest, err := manifest.ParseFile(from)
			if err != nil {
				return fmt.Errorf("failed to parse from manifest: %w", err)
			}

			toManifest, err := manifest.ParseFile(to)
			if err != nil {
				return fmt.Errorf("failed to parse to manifest: %w", err)
			}

			fmt.Printf("Versao: %s → %s\n\n", fromManifest.Metadata.Version, toManifest.Metadata.Version)

			// Simple comparison
			fmt.Println("Mudancas detectadas:")

			// Compare entities
			fromEntities := make(map[string]bool)
			for _, e := range fromManifest.DataModel.Entities {
				fromEntities[e.Name] = true
			}

			for _, e := range toManifest.DataModel.Entities {
				if !fromEntities[e.Name] {
					fmt.Printf("  ✓ ADD_ENTITY: %s\n", e.Name)
				}
			}

			// Compare version
			if fromManifest.Metadata.Version != toManifest.Metadata.Version {
				fmt.Printf("  ✓ VERSION_CHANGE: %s → %s\n", fromManifest.Metadata.Version, toManifest.Metadata.Version)
			}

			fmt.Println("\nPara aplicar estas mudancas, execute:")
			fmt.Printf("  picoclaw agentos bootstrap --manifest %s\n", to)

			return nil
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Path to the source manifest")
	cmd.Flags().StringVar(&to, "to", "", "Path to the target manifest")

	return cmd
}

// newVersionsCommand cria o comando para listar versoes
func newVersionsCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "versions",
		Short: "Lista as versoes do manifesto",
		Long:  `Exibe o historico de versoes do sistema.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dataDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				dataDir = filepath.Join(homeDir, "picoclaw-data")
			}

			state, err := loadInstanceState(dataDir)
			if err != nil {
				fmt.Println("No versions found. Run 'picoclaw agentos bootstrap' first.")
				return nil
			}

			fmt.Println("=== Versions ===")
			if state.ManifestPath != "" {
				m, _ := manifest.ParseFile(state.ManifestPath)
				if m != nil {
					fmt.Printf("Current: %s v%s\n", m.Metadata.Name, m.Metadata.Version)
					fmt.Printf("Path: %s\n", state.ManifestPath)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")

	return cmd
}

// newConfigCommand cria o comando para gerenciar configuracao
func newConfigCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Gerencia a configuracao do AgentOS",
		Long:  `Visualiza e modifica as configuracoes do AgentOS.`,
	}

	getCmd := &cobra.Command{
		Use:   "get [key]",
		Short: "Obtem um valor de configuracao",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			if dataDir == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				dataDir = filepath.Join(homeDir, "picoclaw-data")
			}

			configPath := filepath.Join(dataDir, "config.yaml")
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				return fmt.Errorf("config not found: %s", configPath)
			}

			data, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("failed to read config: %w", err)
			}

			// Simple YAML-like parsing for demonstration
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.Contains(line, key+":") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						fmt.Printf("%s: %s\n", key, strings.TrimSpace(parts[1]))
						return nil
					}
				}
			}

			return fmt.Errorf("key not found: %s", key)
		},
	}

	getCmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")

	cmd.AddCommand(getCmd)

	return cmd
}

// InstanceState represents the saved state of a bootstrapped instance
type InstanceState struct {
	ManifestPath string `json:"manifest_path"`
	DBConnection string `json:"db_connection"`
	ServerURL    string `json:"server_url"`
}

func saveInstanceState(dataDir, manifestPath, dbConnection string) error {
	state := InstanceState{
		ManifestPath: manifestPath,
		DBConnection: dbConnection,
		ServerURL:    "http://localhost:8080",
	}

	statePath := filepath.Join(dataDir, ".agentos_state.json")

	// Simple JSON marshaling without importing encoding/json
	content := fmt.Sprintf(`{
  "manifest_path": %q,
  "db_connection": %q,
  "server_url": %q
}`, state.ManifestPath, state.DBConnection, state.ServerURL)

	return os.WriteFile(statePath, []byte(content), 0600)
}

func loadInstanceState(dataDir string) (InstanceState, error) {
	var state InstanceState
	statePath := filepath.Join(dataDir, ".agentos_state.json")

	data, err := os.ReadFile(statePath)
	if err != nil {
		return state, err
	}

	// Simple parsing - look for values in the JSON
	content := string(data)
	if start := strings.Index(content, `"manifest_path":`); start != -1 {
		start += len(`"manifest_path":`)
		if quote := strings.Index(content[start:], `"`); quote != -1 {
			start += quote + 1
			if end := strings.Index(content[start:], `"`); end != -1 {
				state.ManifestPath = content[start : start+end]
			}
		}
	}

	if start := strings.Index(content, `"db_connection":`); start != -1 {
		start += len(`"db_connection":`)
		if quote := strings.Index(content[start:], `"`); quote != -1 {
			start += quote + 1
			if end := strings.Index(content[start:], `"`); end != -1 {
				state.DBConnection = content[start : start+end]
			}
		}
	}

	if start := strings.Index(content, `"server_url":`); start != -1 {
		start += len(`"server_url":`)
		if quote := strings.Index(content[start:], `"`); quote != -1 {
			start += quote + 1
			if end := strings.Index(content[start:], `"`); end != -1 {
				state.ServerURL = content[start : start+end]
			}
		}
	}

	return state, nil
}
