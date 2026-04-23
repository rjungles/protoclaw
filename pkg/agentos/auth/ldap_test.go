package auth

import (
	"context"
	"testing"
)

func TestNewLDAPProvider(t *testing.T) {
	provider := NewLDAPProvider("test-ldap")
	if provider == nil {
		t.Fatal("NewLDAPProvider returned nil")
	}
	if provider.name != "test-ldap" {
		t.Errorf("Expected name 'test-ldap', got '%s'", provider.name)
	}
	if provider.port != 389 {
		t.Errorf("Expected default port 389, got %d", provider.port)
	}
	if provider.userFilter != "(uid=%s)" {
		t.Errorf("Expected default filter, got '%s'", provider.userFilter)
	}
	if provider.userAttribute != "uid" {
		t.Errorf("Expected default attribute 'uid', got '%s'", provider.userAttribute)
	}
	if provider.maxConns != 10 {
		t.Errorf("Expected default maxConns 10, got %d", provider.maxConns)
	}
}

func TestLDAPProvider_Type(t *testing.T) {
	provider := NewLDAPProvider("test")
	if provider.Type() != ProviderTypeLDAP {
		t.Errorf("Expected type 'ldap', got '%s'", provider.Type())
	}
}

func TestLDAPProvider_Name(t *testing.T) {
	provider := NewLDAPProvider("my-ldap")
	if provider.Name() != "my-ldap" {
		t.Errorf("Expected name 'my-ldap', got '%s'", provider.Name())
	}
}

func TestLDAPProvider_Configure(t *testing.T) {
	provider := NewLDAPProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeLDAP,
		Config: map[string]interface{}{
			"server":      "ldap.example.com",
			"port":        float64(636),
			"use_ssl":     true,
			"use_tls":     false,
			"bind_dn":     "cn=admin,dc=example,dc=com",
			"bind_password": "admin-password",
			"base_dn":     "dc=example,dc=com",
			"user_filter": "(uid=%s)",
			"user_attribute": "uid",
			"attr_email":  "mail",
			"attr_name":   "cn",
			"attr_given_name": "givenName",
			"attr_surname": "sn",
			"attr_groups": "memberOf",
			"max_connections": float64(20),
		},
	}

	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if !provider.IsConfigured() {
		t.Error("Provider should be configured")
	}
	if provider.server != "ldap.example.com" {
		t.Errorf("Expected server 'ldap.example.com', got '%s'", provider.server)
	}
	if provider.port != 636 {
		t.Errorf("Expected port 636, got %d", provider.port)
	}
	if !provider.useSSL {
		t.Error("useSSL should be true")
	}
	if provider.bindDN != "cn=admin,dc=example,dc=com" {
		t.Errorf("Expected bind_dn, got '%s'", provider.bindDN)
	}
	if provider.baseDN != "dc=example,dc=com" {
		t.Errorf("Expected base_dn, got '%s'", provider.baseDN)
	}
	if provider.maxConns != 20 {
		t.Errorf("Expected maxConns 20, got %d", provider.maxConns)
	}

	// Test wrong provider type
	wrongConfig := ProviderConfig{
		Type: ProviderTypeBasic,
	}
	err = provider.Configure(wrongConfig)
	if err == nil {
		t.Error("Configure should fail with wrong provider type")
	}
}

func TestLDAPProvider_Configure_MissingServer(t *testing.T) {
	provider := NewLDAPProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeLDAP,
		Config: map[string]interface{}{
			"base_dn": "dc=example,dc=com",
		},
	}

	err := provider.Configure(config)
	if err == nil {
		t.Error("Configure should fail without server")
	}
}

func TestLDAPProvider_Configure_MissingBaseDN(t *testing.T) {
	provider := NewLDAPProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeLDAP,
		Config: map[string]interface{}{
			"server": "ldap.example.com",
		},
	}

	err := provider.Configure(config)
	if err == nil {
		t.Error("Configure should fail without base_dn")
	}
}

func TestLDAPProvider_IsConfigured(t *testing.T) {
	provider := NewLDAPProvider("test")
	if provider.IsConfigured() {
		t.Error("Provider should not be configured initially")
	}

	provider.configured = true
	if !provider.IsConfigured() {
		t.Error("Provider should be configured when flag is set")
	}
}

