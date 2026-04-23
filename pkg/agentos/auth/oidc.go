package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OIDCProvider implements OpenID Connect authentication
type OIDCProvider struct {
	name string

	// OIDC endpoints
	issuer      string
	authURL     string
	tokenURL    string
	userInfoURL string
	jwksURL     string

	// OAuth2 credentials
	clientID     string
	clientSecret string
	redirectURL  string
	scopes       []string

	// PKCE support
	usePKCE bool

	// State management
	stateMu sync.RWMutex
	states  map[string]*stateData

	// Configuration
	configured bool
	configMu   sync.RWMutex
}

// stateData holds state with expiration and PKCE verifier
type stateData struct {
	Expiry   time.Time
	Verifier string // PKCE code verifier
	Nonce    string // OIDC nonce
}

// OIDCConfig represents OIDC provider configuration
type OIDCConfig struct {
	Issuer       string   `json:"issuer"`
	AuthURL      string   `json:"auth_url,omitempty"`
	TokenURL     string   `json:"token_url,omitempty"`
	UserInfoURL  string   `json:"userinfo_url,omitempty"`
	JWKSURL      string   `json:"jwks_url,omitempty"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURL  string   `json:"redirect_url"`
	Scopes       []string `json:"scopes,omitempty"`
	UsePKCE      bool     `json:"use_pkce,omitempty"`
}

// OIDCDiscovery represents OIDC discovery document
type OIDCDiscovery struct {
	Issuer                string   `json:"issuer"`
	AuthURL               string   `json:"authorization_endpoint"`
	TokenURL              string   `json:"token_endpoint"`
	UserInfoURL           string   `json:"userinfo_endpoint"`
	JWKSURL               string   `json:"jwks_uri"`
	EndSessionURL         string   `json:"end_session_endpoint,omitempty"`
	ScopesSupported       []string `json:"scopes_supported,omitempty"`
	ClaimsSupported       []string `json:"claims_supported,omitempty"`
	GrantTypesSupported   []string `json:"grant_types_supported,omitempty"`
	ResponseTypesSupported []string `json:"response_types_supported,omitempty"`
}

// IDToken represents OpenID Connect ID token claims (parsed from JSON)
type IDToken struct {
	Issuer      string   `json:"iss"`
	Subject     string   `json:"sub"`
	Audience    []string `json:"aud"`
	Expiration  int64    `json:"exp"`
	IssuedAt    int64    `json:"iat"`
	AuthTime    int64    `json:"auth_time,omitempty"`
	Nonce       string   `json:"nonce,omitempty"`
	ACR         string   `json:"acr,omitempty"`
	AMR         []string `json:"amr,omitempty"`
	Azp         string   `json:"azp,omitempty"`

	// Profile claims
	Name              string `json:"name,omitempty"`
	GivenName         string `json:"given_name,omitempty"`
	FamilyName        string `json:"family_name,omitempty"`
	MiddleName        string `json:"middle_name,omitempty"`
	Nickname          string `json:"nickname,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Profile           string `json:"profile,omitempty"`
	Picture           string `json:"picture,omitempty"`
	Website           string `json:"website,omitempty"`
	Gender            string `json:"gender,omitempty"`
	Birthdate         string `json:"birthdate,omitempty"`
	Zoneinfo          string `json:"zoneinfo,omitempty"`
	Locale            string `json:"locale,omitempty"`

	// Email claims
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`

	// Phone claims
	PhoneNumber         string `json:"phone_number,omitempty"`
	PhoneNumberVerified bool   `json:"phone_number_verified,omitempty"`

	// Address claims
	Address OIDCAddress `json:"address,omitempty"`

	// Custom claims
	RawClaims map[string]interface{} `json:"-"`
}

// OIDCAddress represents OIDC address claim
type OIDCAddress struct {
	Formatted     string `json:"formatted,omitempty"`
	StreetAddress string `json:"street_address,omitempty"`
	Locality      string `json:"locality,omitempty"`
	Region        string `json:"region,omitempty"`
	PostalCode    string `json:"postal_code,omitempty"`
	Country       string `json:"country,omitempty"`
}

// NewOIDCProvider creates a new OIDC provider
func NewOIDCProvider(name string) *OIDCProvider {
	return &OIDCProvider{
		name:     name,
		states:   make(map[string]*stateData),
		usePKCE:  true,
		scopes:   []string{"openid", "email", "profile"},
	}
}

// Type returns the provider type
func (o *OIDCProvider) Type() ProviderType {
	return ProviderTypeOIDC
}

// Name returns the provider name
func (o *OIDCProvider) Name() string {
	return o.name
}

// Configure sets up the OIDC provider
func (o *OIDCProvider) Configure(config ProviderConfig) error {
	if config.Type != ProviderTypeOIDC && config.Type != ProviderTypeOAuth2 {
		return fmt.Errorf("invalid provider type: %s", config.Type)
	}

	o.configMu.Lock()
	defer o.configMu.Unlock()

	// Parse required configuration
	if issuer, ok := config.Config["issuer"].(string); ok && issuer != "" {
		o.issuer = issuer
	} else if config.Type == ProviderTypeOIDC {
		return fmt.Errorf("issuer is required for OIDC")
	}

	if clientID, ok := config.Config["client_id"].(string); ok {
		o.clientID = clientID
	}

	if clientSecret, ok := config.Config["client_secret"].(string); ok {
		o.clientSecret = clientSecret
	}

	if redirectURL, ok := config.Config["redirect_url"].(string); ok {
		o.redirectURL = redirectURL
	}

	// Parse scopes
	if scopes, ok := config.Config["scopes"].([]interface{}); ok {
		o.scopes = make([]string, 0, len(scopes))
		for _, scope := range scopes {
			if s, ok := scope.(string); ok {
				o.scopes = append(o.scopes, s)
			}
		}
	}

	// Check if PKCE should be used
	if usePKCE, ok := config.Config["use_pkce"].(bool); ok {
		o.usePKCE = usePKCE
	}

	// If issuer is set, try discovery
	if o.issuer != "" {
		if err := o.discoverEndpoints(); err != nil {
			// Discovery failed, try manual endpoints
			if config.Config["auth_url"] == nil {
				return fmt.Errorf("failed to discover OIDC endpoints: %w", err)
			}
		}
	}

	// Parse manual endpoints (override discovery)
	if authURL, ok := config.Config["auth_url"].(string); ok && authURL != "" {
		o.authURL = authURL
	}

	if tokenURL, ok := config.Config["token_url"].(string); ok && tokenURL != "" {
		o.tokenURL = tokenURL
	}

	if userInfoURL, ok := config.Config["userinfo_url"].(string); ok && userInfoURL != "" {
		o.userInfoURL = userInfoURL
	}

	if jwksURL, ok := config.Config["jwks_url"].(string); ok && jwksURL != "" {
		o.jwksURL = jwksURL
	}

	// Validate required fields
	if o.clientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if o.clientSecret == "" {
		return fmt.Errorf("client_secret is required")
	}
	if o.redirectURL == "" {
		return fmt.Errorf("redirect_url is required")
	}
	if o.authURL == "" {
		return fmt.Errorf("auth_url is required (or issuer for discovery)")
	}
	if o.tokenURL == "" {
		return fmt.Errorf("token_url is required (or issuer for discovery)")
	}

	o.configured = true

	// Start state cleanup
	go o.cleanupStates()

	return nil
}

// IsConfigured returns true if the provider is configured
func (o *OIDCProvider) IsConfigured() bool {
	o.configMu.RLock()
	defer o.configMu.RUnlock()
	return o.configured
}

// Authenticate authenticates using OIDC
func (o *OIDCProvider) Authenticate(ctx context.Context, credentials Credentials) (*Identity, error) {
	return nil, fmt.Errorf("OIDC uses redirect flow - use GetLoginURL and HandleCallback")
}

// Authorize checks if the user has permission
func (o *OIDCProvider) Authorize(ctx context.Context, identity *Identity, resource string, action string) (bool, error) {
	return false, fmt.Errorf("OIDC provider doesn't handle authorization")
}

// GetLoginURL returns the OIDC login URL
func (o *OIDCProvider) GetLoginURL(state string) (string, error) {
	o.configMu.RLock()
	defer o.configMu.RUnlock()

	if !o.configured {
		return "", fmt.Errorf("OIDC provider not configured")
	}

	// Generate PKCE code verifier if using PKCE
	var codeVerifier string
	if o.usePKCE {
		codeVerifier = generateCodeVerifier()
	}

	// Generate nonce
	nonce := generateNonce()

	// Store state
	o.stateMu.Lock()
	o.states[state] = &stateData{
		Expiry:   time.Now().Add(10 * time.Minute),
		Verifier: codeVerifier,
		Nonce:    nonce,
	}
	o.stateMu.Unlock()

	// Build authorization URL
	params := url.Values{
		"client_id":     {o.clientID},
		"redirect_uri":  {o.redirectURL},
		"response_type": {"code"},
		"scope":         {strings.Join(o.scopes, " ")},
		"state":         {state},
	}

	// Add PKCE challenge if enabled
	if o.usePKCE && codeVerifier != "" {
		challenge := generateCodeChallenge(codeVerifier)
		params.Set("code_challenge", challenge)
		params.Set("code_challenge_method", "S256")
	}

	// Add nonce
	params.Set("nonce", nonce)

	return o.authURL + "?" + params.Encode(), nil
}

// HandleCallback handles the OIDC callback
func (o *OIDCProvider) HandleCallback(ctx context.Context, code string, state string) (*Identity, error) {
	o.configMu.RLock()
	defer o.configMu.RUnlock()

	if !o.configured {
		return nil, fmt.Errorf("OIDC provider not configured")
	}

	// Verify state
	o.stateMu.Lock()
	stateData, ok := o.states[state]
	if ok {
		delete(o.states, state)
	}
	o.stateMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("invalid state parameter")
	}

	if time.Now().After(stateData.Expiry) {
		return nil, fmt.Errorf("state expired")
	}

	// Exchange code for token
	token, err := o.exchangeCode(ctx, code, stateData.Verifier)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Parse and verify ID token if present
	var identity *Identity
	if token.IDToken != "" {
		idToken, err := o.parseIDToken(ctx, token.IDToken)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ID token: %w", err)
		}

		// Verify nonce
		if idToken.Nonce != stateData.Nonce {
			return nil, fmt.Errorf("nonce mismatch")
		}

		identity = o.buildIdentityFromIDToken(idToken)
	}

	// If no ID token or we need more info, call userinfo
	if identity == nil || o.userInfoURL != "" {
		userInfo, err := o.getUserInfo(ctx, token.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to get user info: %w", err)
		}

		if identity == nil {
			identity = o.buildIdentityFromUserInfo(userInfo, token)
		} else {
			// Merge user info
			o.mergeUserInfo(identity, userInfo)
		}
	}

	return identity, nil
}

// RefreshToken refreshes the access token
func (o *OIDCProvider) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	o.configMu.RLock()
	defer o.configMu.RUnlock()

	if !o.configured {
		return nil, fmt.Errorf("OIDC provider not configured")
	}

	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token required")
	}

	params := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {o.clientID},
		"client_secret": {o.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.tokenURL, strings.NewReader(params.Encode()))
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
		IDToken:      oauthToken.IDToken,
		TokenType:    oauthToken.TokenType,
		ExpiresIn:    oauthToken.ExpiresIn,
		ExpiresAt:    time.Now().Add(time.Duration(oauthToken.ExpiresIn) * time.Second),
	}, nil
}

// Logout logs out the user
func (o *OIDCProvider) Logout(ctx context.Context, token string) error {
	// OIDC logout is provider-specific
	// Some providers support end_session_endpoint
	return nil
}

// discoverEndpoints fetches OIDC discovery document
func (o *OIDCProvider) discoverEndpoints() error {
	discoveryURL := strings.TrimSuffix(o.issuer, "/") + "/.well-known/openid-configuration"

	resp, err := http.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("failed to fetch discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery returned status %d", resp.StatusCode)
	}

	var discovery OIDCDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return fmt.Errorf("failed to parse discovery document: %w", err)
	}

	o.authURL = discovery.AuthURL
	o.tokenURL = discovery.TokenURL
	o.userInfoURL = discovery.UserInfoURL
	o.jwksURL = discovery.JWKSURL

	return nil
}

// exchangeCode exchanges authorization code for tokens
func (o *OIDCProvider) exchangeCode(ctx context.Context, code, codeVerifier string) (*OAuth2Token, error) {
	params := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {o.redirectURL},
		"client_id":    {o.clientID},
		"client_secret": {o.clientSecret},
	}

	if codeVerifier != "" {
		params.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.tokenURL, strings.NewReader(params.Encode()))
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
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var token OAuth2Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	return &token, nil
}

// getUserInfo retrieves user information from userinfo endpoint
func (o *OIDCProvider) getUserInfo(ctx context.Context, accessToken string) (*SocialUserInfo, error) {
	if o.userInfoURL == "" {
		return &SocialUserInfo{}, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", o.userInfoURL, nil)
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
		return nil, fmt.Errorf("userinfo returned status %d", resp.StatusCode)
	}

	var userInfo SocialUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

// parseIDToken parses and validates an ID token (simplified - extracts claims from JWT payload)
func (o *OIDCProvider) parseIDToken(ctx context.Context, tokenString string) (*IDToken, error) {
	// Parse JWT token (simplified - just extract payload)
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	// Decode payload (middle part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode token payload: %w", err)
	}

	var claims IDToken
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	// Validate claims
	if claims.Issuer != "" && claims.Issuer != o.issuer {
		return nil, fmt.Errorf("invalid issuer: %s", claims.Issuer)
	}

	// Check expiration
	if claims.Expiration > 0 && time.Now().Unix() > claims.Expiration {
		return nil, fmt.Errorf("token expired")
	}

	// Check audience
	if len(claims.Audience) > 0 {
		found := false
		for _, aud := range claims.Audience {
			if aud == o.clientID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("invalid audience")
		}
	}

	return &claims, nil
}

// buildIdentityFromIDToken builds identity from ID token claims
func (o *OIDCProvider) buildIdentityFromIDToken(token *IDToken) *Identity {
	identity := &Identity{
		ID:                token.Subject,
		Subject:           token.Subject,
		Email:             token.Email,
		Name:              token.Name,
		GivenName:         token.GivenName,
		FamilyName:        token.FamilyName,
		PreferredUsername: token.PreferredUsername,
		Provider:          ProviderTypeOIDC,
		ProviderName:      o.name,
		AuthenticatedAt:   time.Unix(token.AuthTime, 0),
		RawClaims:         make(map[string]interface{}),
	}

	// Set expiration
	if token.Expiration > 0 {
		expiresAt := time.Unix(token.Expiration, 0)
		identity.ExpiresAt = &expiresAt
	}

	// Set name from given_name + family_name if empty
	if identity.Name == "" {
		if token.GivenName != "" || token.FamilyName != "" {
			identity.Name = strings.TrimSpace(token.GivenName + " " + token.FamilyName)
		}
	}

	// Set name from nickname if still empty
	if identity.Name == "" && token.Nickname != "" {
		identity.Name = token.Nickname
	}

	return identity
}

// buildIdentityFromUserInfo builds identity from userinfo response
func (o *OIDCProvider) buildIdentityFromUserInfo(userInfo *SocialUserInfo, token *OAuth2Token) *Identity {
	identity := &Identity{
		ID:              userInfo.ID,
		Subject:         userInfo.Sub,
		Email:           userInfo.Email,
		Name:            userInfo.Name,
		GivenName:       userInfo.GivenName,
		FamilyName:      userInfo.FamilyName,
		Provider:        ProviderTypeOIDC,
		ProviderName:    o.name,
		AuthenticatedAt: time.Now(),
		RawClaims:       make(map[string]interface{}),
	}

	// Set ID from sub if empty
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

	// Set expiration from token
	if token.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
		identity.ExpiresAt = &expiresAt
	}

	return identity
}

// mergeUserInfo merges user info into identity
func (o *OIDCProvider) mergeUserInfo(identity *Identity, userInfo *SocialUserInfo) {
	if identity.Name == "" && userInfo.Name != "" {
		identity.Name = userInfo.Name
	}
	if identity.Email == "" && userInfo.Email != "" {
		identity.Email = userInfo.Email
	}
	if identity.GivenName == "" && userInfo.GivenName != "" {
		identity.GivenName = userInfo.GivenName
	}
	if identity.FamilyName == "" && userInfo.FamilyName != "" {
		identity.FamilyName = userInfo.FamilyName
	}
}

// cleanupStates periodically removes expired states
func (o *OIDCProvider) cleanupStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		o.stateMu.Lock()
		now := time.Now()
		for state, data := range o.states {
			if now.After(data.Expiry) {
				delete(o.states, state)
			}
		}
		o.stateMu.Unlock()
	}
}

// generateCodeVerifier generates a PKCE code verifier
func generateCodeVerifier() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// generateCodeChallenge generates a PKCE code challenge
func generateCodeChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// generateNonce generates a random nonce
func generateNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}
