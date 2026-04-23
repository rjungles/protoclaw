package channels

import (
	"context"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.channels == nil {
		t.Error("channels map is nil")
	}
	if m.handlers == nil {
		t.Error("handlers map is nil")
	}
}

func TestManager_Register(t *testing.T) {
	m := NewManager()

	// Create a mock channel
	channel := &MockChannel{
		name:       "mock",
		channelType: ChannelTypeTelegram,
		configured: true,
	}

	err := m.Register(channel)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Try to register nil channel
	err = m.Register(nil)
	if err == nil {
		t.Error("Register should fail with nil channel")
	}

	// Try to register unconfigured channel
	unconfiguredChannel := &MockChannel{
		name:       "unconfigured",
		channelType: ChannelTypeTelegram,
		configured: false,
	}
	err = m.Register(unconfiguredChannel)
	if err == nil {
		t.Error("Register should fail with unconfigured channel")
	}
}

func TestManager_Get(t *testing.T) {
	m := NewManager()

	channel := &MockChannel{
		name:       "test",
		channelType: ChannelTypeTelegram,
		configured: true,
	}
	m.Register(channel)

	// Get channel
	ch, err := m.Get(ChannelTypeTelegram)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if ch == nil {
		t.Error("Get returned nil channel")
	}
	if ch.Name() != "test" {
		t.Errorf("Expected name 'test', got '%s'", ch.Name())
	}

	// Get non-existent channel
	_, err = m.Get(ChannelTypeEmail)
	if err == nil {
		t.Error("Get should fail for non-existent channel")
	}
}

func TestManager_GetAll(t *testing.T) {
	m := NewManager()

	channels := []Channel{
		&MockChannel{name: "telegram", channelType: ChannelTypeTelegram, configured: true},
		&MockChannel{name: "email", channelType: ChannelTypeEmail, configured: true},
		&MockChannel{name: "webhook", channelType: ChannelTypeWebhook, configured: true},
	}

	for _, ch := range channels {
		m.Register(ch)
	}

	all := m.GetAll()
	if len(all) != len(channels) {
		t.Errorf("Expected %d channels, got %d", len(channels), len(all))
	}
}

func TestManager_Send(t *testing.T) {
	m := NewManager()

	channel := &MockChannel{
		name:       "test",
		channelType: ChannelTypeTelegram,
		configured: true,
	}
	m.Register(channel)

	message := Message{
		ID:         "msg-1",
		Subject:    "Test",
		Body:       "Hello",
		Recipients: []string{"user123"},
		Priority:   PriorityNormal,
	}

	err := m.Send(context.Background(), ChannelTypeTelegram, message)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Send to non-existent channel
	err = m.Send(context.Background(), ChannelTypeSMS, message)
	if err == nil {
		t.Error("Send should fail for non-existent channel")
	}
}