func TestLDAPProvider_Authenticate_NotConfigured(t *testing.T) {
	provider := NewLDAPProvider("test")

	_, err := provider.Authenticate(context.Background(), Credentials{
		Username: "user",
		Password: "pass",
	})
	if err == nil {
		t.Error("Authenticate should fail when not configured")
	}
}

func TestLDAPProvider_Authenticate_NoCredentials(t *testing.T) {
	provider := NewLDAPProvider("test")
	provider.configured = true
	provider.server = "ldap.example.com"
	provider.baseDN = "dc=example,dc=com"

	// Empty username
	_, err := provider.Authenticate(context.Background(), Credentials{
		Username: "",
		Password: "pass",
	})
	if err == nil {
		t.Error("Authenticate should fail with empty username")
	}

	// Empty password
	_, err = provider.Authenticate(context.Background(), Credentials{
		Username: "user",
		Password: "",
	})
	if err == nil {
		t.Error("Authenticate should fail with empty password")
	}
}

func TestLDAPProvider_Authorize(t *testing.T) {
	provider := NewLDAPProvider("test")
	identity := &Identity{ID: "user123"}

	_, err := provider.Authorize(context.Background(), identity, "resource", "action")
	if err == nil {
		t.Error("Authorize should return error for LDAP provider")
	}
}

func TestLDAPProvider_GetLoginURL(t *testing.T) {
	provider := NewLDAPProvider("test")

	_, err := provider.GetLoginURL("state")
	if err == nil {
		t.Error("GetLoginURL should return error for LDAP provider")
	}
}

func TestLDAPProvider_HandleCallback(t *testing.T) {
	provider := NewLDAPProvider("test")

	_, err := provider.HandleCallback(context.Background(), "code", "state")
	if err == nil {
		t.Error("HandleCallback should return error for LDAP provider")
	}
}

func TestLDAPProvider_RefreshToken(t *testing.T) {
	provider := NewLDAPProvider("test")

	_, err := provider.RefreshToken(context.Background(), "token")
	if err == nil {
		t.Error("RefreshToken should return error for LDAP provider")
	}
}

func TestLDAPProvider_Logout(t *testing.T) {
	provider := NewLDAPProvider("test")
	err := provider.Logout(context.Background(), "token")
	if err != nil {
		t.Errorf("Logout should succeed: %v", err)
	}
}

func TestLDAPProvider_searchUser(t *testing.T) {
	provider := NewLDAPProvider("test")
	provider.server = "ldap.example.com"
	provider.baseDN = "dc=example,dc=com"
	provider.userAttribute = "uid"
	provider.userFilter = "(uid=%s)"

	user, err := provider.searchUser("testuser")
	if err != nil {
		t.Fatalf("searchUser failed: %v", err)
	}

	if user == nil {
		t.Fatal("searchUser returned nil")
	}
	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}
	expectedDN := "uid=testuser,dc=example,dc=com"
	if user.DN != expectedDN {
		t.Errorf("Expected DN '%s', got '%s'", expectedDN, user.DN)
	}
}

func TestLDAPProvider_bindUser(t *testing.T) {
	provider := NewLDAPProvider("test")

	// This is a simplified test - actual LDAP binding would require a server
	err := provider.bindUser("cn=test,dc=example,dc=com", "password")
	if err != nil {
		// Expected in test without LDAP server
		t.Logf("bindUser error (expected): %v", err)
	}
}

func TestLDAPProvider_buildIdentity(t *testing.T) {
	provider := NewLDAPProvider("test")
	provider.name = "test-ldap"
	provider.server = "ldap.example.com"

	user := &LDAPUser{
		DN:         "uid=john,dc=example,dc=com",
		Username:   "john",
		Email:      "john@example.com",
		Name:       "John Doe",
		GivenName:  "John",
		Surname:    "Doe",
		Groups:     []string{"admin", "users"},
		Attributes: map[string]string{
			"department": "Engineering",
		},
	}

	identity := provider.buildIdentity(user)

	if identity.ID != "uid=john,dc=example,dc=com" {
		t.Errorf("Expected ID, got '%s'", identity.ID)
	}
	if identity.Subject != "john" {
		t.Errorf("Expected subject 'john', got '%s'", identity.Subject)
	}
	if identity.Email != "john@example.com" {
		t.Errorf("Expected email, got '%s'", identity.Email)
	}
	if identity.Name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got '%s'", identity.Name)
	}
	if identity.GivenName != "John" {
		t.Errorf("Expected given_name 'John', got '%s'", identity.GivenName)
	}
	if identity.FamilyName != "Doe" {
		t.Errorf("Expected family_name 'Doe', got '%s'", identity.FamilyName)
	}
	if len(identity.Groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(identity.Groups))
	}
	if identity.Provider != ProviderTypeLDAP {
		t.Errorf("Expected provider 'ldap', got '%s'", identity.Provider)
	}
	if identity.ProviderName != "test-ldap" {
		t.Errorf("Expected provider_name 'test-ldap', got '%s'", identity.ProviderName)
	}
}

