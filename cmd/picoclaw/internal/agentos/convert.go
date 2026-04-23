package agentos

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/spf13/cobra"
)

// newConvertCommand cria o comando para converter um sistema
func newConvertCommand() *cobra.Command {
	var (
		dataDir     string
		system      string
		outputDir   string
		format      string
		includeData bool
	)

	cmd := &cobra.Command{
		Use:   "convert [system]",
		Short: "Converte um sistema para Python + SvelteKit + PostgreSQL",
		Long: `Converte um sistema AgentOS para uma stack completa:
- Backend Python (FastAPI) com models SQLAlchemy
- Frontend SvelteKit com TypeScript
- Scripts de migração PostgreSQL
- Docker Compose para orquestração
- Dump opcional da base de dados SQLite para PostgreSQL`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = getDataDir(dataDir)

			// Get system name from args or flag
			if len(args) > 0 {
				system = args[0]
			}

			if system == "" {
				return fmt.Errorf("system name is required")
			}

			// Load registry
			registry, err := LoadRegistry(dataDir)
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}

			// Check if system exists
			sysInfo, err := registry.GetSystem(system)
			if err != nil {
				return fmt.Errorf("system not found: %s", system)
			}

			// Load manifest
			m, err := manifest.ParseFile(sysInfo.ManifestPath)
			if err != nil {
				return fmt.Errorf("failed to parse manifest: %w", err)
			}

			// Default output directory
			if outputDir == "" {
				outputDir = fmt.Sprintf("%s-export-%s", system, time.Now().Format("20060102-150405"))
			}

			// Create output directory
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			fmt.Printf("=== Converting System: %s ===\n", system)
			fmt.Printf("Output: %s\n\n", outputDir)

			// Convert based on format
			switch format {
			case "python-svelte":
				if err := convertPythonSvelte(m, sysInfo, outputDir, includeData); err != nil {
					return fmt.Errorf("convert failed: %w", err)
				}
			default:
				return fmt.Errorf("unsupported format: %s", format)
			}

			fmt.Printf("\nConversion completed successfully!\n")
			fmt.Printf("Output directory: %s\n", outputDir)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")
	cmd.Flags().StringVarP(&system, "system", "s", "", "System name to export")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory (default: <system>-convert-<timestamp>)")
	cmd.Flags().StringVarP(&format, "format", "f", "python-svelte", "Export format (python-svelte)")
	cmd.Flags().BoolVar(&includeData, "include-data", false, "Include database dump")

	return cmd
}

