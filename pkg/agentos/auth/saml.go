package auth

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// SAMLProvider implements SAML 2.0 authentication
type SAMLProvider struct {
	name string
	
	// Service Provider settings
	spEntityID string
	spACSURL   string
	spMetadataURL string
	
	// Identity Provider settings
	idpMetadataURL string
	idpSsoURL      string
	idpIssuer      string
	
	// Certificates
	spCert *x509.Certificate
	spKey  *rsa.PrivateKey
	idpCert *x509.Certificate
	
	// Configuration
	configured bool
	configMu   sync.RWMutex
}

// SAMLConfig represents SAML configuration
type SAMLConfig struct {
	EntityID       string `json:"entity_id"`
	AssertionConsumerServiceURL string `json:"acs_url"`
	SPMetadataURL  string `json:"sp_metadata_url,omitempty"`
	IDPMetadataURL string `json:"idp_metadata_url"`
	SPCertificate  string `json:"sp_certificate"`
	SPPrivateKey   string `json:"sp_private_key"`
	IDPCertificate string `json:"idp_certificate,omitempty"`
	NameIDFormat   string `json:"name_id_format,omitempty"`
}

// SAMLRequest represents a SAML AuthnRequest
type SAMLRequest struct {
	XMLName    xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:protocol AuthnRequest"`
	ID         string   `xml:"ID,attr"`
	Version    string   `xml:"Version,attr"`
	IssueInstant string `xml:"IssueInstant,attr"`
	Destination string  `xml:"Destination,attr,omitempty"`
	AssertionConsumerServiceURL string `xml:"AssertionConsumerServiceURL,attr"`
	ProtocolBinding string `xml:"ProtocolBinding,attr"`
	Issuer     SAMLIssuer `xml:"Issuer"`
	NameIDPolicy SAMLNameIDPolicy `xml:"NameIDPolicy,omitempty"`
}

// SAMLIssuer represents SAML Issuer element
type SAMLIssuer struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion Issuer"`
	Value   string   `xml:",chardata"`
}

// SAMLNameIDPolicy represents SAML NameIDPolicy element
type SAMLNameIDPolicy struct {
	XMLName         xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:protocol NameIDPolicy"`
	AllowCreate     bool     `xml:"AllowCreate,attr,omitempty"`
	Format          string   `xml:"Format,attr,omitempty"`
}

// SAMLResponse represents a SAML Response
type SAMLResponse struct {
	XMLName      xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:protocol Response"`
	ID           string   `xml:"ID,attr"`
	Version      string   `xml:"Version,attr"`
	IssueInstant string   `xml:"IssueInstant,attr"`
	Destination  string   `xml:"Destination,attr,omitempty"`
	InResponseTo string   `xml:"InResponseTo,attr,omitempty"`
	Status       SAMLStatus `xml:"Status"`
	Assertion    *SAMLAssertion `xml:"Assertion,omitempty"`
	Issuer       SAMLIssuer `xml:"Issuer"`
}

// SAMLStatus represents SAML Status element
type SAMLStatus struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:protocol Status"`
	Code    SAMLStatusCode `xml:"StatusCode"`
	Message string `xml:"StatusMessage,omitempty"`
}

// SAMLStatusCode represents SAML StatusCode element
type SAMLStatusCode struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:protocol StatusCode"`
	Value   string   `xml:"Value,attr"`
}

// SAMLAssertion represents SAML Assertion element
type SAMLAssertion struct {
	XMLName      xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion Assertion"`
	ID           string   `xml:"ID,attr"`
	Version      string   `xml:"Version,attr"`
	IssueInstant string   `xml:"IssueInstant,attr"`
	Issuer       SAMLIssuer `xml:"Issuer"`
	Subject      SAMLSubject `xml:"Subject"`
	Conditions   SAMLConditions `xml:"Conditions"`
	AuthnStatement SAMLAuthnStatement `xml:"AuthnStatement"`
	AttributeStatement SAMLAttributeStatement `xml:"AttributeStatement,omitempty"`
}

// SAMLSubject represents SAML Subject element
type SAMLSubject struct {
	XMLName             xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion Subject"`
	NameID              SAMLNameID `xml:"NameID"`
	SubjectConfirmation SAMLSubjectConfirmation `xml:"SubjectConfirmation"`
}

