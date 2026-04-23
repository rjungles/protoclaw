package agentos

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sipeed/picoclaw/pkg/agentos"
	"github.com/sipeed/picoclaw/pkg/mcp"
	mcpserver "github.com/sipeed/picoclaw/pkg/mcp/server"
	"github.com/spf13/cobra"
)

// ManagedSystem represents a running system instance in the multi-server
type ManagedSystem struct {
	System    *SystemInfo
	Instance  *agentos.SystemInstance
	Prefix    string
	MCPServer *mcpserver.MCPServer
	apiKeys   map[string]bool // Valid API keys for this system
	apiKeysMu sync.RWMutex
}

// MultiSystemServer manages multiple AgentOS systems on a single HTTP server
type MultiSystemServer struct {
	DataDir    string
	Host       string
	Port       int
	Systems    map[string]*ManagedSystem
	mux        *http.ServeMux
	registry   *SystemRegistry
	globalAuth GlobalAuthConfig
	dashboard  *DashboardHandler
}

// GlobalAuthConfig holds global authentication settings
type GlobalAuthConfig struct {
	Enabled     bool
	AdminKey    string
	RequireAuth bool
}

// newMultiServeCommand creates the command to serve multiple systems
func newMultiServeCommand() *cobra.Command {
	var (
		dataDir     string
		host        string
		port        int
		requireAuth bool
		adminKey    string
	)

	cmd := &cobra.Command{
		Use:   "multi-serve",
		Short: "Inicia servidor HTTP com múltiplos sistemas AgentOS",
		Long: `Inicia o servidor HTTP servindo todos os sistemas registrados.
Cada sistema é acessível através de um prefixo de URL único.

Endpoints:
  GET /_systems                 - Lista todos os sistemas
  GET /_systems/{name}/health - Health check de um sistema
  GET /admin                   - Dashboard de administração
  GET /{system}/api/v1/...    - API REST de um sistema específico
  GET /{system}/_system/...   - Endpoints de sistema específicos
  POST /{system}/mcp          - Endpoint MCP JSON-RPC`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = getDataDir(dataDir)

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
					fmt.Printf("  Warning: Manifest not found for %s, skipping\n", name)
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
				fmt.Printf("  %s bootstrapped (prefix: %s, api_key: %s)\n", name, systemPrefix, systemAPIKey[:8]+"...")
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
				fmt.Printf("  API:    http://%s%s/api/v1/...\n", addr, sys.Prefix)
				fmt.Printf("  System: http://%s%s/_system/...\n", addr, sys.Prefix)
				fmt.Printf("  Health: http://%s%s/_health\n", addr, sys.Prefix)
				fmt.Printf("  MCP:    http://%s%s/mcp\n", addr, sys.Prefix)
			}

			fmt.Println("\n=== Global Endpoints ===")
			fmt.Printf("  Systems List: http://%s/_systems\n", addr)
			fmt.Printf("  Health:       http://%s/_health\n", addr)
			fmt.Printf("  Admin:        http://%s/admin\n", addr)

			if server.globalAuth.Enabled {
				fmt.Println("\n=== Authentication ===")
				fmt.Printf("  Admin Key: %s\n", server.globalAuth.AdminKey)
				fmt.Println("  Pass X-API-Key header for authentication")
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
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				// Shutdown all systems
				for name, sys := range server.Systems {
					fmt.Printf("Shutting down %s...\n", name)
					if err := sys.Instance.Shutdown(shutdownCtx); err != nil {
						fmt.Printf("  Error shutting down %s: %v\n", name, err)
					}
				}

				if err := httpServer.Shutdown(shutdownCtx); err != nil {
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
	cmd.Flags().BoolVar(&requireAuth, "require-auth", false, "Require authentication for all requests")
	cmd.Flags().StringVar(&adminKey, "admin-key", "", "Admin API key (auto-generated if not provided)")

	return cmd
}

// generateAPIKey generates a random API key
func generateAPIKey() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// setupRoutes configures the HTTP router for all systems
func (s *MultiSystemServer) setupRoutes() {
	// Global endpoints
	s.mux.HandleFunc("GET /_systems", s.authMiddleware(s.serveSystemsList))
	s.mux.HandleFunc("GET /_health", s.serveGlobalHealth)

	// Admin dashboard
	s.mux.HandleFunc("GET /admin", s.authMiddleware(s.dashboard.ServeHTTP))
	s.mux.HandleFunc("GET /admin/", s.authMiddleware(s.dashboard.ServeHTTP))
	s.mux.HandleFunc("POST /admin/", s.authMiddleware(s.dashboard.ServeHTTP))

	// Inter-system communication endpoint
	s.mux.HandleFunc("POST /_inter-system/call", s.authMiddleware(s.handleInterSystemCall))

	// System-specific endpoints
	for name, sys := range s.Systems {
		name := name // capture for closure
		sys := sys

		// Health check for specific system (public)
		s.mux.HandleFunc(fmt.Sprintf("GET /%s/_health", name), func(w http.ResponseWriter, r *http.Request) {
			s.serveSystemHealth(w, r, sys)
		})

		// System endpoints with auth
		s.mux.HandleFunc(fmt.Sprintf("GET /%s/_system/", name), s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			s.proxyToSystem(w, r, sys, strings.TrimPrefix(r.URL.Path, sys.Prefix))
		}))

		// API endpoints with auth
		s.mux.HandleFunc(fmt.Sprintf("/%s/", name), s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			// Strip system prefix and forward
			newPath := strings.TrimPrefix(r.URL.Path, sys.Prefix)
			if newPath == "" {
				newPath = "/"
			}
			r.URL.Path = newPath
			r.URL.RawPath = ""
			sys.Instance.ServeHTTP(w, r)
		}))

		// MCP endpoint
		s.mux.HandleFunc(fmt.Sprintf("POST /%s/mcp", name), s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			s.handleMCPRequest(w, r, sys)
		}))
	}
}

