package auth

import (
	"context"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.providers == nil {
		t.Error("providers map is nil")
	}
	if m.sessions == nil {
		t.Error("sessions map is nil")
	}
}

func TestManager_Register(t *testing.T) {
	m := NewManager()

	// Create a mock provider
	provider := &MockProvider{
		name:       "mock",
		ptype:      ProviderTypeBasic,
		configured: true,
	}

	err := m.Register(provider)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Try to register nil provider
	err = m.Register(nil)
	if err == nil {
		t.Error("Register should fail with nil provider")
	}

	// Try to register unconfigured provider
	unconfiguredProvider := &MockProvider{
		name:       "unconfigured",
		ptype:      ProviderTypeBasic,
		configured: false,
	}
	err = m.Register(unconfiguredProvider)
	if err == nil {
		t.Error("Register should fail with unconfigured provider")
	}
}

func TestManager_Get(t *testing.T) {
	m := NewManager()

	// Create and register a provider
	provider := &MockProvider{
		name:       "mock",
		ptype:      ProviderTypeBasic,
		configured: true,
	}
	m.Register(provider)

	// Get provider
	p, err := m.Get(ProviderTypeBasic)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if p == nil {
		t.Error("Get returned nil provider")
	}
	if p.Name() != "mock" {
		t.Errorf("Expected provider name 'mock', got '%s'", p.Name())
	}

	// Get non-existent provider
	_, err = m.Get(ProviderTypeOIDC)
	if err == nil {
		t.Error("Get should fail for non-existent provider")
	}
}

func TestManager_GetAll(t *testing.T) {
	m := NewManager()

	// Register multiple providers
	providers := []Provider{
		&MockProvider{name: "basic", ptype: ProviderTypeBasic, configured: true},
		&MockProvider{name: "oidc", ptype: ProviderTypeOIDC, configured: true},
		&MockProvider{name: "saml", ptype: ProviderTypeSAML, configured: true},
	}

	for _, p := range providers {
		m.Register(p)
	}

	all := m.GetAll()
	if len(all) != len(providers) {
		t.Errorf("Expected %d providers, got %d", len(providers), len(all))
	}
}

func TestManager_Authenticate(t *testing.T) {
	m := NewManager()

	// Create and register a provider
	provider := &MockProvider{
		name:       "mock",
		ptype:      ProviderTypeBasic,
		configured: true,
		identity: &Identity{
			ID:      "user123",
			Subject: "user123",
			Email:   "user@example.com",
			Name:    "Test User",
		},
	}
	m.Register(provider)

	// Authenticate
	creds := Credentials{
		Username: "testuser",
		Password: "password",
	}
	identity, err := m.Authenticate(context.Background(), ProviderTypeBasic, creds)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if identity == nil {
		t.Error("Authenticate returned nil identity")
	}
	if identity.ID != "user123" {
		t.Errorf("Expected ID 'user123', got '%s'", identity.ID)
	}
}

