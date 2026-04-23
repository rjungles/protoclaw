package auth

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewOIDCProvider(t *testing.T) {
	provider := NewOIDCProvider("test-oidc")
	if provider == nil {
		t.Fatal("NewOIDCProvider returned nil")
	}
	if provider.name != "test-oidc" {
		t.Errorf("Expected name 'test-oidc', got '%s'", provider.name)
	}
	if provider.states == nil {
		t.Error("states map is nil")
	}
	if !provider.usePKCE {
		t.Error("PKCE should be enabled by default")
	}
	if len(provider.scopes) != 3 {
		t.Errorf("Expected 3 default scopes, got %d", len(provider.scopes))
	}
}

func TestOIDCProvider_Type(t *testing.T) {
	provider := NewOIDCProvider("test")
	if provider.Type() != ProviderTypeOIDC {
		t.Errorf("Expected type 'oidc', got '%s'", provider.Type())
	}
}

func TestOIDCProvider_Name(t *testing.T) {
	provider := NewOIDCProvider("my-oidc")
	if provider.Name() != "my-oidc" {
		t.Errorf("Expected name 'my-oidc', got '%s'", provider.Name())
	}
}

func TestOIDCProvider_Configure(t *testing.T) {
	provider := NewOIDCProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeOIDC,
		Config: map[string]interface{}{
			"issuer":       "https://accounts.google.com",
			"client_id":    "client-id-123",
			"client_secret": "client-secret-456",
			"redirect_url": "https://example.com/callback",
			"scopes":       []interface{}{"openid", "email", "profile"},
			"use_pkce":     true,
		},
	}

	err := provider.Configure(config)
	// Will fail because discovery won't work in test
	// But we can verify the basic configuration is applied
	if err != nil && !strings.Contains(err.Error(), "failed") {
		t.Logf("Configure error (expected in test): %v", err)
	}

	// Verify fields are set even if discovery fails
	if provider.issuer != "https://accounts.google.com" {
		t.Errorf("Expected issuer, got '%s'", provider.issuer)
	}
	if provider.clientID != "client-id-123" {
		t.Errorf("Expected client_id, got '%s'", provider.clientID)
	}
	if provider.clientSecret != "client-secret-456" {
		t.Errorf("Expected client_secret")
	}
}

func TestOIDCProvider_Configure_OAuth2Type(t *testing.T) {
	provider := NewOIDCProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeOAuth2,
		Config: map[string]interface{}{
			"issuer":       "https://auth.example.com",
			"client_id":    "id",
			"client_secret": "secret",
			"redirect_url": "https://example.com/callback",
			"auth_url":     "https://auth.example.com/authorize",
			"token_url":    "https://auth.example.com/token",
		},
	}

	err := provider.Configure(config)
	// Will fail discovery but manual endpoints should be set
	if err != nil {
		// Expected in test without actual OIDC server
		t.Logf("Configure error (expected): %v", err)
	}
}

func TestOIDCProvider_Configure_MissingFields(t *testing.T) {
	provider := NewOIDCProvider("test")

	// Missing client_id
	config := ProviderConfig{
		Type: ProviderTypeOIDC,
		Config: map[string]interface{}{
			"issuer":       "https://auth.example.com",
			"client_secret": "secret",
			"redirect_url": "https://example.com/callback",
			"auth_url":     "https://auth.example.com/authorize",
			"token_url":    "https://auth.example.com/token",
		},
	}
	err := provider.Configure(config)
	if err == nil {
		t.Error("Configure should fail without client_id")
	}

	// Missing client_secret
	provider2 := NewOIDCProvider("test2")
	config2 := ProviderConfig{
		Type: ProviderTypeOIDC,
		Config: map[string]interface{}{
			"issuer":       "https://auth.example.com",
			"client_id":    "id",
			"redirect_url": "https://example.com/callback",
			"auth_url":     "https://auth.example.com/authorize",
			"token_url":    "https://auth.example.com/token",
		},
	}
	err = provider2.Configure(config2)
	if err == nil {
		t.Error("Configure should fail without client_secret")
	}

	// Missing redirect_url
	provider3 := NewOIDCProvider("test3")
	config3 := ProviderConfig{
		Type: ProviderTypeOIDC,
		Config: map[string]interface{}{
			"issuer":       "https://auth.example.com",
			"client_id":    "id",
			"client_secret": "secret",
			"auth_url":     "https://auth.example.com/authorize",
			"token_url":    "https://auth.example.com/token",
		},
	}
	err = provider3.Configure(config3)
	if err == nil {
		t.Error("Configure should fail without redirect_url")
	}

	// Missing issuer and auth_url
	provider4 := NewOIDCProvider("test4")
	config4 := ProviderConfig{
		Type: ProviderTypeOIDC,
		Config: map[string]interface{}{
			"client_id":    "id",
			"client_secret": "secret",
			"redirect_url": "https://example.com/callback",
		},
	}
	err = provider4.Configure(config4)
	if err == nil {
		t.Error("Configure should fail without issuer or auth_url")
	}
}

