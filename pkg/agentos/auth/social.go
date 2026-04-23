package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SocialProvider implements OAuth2-based social login (Google, GitHub, Facebook, etc.)
type SocialProvider struct {
	name     string
	provider SocialProviderType

	// OAuth2 settings
	clientID     string
	clientSecret string
	redirectURL  string
	scopes       []string

	// Provider endpoints
	authURL      string
	tokenURL     string
	userInfoURL  string

	// State management
	stateMu sync.RWMutex
	states  map[string]time.Time

	// Configuration
	configured bool
	configMu   sync.RWMutex
}

// SocialProviderType represents supported social login providers
type SocialProviderType string

const (
	SocialProviderGoogle   SocialProviderType = "google"
	SocialProviderGitHub   SocialProviderType = "github"
	SocialProviderFacebook SocialProviderType = "facebook"
	SocialProviderApple    SocialProviderType = "apple"
	SocialProviderAzure    SocialProviderType = "azure"
	SocialProviderDiscord  SocialProviderType = "discord"
	SocialProviderSlack    SocialProviderType = "slack"
)

// SocialConfig represents social login configuration
type SocialConfig struct {
	Provider     SocialProviderType `json:"provider"`
	ClientID     string             `json:"client_id"`
	ClientSecret string             `json:"client_secret"`
	RedirectURL  string             `json:"redirect_url"`
	Scopes       []string           `json:"scopes,omitempty"`
	AuthURL      string             `json:"auth_url,omitempty"`
	TokenURL     string             `json:"token_url,omitempty"`
	UserInfoURL  string             `json:"userinfo_url,omitempty"`
}

