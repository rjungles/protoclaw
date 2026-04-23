package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"
)

// randReader is the source of randomness for auth operations
var randReader = rand.Reader

// BasicAuthProvider implements HTTP Basic authentication
type BasicAuthProvider struct {
	name     string
	realm    string
	users    map[string]*BasicAuthUser
	usersMu  sync.RWMutex
	configured bool
}

// BasicAuthUser represents a user for Basic Auth
type BasicAuthUser struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	Salt         string    `json:"salt"`
	Identity     *Identity `json:"identity"`
	CreatedAt    time.Time `json:"created_at"`
	LastLogin    time.Time `json:"last_login,omitempty"`
}

// BasicAuthConfig represents Basic Auth configuration
type BasicAuthConfig struct {
	Realm string `json:"realm"`
	Users []struct {
		Username string   `json:"username"`
		Password string   `json:"password"`
		Email    string   `json:"email"`
		Name     string   `json:"name"`
		Roles    []string `json:"roles"`
	} `json:"users"`
}

// NewBasicAuthProvider creates a new Basic Auth provider
func NewBasicAuthProvider(name string) *BasicAuthProvider {
	return &BasicAuthProvider{
		name:  name,
		realm: "AgentOS",
		users: make(map[string]*BasicAuthUser),
	}
}

// Type returns the provider type
func (b *BasicAuthProvider) Type() ProviderType {
	return ProviderTypeBasic
}

// Name returns the provider name
func (b *BasicAuthProvider) Name() string {
	return b.name
}

// Configure sets up the Basic Auth provider
func (b *BasicAuthProvider) Configure(config ProviderConfig) error {
	if config.Type != ProviderTypeBasic {
		return fmt.Errorf("invalid provider type: %s", config.Type)
	}

	// Parse config
	realm, ok := config.Config["realm"].(string)
	if ok && realm != "" {
		b.realm = realm
	}

	// Load users from config
	if usersConfig, ok := config.Config["users"].([]interface{}); ok {
		for _, u := range usersConfig {
			if userMap, ok := u.(map[string]interface{}); ok {
				username, _ := userMap["username"].(string)
				password, _ := userMap["password"].(string)
				email, _ := userMap["email"].(string)
				name, _ := userMap["name"].(string)

				var roles []string
				if rolesList, ok := userMap["roles"].([]interface{}); ok {
					for _, r := range rolesList {
						if role, ok := r.(string); ok {
							roles = append(roles, role)
						}
					}
				}

				if err := b.CreateUser(username, password, email, name, roles); err != nil {
					return fmt.Errorf("failed to create user %s: %w", username, err)
				}
			}
		}
	}

	b.configured = true
	return nil
}

// IsConfigured returns true if the provider is configured
func (b *BasicAuthProvider) IsConfigured() bool {
	return b.configured
}

// Authenticate authenticates using Basic Auth
func (b *BasicAuthProvider) Authenticate(ctx context.Context, credentials Credentials) (*Identity, error) {
	if !b.IsConfigured() {
		return nil, fmt.Errorf("basic auth provider not configured")
	}

	username := credentials.Username
	password := credentials.Password

	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password required")
	}

	user, err := b.ValidateCredentials(username, password)
	if err != nil {
		return nil, err
	}

	// Update last login
	b.usersMu.Lock()
	user.LastLogin = time.Now()
	b.usersMu.Unlock()

	return user.Identity, nil
}

// Authorize checks if the user has permission
func (b *BasicAuthProvider) Authorize(ctx context.Context, identity *Identity, resource string, action string) (bool, error) {
	// Basic auth doesn't handle authorization directly
	return false, fmt.Errorf("basic auth provider doesn't handle authorization")
}

// GetLoginURL returns the login URL (not applicable for Basic Auth)
func (b *BasicAuthProvider) GetLoginURL(state string) (string, error) {
	return "", fmt.Errorf("basic auth doesn't use browser redirect flow")
}