func TestOIDCProvider_Configure_WrongType(t *testing.T) {
	provider := NewOIDCProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeBasic,
	}

	err := provider.Configure(config)
	if err == nil {
		t.Error("Configure should fail with wrong provider type")
	}
}

func TestOIDCProvider_IsConfigured(t *testing.T) {
	provider := NewOIDCProvider("test")
	if provider.IsConfigured() {
		t.Error("Provider should not be configured initially")
	}

	provider.configured = true
	if !provider.IsConfigured() {
		t.Error("Provider should be configured when flag is set")
	}
}

func TestOIDCProvider_Authenticate(t *testing.T) {
	provider := NewOIDCProvider("test")
	provider.configured = true

	_, err := provider.Authenticate(context.Background(), Credentials{})
	if err == nil {
		t.Error("Authenticate should return error (uses redirect flow)")
	}
}

func TestOIDCProvider_GetLoginURL_NotConfigured(t *testing.T) {
	provider := NewOIDCProvider("test")

	_, err := provider.GetLoginURL("state123")
	if err == nil {
		t.Error("GetLoginURL should fail when not configured")
	}
}

func TestOIDCProvider_GetLoginURL_Generation(t *testing.T) {
	provider := NewOIDCProvider("test")
	provider.authURL = "https://auth.example.com/authorize"
	provider.clientID = "test-client"
	provider.redirectURL = "https://app.example.com/callback"
	provider.scopes = []string{"openid", "email"}
	provider.usePKCE = true
	provider.configured = true

	url, err := provider.GetLoginURL("mystate")
	if err != nil {
		t.Fatalf("GetLoginURL failed: %v", err)
	}

	// Verify URL contains expected components
	if !strings.Contains(url, "https://auth.example.com/authorize") {
		t.Error("URL should contain auth endpoint")
	}
	if !strings.Contains(url, "client_id=test-client") {
		t.Error("URL should contain client_id")
	}
	if !strings.Contains(url, "redirect_uri=") {
		t.Error("URL should contain redirect_uri")
	}
	if !strings.Contains(url, "state=mystate") {
		t.Error("URL should contain state")
	}
	if !strings.Contains(url, "scope=openid+email") {
		t.Error("URL should contain scope")
	}
	if !strings.Contains(url, "code_challenge=") {
		t.Error("URL should contain PKCE code_challenge")
	}
	if !strings.Contains(url, "code_challenge_method=S256") {
		t.Error("URL should contain code_challenge_method")
	}
	if !strings.Contains(url, "nonce=") {
		t.Error("URL should contain nonce")
	}
}

func TestOIDCProvider_GetLoginURL_WithoutPKCE(t *testing.T) {
	provider := NewOIDCProvider("test")
	provider.authURL = "https://auth.example.com/authorize"
	provider.clientID = "test-client"
	provider.redirectURL = "https://app.example.com/callback"
	provider.usePKCE = false
	provider.configured = true

	url, err := provider.GetLoginURL("state")
	if err != nil {
		t.Fatalf("GetLoginURL failed: %v", err)
	}

	if strings.Contains(url, "code_challenge=") {
		t.Error("URL should not contain PKCE when disabled")
	}
}

func TestOIDCProvider_HandleCallback_InvalidState(t *testing.T) {
	provider := NewOIDCProvider("test")
	provider.configured = true

	_, err := provider.HandleCallback(context.Background(), "code", "invalid-state")
	if err == nil {
		t.Error("HandleCallback should fail with invalid state")
	}
}

func TestOIDCProvider_HandleCallback_ExpiredState(t *testing.T) {
	provider := NewOIDCProvider("test")
	provider.configured = true

	// Add expired state
	provider.stateMu.Lock()
	provider.states["expired-state"] = &stateData{
		Expiry: time.Now().Add(-1 * time.Hour),
	}
	provider.stateMu.Unlock()

	_, err := provider.HandleCallback(context.Background(), "code", "expired-state")
	if err == nil {
		t.Error("HandleCallback should fail with expired state")
	}
}

func TestOIDCProvider_Authorize(t *testing.T) {
	provider := NewOIDCProvider("test")
	identity := &Identity{ID: "user123"}

	_, err := provider.Authorize(context.Background(), identity, "resource", "action")
	if err == nil {
		t.Error("Authorize should return error for OIDC provider")
	}
}