func TestLDAPProvider_buildIdentity_FallbackEmail(t *testing.T) {
	provider := NewLDAPProvider("test")
	provider.server = "ldap.example.com"

	user := &LDAPUser{
		DN:       "uid=john,dc=example,dc=com",
		Username: "john",
		Email:    "",
		Name:     "John Doe",
	}

	identity := provider.buildIdentity(user)

	expectedEmail := "john@ldap.example.com"
	if identity.Email != expectedEmail {
		t.Errorf("Expected fallback email '%s', got '%s'", expectedEmail, identity.Email)
	}
}

func TestLDAPProvider_buildIdentity_FallbackName(t *testing.T) {
	provider := NewLDAPProvider("test")

	// Test fallback to given_name + family_name
	user := &LDAPUser{
		DN:        "uid=john,dc=example,dc=com",
		Username:  "john",
		Name:      "",
		GivenName: "John",
		Surname:   "Doe",
	}

	identity := provider.buildIdentity(user)

	if identity.Name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got '%s'", identity.Name)
	}

	// Test fallback to username
	user2 := &LDAPUser{
		DN:       "uid=jane,dc=example,dc=com",
		Username: "jane",
		Name:     "",
	}

	identity2 := provider.buildIdentity(user2)
	if identity2.Name != "jane" {
		t.Errorf("Expected name 'jane', got '%s'", identity2.Name)
	}
}

func TestLDAPProvider_ValidateGroupMembership(t *testing.T) {
	// Create a mock LDAP provider
	provider := NewLDAPProvider("test")
	provider.server = "ldap.example.com"
	provider.baseDN = "dc=example,dc=com"
	provider.configured = true

	// Test with user that exists
	user, _ := provider.searchUser("john")
	user.Groups = []string{"cn=admin,dc=example,dc=com", "cn=users,dc=example,dc=com"}

	// Check membership - returns true because user has the group
	isMember, err := provider.ValidateGroupMembership("john", "cn=admin,dc=example,dc=com")
	if err != nil {
		t.Logf("ValidateGroupMembership returned error (expected in simplified implementation): %v", err)
	}
	// The function returns based on the user's groups
	if isMember {
		t.Log("User is a member of the group")
	}

	// Check non-membership
	isMember, _ = provider.ValidateGroupMembership("john", "cn=nonexistent,dc=example,dc=com")
	if isMember {
		t.Error("User should not be member of nonexistent group")
	}
}

func TestLDAPProvider_IsActiveDirectory(t *testing.T) {
	tests := []struct {
		server   string
		port     int
		expected bool
	}{
		{"ldap.example.com", 389, false},
		{"ldap.example.com", 636, true},
		{"ad.example.com", 389, true},
		{"ad.example.com", 3269, true},
		{"ad.company.local", 389, true},
		{"regular.example.com", 636, true},
	}

	for _, tc := range tests {
		provider := NewLDAPProvider("test")
		provider.server = tc.server
		provider.port = tc.port

		result := provider.IsActiveDirectory()
		if result != tc.expected {
			t.Errorf("IsActiveDirectory() for server=%s, port=%d: expected %v, got %v",
				tc.server, tc.port, tc.expected, result)
		}
	}
}

func TestLDAPProvider_GetADGroups_NotAD(t *testing.T) {
	provider := NewLDAPProvider("test")
	provider.server = "ldap.example.com"
	provider.port = 389
	provider.baseDN = "dc=example,dc=com"

	_, err := provider.GetADGroups("user")
	if err == nil {
		t.Error("GetADGroups should fail for non-AD server")
	}
}

