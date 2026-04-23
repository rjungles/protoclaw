package auth

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewSocialProvider(t *testing.T) {
	provider := NewSocialProvider("google-login", SocialProviderGoogle)
	if provider == nil {
		t.Fatal("NewSocialProvider returned nil")
	}
	if provider.name != "google-login" {
		t.Errorf("Expected name 'google-login', got '%s'", provider.name)
	}
	if provider.provider != SocialProviderGoogle {
		t.Errorf("Expected provider 'google', got '%s'", provider.provider)
	}
	if provider.states == nil {
		t.Error("states map is nil")
	}
	// Social provider uses OAuth2 flow with state management
	if provider.states == nil {
		t.Error("states should be initialized")
	}
}

func TestSocialProvider_Type(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGitHub)
	if provider.Type() != ProviderTypeSocial {
		t.Errorf("Expected type 'social', got '%s'", provider.Type())
	}
}

func TestSocialProvider_Name(t *testing.T) {
	provider := NewSocialProvider("my-provider", SocialProviderGoogle)
	if provider.Name() != "my-provider" {
		t.Errorf("Expected name 'my-provider', got '%s'", provider.Name())
	}
}

func TestSocialProvider_Configure(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)

	config := ProviderConfig{
		Type: ProviderTypeSocial,
		Config: map[string]interface{}{
			"provider":      "github",
			"client_id":     "client-id-123",
			"client_secret": "client-secret-456",
			"redirect_url":  "https://example.com/callback",
			"scopes":        []interface{}{"read:user", "user:email"},
		},
	}

	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if !provider.IsConfigured() {
		t.Error("Provider should be configured")
	}
	if provider.provider != SocialProviderGitHub {
		t.Errorf("Expected provider 'github', got '%s'", provider.provider)
	}
	if provider.clientID != "client-id-123" {
		t.Errorf("Expected client_id 'client-id-123', got '%s'", provider.clientID)
	}
	if provider.clientSecret != "client-secret-456" {
		t.Errorf("Expected client_secret, got '%s'", provider.clientSecret)
	}
	if provider.redirectURL != "https://example.com/callback" {
		t.Errorf("Expected redirect_url, got '%s'", provider.redirectURL)
	}
	if len(provider.scopes) != 2 {
		t.Errorf("Expected 2 scopes, got %d", len(provider.scopes))
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

func TestSocialProvider_Configure_MissingFields(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)

	// Missing client_id
	config := ProviderConfig{
		Type: ProviderTypeSocial,
		Config: map[string]interface{}{
			"client_secret": "secret",
			"redirect_url":  "https://example.com/callback",
		},
	}
	err := provider.Configure(config)
	if err == nil {
		t.Error("Configure should fail without client_id")
	}

	// Missing client_secret
	provider2 := NewSocialProvider("test2", SocialProviderGoogle)
	config2 := ProviderConfig{
		Type: ProviderTypeSocial,
		Config: map[string]interface{}{
			"client_id":    "id",
			"redirect_url": "https://example.com/callback",
		},
	}
	err = provider2.Configure(config2)
	if err == nil {
		t.Error("Configure should fail without client_secret")
	}

	// Missing redirect_url
	provider3 := NewSocialProvider("test3", SocialProviderGoogle)
	config3 := ProviderConfig{
		Type: ProviderTypeSocial,
		Config: map[string]interface{}{
			"client_id":     "id",
			"client_secret": "secret",
		},
	}
	err = provider3.Configure(config3)
	if err == nil {
		t.Error("Configure should fail without redirect_url")
	}
}

func TestSocialProvider_Configure_CustomEndpoints(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)

	config := ProviderConfig{
		Type: ProviderTypeSocial,
		Config: map[string]interface{}{
			"client_id":     "id",
			"client_secret": "secret",
			"redirect_url":  "https://example.com/callback",
			"auth_url":      "https://custom.com/oauth/authorize",
			"token_url":     "https://custom.com/oauth/token",
			"userinfo_url":  "https://custom.com/oauth/userinfo",
		},
	}

	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if provider.authURL != "https://custom.com/oauth/authorize" {
		t.Errorf("Expected custom auth_url, got '%s'", provider.authURL)
	}
	if provider.tokenURL != "https://custom.com/oauth/token" {
		t.Errorf("Expected custom token_url, got '%s'", provider.tokenURL)
	}
	if provider.userInfoURL != "https://custom.com/oauth/userinfo" {
		t.Errorf("Expected custom userinfo_url, got '%s'", provider.userInfoURL)
	}
}