func TestManager_SendAll(t *testing.T) {
	m := NewManager()

	// Register multiple channels
	m.Register(&MockChannel{name: "telegram", channelType: ChannelTypeTelegram, configured: true, sendError: nil})
	m.Register(&MockChannel{name: "email", channelType: ChannelTypeEmail, configured: true, sendError: nil})
	m.Register(&MockChannel{name: "failing", channelType: ChannelTypeWebhook, configured: true, sendError: context.DeadlineExceeded})

	message := Message{
		ID:         "msg-1",
		Body:       "Test",
		Recipients: []string{"user"},
	}

	errors := m.SendAll(context.Background(), message)
	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

func TestManager_RegisterHandler(t *testing.T) {
	m := NewManager()

	handler := func(ctx context.Context, msg IncomingMessage) error {
		return nil
	}

	m.RegisterHandler(ChannelTypeTelegram, handler)
	m.RegisterHandler(ChannelTypeTelegram, handler)

	if len(m.handlers[ChannelTypeTelegram]) != 2 {
		t.Errorf("Expected 2 handlers, got %d", len(m.handlers[ChannelTypeTelegram]))
	}
}

func TestManager_StartReceiving(t *testing.T) {
	m := NewManager()

	channel := &MockChannel{
		name:       "test",
		channelType: ChannelTypeTelegram,
		configured: true,
	}
	m.Register(channel)

	// Register handler
	handler := func(ctx context.Context, msg IncomingMessage) error {
		return nil
	}
	m.RegisterHandler(ChannelTypeTelegram, handler)

	// Start receiving (will run in background)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := m.StartReceiving(ctx)
	if err != nil {
		t.Fatalf("StartReceiving failed: %v", err)
	}

	// Wait for context to timeout
	<-ctx.Done()

	// Test completed successfully
}

func TestManager_Close(t *testing.T) {
	m := NewManager()

	// Register channels
	m.Register(&MockChannel{name: "telegram", channelType: ChannelTypeTelegram, configured: true})
	m.Register(&MockChannel{name: "email", channelType: ChannelTypeEmail, configured: true})

	err := m.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestNewMessageFormatter(t *testing.T) {
	f := NewMessageFormatter()
	if f == nil {
		t.Fatal("NewMessageFormatter returned nil")
	}
	if f.templates == nil {
		t.Error("templates map is nil")
	}
}

func TestMessageFormatter_RegisterTemplate(t *testing.T) {
	f := NewMessageFormatter()
	f.RegisterTemplate("welcome", "Hello {{name}}!")

	if f.templates["welcome"] != "Hello {{name}}!" {
		t.Error("Template was not registered")
	}
}

func TestMessageFormatter_Format(t *testing.T) {
	f := NewMessageFormatter()
	f.RegisterTemplate("welcome", "Hello {{name}}! Your code is {{code}}.")

	result, err := f.Format("welcome", map[string]interface{}{
		"name": "John",
		"code": 12345,
	})
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	expected := "Hello John! Your code is 12345."
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestMessageFormatter_Format_NotFound(t *testing.T) {
	f := NewMessageFormatter()

	_, err := f.Format("nonexistent", nil)
	if err == nil {
		t.Error("Format should fail for non-existent template")
	}
}

func TestMessagePriority_String(t *testing.T) {
	tests := []struct {
		priority MessagePriority
		expected string
	}{
		{PriorityLow, "low"},
		{PriorityNormal, "normal"},
		{PriorityHigh, "high"},
		{PriorityUrgent, "urgent"},
	}

	for _, test := range tests {
		if string(test.priority) != test.expected {
			t.Errorf("Expected '%s', got '%s'", test.expected, string(test.priority))
		}
	}
}

func TestChannelType_String(t *testing.T) {
	tests := []struct {
		channel ChannelType
		expected string
	}{
		{ChannelTypeTelegram, "telegram"},
		{ChannelTypeEmail, "email"},
		{ChannelTypeWebhook, "webhook"},
		{ChannelTypeSMS, "sms"},
		{ChannelTypeWhatsApp, "whatsapp"},
	}

	for _, test := range tests {
		if string(test.channel) != test.expected {
			t.Errorf("Expected '%s', got '%s'", test.expected, string(test.channel))
		}
	}
}

func TestAttachment_Creation(t *testing.T) {
	attachment := Attachment{
		Name:     "test.pdf",
		Content:  []byte("PDF content"),
		MIMEType: "application/pdf",
	}

	if attachment.Name != "test.pdf" {
		t.Errorf("Expected name 'test.pdf', got '%s'", attachment.Name)
	}
	if string(attachment.Content) != "PDF content" {
		t.Error("Content mismatch")
	}
	if attachment.MIMEType != "application/pdf" {
		t.Errorf("Expected MIME type 'application/pdf', got '%s'", attachment.MIMEType)
	}
}

func TestMessage_WithAttachments(t *testing.T) {
	now := time.Now()
	message := Message{
		ID:         "msg-1",
		Subject:    "Test",
		Body:       "Hello",
		HTMLBody:   "<p>Hello</p>",
		Recipients: []string{"user@example.com"},
		Attachments: []Attachment{
			{Name: "file1.pdf", Content: []byte("content1"), MIMEType: "application/pdf"},
			{Name: "file2.jpg", Content: []byte("content2"), MIMEType: "image/jpeg"},
		},
		Metadata: map[string]interface{}{
			"key": "value",
		},
		Priority:    PriorityHigh,
		ScheduledAt: &now,
		ReplyTo:     "reply@example.com",
	}

	if len(message.Attachments) != 2 {
		t.Errorf("Expected 2 attachments, got %d", len(message.Attachments))
	}
	if message.Priority != PriorityHigh {
		t.Error("Priority mismatch")
	}
	if message.ReplyTo != "reply@example.com" {
		t.Errorf("Expected ReplyTo 'reply@example.com', got '%s'", message.ReplyTo)
	}
}

func TestIncomingMessage_Creation(t *testing.T) {
	now := time.Now()
	msg := IncomingMessage{
		ID:         "inc-1",
		Channel:    ChannelTypeTelegram,
		From:       "user123",
		Body:       "Hello",
		RawData:    []byte(`{"text":"Hello"}`),
		ReceivedAt: now,
		Metadata: map[string]interface{}{
			"chat_id": "12345",
		},
	}

	if msg.ID != "inc-1" {
		t.Errorf("Expected ID 'inc-1', got '%s'", msg.ID)
	}
	if msg.Channel != ChannelTypeTelegram {
		t.Errorf("Expected channel 'telegram', got '%s'", msg.Channel)
	}
	if msg.From != "user123" {
		t.Errorf("Expected from 'user123', got '%s'", msg.From)
	}
}

func TestNotification_Creation(t *testing.T) {
	notification := Notification{
		Title:    "Test",
		Message:  "Hello World",
		Priority: PriorityNormal,
		Data: map[string]interface{}{
			"key": "value",
		},
		Template: "welcome",
		Channel:  ChannelTypeEmail,
	}

	if notification.Title != "Test" {
		t.Errorf("Expected title 'Test', got '%s'", notification.Title)
	}
	if notification.Channel != ChannelTypeEmail {
		t.Errorf("Expected channel 'email', got '%s'", notification.Channel)
	}
}

func TestNotification_ToJSON(t *testing.T) {
	notification := Notification{
		Title:   "Test",
		Message: "Hello",
		Priority: PriorityHigh,
	}

	json, err := notification.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	if len(json) == 0 {
		t.Error("ToJSON returned empty result")
	}

	// Check that JSON contains expected fields
	str := string(json)
	if !contains(str, "Test") {
		t.Error("JSON should contain 'Test'")
	}
	if !contains(str, "Hello") {
		t.Error("JSON should contain 'Hello'")
	}
}

func TestNewNotificationService(t *testing.T) {
	manager := NewManager()
	service := NewNotificationService(manager)

	if service == nil {
		t.Fatal("NewNotificationService returned nil")
	}
	if service.manager != manager {
		t.Error("Manager not set correctly")
	}
	if service.formatter == nil {
		t.Error("Formatter is nil")
	}
}

// MockChannel is a mock implementation for testing
type MockChannel struct {
	name        string
	channelType ChannelType
	configured  bool
	sendError   error
}

func (m *MockChannel) Type() ChannelType {
	return m.channelType
}

func (m *MockChannel) Name() string {
	return m.name
}

func (m *MockChannel) Configure(config ChannelConfig) error {
	return nil
}

func (m *MockChannel) IsConfigured() bool {
	return m.configured
}

func (m *MockChannel) Send(ctx context.Context, message Message) error {
	return m.sendError
}

func (m *MockChannel) SendTemplate(ctx context.Context, templateName string, data map[string]interface{}, recipients []string) error {
	return m.sendError
}

func (m *MockChannel) Receive(ctx context.Context, handler MessageHandler) error {
	// Simulate receiving a message
	go func() {
		<-ctx.Done()
	}()
	return nil
}

func (m *MockChannel) Close() error {
	return nil
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
