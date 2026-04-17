package policy

import (
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEngine(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Admin",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "users", Actions: []string{"read", "write", "delete"}},
				},
			},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: true,
				RoleHierarchy: []manifest.RoleHierarchy{
					{Role: "admin", Inherits: []string{"user"}},
				},
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, err := NewEngine(m)
	require.NoError(t, err)
	require.NotNil(t, engine)

	assert.Equal(t, []string{"admin"}, engine.actorRoles["admin"])
	assert.Contains(t, engine.roleHierarchy, "admin")
}

func TestGetAllRoles(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Admin",
				Roles: []string{"admin"},
			},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: false,
				RoleHierarchy: []manifest.RoleHierarchy{
					{Role: "admin", Inherits: []string{"user", "viewer"}},
					{Role: "user", Inherits: []string{"viewer"}},
				},
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, _ := NewEngine(m)
	
	roles := engine.GetAllRoles("admin")
	assert.Len(t, roles, 3) // admin, user, viewer
	assert.Contains(t, roles, "admin")
	assert.Contains(t, roles, "user")
	assert.Contains(t, roles, "viewer")
}

func TestCheckPermission_Allow(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []manifest.Actor{
			{
				ID:    "user1",
				Name:  "User 1",
				Roles: []string{"user"},
				Permissions: []manifest.Permission{
					{Resource: "orders", Actions: []string{"read", "write"}},
				},
			},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: true,
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, _ := NewEngine(m)
	
	ctx := &Context{
		ActorID:  "user1",
		Resource: "orders",
		Action:   "read",
		Time:     time.Now(),
	}

	result := engine.CheckPermission(ctx)
	assert.True(t, result.Allowed)
	assert.False(t, result.Denied)
	assert.Contains(t, result.Reason, "permission granted")
}

func TestCheckPermission_Deny(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []manifest.Actor{
			{
				ID:    "user1",
				Name:  "User 1",
				Roles: []string{"user"},
				Permissions: []manifest.Permission{
					{Resource: "orders", Actions: []string{"read"}},
				},
			},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: true,
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, _ := NewEngine(m)
	
	ctx := &Context{
		ActorID:  "user1",
		Resource: "orders",
		Action:   "delete", // Não permitido
		Time:     time.Now(),
	}

	result := engine.CheckPermission(ctx)
	assert.False(t, result.Allowed)
	assert.True(t, result.Denied)
	assert.Contains(t, result.Reason, "denied")
}

func TestCheckPermission_Wildcard(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []manifest.Actor{
			{
				ID:    "admin",
				Name:  "Admin",
				Roles: []string{"admin"},
				Permissions: []manifest.Permission{
					{Resource: "*", Actions: []string{"*"}},
				},
			},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: true,
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, _ := NewEngine(m)
	
	ctx := &Context{
		ActorID:  "admin",
		Resource: "any_resource",
		Action:   "any_action",
		Time:     time.Now(),
	}

	result := engine.CheckPermission(ctx)
	assert.True(t, result.Allowed)
}

func TestCheckPermission_Condition(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []manifest.Actor{
			{
				ID:    "user1",
				Name:  "User 1",
				Roles: []string{"user"},
				Permissions: []manifest.Permission{
					{
						Resource:  "orders",
						Actions:   []string{"read", "write"},
						Condition: "owner == self",
					},
				},
			},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: true,
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, _ := NewEngine(m)
	
	// Teste com owner correto
	ctx := &Context{
		ActorID:  "user1",
		Resource: "orders",
		Action:   "read",
		Attributes: map[string]interface{}{
			"owner": "user1",
		},
		Time: time.Now(),
	}

	result := engine.CheckPermission(ctx)
	assert.True(t, result.Allowed)
	assert.Equal(t, "owner == self", result.Condition)
	
	// Teste com owner incorreto
	ctx.Attributes["owner"] = "user2"
	result = engine.CheckPermission(ctx)
	assert.False(t, result.Allowed)
	assert.True(t, result.Denied)
}

func TestCheckAccess_ContextConditions(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []manifest.Actor{
			{
				ID:    "user1",
				Name:  "User 1",
				Roles: []string{"user"},
				Permissions: []manifest.Permission{
					{Resource: "orders", Actions: []string{"read"}},
				},
			},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: true,
				ContextConditions: []manifest.ContextCondition{
					{
						Name:       "business_hours",
						Expression: "hour >= 8 && hour <= 18",
						Message:    "Acesso permitido apenas em horário comercial",
					},
				},
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, _ := NewEngine(m)
	
	// Teste dentro do horário comercial (10h)
	ctx := &Context{
		ActorID:  "user1",
		Resource: "orders",
		Action:   "read",
		Time:     time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
	}

	result := engine.CheckAccess(ctx)
	assert.True(t, result.Allowed)
	
	// Teste fora do horário comercial (20h)
	ctx.Time = time.Date(2024, 1, 1, 20, 0, 0, 0, time.UTC)
	result = engine.CheckAccess(ctx)
	assert.False(t, result.Allowed)
	assert.True(t, result.Denied)
	assert.Contains(t, result.Reason, "horário comercial")
}

func TestValidateManifest_Valid(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []manifest.Actor{
			{ID: "user", Name: "User", Roles: []string{"user"}},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: true,
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	err := ValidateManifest(m)
	assert.NoError(t, err)
}

func TestValidateManifest_InvalidModel(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model: "invalid",
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	err := ValidateManifest(m)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid authorization model")
}

func TestValidateManifest_NoAuthMethods(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model: "rbac",
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{},
			},
		},
	}

	err := ValidateManifest(m)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication method is required")
}

func TestValidateManifest_CycleInHierarchy(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: true,
				RoleHierarchy: []manifest.RoleHierarchy{
					{Role: "admin", Inherits: []string{"user"}},
					{Role: "user", Inherits: []string{"admin"}}, // Ciclo!
				},
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	err := ValidateManifest(m)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle detected")
}

func TestMatchesResource(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model: "rbac",
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, _ := NewEngine(m)
	
	assert.True(t, engine.matchesResource("*", "anything"))
	assert.True(t, engine.matchesResource("users*", "users"))
	assert.True(t, engine.matchesResource("users*", "users_list"))
	assert.False(t, engine.matchesResource("users*", "orders"))
	assert.True(t, engine.matchesResource("users", "users"))
	assert.False(t, engine.matchesResource("users", "orders"))
}

func TestMatchesAction(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model: "rbac",
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, _ := NewEngine(m)
	
	assert.True(t, engine.matchesAction([]string{"*"}, "anything"))
	assert.True(t, engine.matchesAction([]string{"read", "write"}, "read"))
	assert.True(t, engine.matchesAction([]string{"read", "write"}, "write"))
	assert.False(t, engine.matchesAction([]string{"read"}, "delete"))
}

func TestDefaultAllow(t *testing.T) {
	m := &manifest.Manifest{
		Metadata: manifest.Metadata{Name: "Test", Version: "1.0.0"},
		Actors: []manifest.Actor{
			{
				ID:    "user1",
				Name:  "User 1",
				Roles: []string{"user"},
			},
		},
		Security: manifest.SecurityPolicy{
			Authorization: manifest.AuthorizationPolicy{
				Model:       "rbac",
				DefaultDeny: false, // Allow by default
			},
			Authentication: manifest.AuthenticationPolicy{
				Methods: []string{"jwt"},
			},
		},
	}

	engine, _ := NewEngine(m)
	
	ctx := &Context{
		ActorID:  "user1",
		Resource: "unknown_resource",
		Action:   "unknown_action",
		Time:     time.Now(),
	}

	result := engine.CheckPermission(ctx)
	assert.True(t, result.Allowed)
	assert.False(t, result.Denied)
	assert.Contains(t, result.Reason, "allowed by default")
}