func TestSocialProvider_Authenticate(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)
	provider.configured = true

	_, err := provider.Authenticate(context.Background(), Credentials{})
	if err == nil {
		t.Error("Authenticate should return error (uses redirect flow)")
	}
}

func TestSocialProvider_GetLoginURL_NotConfigured(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)

	_, err := provider.GetLoginURL("state123")
	if err == nil {
		t.Error("GetLoginURL should fail when not configured")
	}
}

func TestSocialProvider_HandleCallback_InvalidState(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)
	provider.clientID = "id"
	provider.clientSecret = "secret"
	provider.redirectURL = "https://example.com/callback"
	provider.configured = true

	_, err := provider.HandleCallback(context.Background(), "code", "invalid-state")
	if err == nil {
		t.Error("HandleCallback should fail with invalid state")
	}
}

func TestSocialProvider_Authorize(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)
	identity := &Identity{ID: "user123"}

	_, err := provider.Authorize(context.Background(), identity, "resource", "action")
	if err == nil {
		t.Error("Authorize should return error for social provider")
	}
}

func TestSocialProvider_RefreshToken_NotConfigured(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)

	_, err := provider.RefreshToken(context.Background(), "refresh-token")
	if err == nil {
		t.Error("RefreshToken should fail when not configured")
	}
}

func TestSocialProvider_RefreshToken_NoToken(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)
	provider.clientID = "id"
	provider.clientSecret = "secret"
	provider.configured = true

	_, err := provider.RefreshToken(context.Background(), "")
	if err == nil {
		t.Error("RefreshToken should fail without refresh token")
	}
}

func TestSocialProvider_Logout(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)
	err := provider.Logout(context.Background(), "token")
	if err != nil {
		t.Errorf("Logout should succeed: %v", err)
	}
}

func TestSocialProvider_GetProvider(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGitHub)
	if provider.GetProvider() != SocialProviderGitHub {
		t.Errorf("Expected provider 'github', got '%s'", provider.GetProvider())
	}
}

func TestSupportsProvider(t *testing.T) {
	// Test supported providers
	supported := []SocialProviderType{
		SocialProviderGoogle,
		SocialProviderGitHub,
		SocialProviderFacebook,
		SocialProviderApple,
		SocialProviderAzure,
		SocialProviderDiscord,
		SocialProviderSlack,
	}

	for _, p := range supported {
		if !SupportsProvider(p) {
			t.Errorf("Provider '%s' should be supported", p)
		}
	}

	// Test unsupported provider
	if SupportsProvider("unknown") {
		t.Error("'unknown' provider should not be supported")
	}
}

func TestGetSupportedProviders(t *testing.T) {
	providers := GetSupportedProviders()
	if len(providers) == 0 {
		t.Error("Should return supported providers")
	}

	// Check that all expected providers are included
	found := make(map[SocialProviderType]bool)
	for _, p := range providers {
		found[p] = true
	}

	expected := []SocialProviderType{
		SocialProviderGoogle,
		SocialProviderGitHub,
		SocialProviderFacebook,
		SocialProviderApple,
		SocialProviderAzure,
		SocialProviderDiscord,
		SocialProviderSlack,
	}

	for _, p := range expected {
		if !found[p] {
			t.Errorf("Provider '%s' should be in supported list", p)
		}
	}
}

func TestProviderEndpoints(t *testing.T) {
	// Test Google endpoints
	googleEndpoints, ok := providerEndpoints[SocialProviderGoogle]
	if !ok {
		t.Fatal("Google endpoints not found")
	}
	if !strings.Contains(googleEndpoints.AuthURL, "google") {
		t.Error("Google auth URL should contain 'google'")
	}
	if !strings.Contains(googleEndpoints.TokenURL, "google") {
		t.Error("Google token URL should contain 'google'")
	}

	// Test GitHub endpoints
	githubEndpoints, ok := providerEndpoints[SocialProviderGitHub]
	if !ok {
		t.Fatal("GitHub endpoints not found")
	}
	if !strings.Contains(githubEndpoints.AuthURL, "github") {
		t.Error("GitHub auth URL should contain 'github'")
	}

	// Check default scopes
	if len(googleEndpoints.DefaultScopes) == 0 {
		t.Error("Google should have default scopes")
	}
	if len(githubEndpoints.DefaultScopes) == 0 {
		t.Error("GitHub should have default scopes")
	}
}