// SAMLNameID represents SAML NameID element
type SAMLNameID struct {
	XMLName  xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion NameID"`
	Format   string   `xml:"Format,attr,omitempty"`
	Value    string   `xml:",chardata"`
}

// SAMLSubjectConfirmation represents SAML SubjectConfirmation element
type SAMLSubjectConfirmation struct {
	XMLName                 xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion SubjectConfirmation"`
	Method                  string   `xml:"Method,attr"`
	SubjectConfirmationData SAMLSubjectConfirmationData `xml:"SubjectConfirmationData"`
}

// SAMLSubjectConfirmationData represents SAML SubjectConfirmationData element
type SAMLSubjectConfirmationData struct {
	XMLName             xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion SubjectConfirmationData"`
	InResponseTo        string   `xml:"InResponseTo,attr,omitempty"`
	NotOnOrAfter        string   `xml:"NotOnOrAfter,attr,omitempty"`
	Recipient           string   `xml:"Recipient,attr,omitempty"`
}

// SAMLConditions represents SAML Conditions element
type SAMLConditions struct {
	XMLName       xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion Conditions"`
	NotBefore     string   `xml:"NotBefore,attr,omitempty"`
	NotOnOrAfter  string   `xml:"NotOnOrAfter,attr,omitempty"`
}

// SAMLAuthnStatement represents SAML AuthnStatement element
type SAMLAuthnStatement struct {
	XMLName         xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion AuthnStatement"`
	AuthnInstant    string   `xml:"AuthnInstant,attr"`
	SessionIndex    string   `xml:"SessionIndex,attr,omitempty"`
	AuthnContext    SAMLAuthnContext `xml:"AuthnContext"`
}

// SAMLAuthnContext represents SAML AuthnContext element
type SAMLAuthnContext struct {
	XMLName              xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion AuthnContext"`
	AuthnContextClassRef string   `xml:"AuthnContextClassRef"`
}

// SAMLAttributeStatement represents SAML AttributeStatement element
type SAMLAttributeStatement struct {
	XMLName    xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion AttributeStatement"`
	Attributes []SAMLAttribute `xml:"Attribute"`
}

// SAMLAttribute represents SAML Attribute element
type SAMLAttribute struct {
	XMLName      xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion Attribute"`
	Name         string   `xml:"Name,attr"`
	NameFormat   string   `xml:"NameFormat,attr,omitempty"`
	Values       []SAMLAttributeValue `xml:"AttributeValue"`
}

// SAMLAttributeValue represents SAML AttributeValue element
type SAMLAttributeValue struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:assertion AttributeValue"`
	Type    string   `xml:"type,attr,omitempty"`
	Value   string   `xml:",chardata"`
}

// NewSAMLProvider creates a new SAML provider
func NewSAMLProvider(name string) *SAMLProvider {
	return &SAMLProvider{
		name: name,
	}
}

// Type returns the provider type
func (s *SAMLProvider) Type() ProviderType {
	return ProviderTypeSAML
}

// Name returns the provider name
func (s *SAMLProvider) Name() string {
	return s.name
}

