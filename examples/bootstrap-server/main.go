package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/sipeed/picoclaw/pkg/agentos"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: bootstrap-server <manifest.yaml> [database.db]")
		fmt.Println()
		fmt.Println("Example: bootstrap-server examples/manifests/cafeteria-loyalty.yaml")
		fmt.Println("         bootstrap-server examples/manifests/parking-ticket.yaml my-system.db")
		os.Exit(1)
	}

	manifestPath := os.Args[1]

	var dbPath string
	if len(os.Args) >= 3 {
		dbPath = os.Args[2]
	}

	cfg := agentos.BootstrapConfig{
		ManifestPath: manifestPath,
		DBDriver:     "sqlite",
		DataDir:      ".",
	}

	if dbPath != "" {
		cfg.DBConnection = dbPath
	}

	bootstrapper := agentos.NewBootstrapper(cfg)

	fmt.Println("=== Bootstrapper ===")
	fmt.Printf("Manifest: %s\n", manifestPath)
	if dbPath != "" {
		fmt.Printf("Database: %s\n", dbPath)
	} else {
		fmt.Println("Database: (in-memory, no persistence)")
	}
	fmt.Println()

	instance, err := bootstrapper.Bootstrap(context.Background())
	if err != nil {
		log.Fatalf("Bootstrap failed: %v", err)
	}

	fmt.Println("=== System Initialized ===")
	fmt.Printf("Name: %s v%s\n", instance.Manifest.Metadata.Name, instance.Manifest.Metadata.Version)
	if instance.Manifest.Metadata.Description != "" {
		fmt.Printf("Description: %s\n", instance.Manifest.Metadata.Description)
	}
	fmt.Printf("Actors: %d\n", len(instance.Manifest.Actors))
	fmt.Printf("Entities: %d\n", len(instance.Manifest.DataModel.Entities))
	fmt.Printf("Business Rules: %d\n", len(instance.Manifest.BusinessRules))
	fmt.Printf("Workflows: %d\n", len(instance.Manifest.Workflows))
	fmt.Printf("Operations: %d\n", len(instance.Catalog.ListAll()))
	fmt.Println()

	actors, _ := instance.ActorStore.ListAll()
	if len(actors) > 0 {
		fmt.Println("=== Actor Credentials ===")
		for _, actor := range actors {
			fmt.Printf("Actor: %s (%s)\n", actor.ActorID, actor.ActorType)
			fmt.Printf("  Roles: %v\n", actor.Roles)
			fmt.Println()
		}
	}

	fmt.Println("=== API Endpoints ===")
	for _, op := range instance.Catalog.ListAll() {
		fmt.Printf("  %s %s - %s\n", op.Method, op.Path, op.Description)
	}
	fmt.Println()

	fmt.Println("=== System Endpoints ===")
	fmt.Println("  GET  /_system/info        - System information")
	fmt.Println("  GET  /_system/actors      - List actors")
	fmt.Println("  GET  /_system/operations  - List all operations")
	fmt.Println("  GET  /_health             - Health check")
	fmt.Println()

	if dbPath != "" {
		fmt.Println("=== Database ===")
		fmt.Printf("  Database file: %s\n", dbPath)
		fmt.Println("  Tables created automatically from manifest")
		fmt.Println()
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port
	fmt.Printf("=== Server ===\n")
	fmt.Printf("Starting server on http://localhost:%s\n", port)
	fmt.Printf("Open http://localhost:%s/_docs for Swagger UI documentation\n", port)
	fmt.Printf("Press Ctrl+C to stop\n")
	fmt.Println()

	server := &http.Server{
		Addr:    addr,
		Handler: instance,
	}

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
			log.Fatalf("Server error: %v", err)
		}
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10)
		defer cancel()
		if err := instance.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}

	fmt.Println("Server stopped gracefully")
}