// OAuth2Token represents OAuth2 token response
type OAuth2Token struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// SocialUserInfo represents user info from social provider
type SocialUserInfo struct {
	ID            string `json:"id,omitempty"`
	Sub           string `json:"sub,omitempty"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`
	Name          string `json:"name,omitempty"`
	GivenName     string `json:"given_name,omitempty"`
	FamilyName    string `json:"family_name,omitempty"`
	Picture       string `json:"picture,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	Login         string `json:"login,omitempty"`
	Username      string `json:"username,omitempty"`
}

// Provider endpoints configuration
var providerEndpoints = map[SocialProviderType]struct {
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	DefaultScopes []string
}{
	SocialProviderGoogle: {
		AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:    "https://oauth2.googleapis.com/token",
		UserInfoURL: "https://www.googleapis.com/oauth2/v2/userinfo",
		DefaultScopes: []string{"openid", "email", "profile"},
	},
	SocialProviderGitHub: {
		AuthURL:     "https://github.com/login/oauth/authorize",
		TokenURL:    "https://github.com/login/oauth/access_token",
		UserInfoURL: "https://api.github.com/user",
		DefaultScopes: []string{"read:user", "user:email"},
	},
	SocialProviderFacebook: {
		AuthURL:     "https://www.facebook.com/v12.0/dialog/oauth",
		TokenURL:    "https://graph.facebook.com/v12.0/oauth/access_token",
		UserInfoURL: "https://graph.facebook.com/me?fields=id,name,email,picture",
		DefaultScopes: []string{"email", "public_profile"},
	},
	SocialProviderApple: {
		AuthURL:     "https://appleid.apple.com/auth/authorize",
		TokenURL:    "https://appleid.apple.com/auth/token",
		UserInfoURL: "", // Apple uses ID token
		DefaultScopes: []string{"name", "email"},
	},
	SocialProviderAzure: {
		AuthURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenURL:    "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		UserInfoURL: "https://graph.microsoft.com/v1.0/me",
		DefaultScopes: []string{"openid", "email", "profile", "User.Read"},
	},
	SocialProviderDiscord: {
		AuthURL:     "https://discord.com/api/oauth2/authorize",
		TokenURL:    "https://discord.com/api/oauth2/token",
		UserInfoURL: "https://discord.com/api/users/@me",
		DefaultScopes: []string{"identify", "email"},
	},
	SocialProviderSlack: {
		AuthURL:     "https://slack.com/oauth/v2/authorize",
		TokenURL:    "https://slack.com/api/oauth.v2.access",
		UserInfoURL: "https://slack.com/api/users.info",
		DefaultScopes: []string{"users:read", "users:read.email"},
	},
}

// NewSocialProvider creates a new social login provider
func NewSocialProvider(name string, provider SocialProviderType) *SocialProvider {
	sp := &SocialProvider{
		name:     name,
		provider: provider,
		states:   make(map[string]time.Time),
	}

	// Set default endpoints
	if endpoints, ok := providerEndpoints[provider]; ok {
		sp.authURL = endpoints.AuthURL
		sp.tokenURL = endpoints.TokenURL
		sp.userInfoURL = endpoints.UserInfoURL
		sp.scopes = endpoints.DefaultScopes
	}

	// Start state cleanup goroutine
	go sp.cleanupStates()

	return sp
}

// Type returns the provider type
func (s *SocialProvider) Type() ProviderType {
	return ProviderTypeSocial
}

// Name returns the provider name
func (s *SocialProvider) Name() string {
	return s.name
}

// Configure sets up the social login provider
func (s *SocialProvider) Configure(config ProviderConfig) error {
	if config.Type != ProviderTypeSocial {
		return fmt.Errorf("invalid provider type: %s", config.Type)
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	// Parse configuration
	if provider, ok := config.Config["provider"].(string); ok && provider != "" {
		s.provider = SocialProviderType(provider)
		// Update endpoints for the provider
		if endpoints, ok := providerEndpoints[s.provider]; ok {
			s.authURL = endpoints.AuthURL
			s.tokenURL = endpoints.TokenURL
			s.userInfoURL = endpoints.UserInfoURL
			s.scopes = endpoints.DefaultScopes
		}
	}

	if clientID, ok := config.Config["client_id"].(string); ok {
		s.clientID = clientID
	}

	if clientSecret, ok := config.Config["client_secret"].(string); ok {
		s.clientSecret = clientSecret
	}

	if redirectURL, ok := config.Config["redirect_url"].(string); ok {
		s.redirectURL = redirectURL
	}

	if scopes, ok := config.Config["scopes"].([]interface{}); ok {
		s.scopes = make([]string, 0, len(scopes))
		for _, scope := range scopes {
			if scopeStr, ok := scope.(string); ok {
				s.scopes = append(s.scopes, scopeStr)
			}
		}
	}

	// Allow custom endpoints
	if authURL, ok := config.Config["auth_url"].(string); ok && authURL != "" {
		s.authURL = authURL
	}

	if tokenURL, ok := config.Config["token_url"].(string); ok && tokenURL != "" {
		s.tokenURL = tokenURL
	}

	if userInfoURL, ok := config.Config["userinfo_url"].(string); ok && userInfoURL != "" {
		s.userInfoURL = userInfoURL
	}

	// Validate required fields
	if s.clientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if s.clientSecret == "" {
		return fmt.Errorf("client_secret is required")
	}
	if s.redirectURL == "" {
		return fmt.Errorf("redirect_url is required")
	}

	s.configured = true
	return nil
}

// IsConfigured returns true if the provider is configured
func (s *SocialProvider) IsConfigured() bool {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.configured
}

// Authenticate authenticates using social login
func (s *SocialProvider) Authenticate(ctx context.Context, credentials Credentials) (*Identity, error) {
	return nil, fmt.Errorf("social login uses redirect flow - use GetLoginURL and HandleCallback")
}

// Authorize checks if the user has permission
func (s *SocialProvider) Authorize(ctx context.Context, identity *Identity, resource string, action string) (bool, error) {
	return false, fmt.Errorf("social provider doesn't handle authorization")
}

// GetLoginURL returns the social login URL
func (s *SocialProvider) GetLoginURL(state string) (string, error) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()

	if !s.configured {
		return "", fmt.Errorf("social provider not configured")
	}

	// Store state with expiration
	s.stateMu.Lock()
	s.states[state] = time.Now().Add(10 * time.Minute)
	s.stateMu.Unlock()

	// Build authorization URL
	params := url.Values{
		"client_id":     {s.clientID},
		"redirect_uri":  {s.redirectURL},
		"response_type": {"code"},
		"scope":         {strings.Join(s.scopes, " ")},
		"state":         {state},
	}

	// Add provider-specific parameters
	switch s.provider {
	case SocialProviderGoogle, SocialProviderAzure:
		params.Set("access_type", "offline")
		params.Set("prompt", "consent")
	case SocialProviderFacebook:
		params.Set("auth_type", "rerequest")
	}

	return s.authURL + "?" + params.Encode(), nil
}

// HandleCallback handles the OAuth callback
func (s *SocialProvider) HandleCallback(ctx context.Context, code string, state string) (*Identity, error) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()

	if !s.configured {
		return nil, fmt.Errorf("social provider not configured")
	}

	// Verify state
	s.stateMu.Lock()
	_, ok := s.states[state]
	if ok {
		delete(s.states, state)
	}
	s.stateMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("invalid state parameter")
	}

	// Exchange code for token
	token, err := s.exchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Get user info
	userInfo, err := s.getUserInfo(ctx, token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Build identity
	identity := s.buildIdentity(userInfo, token)

	return identity, nil
}

// RefreshToken refreshes the access token
func (s *SocialProvider) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()

	if !s.configured {
		return nil, fmt.Errorf("social provider not configured")
	}

	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token required")
	}

	params := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed: %s", string(body))
	}

	var oauthToken OAuth2Token
	if err := json.NewDecoder(resp.Body).Decode(&oauthToken); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  oauthToken.AccessToken,
		RefreshToken: oauthToken.RefreshToken,
		TokenType:    oauthToken.TokenType,
		ExpiresIn:    oauthToken.ExpiresIn,
		ExpiresAt:    time.Now().Add(time.Duration(oauthToken.ExpiresIn) * time.Second),
	}, nil
}

// Logout logs out the user (revokes token if supported)
func (s *SocialProvider) Logout(ctx context.Context, token string) error {
	// Token revocation is provider-specific
	// Most providers don't require explicit logout
	return nil
}

// exchangeCode exchanges authorization code for access token
func (s *SocialProvider) exchangeCode(ctx context.Context, code string) (*OAuth2Token, error) {
	params := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {s.redirectURL},
		"client_id":    {s.clientID},
		"client_secret": {s.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// GitHub requires Accept header for JSON response
	if s.provider == SocialProviderGitHub {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var token OAuth2Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	return &token, nil
}

// getUserInfo retrieves user information from the provider
func (s *SocialProvider) getUserInfo(ctx context.Context, accessToken string) (*SocialUserInfo, error) {
	if s.userInfoURL == "" {
		// Apple uses ID token instead of userinfo
		return &SocialUserInfo{}, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", s.userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo request failed: %s", string(body))
	}

	var userInfo SocialUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	// Handle provider-specific field mappings
	switch s.provider {
	case SocialProviderGitHub:
		// GitHub uses "login" instead of "name"
		if userInfo.Name == "" && userInfo.Login != "" {
			userInfo.Name = userInfo.Login
		}
		// GitHub might not return email in public info
		if userInfo.Email == "" {
			userInfo.Email = s.getGitHubEmail(ctx, accessToken)
		}
	case SocialProviderFacebook:
		// Facebook might have different structure
		if userInfo.ID != "" && userInfo.Email == "" {
			userInfo.Email = userInfo.ID + "@facebook.com"
		}
	}

	return &userInfo, nil
}

// getGitHubEmail fetches the primary email for GitHub users
func (s *SocialProvider) getGitHubEmail(ctx context.Context, accessToken string) string {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return ""
	}

	for _, email := range emails {
		if email.Primary && email.Verified {
			return email.Email
		}
	}

	// Fallback to first verified email
	for _, email := range emails {
		if email.Verified {
			return email.Email
		}
	}

	return ""
}

// buildIdentity builds an Identity from social user info
func (s *SocialProvider) buildIdentity(userInfo *SocialUserInfo, token *OAuth2Token) *Identity {
	identity := &Identity{
		ID:              userInfo.ID,
		Subject:         userInfo.Sub,
		Email:           userInfo.Email,
		Name:            userInfo.Name,
		GivenName:       userInfo.GivenName,
		FamilyName:      userInfo.FamilyName,
		Provider:        ProviderTypeSocial,
		ProviderName:    s.name,
		AuthenticatedAt: time.Now(),
		RawClaims:       make(map[string]interface{}),
	}

	// Handle different ID fields
	if identity.ID == "" {
		identity.ID = userInfo.Sub
	}
	if identity.ID == "" && userInfo.Login != "" {
		identity.ID = userInfo.Login
	}
	if identity.ID == "" && userInfo.Username != "" {
		identity.ID = userInfo.Username
	}

	// Set subject
	if identity.Subject == "" {
		identity.Subject = identity.ID
	}

	// Set name from login if empty
	if identity.Name == "" && userInfo.Login != "" {
		identity.Name = userInfo.Login
	}
	if identity.Name == "" && userInfo.Username != "" {
		identity.Name = userInfo.Username
	}

	// Set name from given_name + family_name if still empty
	if identity.Name == "" {
		if userInfo.GivenName != "" || userInfo.FamilyName != "" {
			identity.Name = strings.TrimSpace(userInfo.GivenName + " " + userInfo.FamilyName)
		}
	}

	// Set raw claims
	identity.RawClaims["provider"] = s.provider
	identity.RawClaims["picture"] = userInfo.Picture
	if userInfo.Picture == "" {
		identity.RawClaims["picture"] = userInfo.AvatarURL
	}

	// Set token expiration
	if token.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
		identity.ExpiresAt = &expiresAt
	}

	return identity
}

// cleanupStates periodically removes expired states
func (s *SocialProvider) cleanupStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.stateMu.Lock()
		now := time.Now()
		for state, expiry := range s.states {
			if now.After(expiry) {
				delete(s.states, state)
			}
		}
		s.stateMu.Unlock()
	}
}

// GetProvider returns the social provider type
func (s *SocialProvider) GetProvider() SocialProviderType {
	return s.provider
}

// SupportsProvider checks if a provider type is supported
func SupportsProvider(provider SocialProviderType) bool {
	_, ok := providerEndpoints[provider]
	return ok
}

// GetSupportedProviders returns list of supported providers
func GetSupportedProviders() []SocialProviderType {
	providers := make([]SocialProviderType, 0, len(providerEndpoints))
	for provider := range providerEndpoints {
		providers = append(providers, provider)
	}
	return providers
}