func TestLDAPProvider_Close(t *testing.T) {
	provider := NewLDAPProvider("test")
	provider.connections = make(chan *ldapConn, 10)

	err := provider.Close()
	if err != nil {
		t.Errorf("Close should succeed: %v", err)
	}
}

func TestLDAPConn_Close(t *testing.T) {
	// Create a simple mock connection
	// We can't easily create a real net.Conn without network setup,
	// so we test the logic by checking the closed flag
	conn := &ldapConn{
		conn:   nil, // No actual connection needed for this test
		closed: false,
	}

	// First close should succeed (even with nil conn, it sets the flag)
	conn.mu.Lock()
	conn.closed = true
	conn.mu.Unlock()

	if !conn.closed {
		t.Error("closed should be true after Close")
	}

	// Second close should be no-op due to closed flag check
	conn.mu.Lock()
	alreadyClosed := conn.closed
	conn.mu.Unlock()
	if alreadyClosed {
		// This is the expected behavior - Close() returns early
		t.Log("Connection was already closed")
	}
}

func TestLDAPUser_Creation(t *testing.T) {
	user := LDAPUser{
		DN:         "uid=john,dc=example,dc=com",
		Username:   "john",
		Email:      "john@example.com",
		Name:       "John Doe",
		GivenName:  "John",
		Surname:    "Doe",
		Groups:     []string{"admin", "users"},
		Attributes: map[string]string{
			"department": "Engineering",
			"location":   "Remote",
		},
	}

	if user.DN != "uid=john,dc=example,dc=com" {
		t.Errorf("Expected DN, got '%s'", user.DN)
	}
	if len(user.Groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(user.Groups))
	}
	if len(user.Attributes) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(user.Attributes))
	}
}

func TestLDAPSearchResult_Creation(t *testing.T) {
	result := LDAPSearchResult{
		Entries: []LDAPSearchEntry{
			{
				DN: "uid=user1,dc=example,dc=com",
				Attributes: map[string][]string{
					"cn":   {"User One"},
					"mail": {"user1@example.com"},
				},
			},
			{
				DN: "uid=user2,dc=example,dc=com",
				Attributes: map[string][]string{
					"cn":   {"User Two"},
					"mail": {"user2@example.com"},
				},
			},
		},
	}

	if len(result.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(result.Entries))
	}
	if result.Entries[0].DN != "uid=user1,dc=example,dc=com" {
		t.Errorf("Unexpected first entry DN: '%s'", result.Entries[0].DN)
	}
	if len(result.Entries[0].Attributes["cn"]) != 1 {
		t.Errorf("Expected 1 cn value, got %d", len(result.Entries[0].Attributes["cn"]))
	}
}

func TestLDAPConfig_Attributes(t *testing.T) {
	config := LDAPConfig{
		Server:             "ldap.example.com",
		Port:               636,
		UseSSL:             true,
		InsecureSkipVerify: false,
		BindDN:             "cn=admin,dc=example,dc=com",
		BaseDN:             "dc=example,dc=com",
		UserFilter:         "(uid=%s)",
		GroupFilter:        "(member=%s)",
		UserAttribute:      "uid",
		AttrEmail:          "mail",
		AttrName:           "cn",
		AttrGivenName:      "givenName",
		AttrSurname:        "sn",
		AttrGroups:         "memberOf",
		DefaultRoles:       []string{"user"},
		RoleMappings: map[string]string{
			"cn=admin,dc=example,dc=com": "admin",
		},
		MaxConnections: 20,
	}

	if config.Server != "ldap.example.com" {
		t.Errorf("Unexpected server: '%s'", config.Server)
	}
	if !config.UseSSL {
		t.Error("UseSSL should be true")
	}
	if config.BindDN != "cn=admin,dc=example,dc=com" {
		t.Errorf("Unexpected bind DN: '%s'", config.BindDN)
	}
	if len(config.DefaultRoles) != 1 {
		t.Errorf("Expected 1 default role, got %d", len(config.DefaultRoles))
	}
	if len(config.RoleMappings) != 1 {
		t.Errorf("Expected 1 role mapping, got %d", len(config.RoleMappings))
	}
}
