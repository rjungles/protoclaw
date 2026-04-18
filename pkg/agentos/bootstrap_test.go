package agentos

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/manifest"
)

func TestOperationCatalog_NewCatalog(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "TestSystem",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "User",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "name", Type: "string", Required: true},
						{Name: "email", Type: "string", Required: true},
					},
				},
				{
					Name: "Task",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "title", Type: "string", Required: true},
						{Name: "status", Type: "string", Required: false},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Administrator",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "User", Actions: []string{"read", "write", "delete"}},
					{Resource: "Task", Actions: []string{"read", "write", "delete"}},
				},
			},
			{
				ID:    "member",
				Name:  "Member",
				Roles: []string{"member"},
				Permissions: []manifest.Permission{
					{Resource: "Task", Actions: []string{"read", "write"}},
				},
			},
		},
	}

	catalog := NewCatalog(m)

	operations := catalog.ListAll()
	if len(operations) == 0 {
		t.Error("expected operations to be generated")
	}

	userOps := catalog.ListByEntity("User")
	if len(userOps) == 0 {
		t.Error("expected User operations to be generated")
	}

	expectedCRUD := []string{"list", "get", "create", "update", "delete"}
	for _, action := range expectedCRUD {
		op := catalog.Get("User." + action)
		if op == nil {
			t.Errorf("expected User.%s operation", action)
		}
	}

	t.Logf("Generated %d operations", len(operations))
	for _, op := range operations {
		t.Logf("  - %s: %s %s", op.Name, op.Method, op.Path)
	}
}

func TestOperationCatalog_GetByEntity(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "TestSystem",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Product",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "name", Type: "string", Required: true},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Administrator",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "Product", Actions: []string{"read", "write"}},
				},
			},
		},
	}

	catalog := NewCatalog(m)

	products := catalog.ListByEntity("Product")
	if len(products) != 5 {
		t.Errorf("expected 5 Product operations, got %d", len(products))
	}

	nonexistent := catalog.ListByEntity("NonExistent")
	if len(nonexistent) != 0 {
		t.Errorf("expected 0 operations for NonExistent entity, got %d", len(nonexistent))
	}
}

func TestOperationCatalog_Get(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "TestSystem",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Order",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
					},
				},
			},
		},
	}

	catalog := NewCatalog(m)

	op := catalog.Get("Order.list")
	if op == nil {
		t.Error("expected Order.list operation")
	}

	op = catalog.Get("Order.create")
	if op == nil {
		t.Error("expected Order.create operation")
	}

	op = catalog.Get("Order.nonexistent")
	if op != nil {
		t.Error("expected nil for nonexistent operation")
	}
}

func TestActorStore_MemoryStore(t *testing.T) {
	store := NewMemoryActorStore()

	actor := manifest.Actor{
		ID:    "test_actor",
		Name:  "Test Actor",
		Roles: []string{"member", "editor"},
	}

	cred, err := store.Provision(actor)
	if err != nil {
		t.Fatalf("failed to provision actor: %v", err)
	}

	if cred.ActorID != "test_actor" {
		t.Errorf("expected actor_id 'test_actor', got '%s'", cred.ActorID)
	}

	if cred.APIKey == "" {
		t.Error("expected API key to be generated")
	}

	retrieved, err := store.GetByID("test_actor")
	if err != nil {
		t.Fatalf("failed to get actor by ID: %v", err)
	}
	if retrieved.ActorID != "test_actor" {
		t.Errorf("expected actor_id 'test_actor', got '%s'", retrieved.ActorID)
	}

	byKey, err := store.GetByAPIKey(cred.APIKey)
	if err != nil {
		t.Fatalf("failed to get actor by API key: %v", err)
	}
	if byKey.ActorID != "test_actor" {
		t.Errorf("expected actor_id 'test_actor', got '%s'", byKey.ActorID)
	}

	all, err := store.ListAll()
	if err != nil {
		t.Fatalf("failed to list actors: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 actor, got %d", len(all))
	}
}

func TestActorStore_ProvisionDuplicate(t *testing.T) {
	store := NewMemoryActorStore()

	actor := manifest.Actor{
		ID:    "duplicate_actor",
		Name:  "Duplicate Actor",
		Roles: []string{"member"},
	}

	_, err := store.Provision(actor)
	if err != nil {
		t.Fatalf("failed to provision first actor: %v", err)
	}

	_, err = store.Provision(actor)
	if err == nil {
		t.Error("expected error when provisioning duplicate actor")
	}
}

