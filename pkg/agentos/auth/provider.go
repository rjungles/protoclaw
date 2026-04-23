package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ProviderType represents the type of authentication provider
type ProviderType string

const (
	ProviderTypeOAuth2       ProviderType = "oauth2"
	ProviderTypeOIDC         ProviderType = "oidc"
	ProviderTypeBasic        ProviderType = "basic"
	ProviderTypeSAML         ProviderType = "saml"
	ProviderTypeLDAP         ProviderType = "ldap"
	ProviderTypeSocial       ProviderType = "social"
)

// Provider is the interface for all authentication providers
type Provider interface {
	// Type returns the provider type
	Type() ProviderType

	// Name returns the provider name
	Name() string

	// Configure sets up the provider with configuration
	Configure(config ProviderConfig) error

	// Authenticate authenticates a user and returns their identity
	Authenticate(ctx context.Context, credentials Credentials) (*Identity, error)

	// Authorize checks if the user has permission to access a resource
	Authorize(ctx context.Context, identity *Identity, resource string, action string) (bool, error)

	// GetLoginURL returns the URL for initiating login (for OAuth/OIDC/SAML)
	GetLoginURL(state string) (string, error)

	// HandleCallback handles the callback from the identity provider
	HandleCallback(ctx context.Context, code string, state string) (*Identity, error)

	// RefreshToken refreshes the access token
	RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)

	// Logout logs out the user
	Logout(ctx context.Context, token string) error

	// IsConfigured returns true if the provider is properly configured
	IsConfigured() bool
}

// ProviderConfig represents configuration for an authentication provider
type ProviderConfig struct {
	Type       ProviderType           `json:"type"`
	Name       string                 `json:"name"`
	Enabled    bool                   `json:"enabled"`
	Config     map[string]interface{} `json:"config"`
}

// Credentials represents user credentials
type Credentials struct {
	Username    string                 `json:"username,omitempty"`
	Password    string                 `json:"password,omitempty"`
	Token       string                 `json:"token,omitempty"`
	APIKey      string                 `json:"api_key,omitempty"`
	Code        string                 `json:"code,omitempty"`
	Provider    string                 `json:"provider,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Identity represents an authenticated user identity
type Identity struct {
	ID                string                 `json:"id"`
	Subject           string                 `json:"sub"`
	Email             string                 `json:"email"`
	Name              string                 `json:"name"`
	GivenName         string                 `json:"given_name,omitempty"`
	FamilyName        string                 `json:"family_name,omitempty"`
	PreferredUsername string                 `json:"preferred_username,omitempty"`
	Groups            []string               `json:"groups,omitempty"`
	Roles             []string               `json:"roles,omitempty"`
	Permissions       []string               `json:"permissions,omitempty"`
	Provider          ProviderType           `json:"provider"`
	ProviderName      string                 `json:"provider_name"`
	AuthenticatedAt   time.Time              `json:"authenticated_at"`
	ExpiresAt         *time.Time             `json:"expires_at,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	RawClaims         map[string]interface{} `json:"-"`
}

// TokenPair represents access and refresh tokens
type TokenPair struct {
	AccessToken  string     `json:"access_token"`
	TokenType    string     `json:"token_type"`
	ExpiresIn    int        `json:"expires_in"`
	RefreshToken string     `json:"refresh_token,omitempty"`
	IDToken      string     `json:"id_token,omitempty"`
	ExpiresAt    time.Time  `json:"expires_at"`
}