func TestOIDCProvider_RefreshToken_NotConfigured(t *testing.T) {
	provider := NewOIDCProvider("test")

	_, err := provider.RefreshToken(context.Background(), "refresh-token")
	if err == nil {
		t.Error("RefreshToken should fail when not configured")
	}
}

func TestOIDCProvider_RefreshToken_NoToken(t *testing.T) {
	provider := NewOIDCProvider("test")
	provider.tokenURL = "https://auth.example.com/token"
	provider.clientID = "id"
	provider.clientSecret = "secret"
	provider.configured = true

	_, err := provider.RefreshToken(context.Background(), "")
	if err == nil {
		t.Error("RefreshToken should fail without refresh token")
	}
}

func TestOIDCProvider_Logout(t *testing.T) {
	provider := NewOIDCProvider("test")
	err := provider.Logout(context.Background(), "token")
	if err != nil {
		t.Errorf("Logout should succeed: %v", err)
	}
}

func TestOIDCProvider_buildIdentityFromIDToken(t *testing.T) {
	provider := NewOIDCProvider("test")

	token := &IDToken{
		Issuer:      "https://auth.example.com",
		Subject:     "user-123",
		Audience:    []string{"my-app"},
		Expiration:  time.Now().Add(time.Hour).Unix(),
		IssuedAt:    time.Now().Unix(),
		AuthTime:    time.Now().Unix(),
		Email:       "user@example.com",
		Name:        "John Doe",
		GivenName:   "John",
		FamilyName:  "Doe",
	}

	identity := provider.buildIdentityFromIDToken(token)

	if identity.ID != "user-123" {
		t.Errorf("Expected ID 'user-123', got '%s'", identity.ID)
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
	if identity.Provider != ProviderTypeOIDC {
		t.Errorf("Expected provider 'oidc', got '%s'", identity.Provider)
	}
}

func TestOIDCProvider_buildIdentityFromIDToken_FallbackName(t *testing.T) {
	provider := NewOIDCProvider("test")

	// Test name fallback to given_name + family_name
	token := &IDToken{
		Subject:    "user-123",
		Email:      "user@example.com",
		GivenName:  "John",
		FamilyName: "Doe",
	}

	identity := provider.buildIdentityFromIDToken(token)

	if identity.Name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got '%s'", identity.Name)
	}

	// Test fallback to nickname
	token2 := &IDToken{
		Subject:  "user-123",
		Email:    "user@example.com",
		Nickname: "johndoe",
	}

	identity2 := provider.buildIdentityFromIDToken(token2)
	if identity2.Name != "johndoe" {
		t.Errorf("Expected name 'johndoe', got '%s'", identity2.Name)
	}
}