// exportPythonSvelte exports system to Python + SvelteKit + PostgreSQL
func convertPythonSvelte(m *manifest.Manifest, sysInfo *SystemInfo, outputDir string, includeData bool) error {
	// Create directory structure
	dirs := []string{
		"backend/app",
		"backend/app/models",
		"backend/app/routers",
		"backend/migrations",
		"frontend/src/lib",
		"frontend/src/routes",
		"frontend/src/components",
		"database",
		"docker",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(outputDir, dir), 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Generate backend files
	fmt.Println("Generating Python backend...")
	if err := generatePythonBackend(m, outputDir); err != nil {
		return fmt.Errorf("failed to generate Python backend: %w", err)
	}

	// Generate frontend files
	fmt.Println("Generating SvelteKit frontend...")
	if err := generateSvelteFrontend(m, outputDir); err != nil {
		return fmt.Errorf("failed to generate SvelteKit frontend: %w", err)
	}

	// Generate Docker files
	fmt.Println("Generating Docker configuration...")
	if err := generateDockerFiles(m, outputDir); err != nil {
		return fmt.Errorf("failed to generate Docker files: %w", err)
	}

	// Generate PostgreSQL migrations
	fmt.Println("Generating PostgreSQL migrations...")
	if err := generatePostgresMigrations(m, outputDir); err != nil {
		return fmt.Errorf("failed to generate PostgreSQL migrations: %w", err)
	}

	// Optional: Include data dump
	if includeData {
		fmt.Println("Dumping database...")
		if err := dumpDatabase(sysInfo.DBConnection, filepath.Join(outputDir, "database", "dump.sql")); err != nil {
			return fmt.Errorf("failed to dump database: %w", err)
		}
	}

	return nil
}

// generatePythonBackend generates FastAPI backend with SQLAlchemy models
func generatePythonBackend(m *manifest.Manifest, outputDir string) error {
	// Generate models
	modelsCode := generateSQLAlchemyModels(m)
	if err := os.WriteFile(filepath.Join(outputDir, "backend/app/models.py"), []byte(modelsCode), 0644); err != nil {
		return err
	}

	// Generate main FastAPI app
	mainCode := generateFastAPIApp(m)
	if err := os.WriteFile(filepath.Join(outputDir, "backend/app/main.py"), []byte(mainCode), 0644); err != nil {
		return err
	}

	// Generate requirements
	requirements := `fastapi==0.104.1
uvicorn[standard]==0.24.0
sqlalchemy==2.0.23
psycopg2-binary==2.9.9
pydantic==2.5.0
python-multipart==0.0.6
alembic==1.12.1
python-jose[cryptography]==3.3.0
passlib[bcrypt]==1.7.4
`
	if err := os.WriteFile(filepath.Join(outputDir, "backend/requirements.txt"), []byte(requirements), 0644); err != nil {
		return err
	}

	// Generate config
	configCode := generatePythonConfig(m)
	if err := os.WriteFile(filepath.Join(outputDir, "backend/app/config.py"), []byte(configCode), 0644); err != nil {
		return err
	}

	// Generate database module
	dbCode := generatePythonDatabase(m)
	if err := os.WriteFile(filepath.Join(outputDir, "backend/app/database.py"), []byte(dbCode), 0644); err != nil {
		return err
	}

	// Generate routers for each entity
	for _, entity := range m.DataModel.Entities {
		routerCode := generateEntityRouter(entity)
		filename := fmt.Sprintf("%s.py", toSnakeCase(entity.Name))
		if err := os.WriteFile(filepath.Join(outputDir, "backend/app/routers", filename), []byte(routerCode), 0644); err != nil {
			return err
		}
	}

	return nil
}

// generateSvelteFrontend generates SvelteKit frontend
func generateSvelteFrontend(m *manifest.Manifest, outputDir string) error {
	// Generate package.json
	packageJSON := generateSveltePackageJSON(m)
	if err := os.WriteFile(filepath.Join(outputDir, "frontend/package.json"), []byte(packageJSON), 0644); err != nil {
		return err
	}

	// Generate svelte.config.js
	svelteConfig := `import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter()
	}
};

export default config;
`
	if err := os.WriteFile(filepath.Join(outputDir, "frontend/svelte.config.js"), []byte(svelteConfig), 0644); err != nil {
		return err
	}

	// Generate app.html
	appHTML := `<!DOCTYPE html>
<html lang="en">
	<head>
		<meta charset="utf-8" />
		<link rel="icon" href="%sveltekit.assets%/favicon.png" />
		<meta name="viewport" content="width=device-width" />
		%sveltekit.head%
	</head>
	<body data-sveltekit-preload-data="hover">
		<div style="display: contents">%sveltekit.body%</div>
	</body>
</html>
`
	if err := os.WriteFile(filepath.Join(outputDir, "frontend/src/app.html"), []byte(appHTML), 0644); err != nil {
		return err
	}

	// Generate main layout
	layoutCode := generateSvelteLayout(m)
	if err := os.WriteFile(filepath.Join(outputDir, "frontend/src/routes/+layout.svelte"), []byte(layoutCode), 0644); err != nil {
		return err
	}

	// Generate home page
	homeCode := generateSvelteHomePage(m)
	if err := os.WriteFile(filepath.Join(outputDir, "frontend/src/routes/+page.svelte"), []byte(homeCode), 0644); err != nil {
		return err
	}

	// Generate entity pages
	for _, entity := range m.DataModel.Entities {
		entityDir := filepath.Join(outputDir, "frontend/src/routes", toSnakeCase(entity.Name))
		if err := os.MkdirAll(entityDir, 0755); err != nil {
			return err
		}

		// List page
		listPage := generateSvelteEntityListPage(entity)
		if err := os.WriteFile(filepath.Join(entityDir, "+page.svelte"), []byte(listPage), 0644); err != nil {
			return err
		}

		// Detail page
		detailDir := filepath.Join(entityDir, "[id]")
		if err := os.MkdirAll(detailDir, 0755); err != nil {
			return err
		}
		detailPage := generateSvelteEntityDetailPage(entity)
		if err := os.WriteFile(filepath.Join(detailDir, "+page.svelte"), []byte(detailPage), 0644); err != nil {
			return err
		}
	}

	// Generate API client
	apiClient := generateSvelteAPIClient(m)
	if err := os.WriteFile(filepath.Join(outputDir, "frontend/src/lib/api.ts"), []byte(apiClient), 0644); err != nil {
		return err
	}

	// Generate types
	typesCode := generateSvelteTypes(m)
	if err := os.WriteFile(filepath.Join(outputDir, "frontend/src/lib/types.ts"), []byte(typesCode), 0644); err != nil {
		return err
	}

	return nil
}

// generateDockerFiles generates Docker Compose configuration
func generateDockerFiles(m *manifest.Manifest, outputDir string) error {
	// docker-compose.yml
	dockerCompose := fmt.Sprintf(`version: '3.8'

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: %s
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5

  backend:
    build: ./backend
    environment:
      DATABASE_URL: postgresql://postgres:postgres@postgres:5432/%s
      SECRET_KEY: change-me-in-production
    ports:
      - "8000:8000"
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - ./backend:/app
    command: uvicorn app.main:app --host 0.0.0.0 --reload

  frontend:
    build: ./frontend
    ports:
      - "3000:3000"
    environment:
      PUBLIC_API_URL: http://localhost:8000
    depends_on:
      - backend
    volumes:
      - ./frontend:/app
      - /app/node_modules

volumes:
  postgres_data:
`, toSnakeCase(m.Metadata.Name), toSnakeCase(m.Metadata.Name))

	if err := os.WriteFile(filepath.Join(outputDir, "docker-compose.yml"), []byte(dockerCompose), 0644); err != nil {
		return err
	}

	// Backend Dockerfile
	backendDockerfile := `FROM python:3.11-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app/ ./app/

EXPOSE 8000

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
`
	if err := os.WriteFile(filepath.Join(outputDir, "backend/Dockerfile"), []byte(backendDockerfile), 0644); err != nil {
		return err
	}

	// Frontend Dockerfile
	frontendDockerfile := `FROM node:20-alpine

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

EXPOSE 3000

CMD ["node", "build"]
`
	if err := os.WriteFile(filepath.Join(outputDir, "frontend/Dockerfile"), []byte(frontendDockerfile), 0644); err != nil {
		return err
	}

	// .env file
	envFile := `# Backend
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/` + toSnakeCase(m.Metadata.Name) + `
SECRET_KEY=change-me-in-production

# Frontend
PUBLIC_API_URL=http://localhost:8000
`
	if err := os.WriteFile(filepath.Join(outputDir, ".env"), []byte(envFile), 0644); err != nil {
		return err
	}

	return nil
}

// generatePostgresMigrations generates PostgreSQL migration scripts
func generatePostgresMigrations(m *manifest.Manifest, outputDir string) error {
	// Create init script
	var migration strings.Builder

	migration.WriteString(fmt.Sprintf("-- PostgreSQL migration for %s\n", m.Metadata.Name))
	migration.WriteString(fmt.Sprintf("-- Generated at %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	migration.WriteString(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;\n\n", toSnakeCase(m.Metadata.Name)))
	migration.WriteString("\\c " + toSnakeCase(m.Metadata.Name) + ";\n\n")

	// Generate tables for each entity
	for _, entity := range m.DataModel.Entities {
		migration.WriteString(generatePostgresTable(entity))
		migration.WriteString("\n")
	}

	// Generate foreign keys
	for _, entity := range m.DataModel.Entities {
		for _, field := range entity.Fields {
			if field.Reference != nil {
				fk := fmt.Sprintf(
					"ALTER TABLE %s ADD CONSTRAINT fk_%s_%s FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE %s;\n",
					toSnakeCase(entity.Name),
					toSnakeCase(entity.Name),
					toSnakeCase(field.Name),
					toSnakeCase(field.Name),
					toSnakeCase(field.Reference.Entity),
					toSnakeCase(field.Reference.Field),
					field.Reference.OnDelete,
				)
				migration.WriteString(fk)
			}
		}
	}

	// Generate indexes
	for _, entity := range m.DataModel.Entities {
		for _, idx := range entity.Indexes {
			fields := make([]string, len(idx.Fields))
			for i, f := range idx.Fields {
				fields[i] = toSnakeCase(f)
			}
			unique := ""
			if idx.Unique {
				unique = "UNIQUE "
			}
			idxSQL := fmt.Sprintf(
				"CREATE %sINDEX %s ON %s(%s);\n",
				unique,
				idx.Name,
				toSnakeCase(entity.Name),
				strings.Join(fields, ", "),
			)
			migration.WriteString(idxSQL)
		}
	}

	if err := os.WriteFile(filepath.Join(outputDir, "database/init.sql"), []byte(migration.String()), 0644); err != nil {
		return err
	}

	return nil
}

// Helper: generate SQLAlchemy models
func generateSQLAlchemyModels(m *manifest.Manifest) string {
	var code strings.Builder

	code.WriteString("from sqlalchemy import Column, String, Integer, Float, Boolean, DateTime, ForeignKey, JSON, Text\n")
	code.WriteString("from sqlalchemy.orm import relationship, declarative_base\n")
	code.WriteString("from datetime import datetime\n\n")
	code.WriteString("Base = declarative_base()\n\n")

	// Generate models
	for _, entity := range m.DataModel.Entities {
		code.WriteString(fmt.Sprintf("class %s(Base):\n", entity.Name))
		code.WriteString(fmt.Sprintf(`    """%s"""`+"\n", entity.Description))
		code.WriteString(fmt.Sprintf("    __tablename__ = '%s'\n\n", toSnakeCase(entity.Name)))

		// Generate fields
		for _, field := range entity.Fields {
			colType := pythonTypeForField(field)
			if field.Reference != nil {
				code.WriteString(fmt.Sprintf("    %s = Column(%s)\n", toSnakeCase(field.Name), colType))
			} else {
				code.WriteString(fmt.Sprintf("    %s = Column(%s)\n", toSnakeCase(field.Name), colType))
			}
		}

		code.WriteString("\n    def to_dict(self):\n")
		code.WriteString("        return {\n")
		for _, field := range entity.Fields {
			code.WriteString(fmt.Sprintf("            '%s': self.%s,\n", toSnakeCase(field.Name), toSnakeCase(field.Name)))
		}
		code.WriteString("        }\n\n")
	}

	return code.String()
}

// Helper: generate FastAPI app
func generateFastAPIApp(m *manifest.Manifest) string {
	var code strings.Builder

	code.WriteString("from fastapi import FastAPI, Depends, HTTPException\n")
	code.WriteString("from fastapi.middleware.cors import CORSMiddleware\n")
	code.WriteString("from sqlalchemy.orm import Session\n")
	code.WriteString("from app.database import get_db, engine\n")
	code.WriteString("from app import models\n\n")

	// Import routers
	for _, entity := range m.DataModel.Entities {
		code.WriteString(fmt.Sprintf("from app.routers import %s as %s_router\n",
			toSnakeCase(entity.Name),
			toSnakeCase(entity.Name)))
	}

	code.WriteString("\n# Create tables\n")
	code.WriteString("models.Base.metadata.create_all(bind=engine)\n\n")

	code.WriteString(fmt.Sprintf(`app = FastAPI(
    title="%s",
    description="%s",
    version="%s"
)`+"\n\n", m.Metadata.Name, m.Metadata.Description, m.Metadata.Version))

	code.WriteString(`app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)` + "\n\n")

	// Include routers
	for _, entity := range m.DataModel.Entities {
		code.WriteString(fmt.Sprintf("app.include_router(%s_router.router, prefix='/api/v1/%s', tags=['%s'])\n",
			toSnakeCase(entity.Name),
			toSnakeCase(entity.Name),
			entity.Name))
	}

	code.WriteString(`
@app.get("/health")
def health_check():
    return {"status": "ok"}
`)

	return code.String()
}

// Helper: generate entity router
func generateEntityRouter(entity manifest.Entity) string {
	var code strings.Builder
	name := entity.Name
	nameLower := toSnakeCase(name)

	code.WriteString(fmt.Sprintf("from fastapi import APIRouter, Depends, HTTPException\n"))
	code.WriteString(fmt.Sprintf("from sqlalchemy.orm import Session\n"))
	code.WriteString(fmt.Sprintf("from app.database import get_db\n"))
	code.WriteString(fmt.Sprintf("from app.models import %s\n\n", name))

	code.WriteString(fmt.Sprintf("router = APIRouter()\n\n"))

	// List
	code.WriteString(fmt.Sprintf("@router.get('/')\n"))
	code.WriteString(fmt.Sprintf("def list_%s(db: Session = Depends(get_db)):\n", nameLower))
	code.WriteString(fmt.Sprintf("    return db.query(%s).all()\n\n", name))

	// Get
	code.WriteString(fmt.Sprintf("@router.get('/{item_id}')\n"))
	code.WriteString(fmt.Sprintf("def get_%s(item_id: str, db: Session = Depends(get_db)):\n", nameLower))
	code.WriteString(fmt.Sprintf("    item = db.query(%s).filter(%s.id == item_id).first()\n", name, name))
	code.WriteString(fmt.Sprintf("    if not item:\n"))
	code.WriteString(fmt.Sprintf("        raise HTTPException(status_code=404, detail='%s not found')\n", name))
	code.WriteString(fmt.Sprintf("    return item\n\n"))

	// Create
	code.WriteString(fmt.Sprintf("@router.post('/')\n"))
	code.WriteString(fmt.Sprintf("def create_%s(data: dict, db: Session = Depends(get_db)):\n", nameLower))
	code.WriteString(fmt.Sprintf("    item = %s(**data)\n", name))
	code.WriteString(fmt.Sprintf("    db.add(item)\n"))
	code.WriteString(fmt.Sprintf("    db.commit()\n"))
	code.WriteString(fmt.Sprintf("    db.refresh(item)\n"))
	code.WriteString(fmt.Sprintf("    return item\n\n"))

	// Update
	code.WriteString(fmt.Sprintf("@router.put('/{item_id}')\n"))
	code.WriteString(fmt.Sprintf("def update_%s(item_id: str, data: dict, db: Session = Depends(get_db)):\n", nameLower))
	code.WriteString(fmt.Sprintf("    item = db.query(%s).filter(%s.id == item_id).first()\n", name, name))
	code.WriteString(fmt.Sprintf("    if not item:\n"))
	code.WriteString(fmt.Sprintf("        raise HTTPException(status_code=404, detail='%s not found')\n", name))
	code.WriteString(fmt.Sprintf("    for key, value in data.items():\n"))
	code.WriteString(fmt.Sprintf("        setattr(item, key, value)\n"))
	code.WriteString(fmt.Sprintf("    db.commit()\n"))
	code.WriteString(fmt.Sprintf("    return item\n\n"))

	// Delete
	code.WriteString(fmt.Sprintf("@router.delete('/{item_id}')\n"))
	code.WriteString(fmt.Sprintf("def delete_%s(item_id: str, db: Session = Depends(get_db)):\n", nameLower))
	code.WriteString(fmt.Sprintf("    item = db.query(%s).filter(%s.id == item_id).first()\n", name, name))
	code.WriteString(fmt.Sprintf("    if not item:\n"))
	code.WriteString(fmt.Sprintf("        raise HTTPException(status_code=404, detail='%s not found')\n", name))
	code.WriteString(fmt.Sprintf("    db.delete(item)\n"))
	code.WriteString(fmt.Sprintf("    db.commit()\n"))
	code.WriteString(fmt.Sprintf("    return {'message': '%s deleted'}\n", name))

	return code.String()
}

// Helper: generate Python config
func generatePythonConfig(m *manifest.Manifest) string {
	return `from pydantic_settings import BaseSettings
from functools import lru_cache

class Settings(BaseSettings):
    DATABASE_URL: str = "postgresql://postgres:postgres@localhost:5432/` + toSnakeCase(m.Metadata.Name) + `"
    SECRET_KEY: str = "change-me-in-production"
    ACCESS_TOKEN_EXPIRE_MINUTES: int = 30
    
    class Config:
        env_file = ".env"

@lru_cache()
def get_settings():
    return Settings()
`
}

// Helper: generate Python database module
func generatePythonDatabase(m *manifest.Manifest) string {
	return `from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker, Session
from app.config import get_settings

settings = get_settings()
engine = create_engine(settings.DATABASE_URL)
SessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine)

def get_db() -> Session:
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()
`
}

// Helper: generate Svelte package.json
func generateSveltePackageJSON(m *manifest.Manifest) string {
	return `{
  "name": "` + toSnakeCase(m.Metadata.Name) + `-frontend",
  "version": "` + m.Metadata.Version + `",
  "private": true,
  "scripts": {
    "dev": "vite dev",
    "build": "vite build",
    "preview": "vite preview",
    "check": "svelte-kit sync && svelte-check --tsconfig ./tsconfig.json",
    "check:watch": "svelte-kit sync && svelte-check --tsconfig ./tsconfig.json --watch"
  },
  "devDependencies": {
    "@sveltejs/adapter-node": "^2.0.0",
    "@sveltejs/kit": "^2.0.0",
    "@sveltejs/vite-plugin-svelte": "^3.0.0",
    "svelte": "^4.0.0",
    "svelte-check": "^3.0.0",
    "typescript": "^5.0.0",
    "vite": "^5.0.0"
  },
  "type": "module",
  "dependencies": {
    "lucide-svelte": "^0.294.0"
  }
}
`
}

// Helper: generate Svelte layout
func generateSvelteLayout(m *manifest.Manifest) string {
	var navItems strings.Builder
	for _, entity := range m.DataModel.Entities {
		navItems.WriteString(fmt.Sprintf(`				<li><a href="/%s">%s</a></li>`+"\n",
			toSnakeCase(entity.Name),
			entity.Name))
	}

	return `<script>
	import { goto } from '$app/navigation';
</script>

<div class="app">
	<nav class="sidebar">
		<div class="logo">
			<h1>` + m.Metadata.Name + `</h1>
		</div>
		<ul class="nav-items">
			<li><a href="/">Home</a></li>
` + navItems.String() + `		</ul>
	</nav>
	
	<main class="content">
		<slot />
	</main>
</div>

<style>
	.app {
		display: flex;
		min-height: 100vh;
	}
	
	.sidebar {
		width: 250px;
		background: #1a1a2e;
		color: white;
		padding: 1rem;
	}
	
	.logo h1 {
		font-size: 1.5rem;
		margin-bottom: 2rem;
	}
	
	.nav-items {
		list-style: none;
		padding: 0;
	}
	
	.nav-items li {
		margin-bottom: 0.5rem;
	}
	
	.nav-items a {
		color: #a0a0a0;
		text-decoration: none;
		display: block;
		padding: 0.5rem;
		border-radius: 4px;
		transition: color 0.2s;
	}
	
	.nav-items a:hover {
		color: white;
		background: rgba(255,255,255,0.1);
	}
	
	.content {
		flex: 1;
		padding: 2rem;
		background: #f5f7fa;
	}
</style>
`
}

// Helper: generate Svelte home page
func generateSvelteHomePage(m *manifest.Manifest) string {
	var cards strings.Builder
	for _, entity := range m.DataModel.Entities {
		cards.WriteString(fmt.Sprintf(`		<a href="/%s" class="card">
			<h2>%s</h2>
			<p>%s</p>
		</a>`+"\n",
			toSnakeCase(entity.Name),
			entity.Name,
			entity.Description))
	}

	return `<div class="home">
	<h1>Welcome to ` + m.Metadata.Name + `</h1>
	<p class="description">` + m.Metadata.Description + `</p>
	
	<div class="cards">
` + cards.String() + `	</div>
</div>

<style>
	.home {
		max-width: 1200px;
		margin: 0 auto;
	}
	
	.home h1 {
		margin-bottom: 1rem;
	}
	
	.description {
		color: #666;
		margin-bottom: 2rem;
	}
	
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
		gap: 1.5rem;
	}
	
	.card {
		background: white;
		border-radius: 12px;
		padding: 1.5rem;
		box-shadow: 0 2px 8px rgba(0,0,0,0.1);
		text-decoration: none;
		color: inherit;
		transition: transform 0.2s, box-shadow 0.2s;
	}
	
	.card:hover {
		transform: translateY(-4px);
		box-shadow: 0 4px 16px rgba(0,0,0,0.15);
	}
	
	.card h2 {
		margin-bottom: 0.5rem;
		color: #333;
	}
	
	.card p {
		color: #666;
		font-size: 0.9rem;
	}
</style>
`
}

// Helper: generate Svelte entity list page
func generateSvelteEntityListPage(entity manifest.Entity) string {
	return `<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	
	let items = [];
	let loading = true;
	let error = null;
	
	onMount(async () => {
		try {
			items = await api.list('` + toSnakeCase(entity.Name) + `');
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});
</script>

<div class="entity-page">
	<div class="header">
		<h1>` + entity.Name + `</h1>
		<a href="/` + toSnakeCase(entity.Name) + `/new" class="btn-primary">New ` + entity.Name + `</a>
	</div>
	
	{#if loading}
		<p>Loading...</p>
	{:else if error}
		<p class="error">Error: {error}</p>
	{:else if items.length === 0}
		<p>No ` + entity.Name + ` found.</p>
	{:else}
		<div class="list">
			{#each items as item}
				<a href="/` + toSnakeCase(entity.Name) + `/{item.id}" class="list-item">
					<h3>{item.name || item.id}</h3>
				</a>
			{/each}
		</div>
	{/if}
</div>

<style>
	.entity-page {
		max-width: 1200px;
	}
	
	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 2rem;
	}
	
	.btn-primary {
		background: #667eea;
		color: white;
		padding: 0.75rem 1.5rem;
		border-radius: 6px;
		text-decoration: none;
	}
	
	.list {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}
	
	.list-item {
		background: white;
		padding: 1rem;
		border-radius: 8px;
		box-shadow: 0 2px 4px rgba(0,0,0,0.1);
		text-decoration: none;
		color: inherit;
	}
	
	.list-item:hover {
		background: #f8fafc;
	}
	
	.error {
		color: #ef4444;
	}
</style>
`
}

// Helper: generate Svelte entity detail page
func generateSvelteEntityDetailPage(entity manifest.Entity) string {
	return `<script>
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	
	let item = null;
	let loading = true;
	let error = null;
	
	$: id = $page.params.id;
	
	onMount(async () => {
		try {
			item = await api.get('` + toSnakeCase(entity.Name) + `', id);
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	});
	
	async function deleteItem() {
		if (!confirm('Are you sure?')) return;
		await api.delete('` + toSnakeCase(entity.Name) + `', id);
		goto('/` + toSnakeCase(entity.Name) + `');
	}
</script>

<div class="entity-page">
	{#if loading}
		<p>Loading...</p>
	{:else if error}
		<p class="error">Error: {error}</p>
	{:else}
		<div class="header">
			<h1>{item.name || item.id}</h1>
			<div class="actions">
				<button class="btn-danger" on:click={deleteItem}>Delete</button>
			</div>
		</div>
		
		<div class="details">
			<h2>Details</h2>
			<pre>{JSON.stringify(item, null, 2)}</pre>
		</div>
	{/if}
</div>

<style>
	.entity-page {
		max-width: 1200px;
	}
	
	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 2rem;
	}
	
	.btn-danger {
		background: #ef4444;
		color: white;
		padding: 0.5rem 1rem;
		border: none;
		border-radius: 6px;
		cursor: pointer;
	}
	
	.details {
		background: white;
		padding: 1.5rem;
		border-radius: 8px;
		box-shadow: 0 2px 4px rgba(0,0,0,0.1);
	}
	
	pre {
		background: #f5f5f5;
		padding: 1rem;
		border-radius: 4px;
		overflow-x: auto;
	}
	
	.error {
		color: #ef4444;
	}
</style>
`
}

// Helper: generate Svelte API client
func generateSvelteAPIClient(m *manifest.Manifest) string {
	return `const API_URL = import.meta.env.PUBLIC_API_URL || 'http://localhost:8000';

export const api = {
	async request(method, path, body) {
		const url = ` + "`${API_URL}/api/v1${path}`" + `;
		const options = {
			method,
			headers: {
				'Content-Type': 'application/json',
			},
		};
		
		if (body) {
			options.body = JSON.stringify(body);
		}
		
		const response = await fetch(url, options);
		
		if (!response.ok) {
			throw new Error(` + "`HTTP ${response.status}: ${await response.text()}`" + `);
		}
		
		return response.json();
	},
	
	list(entity) {
		return this.request('GET', ` + "`/${entity}`" + `);
	},
	
	get(entity, id) {
		return this.request('GET', ` + "`/${entity}/${id}`" + `);
	},
	
	create(entity, data) {
		return this.request('POST', ` + "`/${entity}`" + `, data);
	},
	
	update(entity, id, data) {
		return this.request('PUT', ` + "`/${entity}/${id}`" + `, data);
	},
	
	delete(entity, id) {
		return this.request('DELETE', ` + "`/${entity}/${id}`" + `);
	},
};
`
}

// Helper: generate Svelte types
func generateSvelteTypes(m *manifest.Manifest) string {
	var code strings.Builder
	code.WriteString("// Auto-generated types\n\n")

	for _, entity := range m.DataModel.Entities {
		code.WriteString(fmt.Sprintf("export interface %s {\n", entity.Name))
		for _, field := range entity.Fields {
			optional := ""
			if !field.Required {
				optional = "?"
			}
			code.WriteString(fmt.Sprintf("  %s%s: %s;\n", toSnakeCase(field.Name), optional, tsTypeForField(field)))
		}
		code.WriteString("}\n\n")
	}

	return code.String()
}

// Helper: generate PostgreSQL table
func generatePostgresTable(entity manifest.Entity) string {
	var code strings.Builder

	code.WriteString(fmt.Sprintf("-- Table: %s\n", entity.Name))
	code.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", toSnakeCase(entity.Name)))

	var columns []string
	for _, field := range entity.Fields {
		col := fmt.Sprintf("    %s %s", toSnakeCase(field.Name), postgresTypeForField(field))
		if field.Required {
			col += " NOT NULL"
		}
		if field.Unique {
			col += " UNIQUE"
		}
		if field.Default != nil {
			col += fmt.Sprintf(" DEFAULT %v", field.Default)
		}
		columns = append(columns, col)
	}

	// Add primary key if not present
	hasPK := false
	for _, field := range entity.Fields {
		if field.Name == "id" && field.Unique {
			hasPK = true
			break
		}
	}
	if hasPK {
		columns = append(columns, "    PRIMARY KEY (id)")
	}

	code.WriteString(strings.Join(columns, ",\n"))
	code.WriteString("\n);\n")

	return code.String()
}

// Helper: dump database to SQL
func dumpDatabase(dbPath, outputPath string) error {
	// For SQLite, we can read the database and generate INSERT statements
	// This is a simplified version
	return fmt.Errorf("database dump not yet implemented for SQLite")
}

// Helper: convert to snake_case
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// Helper: Python type for field
func pythonTypeForField(field manifest.Field) string {
	switch field.Type {
	case "string", "text":
		return "String"
	case "integer", "int":
		return "Integer"
	case "float", "number", "decimal":
		return "Float"
	case "boolean", "bool":
		return "Boolean"
	case "datetime":
		return "DateTime"
	case "json":
		return "JSON"
	default:
		return "String"
	}
}

// Helper: TypeScript type for field
func tsTypeForField(field manifest.Field) string {
	switch field.Type {
	case "string", "text":
		return "string"
	case "integer", "int", "float", "number", "decimal":
		return "number"
	case "boolean", "bool":
		return "boolean"
	case "datetime":
		return "string"
	case "json":
		return "any"
	default:
		return "string"
	}
}

// Helper: PostgreSQL type for field
func postgresTypeForField(field manifest.Field) string {
	switch field.Type {
	case "string":
		if field.MaxLength != nil && *field.MaxLength > 0 {
			return fmt.Sprintf("VARCHAR(%d)", *field.MaxLength)
		}
		return "TEXT"
	case "text":
		return "TEXT"
	case "integer", "int":
		return "INTEGER"
	case "float", "number", "decimal":
		return "REAL"
	case "boolean", "bool":
		return "BOOLEAN"
	case "datetime":
		return "TIMESTAMP"
	case "json":
		return "JSONB"
	default:
		return "TEXT"
	}
}