// HandleCallback handles OAuth callback (not applicable for Basic Auth)
func (b *BasicAuthProvider) HandleCallback(ctx context.Context, code string, state string) (*Identity, error) {
	return nil, fmt.Errorf("basic auth doesn't use callback flow")
}

// RefreshToken refreshes token (not applicable for Basic Auth)
func (b *BasicAuthProvider) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return nil, fmt.Errorf("basic auth doesn't use tokens")
}

// Logout logs out the user (no-op for Basic Auth)
func (b *BasicAuthProvider) Logout(ctx context.Context, token string) error {
	return nil
}

// CreateUser creates a new user
func (b *BasicAuthProvider) CreateUser(username, password, email, name string, roles []string) error {
	if username == "" || password == "" {
		return fmt.Errorf("username and password required")
	}

	// Generate salt and hash password
	salt := generateSalt()
	hash := hashPassword(password, salt)

	identity := &Identity{
		ID:              username,
		Subject:         username,
		Email:           email,
		Name:            name,
		Provider:        ProviderTypeBasic,
		ProviderName:    b.name,
		AuthenticatedAt: time.Now(),
		Roles:           roles,
	}

	user := &BasicAuthUser{
		Username:     username,
		PasswordHash: hash,
		Salt:         salt,
		Identity:     identity,
		CreatedAt:    time.Now(),
	}

	b.usersMu.Lock()
	b.users[username] = user
	b.usersMu.Unlock()

	return nil
}

// UpdateUser updates a user's password
func (b *BasicAuthProvider) UpdateUser(username, newPassword string) error {
	if newPassword == "" {
		return fmt.Errorf("new password required")
	}

	b.usersMu.Lock()
	defer b.usersMu.Unlock()

	user, ok := b.users[username]
	if !ok {
		return fmt.Errorf("user not found")
	}

	user.Salt = generateSalt()
	user.PasswordHash = hashPassword(newPassword, user.Salt)

	return nil
}

// DeleteUser deletes a user
func (b *BasicAuthProvider) DeleteUser(username string) error {
	b.usersMu.Lock()
	defer b.usersMu.Unlock()

	delete(b.users, username)
	return nil
}

// ValidateCredentials validates username and password
func (b *BasicAuthProvider) ValidateCredentials(username, password string) (*BasicAuthUser, error) {
	b.usersMu.RLock()
	defer b.usersMu.RUnlock()

	user, ok := b.users[username]
	if !ok {
		return nil, fmt.Errorf("invalid credentials")
	}

	hash := hashPassword(password, user.Salt)
	if hash != user.PasswordHash {
		return nil, fmt.Errorf("invalid credentials")
	}

	return user, nil
}

// GetUser retrieves a user by username
func (b *BasicAuthProvider) GetUser(username string) (*BasicAuthUser, error) {
	b.usersMu.RLock()
	defer b.usersMu.RUnlock()

	user, ok := b.users[username]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}

	return user, nil
}

// GetRealm returns the authentication realm
func (b *BasicAuthProvider) GetRealm() string {
	return b.realm
}

// GetWWWAuthenticateHeader returns the WWW-Authenticate header value
func (b *BasicAuthProvider) GetWWWAuthenticateHeader() string {
	return fmt.Sprintf(`Basic realm="%s"`, b.realm)
}

// generateSalt generates a random salt
func generateSalt() string {
	b := make([]byte, 16)
	randReader.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// hashPassword hashes a password with salt
func hashPassword(password, salt string) string {
	h := sha256.New()
	h.Write([]byte(password + salt))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// ParseBasicAuthHeader parses an Authorization header for Basic Auth
func ParseBasicAuthHeader(header string) (username, password string, err error) {
	const prefix = "Basic "
	if !strings.HasPrefix(header, prefix) {
		return "", "", fmt.Errorf("invalid authorization header")
	}

	encoded := header[len(prefix):]
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", fmt.Errorf("failed to decode credentials")
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid credentials format")
	}

	return parts[0], parts[1], nil
}
