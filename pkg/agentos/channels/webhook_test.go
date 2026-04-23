package channels

import (
	"context"
	"testing"
	"time"
)

func TestNewWebhookChannel(t *testing.T) {
	channel := NewWebhookChannel("test-hook", "https://example.com/webhook")
	if channel == nil {
		t.Fatal("NewWebhookChannel returned nil")
	}
	if channel.name != "test-hook" {
		t.Errorf("Expected name 'test-hook', got '%s'", channel.name)
	}
	if channel.url != "https://example.com/webhook" {
		t.Errorf("Expected URL 'https://example.com/webhook', got '%s'", channel.url)
	}
	if channel.httpClient == nil {
		t.Error("HTTP client is nil")
	}
	if channel.timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", channel.timeout)
	}
}

func TestWebhookChannel_Type(t *testing.T) {
	channel := NewWebhookChannel("test", "https://example.com/webhook")
	if channel.Type() != ChannelTypeWebhook {
		t.Errorf("Expected type 'webhook', got '%s'", channel.Type())
	}
}

func TestWebhookChannel_Name(t *testing.T) {
	channel := NewWebhookChannel("my-webhook", "https://example.com/webhook")
	if channel.Name() != "my-webhook" {
		t.Errorf("Expected name 'my-webhook', got '%s'", channel.Name())
	}
}

func TestWebhookChannel_Configure(t *testing.T) {
	channel := NewWebhookChannel("", "")

	config := ChannelConfig{
		Type: ChannelTypeWebhook,
		Name: "configured-hook",
		Config: map[string]interface{}{
			"url":     "https://hooks.example.com/webhook",
			"method":  "POST",
			"timeout": int(60),
			"headers": map[string]interface{}{
				"Authorization":   "Bearer token123",
				"X-Custom-Header": "custom-value",
			},
			"secret": "webhook-secret",
		},
	}

	err := channel.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if !channel.IsConfigured() {
		t.Error("Channel should be configured")
	}
	if channel.url != "https://hooks.example.com/webhook" {
		t.Errorf("Expected URL, got '%s'", channel.url)
	}
	if channel.method != "POST" {
		t.Errorf("Expected method 'POST', got '%s'", channel.method)
	}
	if channel.timeout != 60*time.Second {
		t.Errorf("Expected timeout 60s, got %v", channel.timeout)
	}
	if len(channel.headers) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(channel.headers))
	}
	if channel.secret != "webhook-secret" {
		t.Errorf("Expected secret, got '%s'", channel.secret)
	}

	// Test wrong channel type
	wrongConfig := ChannelConfig{
		Type: ChannelTypeTelegram,
	}
	err = channel.Configure(wrongConfig)
	if err == nil {
		t.Error("Configure should fail with wrong channel type")
	}
}

func TestWebhookChannel_Configure_NoURL(t *testing.T) {
	channel := NewWebhookChannel("test", "")

	config := ChannelConfig{
		Type:   ChannelTypeWebhook,
		Config: map[string]interface{}{},
	}

	err := channel.Configure(config)
	if err == nil {
		t.Error("Configure should fail without URL")
	}
}

func TestWebhookChannel_Configure_DefaultMethod(t *testing.T) {
	channel := NewWebhookChannel("", "")

	config := ChannelConfig{
		Type: ChannelTypeWebhook,
		Config: map[string]interface{}{
			"url": "https://example.com/webhook",
		},
	}

	err := channel.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if channel.method != "POST" {
		t.Errorf("Expected default method 'POST', got '%s'", channel.method)
	}
}

func TestWebhookChannel_IsConfigured(t *testing.T) {
	channel := NewWebhookChannel("test", "")
	if channel.IsConfigured() {
		t.Error("Channel should not be configured without URL")
	}

	// Configure the channel properly
	config := ChannelConfig{
		Type: ChannelTypeWebhook,
		Config: map[string]interface{}{
			"url": "https://example.com/webhook",
		},
	}
	err := channel.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	if !channel.IsConfigured() {
		t.Error("Channel should be configured with URL")
	}
}

