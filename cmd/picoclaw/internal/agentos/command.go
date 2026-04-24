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
	mcpserver "github.com/sipeed/picoclaw/pkg/mcp/server"
	"github.com/sipeed/picoclaw/pkg/manifest"
)

func NewAgentosCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agentos",
		Short: "AgentOS - Sistema Operacional de Agentes",
		Long: `AgentOS é um Sistema Operacional de Agentes que transforma o PicoClaw
em uma plataforma capaz de gerar automaticamente infraestrutura completa
a partir de um manifesto declarativo YAML/JSON.

Cada sistema gerenciado pelo AgentOS é independente e pode coexistir
no mesmo diretório de dados. Use --system para selecionar qual sistema
operar em comandos que suportam múltiplos sistemas.`,
	}

	cmd.AddCommand(
		newInitCommand(),
		newBootstrapCommand(),
		newServeCommand(),
		newMultiServeCommand(),
		newRemoveCommand(),
		newConvertCommand(),
		newValidateCommand(),
		newStatusCommand(),
		newLogsCommand(),
		newDiffCommand(),
		newListCommand(),
		newVersionsCommand(),
		newConfigCommand(),
	)

	return cmd
}

// getDataDir returns the data directory, using default if not specified
func getDataDir(flag string) string {
	if flag != "" {
		return flag
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, "picoclaw-data")
}

// newInitCommand cria o comando para inicializar a estrutura do AgentOS
func newInitCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Inicializa a estrutura de dados do AgentOS",
		Long:  `Cria os diretórios necessários para o funcionamento do AgentOS.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = getDataDir(dataDir)

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

			// Create sample hello-world manifest
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
			fmt.Println("  1. Place your manifest files in:", filepath.Join(dataDir, "manifests"))
			fmt.Println("  2. Run: picoclaw agentos bootstrap --manifest <your-manifest.yaml>")

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
		systemName   string
		interactive  bool
	)

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Materializa o sistema descrito no manifesto",
		Long:  `Executa o pipeline de bootstrap que cria toda a infraestrutura do sistema.