func TestManager_CreateSession(t *testing.T) {
	m := NewManager()

	identity := &Identity{
		ID:      "user123",
		Subject: "user123",
		Email:   "user@example.com",
	}

	tokens := &TokenPair{
		AccessToken:  "access_token_123",
		RefreshToken: "refresh_token_123",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	session := m.CreateSession(identity, tokens, "192.168.1.1", "Mozilla/5.0")

	if session == nil {
		t.Fatal("CreateSession returned nil")
	}
	if session.ID == "" {
		t.Error("Session ID is empty")
	}
	if session.IdentityID != identity.ID {
		t.Errorf("Expected IdentityID '%s', got '%s'", identity.ID, session.IdentityID)
	}
	if session.IPAddress != "192.168.1.1" {
		t.Errorf("Expected IP '192.168.1.1', got '%s'", session.IPAddress)
	}
	if session.UserAgent != "Mozilla/5.0" {
		t.Errorf("Expected UserAgent 'Mozilla/5.0', got '%s'", session.UserAgent)
	}

	// Get session
	retrieved, err := m.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved.ID != session.ID {
		t.Errorf("Expected session ID '%s', got '%s'", session.ID, retrieved.ID)
	}
}

func TestManager_GetSession_Expired(t *testing.T) {
	m := &Manager{
		providers:      make(map[ProviderType]Provider),
		sessions:       make(map[string]*Session),
		sessionTimeout: 1 * time.Millisecond,
	}

	identity := &Identity{ID: "user123"}
	session := m.CreateSession(identity, nil, "", "")

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	_, err := m.GetSession(session.ID)
	if err == nil {
		t.Error("GetSession should fail for expired session")
	}
}

func TestManager_InvalidateSession(t *testing.T) {
	m := NewManager()

	identity := &Identity{ID: "user123"}
	session := m.CreateSession(identity, nil, "", "")

	err := m.InvalidateSession(session.ID)
	if err != nil {
		t.Fatalf("InvalidateSession failed: %v", err)
	}

	_, err = m.GetSession(session.ID)
	if err == nil {
		t.Error("GetSession should fail after invalidation")
	}
}

func TestIdentity_HasRole(t *testing.T) {
	identity := &Identity{
		Roles: []string{"admin", "user", "editor"},
	}

	if !identity.HasRole("admin") {
		t.Error("HasRole should return true for 'admin'")
	}
	if !identity.HasRole("user") {
		t.Error("HasRole should return true for 'user'")
	}
	if identity.HasRole("superadmin") {
		t.Error("HasRole should return false for 'superadmin'")
	}
}

func TestIdentity_HasPermission(t *testing.T) {
	identity := &Identity{
		Permissions: []string{"read", "write", "delete"},
	}

	if !identity.HasPermission("read") {
		t.Error("HasPermission should return true for 'read'")
	}
	if !identity.HasPermission("write") {
		t.Error("HasPermission should return true for 'write'")
	}
	if identity.HasPermission("execute") {
		t.Error("HasPermission should return false for 'execute'")
	}
}

func TestIdentity_ToJSON(t *testing.T) {
	identity := &Identity{
		ID:      "user123",
		Subject: "user123",
		Email:   "user@example.com",
		Name:    "Test User",
		Roles:   []string{"user"},
	}

	json, err := identity.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	if len(json) == 0 {
		t.Error("ToJSON returned empty result")
	}

	// Check that JSON contains expected fields
	if !contains(string(json), "user123") {
		t.Error("JSON should contain 'user123'")
	}
	if !contains(string(json), "user@example.com") {
		t.Error("JSON should contain 'user@example.com'")
	}
}

func TestGetIdentityFromContext(t *testing.T) {
	identity := &Identity{
		ID:      "user123",
		Subject: "user123",
	}

	ctx := context.WithValue(context.Background(), IdentityContextKey, identity)

	retrieved, ok := GetIdentityFromContext(ctx)
	if !ok {
		t.Error("GetIdentityFromContext should return true")
	}
	if retrieved == nil {
		t.Fatal("GetIdentityFromContext returned nil")
	}
	if retrieved.ID != identity.ID {
		t.Errorf("Expected ID '%s', got '%s'", identity.ID, retrieved.ID)
	}

	// Test with no identity in context
	ctx = context.Background()
	_, ok = GetIdentityFromContext(ctx)
	if ok {
		t.Error("GetIdentityFromContext should return false when no identity")
	}
}

func TestProviderType_String(t *testing.T) {
	tests := []struct {
		provider ProviderType
		expected string
	}{
		{ProviderTypeOAuth2, "oauth2"},
		{ProviderTypeOIDC, "oidc"},
		{ProviderTypeBasic, "basic"},
		{ProviderTypeSAML, "saml"},
		{ProviderTypeLDAP, "ldap"},
		{ProviderTypeSocial, "social"},
	}

	for _, test := range tests {
		if string(test.provider) != test.expected {
			t.Errorf("Expected '%s', got '%s'", test.expected, string(test.provider))
		}
	}
}

// MockProvider is a mock implementation for testing
type MockProvider struct {
	name       string
	ptype      ProviderType
	configured bool
	identity   *Identity
	tokenPair  *TokenPair
}

func (m *MockProvider) Type() ProviderType {
	return m.ptype
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) Configure(config ProviderConfig) error {
	return nil
}

func (m *MockProvider) IsConfigured() bool {
	return m.configured
}

func (m *MockProvider) Authenticate(ctx context.Context, credentials Credentials) (*Identity, error) {
	return m.identity, nil
}

func (m *MockProvider) Authorize(ctx context.Context, identity *Identity, resource string, action string) (bool, error) {
	return true, nil
}

func (m *MockProvider) GetLoginURL(state string) (string, error) {
	return "https://example.com/login?state=" + state, nil
}

func (m *MockProvider) HandleCallback(ctx context.Context, code string, state string) (*Identity, error) {
	return m.identity, nil
}

func (m *MockProvider) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return m.tokenPair, nil
}

func (m *MockProvider) Logout(ctx context.Context, token string) error {
	return nil
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	if start >= len(s) {
		return false
	}
	if start+len(substr) > len(s) {
		return false
	}
	return s[start:start+len(substr)] == substr || containsAt(s, substr, start+1)
}