// Session represents an authenticated session
type Session struct {
	ID           string     `json:"id"`
	IdentityID   string     `json:"identity_id"`
	Provider     string     `json:"provider"`
	Tokens       *TokenPair `json:"tokens,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	LastAccessed time.Time  `json:"last_accessed"`
	IPAddress    string     `json:"ip_address,omitempty"`
	UserAgent    string     `json:"user_agent,omitempty"`
}

// Manager manages multiple authentication providers
type Manager struct {
	providers      map[ProviderType]Provider
	sessions       map[string]*Session
	sessionTimeout time.Duration
}

// NewManager creates a new authentication manager
func NewManager() *Manager {
	return &Manager{
		providers:      make(map[ProviderType]Provider),
		sessions:       make(map[string]*Session),
		sessionTimeout: 24 * time.Hour,
	}
}

// Register registers a provider with the manager
func (m *Manager) Register(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	if !provider.IsConfigured() {
		return fmt.Errorf("provider %s is not configured", provider.Name())
	}

	m.providers[provider.Type()] = provider
	return nil
}

// Get retrieves a provider by type
func (m *Manager) Get(providerType ProviderType) (Provider, error) {
	provider, ok := m.providers[providerType]
	if !ok {
		return nil, fmt.Errorf("provider type %s not found", providerType)
	}
	return provider, nil
}

// GetAll returns all registered providers
func (m *Manager) GetAll() []Provider {
	providers := make([]Provider, 0, len(m.providers))
	for _, p := range m.providers {
		providers = append(providers, p)
	}
	return providers
}

// Authenticate authenticates using the specified provider
func (m *Manager) Authenticate(ctx context.Context, providerType ProviderType, credentials Credentials) (*Identity, error) {
	provider, err := m.Get(providerType)
	if err != nil {
		return nil, err
	}
	return provider.Authenticate(ctx, credentials)
}

// CreateSession creates a new session for an identity
func (m *Manager) CreateSession(identity *Identity, tokens *TokenPair, ipAddress, userAgent string) *Session {
	session := &Session{
		ID:           generateSessionID(),
		IdentityID:   identity.ID,
		Provider:     string(identity.Provider),
		Tokens:       tokens,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(m.sessionTimeout),
		LastAccessed: time.Now(),
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
	}

	if tokens != nil && !tokens.ExpiresAt.IsZero() {
		session.ExpiresAt = tokens.ExpiresAt
	}

	m.sessions[session.ID] = session
	return session
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(sessionID string) (*Session, error) {
	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}

	if time.Now().After(session.ExpiresAt) {
		delete(m.sessions, sessionID)
		return nil, fmt.Errorf("session expired")
	}

	session.LastAccessed = time.Now()
	return session, nil
}

// InvalidateSession invalidates a session
func (m *Manager) InvalidateSession(sessionID string) error {
	delete(m.sessions, sessionID)
	return nil
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("sess_%d_%d", time.Now().UnixNano(), time.Now().Unix())
}

// Middleware creates HTTP middleware for authentication
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, err := m.extractIdentityFromRequest(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), IdentityContextKey, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractIdentityFromRequest extracts identity from HTTP request
func (m *Manager) extractIdentityFromRequest(r *http.Request) (*Identity, error) {
	// Check Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("no authorization header")
	}

	// Bearer token
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token := authHeader[7:]
		return m.validateToken(r.Context(), token)
	}

	// Basic auth
	if len(authHeader) > 6 && authHeader[:6] == "Basic " {
		// Basic auth handling
		return nil, fmt.Errorf("basic auth not implemented")
	}

	return nil, fmt.Errorf("unsupported authorization scheme")
}

// validateToken validates a token and returns the identity
func (m *Manager) validateToken(ctx context.Context, token string) (*Identity, error) {
	// Try to get session by token
	for _, session := range m.sessions {
		if session.Tokens != nil && session.Tokens.AccessToken == token {
			if time.Now().After(session.ExpiresAt) {
				return nil, fmt.Errorf("token expired")
			}
			// Return identity from session
			return &Identity{
				ID:              session.IdentityID,
				Provider:        ProviderType(session.Provider),
				AuthenticatedAt: session.CreatedAt,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid token")
}

// IdentityContextKey is the context key for identity
const IdentityContextKey = "identity"

// GetIdentityFromContext retrieves identity from context
func GetIdentityFromContext(ctx context.Context) (*Identity, bool) {
	identity, ok := ctx.Value(IdentityContextKey).(*Identity)
	return identity, ok
}

// ToJSON converts identity to JSON
func (i *Identity) ToJSON() ([]byte, error) {
	return json.Marshal(i)
}

// HasRole checks if identity has a specific role
func (i *Identity) HasRole(role string) bool {
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasPermission checks if identity has a specific permission
func (i *Identity) HasPermission(permission string) bool {
	for _, p := range i.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}