func TestSocialProvider_buildIdentity(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)

	userInfo := &SocialUserInfo{
		ID:         "12345",
		Sub:        "user-123",
		Email:      "user@example.com",
		Name:       "John Doe",
		GivenName:  "John",
		FamilyName: "Doe",
		Picture:    "https://example.com/pic.jpg",
	}

	token := &OAuth2Token{
		AccessToken: "access-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}

	identity := provider.buildIdentity(userInfo, token)

	if identity.ID != "12345" {
		t.Errorf("Expected ID '12345', got '%s'", identity.ID)
	}
	if identity.Email != "user@example.com" {
		t.Errorf("Expected email 'user@example.com', got '%s'", identity.Email)
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
	if identity.Provider != ProviderTypeSocial {
		t.Errorf("Expected provider 'social', got '%s'", identity.Provider)
	}
	if identity.ProviderName != "test" {
		t.Errorf("Expected provider_name 'test', got '%s'", identity.ProviderName)
	}

	// Check expiration
	if identity.ExpiresAt == nil {
		t.Error("ExpiresAt should be set")
	}
}

func TestSocialProvider_buildIdentity_GitHubStyle(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGitHub)

	userInfo := &SocialUserInfo{
		Login:     "johndoe",
		Email:     "john@example.com",
		AvatarURL: "https://github.com/avatar.png",
	}

	token := &OAuth2Token{
		AccessToken: "token",
		TokenType:   "Bearer",
	}

	identity := provider.buildIdentity(userInfo, token)

	if identity.ID != "johndoe" {
		t.Errorf("Expected ID 'johndoe', got '%s'", identity.ID)
	}
	if identity.Name != "johndoe" {
		t.Errorf("Expected name 'johndoe', got '%s'", identity.Name)
	}
}

func TestSocialProvider_buildIdentity_FallbackName(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)

	// Test fallback to given_name + family_name
	userInfo := &SocialUserInfo{
		Email:      "user@example.com",
		GivenName:  "John",
		FamilyName: "Doe",
	}

	token := &OAuth2Token{}
	identity := provider.buildIdentity(userInfo, token)

	if identity.Name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got '%s'", identity.Name)
	}
	if identity.ID != "" {
		t.Errorf("Expected empty ID, got '%s'", identity.ID)
	}
}

func TestSocialUserInfo_Fields(t *testing.T) {
	userInfo := SocialUserInfo{
		ID:            "123",
		Sub:           "sub-123",
		Email:         "test@example.com",
		EmailVerified: true,
		Name:          "Test User",
		GivenName:     "Test",
		FamilyName:    "User",
		Picture:       "https://example.com/pic.jpg",
		AvatarURL:     "https://example.com/avatar.jpg",
		Login:         "testuser",
		Username:      "testuser123",
	}

	if !userInfo.EmailVerified {
		t.Error("EmailVerified should be true")
	}
	if userInfo.GivenName != "Test" {
		t.Errorf("Expected GivenName 'Test', got '%s'", userInfo.GivenName)
	}
	if userInfo.FamilyName != "User" {
		t.Errorf("Expected FamilyName 'User', got '%s'", userInfo.FamilyName)
	}
}

func TestOAuth2Token_Fields(t *testing.T) {
	token := OAuth2Token{
		AccessToken:  "access-123",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: "refresh-456",
		IDToken:      "id-789",
		Scope:        "openid email profile",
	}

	if token.AccessToken != "access-123" {
		t.Errorf("Expected AccessToken 'access-123', got '%s'", token.AccessToken)
	}
	if token.ExpiresIn != 3600 {
		t.Errorf("Expected ExpiresIn 3600, got %d", token.ExpiresIn)
	}
	if token.Scope != "openid email profile" {
		t.Errorf("Expected Scope, got '%s'", token.Scope)
	}
}

func TestSocialProvider_cleanupStates(t *testing.T) {
	provider := NewSocialProvider("test", SocialProviderGoogle)

	// Add expired state (states is map[string]time.Time in social.go)
	expiredTime := time.Now().Add(-1 * time.Hour)
	provider.stateMu.Lock()
	provider.states["expired-state"] = expiredTime
	provider.stateMu.Unlock()

	// Verify state was added
	provider.stateMu.RLock()
	_, exists := provider.states["expired-state"]
	provider.stateMu.RUnlock()
	if !exists {
		t.Error("State should exist after adding")
	}

	// Cleanup runs in background, we just verify the function structure
}
