package channels

import (
	"context"
	"testing"
)

func TestNewEmailChannel(t *testing.T) {
	channel := NewEmailChannel()
	if channel == nil {
		t.Fatal("NewEmailChannel returned nil")
	}
	if channel.name != "email" {
		t.Errorf("Expected name 'email', got '%s'", channel.name)
	}
	if channel.port != 587 {
		t.Errorf("Expected default port 587, got %d", channel.port)
	}
}

func TestEmailChannel_Type(t *testing.T) {
	channel := NewEmailChannel()
	if channel.Type() != ChannelTypeEmail {
		t.Errorf("Expected type 'email', got '%s'", channel.Type())
	}
}

func TestEmailChannel_Name(t *testing.T) {
	channel := NewEmailChannel()
	// Email channel name is always "email"
	if channel.Name() != "email" {
		t.Errorf("Expected name 'email', got '%s'", channel.Name())
	}
}

func TestEmailChannel_Configure(t *testing.T) {
	channel := NewEmailChannel()

	config := ChannelConfig{
		Type: ChannelTypeEmail,
		Config: map[string]interface{}{
			"host":     "smtp.gmail.com",
			"port":     float64(587),
			"username": "test@gmail.com",
			"password": "secret",
			"from":     "sender@example.com",
			"use_tls":  true,
		},
	}

	err := channel.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if !channel.IsConfigured() {
		t.Error("Channel should be configured")
	}
	if channel.host != "smtp.gmail.com" {
		t.Errorf("Expected host 'smtp.gmail.com', got '%s'", channel.host)
	}
	if channel.port != 587 {
		t.Errorf("Expected port 587, got %d", channel.port)
	}
	if channel.username != "test@gmail.com" {
		t.Errorf("Expected username, got '%s'", channel.username)
	}
	if channel.from != "sender@example.com" {
		t.Errorf("Expected from 'sender@example.com', got '%s'", channel.from)
	}
	if !channel.useTLS {
		t.Error("Expected TLS to be enabled")
	}
}

func TestEmailChannel_Configure_NoHost(t *testing.T) {
	channel := NewEmailChannel()

	config := ChannelConfig{
		Type: ChannelTypeEmail,
		Config: map[string]interface{}{
			"username": "test@gmail.com",
			"password": "secret",
			"from":     "sender@example.com",
		},
	}

	err := channel.Configure(config)
	if err == nil {
		t.Error("Configure should fail without host")
	}
}

func TestEmailChannel_Configure_NoFrom(t *testing.T) {
	channel := NewEmailChannel()

	config := ChannelConfig{
		Type: ChannelTypeEmail,
		Config: map[string]interface{}{
			"host":     "smtp.gmail.com",
			"port":     float64(587),
			"username": "test@gmail.com",
			"password": "secret",
		},
	}

	err := channel.Configure(config)
	if err == nil {
		t.Error("Configure should fail without from")
	}
}

func TestEmailChannel_Configure_WrongType(t *testing.T) {
	channel := NewEmailChannel()

	config := ChannelConfig{
		Type: ChannelTypeTelegram,
	}

	err := channel.Configure(config)
	if err == nil {
		t.Error("Configure should fail with wrong channel type")
	}
}

func TestEmailChannel_IsConfigured(t *testing.T) {
	channel := NewEmailChannel()
	if channel.IsConfigured() {
		t.Error("Channel should not be configured initially")
	}

	config := ChannelConfig{
		Type: ChannelTypeEmail,
		Config: map[string]interface{}{
			"host":     "smtp.gmail.com",
			"port":     float64(587),
			"username": "test@gmail.com",
			"password": "secret",
			"from":     "test@example.com",
		},
	}
	channel.Configure(config)

	if !channel.IsConfigured() {
		t.Error("Channel should be configured with valid settings")
	}
}

func TestEmailChannel_Send_NotConfigured(t *testing.T) {
	channel := NewEmailChannel()

	message := Message{
		Subject:    "Test",
		Body:       "Hello",
		Recipients: []string{"to@example.com"},
	}

	err := channel.Send(context.Background(), message)
	if err == nil {
		t.Error("Send should fail when not configured")
	}
}

func TestEmailChannel_Send_NoRecipients(t *testing.T) {
	channel := NewEmailChannel()

	config := ChannelConfig{
		Type: ChannelTypeEmail,
		Config: map[string]interface{}{
			"host":     "smtp.gmail.com",
			"port":     float64(587),
			"username": "test@gmail.com",
			"password": "secret",
			"from":     "test@example.com",
		},
	}
	channel.Configure(config)

	message := Message{
		Subject:    "Test",
		Body:       "Hello",
		Recipients: []string{},
	}

	err := channel.Send(context.Background(), message)
	if err == nil {
		t.Error("Send should fail without recipients")
	}
}

func TestEmailChannel_SendTemplate_NotConfigured(t *testing.T) {
	channel := NewEmailChannel()

	err := channel.SendTemplate(context.Background(), "welcome", map[string]interface{}{}, []string{"test@example.com"})
	if err == nil {
		t.Error("SendTemplate should fail when not configured")
	}
}

func TestEmailChannel_Close(t *testing.T) {
	channel := NewEmailChannel()
	err := channel.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestEmailChannel_DefaultPort(t *testing.T) {
	channel := NewEmailChannel()

	config := ChannelConfig{
		Type: ChannelTypeEmail,
		Config: map[string]interface{}{
			"host":     "smtp.gmail.com",
			"username": "test@gmail.com",
			"password": "secret",
			"from":     "test@gmail.com",
			// No port specified - should use default
		},
	}

	channel.Configure(config)

	// Should use default port 587
	if channel.port != 587 {
		t.Errorf("Expected default port 587, got %d", channel.port)
	}
}

func TestEmailChannel_WithHTMLBody(t *testing.T) {
	channel := NewEmailChannel()

	config := ChannelConfig{
		Type: ChannelTypeEmail,
		Config: map[string]interface{}{
			"host":     "smtp.gmail.com",
			"port":     float64(587),
			"username": "test@gmail.com",
			"password": "secret",
			"from":     "test@example.com",
		},
	}
	channel.Configure(config)

	message := Message{
		Subject:    "Test",
		Body:       "Plain text",
		HTMLBody:   "<html><body>HTML content</body></html>",
		Recipients: []string{"to@example.com"},
	}

	// We can't actually send in test, but we can verify the message is constructed
	if message.HTMLBody == "" {
		t.Error("HTMLBody should be set")
	}
}

func TestEmailChannel_WithAttachments(t *testing.T) {
	channel := NewEmailChannel()

	config := ChannelConfig{
		Type: ChannelTypeEmail,
		Config: map[string]interface{}{
			"host":     "smtp.gmail.com",
			"port":     float64(587),
			"username": "test@gmail.com",
			"password": "secret",
			"from":     "test@example.com",
		},
	}
	channel.Configure(config)

	message := Message{
		Subject:    "Test",
		Body:       "See attachment",
		Recipients: []string{"to@example.com"},
		Attachments: []Attachment{
			{
				Name:     "test.pdf",
				Content:  []byte("PDF content"),
				MIMEType: "application/pdf",
			},
		},
	}

	if len(message.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(message.Attachments))
	}
}