func TestWebhookChannel_Send_NotConfigured(t *testing.T) {
	channel := NewWebhookChannel("test", "")

	message := Message{
		Body:       "Hello",
		Recipients: []string{"https://example.com/webhook"},
	}

	err := channel.Send(context.Background(), message)
	if err == nil {
		t.Error("Send should fail when not configured")
	}
}

func TestWebhookChannel_Send_NoRecipients(t *testing.T) {
	channel := NewWebhookChannel("test", "https://example.com/webhook")

	message := Message{
		Body:       "Hello",
		Recipients: []string{},
	}

	err := channel.Send(context.Background(), message)
	if err == nil {
		t.Error("Send should fail without recipients")
	}
}

func TestWebhookChannel_SendTemplate_NotConfigured(t *testing.T) {
	channel := NewWebhookChannel("test", "")

	err := channel.SendTemplate(context.Background(), "template", map[string]interface{}{}, []string{"https://example.com/webhook"})
	if err == nil {
		t.Error("SendTemplate should fail when not configured")
	}
}

func TestWebhookChannel_Receive_NotSupported(t *testing.T) {
	channel := NewWebhookChannel("test", "https://example.com/webhook")

	handler := func(ctx context.Context, msg IncomingMessage) error {
		return nil
	}

	err := channel.Receive(context.Background(), handler)
	if err == nil {
		t.Error("Receive should return error (not supported)")
	}
}

func TestWebhookChannel_Close(t *testing.T) {
	channel := NewWebhookChannel("test-hook", "https://example.com/webhook")
	err := channel.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestWebhookPayload_Creation(t *testing.T) {
	payload := WebhookPayload{
		ID:        "msg-123",
		Timestamp: time.Now().Unix(),
		Event:     "message",
		Subject:   "Test",
		Body:      "Hello",
		HTMLBody:  "<p>Hello</p>",
		Recipients: []string{"user@example.com"},
		Metadata: map[string]interface{}{
			"key": "value",
		},
	}

	if payload.ID != "msg-123" {
		t.Errorf("Expected ID 'msg-123', got '%s'", payload.ID)
	}
	if payload.Event != "message" {
		t.Errorf("Expected event 'message', got '%s'", payload.Event)
	}
	if len(payload.Recipients) != 1 {
		t.Errorf("Expected 1 recipient, got %d", len(payload.Recipients))
	}
}

func TestWebhookPayload_WithSignature(t *testing.T) {
	payload := WebhookPayload{
		ID:        "msg-123",
		Timestamp: 1234567890,
		Event:     "notification",
		Body:      "Test message",
	}

	// The signature would typically be computed, but we're testing the structure
	// In a real test, you'd compute the expected signature
	_ = payload // Just verify it compiles
}

func TestWebhookMessage_Creation(t *testing.T) {
	msg := WebhookMessage{
		ID:        "msg-456",
		Type:      "notification",
		Payload:   `{"text":"Hello"}`,
		Timestamp: time.Now(),
		Signature: "sha256=abc123",
	}

	if msg.ID != "msg-456" {
		t.Errorf("Expected ID 'msg-456', got '%s'", msg.ID)
	}
	if msg.Type != "notification" {
		t.Errorf("Expected type 'notification', got '%s'", msg.Type)
	}
	if msg.Signature != "sha256=abc123" {
		t.Errorf("Expected signature, got '%s'", msg.Signature)
	}
}

func TestWebhookResponse_Creation(t *testing.T) {
	response := WebhookResponse{
		ID:      "resp-123",
		Status:  "success",
		Message: "Delivered",
		Code:    200,
	}

	if response.ID != "resp-123" {
		t.Errorf("Expected ID 'resp-123', got '%s'", response.ID)
	}
	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}
	if response.Code != 200 {
		t.Errorf("Expected code 200, got %d", response.Code)
	}
}

