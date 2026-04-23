package auth

import (
	"context"
	"testing"
	"time"
)

func TestNewSAMLProvider(t *testing.T) {
	provider := NewSAMLProvider("test-saml")
	if provider == nil {
		t.Fatal("NewSAMLProvider returned nil")
	}
	if provider.name != "test-saml" {
		t.Errorf("Expected name 'test-saml', got '%s'", provider.name)
	}
}

func TestSAMLProvider_Type(t *testing.T) {
	provider := NewSAMLProvider("test")
	if provider.Type() != ProviderTypeSAML {
		t.Errorf("Expected type 'saml', got '%s'", provider.Type())
	}
}

func TestSAMLProvider_Name(t *testing.T) {
	provider := NewSAMLProvider("my-saml")
	if provider.Name() != "my-saml" {
		t.Errorf("Expected name 'my-saml', got '%s'", provider.Name())
	}
}

func TestSAMLProvider_Configure(t *testing.T) {
	provider := NewSAMLProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeSAML,
		Config: map[string]interface{}{
			"entity_id": "https://app.example.com",
			"acs_url":   "https://app.example.com/saml/acs",
		},
	}

	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if !provider.IsConfigured() {
		t.Error("Provider should be configured")
	}
	if provider.spEntityID != "https://app.example.com" {
		t.Errorf("Expected entity_id, got '%s'", provider.spEntityID)
	}
	if provider.spACSURL != "https://app.example.com/saml/acs" {
		t.Errorf("Expected acs_url, got '%s'", provider.spACSURL)
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

func TestSAMLProvider_Configure_NoEntityID(t *testing.T) {
	provider := NewSAMLProvider("test")

	config := ProviderConfig{
		Type: ProviderTypeSAML,
		Config: map[string]interface{}{
			"acs_url": "https://app.example.com/saml/acs",
		},
	}

	// Should still configure but with empty entity_id
	err := provider.Configure(config)
	if err != nil {
		t.Fatalf("Configure should not fail: %v", err)
	}

	if provider.spEntityID != "" {
		t.Error("entity_id should be empty if not provided")
	}
}

func TestSAMLProvider_IsConfigured(t *testing.T) {
	provider := NewSAMLProvider("test")
	if provider.IsConfigured() {
		t.Error("Provider should not be configured initially")
	}

	provider.configured = true
	if !provider.IsConfigured() {
		t.Error("Provider should be configured when flag is set")
	}
}

func TestSAMLProvider_Authenticate(t *testing.T) {
	provider := NewSAMLProvider("test")
	provider.configured = true

	_, err := provider.Authenticate(context.Background(), Credentials{})
	if err == nil {
		t.Error("Authenticate should return error (uses redirect flow)")
	}
}

func TestSAMLProvider_GetLoginURL_NotConfigured(t *testing.T) {
	provider := NewSAMLProvider("test")

	_, err := provider.GetLoginURL("relay-state")
	if err == nil {
		t.Error("GetLoginURL should fail when not configured")
	}
}

func TestSAMLProvider_Authorize(t *testing.T) {
	provider := NewSAMLProvider("test")
	identity := &Identity{ID: "user123"}

	_, err := provider.Authorize(context.Background(), identity, "resource", "action")
	if err == nil {
		t.Error("Authorize should return error for SAML provider")
	}
}

func TestSAMLProvider_RefreshToken(t *testing.T) {
	provider := NewSAMLProvider("test")
	provider.configured = true

	_, err := provider.RefreshToken(context.Background(), "refresh-token")
	if err == nil {
		t.Error("RefreshToken should return error for SAML provider")
	}
}

func TestSAMLProvider_Logout(t *testing.T) {
	provider := NewSAMLProvider("test")
	err := provider.Logout(context.Background(), "nameid")
	if err != nil {
		t.Errorf("Logout should succeed: %v", err)
	}
}

func TestSAMLProvider_GetMetadata(t *testing.T) {
	provider := NewSAMLProvider("test")
	provider.spEntityID = "https://app.example.com"
	provider.spACSURL = "https://app.example.com/saml/acs"

	metadata, err := provider.GetMetadata()
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}

	if len(metadata) == 0 {
		t.Error("Metadata should not be empty")
	}

	metadataStr := string(metadata)
	if !containsStr(metadataStr, "EntityDescriptor") {
		t.Error("Metadata should contain EntityDescriptor")
	}
	if !containsStr(metadataStr, provider.spEntityID) {
		t.Error("Metadata should contain entityID")
	}
	if !containsStr(metadataStr, provider.spACSURL) {
		t.Error("Metadata should contain ACS URL")
	}
}

func TestSAMLProvider_GetMetadata_NotConfigured(t *testing.T) {
	provider := NewSAMLProvider("test")

	// Should still return metadata even if not fully configured
	metadata, err := provider.GetMetadata()
	if err != nil {
		t.Fatalf("GetMetadata should not error: %v", err)
	}
	if len(metadata) == 0 {
		t.Error("Metadata should not be empty")
	}
}

