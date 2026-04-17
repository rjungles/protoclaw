package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"github.com/sipeed/picoclaw/pkg/api"
	"github.com/sipeed/picoclaw/pkg/infra/db"
	"github.com/sipeed/picoclaw/pkg/manifest"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <manifest.yaml> [database.db]")
		os.Exit(1)
	}

	manifestPath := os.Args[1]
	m, err := manifest.ParseFile(manifestPath)
	if err != nil {
		fmt.Printf("Error loading manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded manifest: %s v%s\n", m.Metadata.Name, m.Metadata.Version)

	parser := &manifest.Parser{}
	if err := parser.Validate(m); err != nil {
		fmt.Printf("Validation error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Manifest validated successfully")

	var gen *api.Generator
	var database *sql.DB

	if len(os.Args) >= 3 {
		dbPath := os.Args[2]
		database, err = sql.Open("sqlite", dbPath)
		if err != nil {
			fmt.Printf("Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		migrator := db.NewMigrator(db.NewSQLDB(database), m)
		if err := migrator.Migrate(nil); err != nil {
			fmt.Printf("Migration error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Database migrated: %s\n", dbPath)

		gen, err = api.NewGeneratorWithDB(m, database)
		if err != nil {
			fmt.Printf("Error creating API generator: %v\n", err)
			os.Exit(1)
		}
	} else {
		gen, err = api.NewGenerator(m)
		if err != nil {
			fmt.Printf("Error creating API generator: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Running without database (endpoints will return placeholder responses)")
	}

	mux, err := gen.BuildMux()
	if err != nil {
		fmt.Printf("Error building mux: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== API Endpoints ===")
	for _, apiConfig := range m.Integrations.APIs {
		fmt.Printf("\nAPI: %s (%s)\n", apiConfig.Name, apiConfig.BasePath)
		for _, ep := range apiConfig.Endpoints {
			perms := ""
			if len(ep.Permissions) > 0 {
				perms = fmt.Sprintf(" [perms: %v]", ep.Permissions)
			}
			fmt.Printf("  %s %s - %s%s\n", ep.Method, ep.Path, ep.Description, perms)
		}
	}

	fmt.Println("\n=== Documentation ===")
	fmt.Println("  GET  /_manifest     - Returns the manifest JSON")
	fmt.Println("  GET  /_health       - Health check")
	fmt.Println("  GET  /_openapi.json - OpenAPI 3.0 specification")
	fmt.Println("  GET  /_docs         - Swagger UI documentation")

	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}
	addr := ":" + port
	fmt.Printf("\nStarting server on %s\n", addr)
	fmt.Println("Open http://localhost:" + port + "/_docs for API documentation")

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
	}
}