// Configure sets up the SAML provider
func (s *SAMLProvider) Configure(config ProviderConfig) error {
	if config.Type != ProviderTypeSAML {
		return fmt.Errorf("invalid provider type: %s", config.Type)
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	// Parse configuration
	if entityID, ok := config.Config["entity_id"].(string); ok {
		s.spEntityID = entityID
	}

	if acsURL, ok := config.Config["acs_url"].(string); ok {
		s.spACSURL = acsURL
	}

	if idpMetadataURL, ok := config.Config["idp_metadata_url"].(string); ok {
		s.idpMetadataURL = idpMetadataURL
		// Fetch IdP metadata
		if err := s.fetchIDPMetadata(); err != nil {
			return fmt.Errorf("failed to fetch IdP metadata: %w", err)
		}
	}

	// Load SP certificate and key
	if certPath, ok := config.Config["sp_certificate_path"].(string); ok {
		if err := s.loadSPCertificate(certPath); err != nil {
			return fmt.Errorf("failed to load SP certificate: %w", err)
		}
	}

	if keyPath, ok := config.Config["sp_private_key_path"].(string); ok {
		if err := s.loadSPPrivateKey(keyPath); err != nil {
			return fmt.Errorf("failed to load SP private key: %w", err)
		}
	}

	// Load IdP certificate
	if idpCertPath, ok := config.Config["idp_certificate_path"].(string); ok {
		if err := s.loadIDPCertificate(idpCertPath); err != nil {
			return fmt.Errorf("failed to load IdP certificate: %w", err)
		}
	}

	s.configured = true
	return nil
}

// IsConfigured returns true if the provider is configured
func (s *SAMLProvider) IsConfigured() bool {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.configured
}

// Authenticate authenticates using SAML (not directly used - flow uses GetLoginURL and HandleCallback)
func (s *SAMLProvider) Authenticate(ctx context.Context, credentials Credentials) (*Identity, error) {
	return nil, fmt.Errorf("SAML uses redirect flow - use GetLoginURL and HandleCallback")
}

// Authorize checks if the user has permission
func (s *SAMLProvider) Authorize(ctx context.Context, identity *Identity, resource string, action string) (bool, error) {
	// SAML doesn't handle authorization directly
	return false, fmt.Errorf("SAML provider doesn't handle authorization")
}

// GetLoginURL returns the SAML login URL
func (s *SAMLProvider) GetLoginURL(relayState string) (string, error) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()

	if !s.configured {
		return "", fmt.Errorf("SAML provider not configured")
	}

	// Create AuthnRequest
	requestID := generateSAMLID()
	authnRequest := SAMLRequest{
		ID:                          requestID,
		Version:                     "2.0",
		IssueInstant:                time.Now().UTC().Format(time.RFC3339),
		Destination:                 s.idpSsoURL,
		AssertionConsumerServiceURL: s.spACSURL,
		ProtocolBinding:             "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST",
		Issuer: SAMLIssuer{Value: s.spEntityID},
		NameIDPolicy: SAMLNameIDPolicy{
			AllowCreate: true,
			Format:      "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
		},
	}

	// Marshal to XML
	requestXML, err := xml.Marshal(authnRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal AuthnRequest: %w", err)
	}

	// Deflate and base64 encode
	deflated, err := deflate(requestXML)
	if err != nil {
		return "", fmt.Errorf("failed to deflate request: %w", err)
	}

	encodedRequest := base64.StdEncoding.EncodeToString(deflated)

	// Build URL with SAMLRequest parameter
	u, err := url.Parse(s.idpSsoURL)
	if err != nil {
		return "", fmt.Errorf("invalid IdP SSO URL: %w", err)
	}

	q := u.Query()
	q.Set("SAMLRequest", encodedRequest)
	if relayState != "" {
		q.Set("RelayState", relayState)
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// HandleCallback handles the SAML ACS callback
func (s *SAMLProvider) HandleCallback(ctx context.Context, samlResponse string, relayState string) (*Identity, error) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()

	if !s.configured {
		return nil, fmt.Errorf("SAML provider not configured")
	}

	// Decode base64 response
	responseXML, err := base64.StdEncoding.DecodeString(samlResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SAML response: %w", err)
	}

	// Parse response
	var response SAMLResponse
	if err := xml.Unmarshal(responseXML, &response); err != nil {
		return nil, fmt.Errorf("failed to parse SAML response: %w", err)
	}

	// Check status
	if response.Status.Code.Value != "urn:oasis:names:tc:SAML:2.0:status:Success" {
		return nil, fmt.Errorf("SAML authentication failed: %s", response.Status.Message)
	}

	if response.Assertion == nil {
		return nil, fmt.Errorf("SAML response missing assertion")
	}

	// Extract identity from assertion
	assertion := response.Assertion
	identity := &Identity{
		ID:              assertion.Subject.NameID.Value,
		Subject:         assertion.Subject.NameID.Value,
		Email:           assertion.Subject.NameID.Value,
		Provider:        ProviderTypeSAML,
		ProviderName:    s.name,
		AuthenticatedAt: time.Now(),
		RawClaims:       make(map[string]interface{}),
	}

	// Extract attributes
	for _, attr := range assertion.AttributeStatement.Attributes {
		switch attr.Name {
		case "email", "Email", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress":
			if len(attr.Values) > 0 {
				identity.Email = attr.Values[0].Value
			}
		case "name", "Name", "displayName", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name":
			if len(attr.Values) > 0 {
				identity.Name = attr.Values[0].Value
			}
		case "given_name", "firstName", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/givenname":
			if len(attr.Values) > 0 {
				identity.GivenName = attr.Values[0].Value
			}
		case "family_name", "surname", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/surname":
			if len(attr.Values) > 0 {
				identity.FamilyName = attr.Values[0].Value
			}
		case "roles", "role", "http://schemas.microsoft.com/ws/2008/06/identity/claims/role":
			for _, val := range attr.Values {
				identity.Roles = append(identity.Roles, val.Value)
			}
		case "groups", "group":
			for _, val := range attr.Values {
				identity.Groups = append(identity.Groups, val.Value)
			}
		}
		
		// Store all attributes in RawClaims
		if len(attr.Values) > 0 {
			identity.RawClaims[attr.Name] = attr.Values[0].Value
		}
	}

	// Set name if empty
	if identity.Name == "" {
		if identity.GivenName != "" || identity.FamilyName != "" {
			identity.Name = strings.TrimSpace(identity.GivenName + " " + identity.FamilyName)
		} else {
			identity.Name = identity.Email
		}
	}

	return identity, nil
}

// RefreshToken is not applicable for SAML
func (s *SAMLProvider) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return nil, fmt.Errorf("SAML doesn't use token refresh")
}