// authMiddleware checks authentication if enabled
func (s *MultiSystemServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.globalAuth.Enabled {
			next(w, r)
			return
		}

		// Check for admin key
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		// Admin key bypasses all checks
		if apiKey == s.globalAuth.AdminKey {
			next(w, r)
			return
		}

		// Check system-specific keys
		path := r.URL.Path
		for _, sys := range s.Systems {
			if strings.HasPrefix(path, sys.Prefix+"/") || path == sys.Prefix {
				sys.apiKeysMu.RLock()
				valid := sys.apiKeys[apiKey]
				sys.apiKeysMu.RUnlock()
				if valid {
					next(w, r)
					return
				}
				break
			}
		}

		// Unauthorized
		w.Header().Set("WWW-Authenticate", "X-API-Key")
		http.Error(w, `{"error":"unauthorized","message":"Valid API key required"}`, http.StatusUnauthorized)
	}
}

// handleMCPRequest handles MCP JSON-RPC requests
func (s *MultiSystemServer) handleMCPRequest(w http.ResponseWriter, r *http.Request, sys *ManagedSystem) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"}}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req mcp.Request
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"}}`, http.StatusBadRequest)
		return
	}

	resp, err := sys.MCPServer.HandleRequest(r.Context(), &req)
	if err != nil {
		resp = &mcp.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &mcp.Error{
				Code:    -32603,
				Message: err.Error(),
			},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleInterSystemCall allows systems to communicate with each other
func (s *MultiSystemServer) handleInterSystemCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		From   string          `json:"from"`
		To     string          `json:"to"`
		Action string          `json:"action"`
		Path   string          `json:"path"`
		Method string          `json:"method"`
		Body   json.RawMessage `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	// Get target system
	_, ok := s.Systems[req.To]
	if !ok {
		http.Error(w, `{"error":"target system not found"}`, http.StatusNotFound)
		return
	}

	// Forward request to target system
	// This is a simplified version - in production you'd want to use internal HTTP client
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "forwarded",
		"from":   req.From,
		"to":     req.To,
		"path":   req.Path,
	})
}

// serveSystemsList returns list of all systems
func (s *MultiSystemServer) serveSystemsList(w http.ResponseWriter, r *http.Request) {
	systems := make([]map[string]interface{}, 0, len(s.Systems))
	for name, sys := range s.Systems {
		systems = append(systems, map[string]interface{}{
			"name":        name,
			"prefix":      sys.Prefix,
			"api_name":    sys.Instance.Manifest.Metadata.Name,
			"version":     sys.Instance.Manifest.Metadata.Version,
			"description": sys.Instance.Manifest.Metadata.Description,
			"operations":  len(sys.Instance.Catalog.ListAll()),
			"entities":    len(sys.Instance.Manifest.DataModel.Entities),
			"actors":      len(sys.Instance.Manifest.Actors),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"systems": systems,
		"count":   len(systems),
	})
}

// serveGlobalHealth returns overall health status
func (s *MultiSystemServer) serveGlobalHealth(w http.ResponseWriter, r *http.Request) {
	systems := make(map[string]string)
	for name := range s.Systems {
		systems[name] = "ok"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"systems":  systems,
		"count":    len(s.Systems),
		"auth":     s.globalAuth.Enabled,
		"address":  fmt.Sprintf("http://%s:%d", s.Host, s.Port),
	})
}

// serveSystemHealth returns health for a specific system
func (s *MultiSystemServer) serveSystemHealth(w http.ResponseWriter, r *http.Request, sys *ManagedSystem) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"system":   sys.System.Name,
		"status":   "ok",
		"database": "ok",
		"mcp":      sys.MCPServer.IsInitialized(),
	})
}

// proxyToSystem forwards requests to a system
func (s *MultiSystemServer) proxyToSystem(w http.ResponseWriter, r *http.Request, sys *ManagedSystem, path string) {
	r.URL.Path = path
	r.URL.RawPath = ""
	sys.Instance.ServeHTTP(w, r)
}