func TestActorStore_GetByInvalidAPIKey(t *testing.T) {
	store := NewMemoryActorStore()

	_, err := store.GetByAPIKey("invalid-key")
	if err == nil {
		t.Error("expected error when getting by invalid API key")
	}

	_, err = store.GetByAPIKey("")
	if err == nil {
		t.Error("expected error when getting by empty API key")
	}
}

func TestActorStore_Deactivate(t *testing.T) {
	store := NewMemoryActorStore()

	actor := manifest.Actor{
		ID:    "deactivate_actor",
		Name:  "Deactivate Actor",
		Roles: []string{"member"},
	}

	cred, err := store.Provision(actor)
	if err != nil {
		t.Fatalf("failed to provision actor: %v", err)
	}

	err = store.Deactivate("deactivate_actor")
	if err != nil {
		t.Fatalf("failed to deactivate actor: %v", err)
	}

	_, err = store.GetByAPIKey(cred.APIKey)
	if err == nil {
		t.Error("expected error when getting deactivated actor by API key")
	}
}

func TestAuthMiddleware_ExtractActorID(t *testing.T) {
	store := NewMemoryActorStore()

	actor := manifest.Actor{
		ID:    "middleware_test",
		Name:  "Middleware Test",
		Roles: []string{"member"},
	}

	cred, err := store.Provision(actor)
	if err != nil {
		t.Fatalf("failed to provision actor: %v", err)
	}

	middleware := NewAuthMiddleware(store)

	t.Run("X-Actor-ID header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Actor-ID", "middleware_test")
		actorID := middleware.authenticate(req)
		if actorID != "middleware_test" {
			t.Errorf("expected 'middleware_test', got '%s'", actorID)
		}
	})

	t.Run("Bearer token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+cred.APIKey)
		actorID := middleware.authenticate(req)
		if actorID != "middleware_test" {
			t.Errorf("expected 'middleware_test', got '%s'", actorID)
		}
	})

	t.Run("X-API-Key header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-API-Key", cred.APIKey)
		actorID := middleware.authenticate(req)
		if actorID != "middleware_test" {
			t.Errorf("expected 'middleware_test', got '%s'", actorID)
		}
	})

	t.Run("Anonymous fallback", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		actorID := middleware.authenticate(req)
		if actorID != "anonymous" {
			t.Errorf("expected 'anonymous', got '%s'", actorID)
		}
	})
}

func TestBootstrapper_ValidateManifest(t *testing.T) {
	t.Run("Valid manifest", func(t *testing.T) {
		instance := &SystemInstance{
			Manifest: &manifest.Manifest{
				Metadata: manifest.Metadata{
					Name:    "Test",
					Version: "1.0.0",
				},
				Actors: []manifest.Actor{
					{ID: "test", Name: "Test"},
				},
			},
		}

		b := &Bootstrapper{}
		err := b.validateManifest(instance)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("Missing actors", func(t *testing.T) {
		instance := &SystemInstance{
			Manifest: &manifest.Manifest{
				Metadata: manifest.Metadata{
					Name:    "Test",
					Version: "1.0.0",
				},
				Actors: []manifest.Actor{},
			},
		}

		b := &Bootstrapper{}
		err := b.validateManifest(instance)
		if err == nil {
			t.Error("expected error for manifest without actors")
		}
	})
}

func TestBootstrapper_Bootstrap_Minimal(t *testing.T) {
	tmpDir := t.TempDir()

	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:        "MinimalTest",
			Version:     "1.0.0",
			Description: "A minimal test system",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Item",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "name", Type: "string", Required: true},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Administrator",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "Item", Actions: []string{"*"}},
				},
			},
		},
		Integrations: manifest.Integrations{
			APIs: []manifest.APIConfig{
				{
					Name:     "items_api",
					BasePath: "/api/v1/items",
					Endpoints: []manifest.Endpoint{
						{Path: "/", Method: "GET", Description: "List items"},
						{Path: "/", Method: "POST", Description: "Create item"},
					},
				},
			},
		},
	}

	b := &Bootstrapper{
		config: BootstrapConfig{
			Manifest:     m,
			DBDriver:     "sqlite",
			DBConnection: filepath.Join(tmpDir, "test.db"),
			SkipMigrate:  false,
		},
	}

	instance, err := b.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	if instance.Manifest == nil {
		t.Error("expected Manifest to be set")
	}

	if instance.ActorStore == nil {
		t.Error("expected ActorStore to be set")
	}

	if instance.Catalog == nil {
		t.Error("expected OperationCatalog to be set")
	}

	if instance.PolicyEngine == nil {
		t.Error("expected PolicyEngine to be set")
	}

	actors, err := instance.ActorStore.ListAll()
	if err != nil {
		t.Fatalf("failed to list actors: %v", err)
	}
	if len(actors) != 1 {
		t.Errorf("expected 1 actor, got %d", len(actors))
	}

	t.Logf("Bootstrap successful: %s v%s", instance.Manifest.Metadata.Name, instance.Manifest.Metadata.Version)
}

