package auth

import (
	"context"
	"encoding/base64"
	"testing"
	"time"
)

func TestNewBasicAuthProvider(t *testing.T) {
	provider := NewBasicAuthProvider("test-basic")
	if provider == nil {
		t.Fatal("NewBasicAuthProvider returned nil")
	}
	if provider.name != "test-basic" {
		t.Errorf("Expected name 'test-basic', got '%s'", provider.name)
	}
	if provider.realm != "AgentOS" {
		t.Errorf("Expected default realm 'AgentOS', got '%s'", provider.realm)
	}
	if provider.users == nil {
		t.Error("users map is nil")
	}
}

func TestBasicAuthProvider_Type(t *testing.T) {
	provider := NewBasicAuthProvider("test")
	if provider.Type() != ProviderTypeBasic {
		t.Errorf("Expected type '%s', got '%s'", ProviderTypeBasic, provider.Type())
	}
}

func TestBasicAuthProvider_Name(t *testing.T) {
	provider := NewBasicAuthProvider("my-provider")
	if provider.Name() != "my-provider" {
		t.Errorf("Expected name 'my-provider', got '%s'", provider.Name())
	}
}

func TestBasicAuthProvider_Configure(t *testing.T) {
	provider := NewBasicAuthProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeBasic,
		Name: "test",
		Config: map[string]interface{}{
			"realm": "TestRealm",
			"users": []interface{}{
				map[string]interface{}{
					"username": "admin",
					"password": "secret123",
					"email":    "admin@example.com",
					"name":     "Admin User",
					"roles":    []interface{}{"admin", "user"},
				},
				map[string]interface{}{
					"username": "john",
					"password": "password456",
					"email":    "john@example.com",
					"name":     "John Doe",
					"roles":    []interface{}{"user"},
				},
			},
		},
	}

	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if !provider.IsConfigured() {
		t.Error("Provider should be configured")
	}

	if provider.realm != "TestRealm" {
		t.Errorf("Expected realm 'TestRealm', got '%s'", provider.realm)
	}

	// Verify users were created
	if len(provider.users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(provider.users))
	}

	// Test wrong provider type
	wrongConfig := ProviderConfig{
		Type: ProviderTypeOIDC,
	}
	err = provider.Configure(wrongConfig)
	if err == nil {
		t.Error("Configure should fail with wrong provider type")
	}
}

func TestBasicAuthProvider_CreateUser(t *testing.T) {
	provider := NewBasicAuthProvider("test")

	err := provider.CreateUser("alice", "password123", "alice@example.com", "Alice Smith", []string{"user"})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Verify user exists
	user, err := provider.GetUser("alice")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("Expected username 'alice', got '%s'", user.Username)
	}
	if user.Identity.Email != "alice@example.com" {
		t.Errorf("Expected email 'alice@example.com', got '%s'", user.Identity.Email)
	}
	if user.Identity.Name != "Alice Smith" {
		t.Errorf("Expected name 'Alice Smith', got '%s'", user.Identity.Name)
	}
	if len(user.Identity.Roles) != 1 || user.Identity.Roles[0] != "user" {
		t.Errorf("Expected roles ['user'], got %v", user.Identity.Roles)
	}

	// Test duplicate user
	err = provider.CreateUser("alice", "anotherpassword", "other@example.com", "Other", nil)
	if err != nil {
		t.Fatalf("CreateUser should overwrite existing user: %v", err)
	}

	// Verify user was updated
	user, _ = provider.GetUser("alice")
	if user.Identity.Email != "other@example.com" {
		t.Errorf("Expected email to be updated to 'other@example.com', got '%s'", user.Identity.Email)
	}
}

func TestBasicAuthProvider_CreateUser_InvalidInput(t *testing.T) {
	provider := NewBasicAuthProvider("test")

	// Test empty username
	err := provider.CreateUser("", "password", "email@example.com", "Name", nil)
	if err == nil {
		t.Error("CreateUser should fail with empty username")
	}

	// Test empty password
	err = provider.CreateUser("user", "", "email@example.com", "Name", nil)
	if err == nil {
		t.Error("CreateUser should fail with empty password")
	}
}