// Logout handles SAML logout
func (s *SAMLProvider) Logout(ctx context.Context, nameID string) error {
	// TODO: Implement SAML logout (SLO)
	return nil
}

// GetMetadata returns the SP metadata XML
func (s *SAMLProvider) GetMetadata() ([]byte, error) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()

	metadata := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s">
  <SPSSODescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress</NameIDFormat>
    <AssertionConsumerService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="%s" index="1"/>
  </SPSSODescriptor>
</EntityDescriptor>`, s.spEntityID, s.spACSURL)

	return []byte(metadata), nil
}

// fetchIDPMetadata fetches and parses IdP metadata
func (s *SAMLProvider) fetchIDPMetadata() error {
	if s.idpMetadataURL == "" {
		return nil
	}

	resp, err := http.Get(s.idpMetadataURL)
	if err != nil {
		return fmt.Errorf("failed to fetch IdP metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("IdP metadata returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read IdP metadata: %w", err)
	}

	// Parse IdP metadata XML
	// This is a simplified implementation
	// In production, you'd want to properly parse the full SAML metadata
	if bytes.Contains(body, []byte("SingleSignOnService")) {
		// Extract SSO URL using simple string parsing
		// Production code should use proper XML parsing
		if s.idpSsoURL == "" {
			// Fallback to config value
			s.idpSsoURL = s.idpMetadataURL
		}
	}

	return nil
}

// loadSPCertificate loads the SP certificate from file
func (s *SAMLProvider) loadSPCertificate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	s.spCert = cert
	return nil
}

// loadSPPrivateKey loads the SP private key from file
func (s *SAMLProvider) loadSPPrivateKey(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("failed to parse key PEM")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8
		keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		key, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("private key is not RSA")
		}
	}

	s.spKey = key
	return nil
}

// loadIDPCertificate loads the IdP certificate from file
func (s *SAMLProvider) loadIDPCertificate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	s.idpCert = cert
	return nil
}

// generateSAMLID generates a unique SAML ID
func generateSAMLID() string {
	return fmt.Sprintf("_%d_%d", time.Now().UnixNano(), time.Now().Unix())
}

// deflate compresses data using deflate
func deflate(data []byte) ([]byte, error) {
	var buf strings.Builder
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// SAMLMetadata represents parsed IdP metadata
type SAMLMetadata struct {
	EntityID         string `xml:"entityID,attr"`
	IDPSSODescriptor struct {
		SingleSignOnServices []struct {
			Binding  string `xml:"Binding,attr"`
			Location string `xml:"Location,attr"`
		} `xml:"SingleSignOnService"`
		KeyDescriptors []struct {
			Use        string `xml:"use,attr"`
			KeyInfo struct {
				X509Data struct {
					X509Certificate string `xml:"X509Certificate"`
				} `xml:"X509Data"`
			} `xml:"KeyInfo"`
		} `xml:"KeyDescriptor"`
	} `xml:"IDPSSODescriptor"`
}