func TestBootstrapper_Bootstrap_WithWorkflow(t *testing.T) {
	tmpDir := t.TempDir()

	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "WorkflowTest",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Document",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
						{Name: "state", Type: "string", Required: true},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{
				ID:    "author",
				Name:  "Author",
				Roles: []string{"creator"},
				Permissions: []manifest.Permission{
					{Resource: "Document", Actions: []string{"read", "write"}},
				},
			},
			{
				ID:    "editor",
				Name:  "Editor",
				Roles: []string{"reviewer"},
				Permissions: []manifest.Permission{
					{Resource: "Document", Actions: []string{"read", "write"}},
				},
			},
		},
		Workflows: []manifest.WorkflowConfig{
			{
				Entity:       "Document",
				InitialState: "draft",
				States: []manifest.WorkflowState{
					{
						ID:          "draft",
						Description: "Document in draft state",
						Transitions: []manifest.WorkflowTransition{
							{To: "review", Action: "submit", AllowedRoles: []string{"creator"}},
						},
					},
					{
						ID:          "review",
						Description: "Document in review",
						Transitions: []manifest.WorkflowTransition{
							{To: "published", Action: "approve", AllowedRoles: []string{"reviewer"}},
							{To: "draft", Action: "reject", AllowedRoles: []string{"reviewer"}},
						},
					},
					{
						ID:          "published",
						Description: "Document published",
						Transitions: []manifest.WorkflowTransition{},
					},
				},
			},
		},
	}

	b := &Bootstrapper{
		config: BootstrapConfig{
			Manifest:     m,
			DBDriver:     "sqlite",
			DBConnection: filepath.Join(tmpDir, "workflow_test.db"),
			SkipMigrate:  false,
		},
	}

	instance, err := b.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	if len(instance.WorkflowEngines) != 1 {
		t.Errorf("expected 1 workflow engine, got %d", len(instance.WorkflowEngines))
	}

	fsm := instance.WorkflowEngines["Document"]
	if fsm == nil {
		t.Error("expected Document workflow engine")
	}

	ops := instance.Catalog.ListAll()
	hasTransition := false
	for _, op := range ops {
		if op.Action == "transition" {
			hasTransition = true
			break
		}
	}
	if !hasTransition {
		t.Error("expected transition operations to be generated")
	}
}

func TestSystemInstance_HTTPEndpoints(t *testing.T) {
	tmpDir := t.TempDir()

	m := &manifest.Manifest{
		Metadata: manifest.Metadata{
			Name:    "HTTPTest",
			Version: "1.0.0",
		},
		DataModel: manifest.DataModel{
			Entities: []manifest.Entity{
				{
					Name: "Resource",
					Fields: []manifest.Field{
						{Name: "id", Type: "string", Required: true},
					},
				},
			},
		},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Admin",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "Resource", Actions: []string{"*"}},
				},
			},
		},
		Integrations: manifest.Integrations{
			APIs: []manifest.APIConfig{
				{
					Name:     "resources_api",
					BasePath: "/api/v1/resources",
					Endpoints: []manifest.Endpoint{
						{Path: "/", Method: "GET", Description: "List resources"},
					},
				},
			},
		},
	}

	b := &Bootstrapper{
		config: BootstrapConfig{
			Manifest:     m,
			DBDriver:     "sqlite",
			DBConnection: filepath.Join(tmpDir, "http_test.db"),
			SkipMigrate:  false,
		},
	}

	instance, err := b.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	t.Run("System info endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_system/info", nil)
		rr := httptest.NewRecorder()
		instance.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("System actors endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_system/actors", nil)
		rr := httptest.NewRecorder()
		instance.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("System operations endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_system/operations", nil)
		rr := httptest.NewRecorder()
		instance.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("Health endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_health", nil)
		rr := httptest.NewRecorder()
		instance.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("Shutdown", func(t *testing.T) {
		err := instance.Shutdown(context.Background())
		if err != nil {
			t.Errorf("unexpected error on shutdown: %v", err)
		}
	})
}
