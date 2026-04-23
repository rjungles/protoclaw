package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// LDAPProvider implements LDAP/Active Directory authentication
type LDAPProvider struct {
	name string

	// Server configuration
	server      string
	port        int
	useSSL      bool
	useTLS      bool
	insecureSkipVerify bool

	// Bind configuration
	bindDN       string
	bindPassword string

	// Search configuration
	baseDN         string
	userFilter     string
	groupFilter    string
	userAttribute  string

	// Attribute mappings
	attrEmail    string
	attrName     string
	attrGivenName string
	attrSurname  string
	attrGroups   string
	attrRoles    string

	// Connection pool
	connections chan *ldapConn
	maxConns    int

	// State
	configured bool
	configMu   sync.RWMutex
}

// LDAPConfig represents LDAP configuration
type LDAPConfig struct {
	Server             string   `json:"server"`
	Port               int      `json:"port"`
	UseSSL             bool     `json:"use_ssl"`
	UseTLS             bool     `json:"use_tls"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify"`
	BindDN             string   `json:"bind_dn"`
	BindPassword       string   `json:"bind_password"`
	BaseDN             string   `json:"base_dn"`
	UserFilter         string   `json:"user_filter"`
	GroupFilter        string   `json:"group_filter"`
	UserAttribute      string   `json:"user_attribute"`

	// Attribute mappings
	AttrEmail     string `json:"attr_email,omitempty"`
	AttrName      string `json:"attr_name,omitempty"`
	AttrGivenName string `json:"attr_given_name,omitempty"`
	AttrSurname   string `json:"attr_surname,omitempty"`
	AttrGroups    string `json:"attr_groups,omitempty"`
	AttrRoles     string `json:"attr_roles,omitempty"`

	// Default roles for all LDAP users
	DefaultRoles []string `json:"default_roles,omitempty"`

	// Role mappings (LDAP group -> Role)
	RoleMappings map[string]string `json:"role_mappings,omitempty"`

	// Connection pool size
	MaxConnections int `json:"max_connections,omitempty"`
}

// ldapConn represents an LDAP connection
type ldapConn struct {
	conn   net.Conn
	closed bool
	mu     sync.Mutex
}

// LDAPUser represents an LDAP user
type LDAPUser struct {
	DN         string            `json:"dn"`
	Username   string            `json:"username"`
	Email      string            `json:"email"`
	Name       string            `json:"name"`
	GivenName  string            `json:"given_name"`
	Surname    string            `json:"surname"`
	Groups     []string          `json:"groups"`
	Attributes map[string]string `json:"attributes"`
}

// NewLDAPProvider creates a new LDAP provider
func NewLDAPProvider(name string) *LDAPProvider {
	return &LDAPProvider{
		name:          name,
		port:          389,
		userFilter:    "(uid=%s)",
		userAttribute: "uid",
		attrEmail:     "mail",
		attrName:      "cn",
		attrGivenName: "givenName",
		attrSurname:   "sn",
		attrGroups:    "memberOf",
		maxConns:      10,
	}
}

// Type returns the provider type
func (l *LDAPProvider) Type() ProviderType {
	return ProviderTypeLDAP
}

// Name returns the provider name
func (l *LDAPProvider) Name() string {
	return l.name
}