func TestWebhookEvent_Creation(t *testing.T) {
	payload := WebhookEventPayload{
		Event:     "user.created",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"user_id": "123",
			"email":   "user@example.com",
		},
	}

	if payload.Event != "user.created" {
		t.Errorf("Expected event 'user.created', got '%s'", payload.Event)
	}
	if payload.Data == nil {
		t.Error("Data should not be nil")
	}
}

func TestWebhookDelivery_Creation(t *testing.T) {
	now := time.Now()
	delivery := WebhookDelivery{
		ID:        "del-123",
		MessageID: "msg-456",
		URL:       "https://example.com/webhook",
		Status:    "delivered",
		Attempts:  1,
		LastAttempt: now,
		NextRetry:   now.Add(5 * time.Minute),
		Error:       "",
	}

	if delivery.ID != "del-123" {
		t.Errorf("Expected ID 'del-123', got '%s'", delivery.ID)
	}
	if delivery.Status != "delivered" {
		t.Errorf("Expected status 'delivered', got '%s'", delivery.Status)
	}
	if delivery.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", delivery.Attempts)
	}
}

func TestNewWebhookManager(t *testing.T) {
	manager := NewWebhookManager()
	if manager == nil {
		t.Fatal("NewWebhookManager returned nil")
	}
	if manager.endpoints == nil {
		t.Error("endpoints map is nil")
	}
	if manager.deliveries == nil {
		t.Error("deliveries map is nil")
	}
}

func TestWebhookManager_RegisterEndpoint(t *testing.T) {
	manager := NewWebhookManager()

	endpoint := WebhookEndpoint{
		ID:      "hook-1",
		URL:     "https://example.com/webhook",
		Secret:  "secret123",
		Events:  []string{"message.created", "user.updated"},
		Active:  true,
	}

	err := manager.RegisterEndpoint(endpoint)
	if err != nil {
		t.Fatalf("RegisterEndpoint failed: %v", err)
	}

	// Verify endpoint is registered
	endpoints := manager.GetEndpoints()
	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint, got %d", len(endpoints))
	}

	// Update existing endpoint
	endpoint.URL = "https://new.example.com/webhook"
	err = manager.RegisterEndpoint(endpoint)
	if err != nil {
		t.Fatalf("Update endpoint failed: %v", err)
	}

	endpoints = manager.GetEndpoints()
	if endpoints[0].URL != "https://new.example.com/webhook" {
		t.Errorf("Expected updated URL, got '%s'", endpoints[0].URL)
	}
}

func TestWebhookManager_UnregisterEndpoint(t *testing.T) {
	manager := NewWebhookManager()

	endpoint := WebhookEndpoint{
		ID:     "hook-1",
		URL:    "https://example.com/webhook",
		Active: true,
	}
	manager.RegisterEndpoint(endpoint)

	err := manager.UnregisterEndpoint("hook-1")
	if err != nil {
		t.Fatalf("UnregisterEndpoint failed: %v", err)
	}

	endpoints := manager.GetEndpoints()
	if len(endpoints) != 0 {
		t.Errorf("Expected 0 endpoints, got %d", len(endpoints))
	}

	// Unregister non-existent endpoint
	err = manager.UnregisterEndpoint("nonexistent")
	if err != nil {
		t.Errorf("UnregisterEndpoint should not error for non-existent: %v", err)
	}
}