func TestBasicAuthProvider_ValidateCredentials(t *testing.T) {
	provider := NewBasicAuthProvider("test")
	provider.CreateUser("bob", "securepass", "bob@example.com", "Bob", nil)

	// Valid credentials
	user, err := provider.ValidateCredentials("bob", "securepass")
	if err != nil {
		t.Errorf("ValidateCredentials should succeed: %v", err)
	}
	if user == nil {
		t.Fatal("ValidateCredentials returned nil user")
	}
	if user.Username != "bob" {
		t.Errorf("Expected username 'bob', got '%s'", user.Username)
	}

	// Invalid username
	_, err = provider.ValidateCredentials("nonexistent", "password")
	if err == nil {
		t.Error("ValidateCredentials should fail with invalid username")
	}

	// Invalid password
	_, err = provider.ValidateCredentials("bob", "wrongpassword")
	if err == nil {
		t.Error("ValidateCredentials should fail with invalid password")
	}
}

func TestBasicAuthProvider_Authenticate(t *testing.T) {
	provider := NewBasicAuthProvider("test")

	// Configure provider
	config := ProviderConfig{
		Type: ProviderTypeBasic,
		Config: map[string]interface{}{
			"users": []interface{}{
				map[string]interface{}{
					"username": "testuser",
					"password": "testpass",
					"email":    "test@example.com",
					"name":     "Test User",
					"roles":    []interface{}{"user"},
				},
			},
		},
	}
	provider.Configure(config)

	// Valid credentials
	creds := Credentials{
		Username: "testuser",
		Password: "testpass",
	}
	identity, err := provider.Authenticate(context.Background(), creds)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if identity == nil {
		t.Fatal("Authenticate returned nil identity")
	}
	if identity.ID != "testuser" {
		t.Errorf("Expected ID 'testuser', got '%s'", identity.ID)
	}
	if identity.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", identity.Email)
	}

	// Check that last login was updated
	user, _ := provider.GetUser("testuser")
	if user.LastLogin.IsZero() {
		t.Error("LastLogin should be set after authentication")
	}

	// Invalid credentials
	creds.Password = "wrongpass"
	_, err = provider.Authenticate(context.Background(), creds)
	if err == nil {
		t.Error("Authenticate should fail with invalid credentials")
	}

	// Empty credentials
	_, err = provider.Authenticate(context.Background(), Credentials{})
	if err == nil {
		t.Error("Authenticate should fail with empty credentials")
	}
}

func TestBasicAuthProvider_Authenticate_NotConfigured(t *testing.T) {
	provider := NewBasicAuthProvider("test")

	_, err := provider.Authenticate(context.Background(), Credentials{
		Username: "user",
		Password: "pass",
	})
	if err == nil {
		t.Error("Authenticate should fail when not configured")
	}
}

func TestBasicAuthProvider_UpdateUser(t *testing.T) {
	provider := NewBasicAuthProvider("test")
	provider.CreateUser("charlie", "oldpassword", "charlie@example.com", "Charlie", nil)

	// Update password
	err := provider.UpdateUser("charlie", "newpassword")
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// Verify old password doesn't work
	_, err = provider.ValidateCredentials("charlie", "oldpassword")
	if err == nil {
		t.Error("Old password should not work after update")
	}

	// Verify new password works
	_, err = provider.ValidateCredentials("charlie", "newpassword")
	if err != nil {
		t.Errorf("New password should work: %v", err)
	}

	// Update non-existent user
	err = provider.UpdateUser("nonexistent", "password")
	if err == nil {
		t.Error("UpdateUser should fail for non-existent user")
	}

	// Update with empty password
	err = provider.UpdateUser("charlie", "")
	if err == nil {
		t.Error("UpdateUser should fail with empty password")
	}
}

func TestBasicAuthProvider_DeleteUser(t *testing.T) {
	provider := NewBasicAuthProvider("test")
	provider.CreateUser("dave", "password", "dave@example.com", "Dave", nil)

	// Verify user exists
	_, err := provider.GetUser("dave")
	if err != nil {
		t.Fatal("User should exist before deletion")
	}

	// Delete user
	err = provider.DeleteUser("dave")
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Verify user no longer exists
	_, err = provider.GetUser("dave")
	if err == nil {
		t.Error("User should not exist after deletion")
	}

	// Delete non-existent user (should not error)
	err = provider.DeleteUser("nonexistent")
	if err != nil {
		t.Errorf("DeleteUser should not error for non-existent user: %v", err)
	}
}

func TestBasicAuthProvider_GetRealm(t *testing.T) {
	provider := NewBasicAuthProvider("test")
	if provider.GetRealm() != "AgentOS" {
		t.Errorf("Expected default realm 'AgentOS', got '%s'", provider.GetRealm())
	}

	// Configure with custom realm
	config := ProviderConfig{
		Type: ProviderTypeBasic,
		Config: map[string]interface{}{
			"realm": "CustomRealm",
		},
	}
	provider.Configure(config)

	if provider.GetRealm() != "CustomRealm" {
		t.Errorf("Expected realm 'CustomRealm', got '%s'", provider.GetRealm())
	}
}