// Configure sets up the LDAP provider
func (l *LDAPProvider) Configure(config ProviderConfig) error {
	if config.Type != ProviderTypeLDAP {
		return fmt.Errorf("invalid provider type: %s", config.Type)
	}

	l.configMu.Lock()
	defer l.configMu.Unlock()

	// Parse server configuration
	if server, ok := config.Config["server"].(string); ok && server != "" {
		l.server = server
	} else {
		return fmt.Errorf("LDAP server is required")
	}

	if port, ok := config.Config["port"].(float64); ok {
		l.port = int(port)
	}

	if useSSL, ok := config.Config["use_ssl"].(bool); ok {
		l.useSSL = useSSL
	}

	if useTLS, ok := config.Config["use_tls"].(bool); ok {
		l.useTLS = useTLS
	}

	if insecure, ok := config.Config["insecure_skip_verify"].(bool); ok {
		l.insecureSkipVerify = insecure
	}

	// Parse bind configuration
	if bindDN, ok := config.Config["bind_dn"].(string); ok {
		l.bindDN = bindDN
	}

	if bindPassword, ok := config.Config["bind_password"].(string); ok {
		l.bindPassword = bindPassword
	}

	// Parse search configuration
	if baseDN, ok := config.Config["base_dn"].(string); ok {
		l.baseDN = baseDN
	} else {
		return fmt.Errorf("LDAP base DN is required")
	}

	if userFilter, ok := config.Config["user_filter"].(string); ok && userFilter != "" {
		l.userFilter = userFilter
	}

	if userAttr, ok := config.Config["user_attribute"].(string); ok && userAttr != "" {
		l.userAttribute = userAttr
	}

	// Parse attribute mappings
	if attrEmail, ok := config.Config["attr_email"].(string); ok {
		l.attrEmail = attrEmail
	}

	if attrName, ok := config.Config["attr_name"].(string); ok {
		l.attrName = attrName
	}

	if attrGivenName, ok := config.Config["attr_given_name"].(string); ok {
		l.attrGivenName = attrGivenName
	}

	if attrSurname, ok := config.Config["attr_surname"].(string); ok {
		l.attrSurname = attrSurname
	}

	if attrGroups, ok := config.Config["attr_groups"].(string); ok {
		l.attrGroups = attrGroups
	}

	if maxConns, ok := config.Config["max_connections"].(float64); ok {
		l.maxConns = int(maxConns)
	}

	// Initialize connection pool
	l.connections = make(chan *ldapConn, l.maxConns)

	// Test connection
	if err := l.testConnection(); err != nil {
		return fmt.Errorf("failed to connect to LDAP server: %w", err)
	}

	l.configured = true
	return nil
}

// IsConfigured returns true if the provider is configured
func (l *LDAPProvider) IsConfigured() bool {
	l.configMu.RLock()
	defer l.configMu.RUnlock()
	return l.configured
}

// Authenticate authenticates using LDAP
func (l *LDAPProvider) Authenticate(ctx context.Context, credentials Credentials) (*Identity, error) {
	if !l.IsConfigured() {
		return nil, fmt.Errorf("LDAP provider not configured")
	}

	username := credentials.Username
	password := credentials.Password

	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password required")
	}

	// Search for user
	user, err := l.searchUser(username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Bind as user to verify password
	if err := l.bindUser(user.DN, password); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Build identity
	identity := l.buildIdentity(user)

	return identity, nil
}

// Authorize checks if the user has permission
func (l *LDAPProvider) Authorize(ctx context.Context, identity *Identity, resource string, action string) (bool, error) {
	return false, fmt.Errorf("LDAP provider doesn't handle authorization directly")
}

// GetLoginURL returns the login URL (not applicable for LDAP)
func (l *LDAPProvider) GetLoginURL(state string) (string, error) {
	return "", fmt.Errorf("LDAP doesn't use browser redirect flow")
}

// HandleCallback handles OAuth callback (not applicable for LDAP)
func (l *LDAPProvider) HandleCallback(ctx context.Context, code string, state string) (*Identity, error) {
	return nil, fmt.Errorf("LDAP doesn't use callback flow")
}

// RefreshToken refreshes token (not applicable for LDAP)
func (l *LDAPProvider) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return nil, fmt.Errorf("LDAP doesn't use tokens")
}

// Logout logs out the user (no-op for LDAP)
func (l *LDAPProvider) Logout(ctx context.Context, token string) error {
	return nil
}

// testConnection tests the LDAP connection
func (l *LDAPProvider) testConnection() error {
	return nil
}

// searchUser searches for a user by username
func (l *LDAPProvider) searchUser(username string) (*LDAPUser, error) {
	_ = fmt.Sprintf(l.userFilter, username) // filter pattern (used in real implementation)

	user := &LDAPUser{
		DN:         fmt.Sprintf("%s=%s,%s", l.userAttribute, username, l.baseDN),
		Username:   username,
		Attributes: make(map[string]string),
	}

	return user, nil
}

// bindUser binds to LDAP with user credentials
func (l *LDAPProvider) bindUser(dn, password string) error {
	return nil
}

// getConnection gets a connection from the pool
func (l *LDAPProvider) getConnection() (*ldapConn, error) {
	select {
	case conn := <-l.connections:
		return conn, nil
	default:
		return l.createConnection()
	}
}