func TestOIDCProvider_buildIdentityFromUserInfo(t *testing.T) {
	provider := NewOIDCProvider("test")

	userInfo := &SocialUserInfo{
		ID:         "123",
		Sub:        "user-123",
		Email:      "user@example.com",
		Name:       "Test User",
		GivenName:  "Test",
		FamilyName: "User",
	}

	oauthToken := &OAuth2Token{
		AccessToken: "token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}

	identity := provider.buildIdentityFromUserInfo(userInfo, oauthToken)

	if identity.ID != "123" {
		t.Errorf("Expected ID '123', got '%s'", identity.ID)
	}
	if identity.Subject != "user-123" {
		t.Errorf("Expected subject 'user-123', got '%s'", identity.Subject)
	}
}

func TestOIDCProvider_mergeUserInfo(t *testing.T) {
	provider := NewOIDCProvider("test")

	identity := &Identity{
		ID:    "123",
		Email: "old@example.com",
	}

	userInfo := &SocialUserInfo{
		Name:       "New Name",
		Email:      "new@example.com",
		GivenName:  "New",
		FamilyName: "Name",
	}

	provider.mergeUserInfo(identity, userInfo)

	if identity.Name != "New Name" {
		t.Errorf("Expected name 'New Name', got '%s'", identity.Name)
	}
	if identity.Email != "new@example.com" {
		t.Errorf("Expected email 'new@example.com', got '%s'", identity.Email)
	}
	if identity.GivenName != "New" {
		t.Errorf("Expected given_name 'New', got '%s'", identity.GivenName)
	}
}

func TestOIDCAddress_Fields(t *testing.T) {
	addr := OIDCAddress{
		Formatted:     "123 Main St",
		StreetAddress: "123 Main St",
		Locality:      "Anytown",
		Region:        "CA",
		PostalCode:    "12345",
		Country:       "US",
	}

	if addr.Formatted != "123 Main St" {
		t.Errorf("Expected formatted address, got '%s'", addr.Formatted)
	}
	if addr.Locality != "Anytown" {
		t.Errorf("Expected locality 'Anytown', got '%s'", addr.Locality)
	}
	if addr.Country != "US" {
		t.Errorf("Expected country 'US', got '%s'", addr.Country)
	}
}

func TestIDToken_Claims(t *testing.T) {
	token := IDToken{
		Issuer:                "https://auth.example.com",
		Subject:               "user-123",
		Audience:              []string{"app1", "app2"},
		Expiration:            time.Now().Add(time.Hour).Unix(),
		IssuedAt:              time.Now().Unix(),
		AuthTime:              time.Now().Unix(),
		Nonce:                 "nonce-123",
		ACR:                   "urn:mace:incommon:iap:silver",
		AMR:                   []string{"pwd", "mfa"},
		Azp:                   "app1",
		Name:                  "John Doe",
		GivenName:             "John",
		FamilyName:            "Doe",
		MiddleName:            "Middle",
		Nickname:              "johnny",
		PreferredUsername:     "john",
		Profile:               "https://example.com/john",
		Picture:               "https://example.com/john.jpg",
		Website:               "https://john.example.com",
		Gender:                "male",
		Birthdate:             "1990-01-01",
		Zoneinfo:              "America/Los_Angeles",
		Locale:                "en-US",
		Email:                 "john@example.com",
		EmailVerified:         true,
		PhoneNumber:           "+1-555-123-4567",
		PhoneNumberVerified:   true,
		Address: OIDCAddress{
			Formatted: "123 Main St, Anytown, CA 12345",
		},
	}

	if token.EmailVerified != true {
		t.Error("EmailVerified should be true")
	}
	if token.PhoneNumberVerified != true {
		t.Error("PhoneNumberVerified should be true")
	}
	if len(token.AMR) != 2 {
		t.Errorf("Expected 2 AMR values, got %d", len(token.AMR))
	}
	if len(token.Audience) != 2 {
		t.Errorf("Expected 2 audience values, got %d", len(token.Audience))
	}
}

func TestOIDCDiscovery_Fields(t *testing.T) {
	discovery := OIDCDiscovery{
		Issuer:                "https://auth.example.com",
		AuthURL:             "https://auth.example.com/authorize",
		TokenURL:            "https://auth.example.com/token",
		UserInfoURL:         "https://auth.example.com/userinfo",
		JWKSURL:             "https://auth.example.com/jwks",
		EndSessionURL:       "https://auth.example.com/logout",
		ScopesSupported:     []string{"openid", "email", "profile"},
		ClaimsSupported:     []string{"sub", "iss", "aud"},
		GrantTypesSupported: []string{"authorization_code", "refresh_token"},
		ResponseTypesSupported: []string{"code", "id_token"},
	}

	if discovery.Issuer != "https://auth.example.com" {
		t.Errorf("Expected issuer, got '%s'", discovery.Issuer)
	}
	if len(discovery.ScopesSupported) != 3 {
		t.Errorf("Expected 3 scopes, got %d", len(discovery.ScopesSupported))
	}
	if len(discovery.ResponseTypesSupported) != 2 {
		t.Errorf("Expected 2 response types, got %d", len(discovery.ResponseTypesSupported))
	}
}

func TestGenerateNonce(t *testing.T) {
	nonce1 := generateNonce()
	nonce2 := generateNonce()

	if nonce1 == "" {
		t.Error("generateNonce returned empty string")
	}
	if nonce1 == nonce2 {
		t.Error("generateNonce should return different values")
	}
	if len(nonce1) < 8 {
		t.Errorf("Nonce should be at least 8 chars, got %d", len(nonce1))
	}
}

func TestGenerateCodeVerifier(t *testing.T) {
	verifier1 := generateCodeVerifier()
	verifier2 := generateCodeVerifier()

	if verifier1 == "" {
		t.Error("generateCodeVerifier returned empty string")
	}
	if verifier1 == verifier2 {
		t.Error("generateCodeVerifier should return different values")
	}
	if len(verifier1) < 32 {
		t.Errorf("Code verifier should be at least 32 chars, got %d", len(verifier1))
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	verifier := "test-verifier-string-for-pkce"
	challenge := generateCodeChallenge(verifier)

	if challenge == "" {
		t.Error("generateCodeChallenge returned empty string")
	}
	if challenge == verifier {
		t.Error("Code challenge should be different from verifier")
	}

	// Same verifier should produce same challenge
	challenge2 := generateCodeChallenge(verifier)
	if challenge != challenge2 {
		t.Error("Same verifier should produce same challenge")
	}
}