func TestBasicAuthProvider_GetWWWAuthenticateHeader(t *testing.T) {
	provider := NewBasicAuthProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeBasic,
		Config: map[string]interface{}{
			"realm": "ProtectedArea",
		},
	}
	provider.Configure(config)

	header := provider.GetWWWAuthenticateHeader()
	expected := `Basic realm="ProtectedArea"`
	if header != expected {
		t.Errorf("Expected header '%s', got '%s'", expected, header)
	}
}

func TestParseBasicAuthHeader(t *testing.T) {
	// Valid header
	credentials := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	header := "Basic " + credentials
	username, password, err := ParseBasicAuthHeader(header)
	if err != nil {
		t.Fatalf("ParseBasicAuthHeader failed: %v", err)
	}
	if username != "user" {
		t.Errorf("Expected username 'user', got '%s'", username)
	}
	if password != "pass" {
		t.Errorf("Expected password 'pass', got '%s'", password)
	}

	// Invalid prefix
	_, _, err = ParseBasicAuthHeader("Bearer token")
	if err == nil {
		t.Error("ParseBasicAuthHeader should fail with invalid prefix")
	}

	// Invalid base64
	_, _, err = ParseBasicAuthHeader("Basic invalid-base64!!!")
	if err == nil {
		t.Error("ParseBasicAuthHeader should fail with invalid base64")
	}

	// Missing colon separator
	invalidCreds := base64.StdEncoding.EncodeToString([]byte("userpass"))
	_, _, err = ParseBasicAuthHeader("Basic " + invalidCreds)
	if err == nil {
		t.Error("ParseBasicAuthHeader should fail without colon separator")
	}
}

func TestGenerateSalt(t *testing.T) {
	salt1 := generateSalt()
	salt2 := generateSalt()

	if salt1 == "" {
		t.Error("generateSalt returned empty string")
	}
	if salt1 == salt2 {
		t.Error("generateSalt should return different values")
	}
	if len(salt1) < 16 {
		t.Errorf("Salt should be at least 16 chars, got %d", len(salt1))
	}
}

func TestHashPassword(t *testing.T) {
	salt := "testsalt"
	password := "mypassword"

	hash1 := hashPassword(password, salt)
	hash2 := hashPassword(password, salt)

	if hash1 == "" {
		t.Error("hashPassword returned empty string")
	}
	if hash1 != hash2 {
		t.Error("Same password and salt should produce same hash")
	}

	// Different salt should produce different hash
	hash3 := hashPassword(password, "differentsalt")
	if hash1 == hash3 {
		t.Error("Different salt should produce different hash")
	}

	// Different password should produce different hash
	hash4 := hashPassword("differentpassword", salt)
	if hash1 == hash4 {
		t.Error("Different password should produce different hash")
	}
}

func TestBasicAuthProvider_Methods_NotImplemented(t *testing.T) {
	provider := NewBasicAuthProvider("test")
	provider.configured = true

	// GetLoginURL should return error
	_, err := provider.GetLoginURL("state")
	if err == nil {
		t.Error("GetLoginURL should return error for Basic Auth")
	}

	// HandleCallback should return error
	_, err = provider.HandleCallback(context.Background(), "code", "state")
	if err == nil {
		t.Error("HandleCallback should return error for Basic Auth")
	}

	// RefreshToken should return error
	_, err = provider.RefreshToken(context.Background(), "token")
	if err == nil {
		t.Error("RefreshToken should return error for Basic Auth")
	}

	// Authorize should return error
	identity := &Identity{ID: "user"}
	_, err = provider.Authorize(context.Background(), identity, "resource", "action")
	if err == nil {
		t.Error("Authorize should return error for Basic Auth")
	}

	// Logout should succeed (no-op)
	err = provider.Logout(context.Background(), "token")
	if err != nil {
		t.Errorf("Logout should succeed: %v", err)
	}
}

func TestBasicAuthUser_CreatedAt(t *testing.T) {
	provider := NewBasicAuthProvider("test")
	
	before := time.Now()
	err := provider.CreateUser("user", "pass", "email@example.com", "Name", nil)
	after := time.Now()
	
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	
	user, _ := provider.GetUser("user")
	if user.CreatedAt.Before(before) || user.CreatedAt.After(after) {
		t.Error("CreatedAt should be set to creation time")
	}
	
	if !user.LastLogin.IsZero() {
		t.Error("LastLogin should be zero before authentication")
	}
}