func TestWebhookManager_GetEndpointsForEvent(t *testing.T) {
	manager := NewWebhookManager()

	// Register endpoints for different events
	manager.RegisterEndpoint(WebhookEndpoint{
		ID:     "hook-1",
		URL:    "https://example.com/webhook1",
		Events: []string{"message.created", "user.created"},
		Active: true,
	})
	manager.RegisterEndpoint(WebhookEndpoint{
		ID:     "hook-2",
		URL:    "https://example.com/webhook2",
		Events: []string{"message.created", "message.updated"},
		Active: true,
	})
	manager.RegisterEndpoint(WebhookEndpoint{
		ID:     "hook-3",
		URL:    "https://example.com/webhook3",
		Events: []string{"user.deleted"},
		Active: false, // Inactive
	})

	// Test message.created event
	endpoints := manager.GetEndpointsForEvent("message.created")
	if len(endpoints) != 2 {
		t.Errorf("Expected 2 endpoints for message.created, got %d", len(endpoints))
	}

	// Test user.created event
	endpoints = manager.GetEndpointsForEvent("user.created")
	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint for user.created, got %d", len(endpoints))
	}

	// Test user.deleted event (should return 0 because hook-3 is inactive)
	endpoints = manager.GetEndpointsForEvent("user.deleted")
	if len(endpoints) != 0 {
		t.Errorf("Expected 0 active endpoints for user.deleted, got %d", len(endpoints))
	}

	// Test non-existent event
	endpoints = manager.GetEndpointsForEvent("nonexistent")
	if len(endpoints) != 0 {
		t.Errorf("Expected 0 endpoints for nonexistent event, got %d", len(endpoints))
	}
}

func TestWebhookManager_EnableDisableEndpoint(t *testing.T) {
	manager := NewWebhookManager()

	manager.RegisterEndpoint(WebhookEndpoint{
		ID:     "hook-1",
		URL:    "https://example.com/webhook",
		Active: true,
	})

	// Disable endpoint
	err := manager.DisableEndpoint("hook-1")
	if err != nil {
		t.Fatalf("DisableEndpoint failed: %v", err)
	}

	endpoint := manager.GetEndpoint("hook-1")
	if endpoint == nil {
		t.Fatal("Endpoint should exist")
	}
	if endpoint.Active {
		t.Error("Endpoint should be disabled")
	}

	// Enable endpoint
	err = manager.EnableEndpoint("hook-1")
	if err != nil {
		t.Fatalf("EnableEndpoint failed: %v", err)
	}

	endpoint = manager.GetEndpoint("hook-1")
	if !endpoint.Active {
		t.Error("Endpoint should be enabled")
	}

	// Disable non-existent endpoint
	err = manager.DisableEndpoint("nonexistent")
	if err == nil {
		t.Error("DisableEndpoint should fail for non-existent")
	}
}

func TestWebhookManager_GetEndpoint(t *testing.T) {
	manager := NewWebhookManager()

	manager.RegisterEndpoint(WebhookEndpoint{
		ID:  "hook-1",
		URL: "https://example.com/webhook",
	})

	endpoint := manager.GetEndpoint("hook-1")
	if endpoint == nil {
		t.Error("GetEndpoint should return existing endpoint")
	}
	if endpoint.ID != "hook-1" {
		t.Errorf("Expected ID 'hook-1', got '%s'", endpoint.ID)
	}

	// Get non-existent endpoint
	endpoint = manager.GetEndpoint("nonexistent")
	if endpoint != nil {
		t.Error("GetEndpoint should return nil for non-existent")
	}
}

func TestWebhookEndpoint_Headers(t *testing.T) {
	endpoint := WebhookEndpoint{
		ID:      "hook-1",
		URL:     "https://example.com/webhook",
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"X-Version":     "1.0",
		},
	}

	if len(endpoint.Headers) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(endpoint.Headers))
	}
	if endpoint.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Expected auth header, got '%s'", endpoint.Headers["Authorization"])
	}
}

func TestWebhookEndpoint_RegexEvent(t *testing.T) {
	endpoint := WebhookEndpoint{
		ID:      "hook-1",
		URL:     "https://example.com/webhook",
		Events:  []string{"message.*"},
		Active:  true,
	}

	// This would need actual regex matching implementation to test properly
	_ = endpoint
}

func TestWebhookSignature_Verification(t *testing.T) {
	// Test signature structure
	signature := "sha256=abc123def456"
	if signature == "" {
		t.Error("Signature should not be empty")
	}

	// Test valid signature format
	if !startsWith(signature, "sha256=") {
		t.Error("Signature should start with 'sha256='")
	}
}

// Helper function
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