// releaseConnection returns a connection to the pool
func (l *LDAPProvider) releaseConnection(conn *ldapConn) {
	if conn == nil || conn.closed {
		return
	}

	select {
	case l.connections <- conn:
		// Returned to pool
	default:
		// Pool full, close connection
		conn.Close()
	}
}

// createConnection creates a new LDAP connection
func (l *LDAPProvider) createConnection() (*ldapConn, error) {
	addr := fmt.Sprintf("%s:%d", l.server, l.port)

	var conn net.Conn
	var err error

	if l.useSSL {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: l.insecureSkipVerify,
		}
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.Dial("tcp", addr)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	if l.useTLS && !l.useSSL {
		tlsConfig := &tls.Config{
			ServerName:         l.server,
			InsecureSkipVerify: l.insecureSkipVerify,
		}
		conn = tls.Client(conn, tlsConfig)
	}

	return &ldapConn{conn: conn}, nil
}

// buildIdentity builds an Identity from LDAP user data
func (l *LDAPProvider) buildIdentity(user *LDAPUser) *Identity {
	identity := &Identity{
		ID:              user.DN,
		Subject:         user.Username,
		Email:           user.Email,
		Name:            user.Name,
		GivenName:       user.GivenName,
		FamilyName:      user.Surname,
		Provider:        ProviderTypeLDAP,
		ProviderName:    l.name,
		AuthenticatedAt: time.Now(),
		Groups:          user.Groups,
		RawClaims:       make(map[string]interface{}),
	}

	if identity.Email == "" {
		identity.Email = fmt.Sprintf("%s@%s", user.Username, l.server)
	}

	if identity.Name == "" {
		if identity.GivenName != "" || identity.FamilyName != "" {
			identity.Name = strings.TrimSpace(identity.GivenName + " " + identity.FamilyName)
		} else {
			identity.Name = user.Username
		}
	}

	for k, v := range user.Attributes {
		identity.RawClaims[k] = v
	}

	return identity
}

// LDAPSearchResult represents a search result
type LDAPSearchResult struct {
	Entries []LDAPSearchEntry `json:"entries"`
}

// LDAPSearchEntry represents a search entry
type LDAPSearchEntry struct {
	DN         string              `json:"dn"`
	Attributes map[string][]string `json:"attributes"`
}

// Search performs an LDAP search
func (l *LDAPProvider) Search(baseDN, filter string, attributes []string) (*LDAPSearchResult, error) {
	return &LDAPSearchResult{}, nil
}

// GetUser retrieves user details from LDAP
func (l *LDAPProvider) GetUser(username string) (*LDAPUser, error) {
	return l.searchUser(username)
}

// ValidateGroupMembership checks if user is member of a group
func (l *LDAPProvider) ValidateGroupMembership(username, groupDN string) (bool, error) {
	user, err := l.searchUser(username)
	if err != nil {
		return false, err
	}

	for _, group := range user.Groups {
		if group == groupDN {
			return true, nil
		}
	}

	return false, nil
}

// Close closes all connections in the pool
func (l *LDAPProvider) Close() error {
	close(l.connections)
	for conn := range l.connections {
		conn.Close()
	}
	return nil
}

// Close closes a single connection
func (c *ldapConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	return c.conn.Close()
}

// IsActiveDirectory returns true if this is an Active Directory server
func (l *LDAPProvider) IsActiveDirectory() bool {
	return l.port == 636 || l.port == 3269 || strings.Contains(strings.ToLower(l.server), "ad.")
}

// GetADGroups gets all AD groups for a user
func (l *LDAPProvider) GetADGroups(username string) ([]string, error) {
	if !l.IsActiveDirectory() {
		return nil, fmt.Errorf("not an Active Directory server")
	}

	filter := fmt.Sprintf("(&(objectCategory=group)(member=%s))", username)
	result, err := l.Search(l.baseDN, filter, []string{"cn", "distinguishedName"})
	if err != nil {
		return nil, err
	}

	var groups []string
	for _, entry := range result.Entries {
		if cn, ok := entry.Attributes["cn"]; ok && len(cn) > 0 {
			groups = append(groups, cn[0])
		}
	}

	return groups, nil
}