func TestSAMLRequest_Structure(t *testing.T) {
	request := SAMLRequest{
		ID:                          "_123456",
		Version:                     "2.0",
		IssueInstant:                time.Now().Format(time.RFC3339),
		Destination:                 "https://idp.example.com/sso",
		AssertionConsumerServiceURL: "https://app.example.com/saml/acs",
		ProtocolBinding:             "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST",
		Issuer: SAMLIssuer{
			Value: "https://app.example.com",
		},
		NameIDPolicy: SAMLNameIDPolicy{
			AllowCreate: true,
			Format:      "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
		},
	}

	if request.Version != "2.0" {
		t.Errorf("Expected version '2.0', got '%s'", request.Version)
	}
	if request.ProtocolBinding != "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" {
		t.Errorf("Unexpected protocol binding: '%s'", request.ProtocolBinding)
	}
	if !request.NameIDPolicy.AllowCreate {
		t.Error("NameIDPolicy.AllowCreate should be true")
	}
}

func TestSAMLResponse_Structure(t *testing.T) {
	response := SAMLResponse{
		ID:           "_response-123",
		Version:      "2.0",
		IssueInstant: time.Now().Format(time.RFC3339),
		InResponseTo: "_request-456",
		Status: SAMLStatus{
			Code: SAMLStatusCode{
				Value: "urn:oasis:names:tc:SAML:2.0:status:Success",
			},
		},
		Assertion: &SAMLAssertion{
			ID:           "_assertion-789",
			Version:      "2.0",
			IssueInstant: time.Now().Format(time.RFC3339),
			Subject: SAMLSubject{
				NameID: SAMLNameID{
					Format: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
					Value:  "user@example.com",
				},
			},
		},
	}

	if response.Status.Code.Value != "urn:oasis:names:tc:SAML:2.0:status:Success" {
		t.Errorf("Unexpected status code: '%s'", response.Status.Code.Value)
	}
	if response.Assertion == nil {
		t.Fatal("Assertion should not be nil")
	}
	if response.Assertion.Subject.NameID.Value != "user@example.com" {
		t.Errorf("Unexpected NameID value: '%s'", response.Assertion.Subject.NameID.Value)
	}
}

func TestSAMLAssertion_Attributes(t *testing.T) {
	assertion := SAMLAssertion{
		ID:           "_assertion-123",
		Version:      "2.0",
		IssueInstant: time.Now().Format(time.RFC3339),
		Issuer: SAMLIssuer{
			Value: "https://idp.example.com",
		},
		Subject: SAMLSubject{
			NameID: SAMLNameID{
				Value: "user@example.com",
			},
		},
		Conditions: SAMLConditions{
			NotBefore:    time.Now().Format(time.RFC3339),
			NotOnOrAfter: time.Now().Add(time.Hour).Format(time.RFC3339),
		},
		AuthnStatement: SAMLAuthnStatement{
			AuthnInstant: time.Now().Format(time.RFC3339),
			AuthnContext: SAMLAuthnContext{
				AuthnContextClassRef: "urn:oasis:names:tc:SAML:2.0:ac:classes:Password",
			},
		},
		AttributeStatement: SAMLAttributeStatement{
			Attributes: []SAMLAttribute{
				{
					Name: "email",
					Values: []SAMLAttributeValue{
						{Value: "user@example.com"},
					},
				},
				{
					Name: "name",
					Values: []SAMLAttributeValue{
						{Value: "John Doe"},
					},
				},
				{
					Name: "roles",
					Values: []SAMLAttributeValue{
						{Value: "admin"},
						{Value: "user"},
					},
				},
			},
		},
	}

	if len(assertion.AttributeStatement.Attributes) != 3 {
		t.Errorf("Expected 3 attributes, got %d", len(assertion.AttributeStatement.Attributes))
	}

	// Find email attribute
	var emailAttr *SAMLAttribute
	for _, attr := range assertion.AttributeStatement.Attributes {
		if attr.Name == "email" {
			emailAttr = &attr
			break
		}
	}
	if emailAttr == nil {
		t.Error("Email attribute not found")
	} else if len(emailAttr.Values) != 1 || emailAttr.Values[0].Value != "user@example.com" {
		t.Errorf("Unexpected email value: %v", emailAttr.Values)
	}

	// Check AuthnContext
	if assertion.AuthnStatement.AuthnContext.AuthnContextClassRef != "urn:oasis:names:tc:SAML:2.0:ac:classes:Password" {
		t.Errorf("Unexpected AuthnContextClassRef: '%s'", assertion.AuthnStatement.AuthnContext.AuthnContextClassRef)
	}
}