Se o mesmo diretório de dados já contém outros sistemas, um novo banco de dados
será criado automaticamente (baseado no nome do sistema no manifesto).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = getDataDir(dataDir)

			if interactive {
				reader := bufio.NewReader(os.Stdin)

				fmt.Print("Manifest path: ")
				input, _ := reader.ReadString('\n')
				manifestPath = strings.TrimSpace(input)

				fmt.Print("System name (optional, uses manifest name if empty): ")
				input, _ = reader.ReadString('\n')
				systemName = strings.TrimSpace(input)
			}

			// Resolve manifest path
			if manifestPath == "" {
				return fmt.Errorf("--manifest is required")
			}

			if !filepath.IsAbs(manifestPath) {
				manifestPath = filepath.Join(dataDir, manifestPath)
			}

			// Verify manifest exists
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				return fmt.Errorf("manifest not found: %s", manifestPath)
			}

			// Load manifest to get system name
			m, err := manifest.ParseFile(manifestPath)
			if err != nil {
				return fmt.Errorf("failed to parse manifest: %w", err)
			}

			// Validate manifest
			parser := &manifest.Parser{}
			if err := parser.Validate(m); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			// Use manifest name if system name not provided
			if systemName == "" {
				systemName = m.Metadata.Name
			}

			// Ensure db directory exists
			dbDir := filepath.Join(dataDir, "db")
			if err := os.MkdirAll(dbDir, 0755); err != nil {
				return fmt.Errorf("failed to create db directory: %w", err)
			}

			// Configure database connection
			if dbDriver == "" {
				dbDriver = "sqlite"
			}
			if dbConnection == "" {
				// Use system name for database file
				dbConnection = filepath.Join(dbDir, fmt.Sprintf("%s.db", systemName))
			}

			// Check if database already exists for another system
			registry, err := LoadRegistry(dataDir)
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}

			// Check for conflicts
			for name, sys := range registry.Systems {
				if name == systemName {
					fmt.Printf("Warning: System '%s' already exists. It will be updated.\n", systemName)
					break
				}
				if sys.DBConnection == dbConnection {
					return fmt.Errorf("database conflict: '%s' is already used by system '%s'", dbConnection, name)
				}
			}

			fmt.Println("=== Bootstrap Pipeline ===")
			fmt.Printf("System: %s\n", systemName)
			fmt.Printf("Manifest: %s\n", manifestPath)
			fmt.Printf("Data Directory: %s\n", dataDir)
			fmt.Printf("Database: %s\n", dbConnection)
			if registry.GetSystemCount() > 0 {
				fmt.Printf("Existing Systems: %d\n", registry.GetSystemCount())
			}
			fmt.Println()

			cfg := agentos.BootstrapConfig{
				ManifestPath: manifestPath,
				DBDriver:     dbDriver,
				DBConnection: dbConnection,
				DataDir:      dataDir,
			}

			bootstrapper := agentos.NewBootstrapper(cfg)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fmt.Println("[1/12] Load Manifest...")
			fmt.Println("[2/12] Validate...")
			fmt.Println("[3/12] Open Database...")
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

			// Register system
			registry.RegisterSystem(systemName, manifestPath, dbConnection, dataDir)
			if err := registry.Save(dataDir); err != nil {
				return fmt.Errorf("failed to save registry: %w", err)
			}

			fmt.Printf("System '%s' registered successfully!\n", systemName)
			if registry.GetSystemCount() > 1 {
				fmt.Println("\nOther systems in this data directory:")
				for name := range registry.Systems {
					if name != systemName {
						fmt.Printf("  - %s\n", name)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to the manifest YAML file")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")
	cmd.Flags().StringVar(&dbDriver, "db-driver", "sqlite", "Database driver (sqlite, postgres, mysql)")
	cmd.Flags().StringVar(&dbConnection, "db-connection", "", "Database connection string (auto-generated if not set)")
	cmd.Flags().StringVar(&systemName, "system", "", "System name (uses manifest name if not set)")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Interactive mode")

	_ = cmd.MarkFlagRequired("manifest")

	return cmd
}

// newServeCommand cria o comando para iniciar o servidor
func newServeCommand() *cobra.Command {
	var (
		dataDir string
		port int
		host string
		systemName string
		forceSingle bool // Force single system mode even with multiple systems
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Inicia o servidor HTTP do sistema AgentOS",
		Long: `Inicia o servidor HTTP com API REST e endpoints de sistema.

Se múltiplos sistemas existem no diretório de dados:
- Por padrão, inicia o servidor multi-sistema (equivalente a 'multi-serve')
- Use --system <name> para servir um sistema específico
- Use --single para forçar o modo de sistema único

Exemplos:
  # Inicia servidor com todos os sistemas (quando múltiplos existem)
  picoclaw agentos serve

  # Serve um sistema específico
  picoclaw agentos serve --system cafeteria

  # Força modo single mesmo com múltiplos sistemas
  picoclaw agentos serve --single --system cafeteria`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir = getDataDir(dataDir)

		// Load registry
		registry, err := LoadRegistry(dataDir)
		if err != nil {
			return fmt.Errorf("failed to load registry: %w", err)
		}

		// Handle multiple systems
		if registry.GetSystemCount() == 0 {
			return fmt.Errorf("no systems found in %s\nRun 'picoclaw agentos bootstrap' first", dataDir)
		}

		// Check if we should run in multi-system mode
		// Multi-system mode is default when:
		// 1. No specific system is requested AND multiple systems exist
		// 2. User explicitly requested multi-serve mode
		shouldRunMultiSystem := !forceSingle && (systemName == "" && registry.HasMultipleSystems())

		if shouldRunMultiSystem {
			fmt.Printf("Detectado %d sistemas. Iniciando em modo multi-sistema...\n\n", registry.GetSystemCount())
			// Delegate to multi-serve implementation
			return runMultiServe(dataDir, host, port, false, "")
		}

		// Get target system
		system, err := registry.GetSystem(systemName)
		if err != nil {
			// Show available systems
			fmt.Println("Available systems:")
			for name, info := range registry.Systems {
				prefix := " "
				if name == registry.Default {
					prefix = "* "
				}
				fmt.Printf("%s%s (manifest: %s)\n", prefix, name, info.ManifestPath)
			}
			fmt.Println("\nUse --system <name> to select a system")
			return err
		}

			// Verify manifest exists
			if _, err := os.Stat(system.ManifestPath); os.IsNotExist(err) {
				return fmt.Errorf("manifest not found: %s\nSystem may need to be re-registered", system.ManifestPath)
			}

			// Bootstrap the system
			cfg := agentos.BootstrapConfig{
				ManifestPath: system.ManifestPath,
				DBDriver:     "sqlite",
				DBConnection: system.DBConnection,
				DataDir:      dataDir,
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
			fmt.Printf("System: %s\n", system.Name)
			fmt.Printf("Name: %s v%s\n", instance.Manifest.Metadata.Name, instance.Manifest.Metadata.Version)
			fmt.Printf("Address: http://%s\n", addr)
			fmt.Printf("Database: %s\n", system.DBConnection)
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
			if registry.HasMultipleSystems() {
				fmt.Println("\nNote: Multiple systems are registered.")
				fmt.Println("      Other systems:")
				for name := range registry.Systems {
					if name != system.Name {
						fmt.Printf("      - %s\n", name)
					}
				}
			}
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
	cmd.Flags().IntVar(&port, "port", 8080, "Port to listen on")
	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "Host to bind to")
	cmd.Flags().StringVar(&systemName, "system", "", "System to serve (uses default if not set)")
	cmd.Flags().BoolVar(&forceSingle, "single", false, "Force single system mode even with multiple systems")

	return cmd
}

// runMultiServe executes the multi-serve functionality
func runMultiServe(dataDir, host string, port int, requireAuth bool, adminKey string) error {
	// Load registry
	registry, err := LoadRegistry(dataDir)
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	if registry.GetSystemCount() == 0 {
		return fmt.Errorf("no systems found in %s\nRun 'picoclaw agentos bootstrap' first", dataDir)
	}

	// Generate admin key if not provided
	if adminKey == "" && requireAuth {
		adminKey = generateAPIKey()
		fmt.Printf("Generated admin API key: %s\n", adminKey)
	}

	// Create multi-system server
	server := &MultiSystemServer{
		DataDir: dataDir,
		Host:    host,
		Port:    port,
		Systems: make(map[string]*ManagedSystem),
		mux:     http.NewServeMux(),
		registry: registry,
		globalAuth: GlobalAuthConfig{
			Enabled:     requireAuth,
			AdminKey:    adminKey,
			RequireAuth: requireAuth,
		},
	}

	// Bootstrap all systems
	fmt.Println("=== Bootstrapping Systems ===")
	for name, sysInfo := range registry.Systems {
		fmt.Printf("Bootstrapping %s...\n", name)

		if _, err := os.Stat(sysInfo.ManifestPath); os.IsNotExist(err) {
			fmt.Printf(" Warning: Manifest not found for %s, skipping\n", name)
			continue
		}

		cfg := agentos.BootstrapConfig{
			ManifestPath: sysInfo.ManifestPath,
			DBDriver:     "sqlite",
			DBConnection: sysInfo.DBConnection,
			DataDir:      dataDir,
		}

		bootstrapper := agentos.NewBootstrapper(cfg)
		ctx := context.Background()
		instance, err := bootstrapper.Bootstrap(ctx)
		if err != nil {
			return fmt.Errorf("failed to bootstrap %s: %w", name, err)
		}

		// Create MCP server for this system
		mcpCfg := mcpserver.ServerConfig{
			Name:        instance.Manifest.Metadata.Name,
			Version:     instance.Manifest.Metadata.Version,
			Description: instance.Manifest.Metadata.Description,
		}
		mcpSvr := mcpserver.NewMCPServer(mcpCfg, instance)

		// Generate API key for this system
		systemAPIKey := generateAPIKey()

		systemPrefix := "/" + name
		server.Systems[name] = &ManagedSystem{
			System:    sysInfo,
			Instance:  instance,
			Prefix:    systemPrefix,
			MCPServer: mcpSvr,
			apiKeys:   map[string]bool{systemAPIKey: true},
		}
		fmt.Printf(" %s bootstrapped (prefix: %s, api_key: %s)\n", name, systemPrefix, systemAPIKey[:8]+"...")
	}

	if len(server.Systems) == 0 {
		return fmt.Errorf("no systems could be bootstrapped")
	}

	// Setup dashboard
	server.dashboard = NewDashboardHandler(server)

	// Setup routes
	server.setupRoutes()

	// Start HTTP server
	addr := fmt.Sprintf("%s:%d", host, port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	fmt.Printf("\n=== Multi-System Server Starting ===\n")
	fmt.Printf("Address: http://%s\n", addr)
	fmt.Printf("Systems: %d\n\n", len(server.Systems))

	// Print system endpoints
	fmt.Println("=== Available Systems ===")
	for name, sys := range server.Systems {
		fmt.Printf("\n%s (%s v%s)\n", name, sys.Instance.Manifest.Metadata.Name, sys.Instance.Manifest.Metadata.Version)
		fmt.Printf(" API: http://%s%s/api/v1/...\n", addr, sys.Prefix)
		fmt.Printf(" System: http://%s%s/_system/...\n", addr, sys.Prefix)
		fmt.Printf(" Health: http://%s%s/_health\n", addr, sys.Prefix)
		fmt.Printf(" MCP: http://%s%s/mcp\n", addr, sys.Prefix)
	}

	fmt.Println("\n=== Global Endpoints ===")
	fmt.Printf(" Systems List: http://%s/_systems\n", addr)
	fmt.Printf(" Health: http://%s/_health\n", addr)
	fmt.Printf(" Admin: http://%s/admin\n", addr)

	if server.globalAuth.Enabled {
		fmt.Println("\n=== Authentication ===")
		fmt.Printf(" Admin Key: %s\n", server.globalAuth.AdminKey)
		fmt.Println(" Pass X-API-Key header for authentication")
	}

	fmt.Println("\nPress Ctrl+C to stop")

	// Handle graceful shutdown
	done := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

		// Shutdown all systems
		for name, sys := range server.Systems {
			fmt.Printf("Shutting down %s...\n", name)
			if err := sys.Instance.Shutdown(shutdownCtx); err != nil {
				fmt.Printf("Shutdown error for %s: %v\n", name, err)
			}
		}

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			fmt.Printf("Server shutdown error: %v\n", err)
		}
	}

	fmt.Println("Server stopped gracefully")
	return nil
}

// newValidateCommand cria o comando para validar manifestos
func newValidateCommand() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "validate [manifest]",
		Short: "Valida um manifesto YAML/JSON",
		Long:  `Valida a estrutura e semântica de um manifesto do AgentOS.`,
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

			// Print warnings if any
			warnings := parser.GetWarnings()
			if len(warnings) > 0 {
				fmt.Println("\nWarnings:")
				for _, w := range warnings {
					fmt.Printf("  - %s\n", w)
				}
			}

			fmt.Println("\n✓ Manifest is valid!")
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
		dataDir    string
		host       string
		port       int
		systemName string
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Verifica o status do sistema AgentOS",
		Long:  `Conecta ao servidor e exibe informações de saúde e status do sistema.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = getDataDir(dataDir)

			// Load registry
			registry, err := LoadRegistry(dataDir)
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}

			// Show status for all systems or specific one
			if systemName == "" {
				// Show registry overview
				fmt.Println("=== AgentOS Registry ===")
				fmt.Printf("Data Directory: %s\n", dataDir)
				fmt.Printf("Systems: %d\n", registry.GetSystemCount())

				if registry.GetSystemCount() == 0 {
					fmt.Println("\nNo systems registered.")
					fmt.Println("Run 'picoclaw agentos bootstrap' to create a system.")
					return nil
				}

				fmt.Printf("\nDefault System: %s\n", registry.Default)
				fmt.Println("\nRegistered Systems:")
				for name, sys := range registry.Systems {
					defaultMark := ""
					if name == registry.Default {
						defaultMark = " (default)"
					}
					fmt.Printf("\n  %s%s\n", name, defaultMark)
					fmt.Printf("    Manifest: %s\n", sys.ManifestPath)
					fmt.Printf("    Database: %s\n", sys.DBConnection)
					fmt.Printf("    Created: %s\n", sys.CreatedAt.Format("2006-01-02 15:04:05"))
				}
			} else {
				// Show specific system status
				system, err := registry.GetSystem(systemName)
				if err != nil {
					return err
				}

				serverURL := fmt.Sprintf("http://%s:%d", host, port)
				if system.ServerURL != "" {
					serverURL = system.ServerURL
				}

				fmt.Printf("=== System Status: %s ===\n", systemName)
				fmt.Printf("Manifest: %s\n", system.ManifestPath)
				fmt.Printf("Database: %s\n", system.DBConnection)
				fmt.Printf("Server: %s\n", serverURL)

				// Check health endpoint
				resp, err := http.Get(serverURL + "/_health")
				if err != nil {
					fmt.Printf("Status: NOT RUNNING (%v)\n", err)
				} else {
					defer resp.Body.Close()
					fmt.Printf("Status: Running (HTTP %d)\n", resp.StatusCode)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")
	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")
	cmd.Flags().IntVar(&port, "port", 8080, "Server port")
	cmd.Flags().StringVar(&systemName, "system", "", "System to check status for (shows all if not set)")

	return cmd
}

// newListCommand cria o comando para listar sistemas
func newListCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista todos os sistemas registrados",
		Long:  `Exibe uma lista de todos os sistemas AgentOS registrados no diretório de dados.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = getDataDir(dataDir)

			registry, err := LoadRegistry(dataDir)
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}

			if registry.GetSystemCount() == 0 {
				fmt.Println("No systems registered.")
				fmt.Println("Run 'picoclaw agentos bootstrap' to create your first system.")
				return nil
			}

			fmt.Printf("Systems in %s:\n\n", dataDir)
			fmt.Printf("%-20s %-30s %s\n", "NAME", "DATABASE", "CREATED")
			fmt.Println(strings.Repeat("-", 80))

			for name, sys := range registry.Systems {
				defaultMark := ""
				if name == registry.Default {
					defaultMark = " *"
				}
				dbFile := filepath.Base(sys.DBConnection)
				fmt.Printf("%-20s %-30s %s%s\n",
					name+defaultMark,
					dbFile,
					sys.CreatedAt.Format("2006-01-02"),
				)
			}

			fmt.Println("\n* = default system")

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")

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
		Long:  `Exibe os logs de operação e auditoria do sistema.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = getDataDir(dataDir)

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
		Short: "Compara dois manifestos e detecta mudanças",
		Long:  `Analisa as diferenças entre duas versões de manifesto.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if from == "" || to == "" {
				return fmt.Errorf("both --from and --to flags are required")
			}

			fmt.Println("Detectando mudanças...")
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

			fmt.Printf("Versão: %s → %s\n\n", fromManifest.Metadata.Version, toManifest.Metadata.Version)

			// Simple comparison
			fmt.Println("Mudanças detectadas:")

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

			fmt.Println("\nPara aplicar estas mudanças, execute:")
			fmt.Printf("  picoclaw agentos bootstrap --manifest %s\n", to)

			return nil
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "Path to the source manifest")
	cmd.Flags().StringVar(&to, "to", "", "Path to the target manifest")

	return cmd
}

// newVersionsCommand cria o comando para listar versões
func newVersionsCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "versions",
		Short: "Lista as versões do manifesto",
		Long:  `Exibe o histórico de versões do sistema.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = getDataDir(dataDir)

			registry, err := LoadRegistry(dataDir)
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}

			if registry.GetSystemCount() == 0 {
				fmt.Println("No systems found.")
				return nil
			}

			fmt.Println("=== Registered Systems ===")
			for name, sys := range registry.Systems {
				m, err := manifest.ParseFile(sys.ManifestPath)
				if err != nil {
					fmt.Printf("  %s - error reading manifest: %v\n", name, err)
					continue
				}
				defaultMark := ""
				if name == registry.Default {
					defaultMark = " (default)"
				}
				fmt.Printf("  %s%s: %s v%s\n", name, defaultMark, m.Metadata.Name, m.Metadata.Version)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")

	return cmd
}

// newConfigCommand cria o comando para gerenciar configuração
func newConfigCommand() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Gerencia a configuração do AgentOS",
		Long:  `Visualiza e modifica as configurações do AgentOS.`,
	}

	getCmd := &cobra.Command{
		Use:   "get [key]",
		Short: "Obtém um valor de configuração",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			dataDir = getDataDir(dataDir)

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