func TestSAMLSubjectConfirmation(t *testing.T) {
	confirmation := SAMLSubjectConfirmation{
		Method: "urn:oasis:names:tc:SAML:2.0:cm:bearer",
		SubjectConfirmationData: SAMLSubjectConfirmationData{
			InResponseTo: "_request-123",
			NotOnOrAfter: time.Now().Add(time.Hour).Format(time.RFC3339),
			Recipient:    "https://app.example.com/saml/acs",
		},
	}

	if confirmation.Method != "urn:oasis:names:tc:SAML:2.0:cm:bearer" {
		t.Errorf("Unexpected method: '%s'", confirmation.Method)
	}
	if confirmation.SubjectConfirmationData.InResponseTo != "_request-123" {
		t.Errorf("Unexpected InResponseTo: '%s'", confirmation.SubjectConfirmationData.InResponseTo)
	}
}

func TestSAMLNameID_Formats(t *testing.T) {
	formats := []string{
		"urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
		"urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified",
		"urn:oasis:names:tc:SAML:2.0:nameid-format:persistent",
		"urn:oasis:names:tc:SAML:2.0:nameid-format:transient",
	}

	for _, format := range formats {
		nameID := SAMLNameID{
			Format: format,
			Value:  "test-user",
		}
		if nameID.Format != format {
			t.Errorf("Expected format '%s', got '%s'", format, nameID.Format)
		}
	}
}

func TestSAMLStatusCode_Values(t *testing.T) {
	codes := []struct {
		code    string
		success bool
	}{
		{"urn:oasis:names:tc:SAML:2.0:status:Success", true},
		{"urn:oasis:names:tc:SAML:2.0:status:Requester", false},
		{"urn:oasis:names:tc:SAML:2.0:status:Responder", false},
		{"urn:oasis:names:tc:SAML:2.0:status:AuthnFailed", false},
	}

	for _, tc := range codes {
		status := SAMLStatus{
			Code: SAMLStatusCode{Value: tc.code},
		}
		if status.Code.Value != tc.code {
			t.Errorf("Expected code '%s', got '%s'", tc.code, status.Code.Value)
		}
	}
}

func TestSAMLMetadata_Structure(t *testing.T) {
	metadata := SAMLMetadata{
		EntityID: "https://idp.example.com",
		IDPSSODescriptor: struct {
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
		}{
			SingleSignOnServices: []struct {
				Binding  string `xml:"Binding,attr"`
				Location string `xml:"Location,attr"`
			}{
				{
					Binding:  "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect",
					Location: "https://idp.example.com/sso",
				},
				{
					Binding:  "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST",
					Location: "https://idp.example.com/sso/post",
				},
			},
			KeyDescriptors: []struct {
				Use        string `xml:"use,attr"`
				KeyInfo struct {
					X509Data struct {
						X509Certificate string `xml:"X509Certificate"`
					} `xml:"X509Data"`
				} `xml:"KeyInfo"`
			}{
				{
					Use: "signing",
					KeyInfo: struct {
						X509Data struct {
							X509Certificate string `xml:"X509Certificate"`
						} `xml:"X509Data"`
					}{
						X509Data: struct {
							X509Certificate string `xml:"X509Certificate"`
						}{
							X509Certificate: "MIICiDCCAfGgAwIBAg...",
						},
					},
				},
			},
		},
	}

	if metadata.EntityID != "https://idp.example.com" {
		t.Errorf("Unexpected EntityID: '%s'", metadata.EntityID)
	}
	if len(metadata.IDPSSODescriptor.SingleSignOnServices) != 2 {
		t.Errorf("Expected 2 SSO services, got %d", len(metadata.IDPSSODescriptor.SingleSignOnServices))
	}
	if len(metadata.IDPSSODescriptor.KeyDescriptors) != 1 {
		t.Errorf("Expected 1 key descriptor, got %d", len(metadata.IDPSSODescriptor.KeyDescriptors))
	}
}

func TestGenerateSAMLID(t *testing.T) {
	id1 := generateSAMLID()
	id2 := generateSAMLID()

	if id1 == "" {
		t.Error("generateSAMLID returned empty string")
	}
	if id1 == id2 {
		t.Error("generateSAMLID should return different values")
	}
	if !startsWithUnderscore(id1) {
		t.Error("SAML ID should start with underscore")
	}
}

func TestDeflate(t *testing.T) {
	data := []byte(`<AuthnRequest xmlns="urn:oasis:names:tc:SAML:2.0:protocol"><Issuer>test</Issuer></AuthnRequest>`)

	deflated, err := deflate(data)
	if err != nil {
		t.Fatalf("deflate failed: %v", err)
	}

	if len(deflated) == 0 {
		t.Error("deflate returned empty data")
	}

	// Deflated data should be smaller than original
	if len(deflated) >= len(data) {
		t.Logf("Note: deflated size (%d) >= original size (%d) - this can happen with small inputs", len(deflated), len(data))
	}
}

// Helper function
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func startsWithUnderscore(s string) bool {
	return len(s) > 0 && s[0] == '_'
}
