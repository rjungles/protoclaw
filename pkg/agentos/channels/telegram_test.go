package channels

import (
	"context"
	"testing"
)

func TestNewTelegramChannel(t *testing.T) {
	channel := NewTelegramChannel()
	if channel == nil {
		t.Fatal("NewTelegramChannel returned nil")
	}
	if channel.name != "telegram" {
		t.Errorf("Expected name 'telegram', got '%s'", channel.name)
	}
	if channel.apiURL != "https://api.telegram.org/bot" {
		t.Errorf("Expected apiURL 'https://api.telegram.org/bot', got '%s'", channel.apiURL)
	}
}

func TestTelegramChannel_Type(t *testing.T) {
	channel := NewTelegramChannel()
	if channel.Type() != ChannelTypeTelegram {
		t.Errorf("Expected type 'telegram', got '%s'", channel.Type())
	}
}

func TestTelegramChannel_Name(t *testing.T) {
	channel := NewTelegramChannel()
	// Telegram channel name is always "telegram"
	if channel.Name() != "telegram" {
		t.Errorf("Expected name 'telegram', got '%s'", channel.Name())
	}
}

func TestTelegramChannel_Configure(t *testing.T) {
	channel := NewTelegramChannel()

	config := ChannelConfig{
		Type: ChannelTypeTelegram,
		Config: map[string]interface{}{
			"bot_token": "test-token-123",
			"webhook_url": "https://example.com/webhook",
		},
	}

	err := channel.Configure(config)
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if !channel.IsConfigured() {
		t.Error("Channel should be configured")
	}
	if channel.botToken != "test-token-123" {
		t.Errorf("Expected token 'test-token-123', got '%s'", channel.botToken)
	}
	if channel.webhook != "https://example.com/webhook" {
		t.Errorf("Expected webhook URL")
	}

	// Test wrong channel type
	wrongConfig := ChannelConfig{
		Type: ChannelTypeEmail,
	}
	err = channel.Configure(wrongConfig)
	if err == nil {
		t.Error("Configure should fail with wrong channel type")
	}
}

func TestTelegramChannel_Configure_NoToken(t *testing.T) {
	channel := NewTelegramChannel()

	config := ChannelConfig{
		Type: ChannelTypeTelegram,
		Config: map[string]interface{}{},
	}

	err := channel.Configure(config)
	if err == nil {
		t.Error("Configure should fail without token")
	}
}

func TestTelegramChannel_IsConfigured(t *testing.T) {
	channel := NewTelegramChannel()
	if channel.IsConfigured() {
		t.Error("Channel should not be configured initially")
	}

	config := ChannelConfig{
		Type: ChannelTypeTelegram,
		Config: map[string]interface{}{
			"bot_token": "test-token",
		},
	}
	channel.Configure(config)

	if !channel.IsConfigured() {
		t.Error("Channel should be configured with token")
	}
}

func TestTelegramChannel_Send_NotConfigured(t *testing.T) {
	channel := NewTelegramChannel()

	message := Message{
		Body:       "Hello",
		Recipients: []string{"12345"},
	}

	err := channel.Send(context.Background(), message)
	if err == nil {
		t.Error("Send should fail when not configured")
	}
}

func TestTelegramChannel_Send_NoRecipients(t *testing.T) {
	channel := NewTelegramChannel()

	config := ChannelConfig{
		Type: ChannelTypeTelegram,
		Config: map[string]interface{}{
			"bot_token": "test-token",
		},
	}
	channel.Configure(config)

	message := Message{
		Body:       "Hello",
		Recipients: []string{},
	}

	err := channel.Send(context.Background(), message)
	if err == nil {
		t.Error("Send should fail without recipients")
	}
}

func TestTelegramChannel_SendTemplate_NotConfigured(t *testing.T) {
	channel := NewTelegramChannel()

	err := channel.SendTemplate(context.Background(), "welcome", map[string]interface{}{}, []string{"12345"})
	if err == nil {
		t.Error("SendTemplate should fail when not configured")
	}
}

func TestTelegramChannel_Close(t *testing.T) {
	channel := NewTelegramChannel()
	err := channel.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestTelegramMessage_Structure(t *testing.T) {
	msg := TelegramMessage{
		MessageID: 123,
		Chat:      &TelegramChat{ID: 456, Type: "private"},
		From:      &TelegramUser{ID: 789, FirstName: "John", Username: "john_doe"},
		Text:      "Hello",
		Date:      1234567890,
	}

	if msg.MessageID != 123 {
		t.Errorf("Expected MessageID 123, got %d", msg.MessageID)
	}
	if msg.Text != "Hello" {
		t.Errorf("Expected text 'Hello', got '%s'", msg.Text)
	}
	if msg.From == nil || msg.From.FirstName != "John" {
		t.Error("Expected FirstName 'John'")
	}
	if msg.Chat == nil || msg.Chat.ID != 456 {
		t.Error("Expected Chat ID 456")
	}
}

func TestTelegramUpdate_Structure(t *testing.T) {
	update := Update{
		UpdateID: 1,
		Message: &TelegramMessage{
			MessageID: 123,
			Text:      "Test",
			Chat:      &TelegramChat{ID: 456, Type: "private"},
			From:      &TelegramUser{ID: 789, FirstName: "Test"},
		},
	}

	if update.UpdateID != 1 {
		t.Errorf("Expected UpdateID 1, got %d", update.UpdateID)
	}
	if update.Message == nil {
		t.Fatal("Message is nil")
	}
	if update.Message.Text != "Test" {
		t.Errorf("Expected text 'Test', got '%s'", update.Message.Text)
	}
}

func TestTelegramChat_Types(t *testing.T) {
	chatTypes := []string{"private", "group", "supergroup", "channel"}
	for _, chatType := range chatTypes {
		chat := TelegramChat{
			ID:   123,
			Type: chatType,
		}
		if chat.Type != chatType {
			t.Errorf("Expected type '%s', got '%s'", chatType, chat.Type)
		}
	}
}

func TestTelegramUser_Fields(t *testing.T) {
	user := TelegramUser{
		ID:        12345,
		FirstName: "John",
		LastName:  "Doe",
		Username:  "johndoe",
	}

	if user.ID != 12345 {
		t.Errorf("Expected ID 12345, got %d", user.ID)
	}
	if user.FirstName != "John" {
		t.Errorf("Expected FirstName 'John', got '%s'", user.FirstName)
	}
	if user.LastName != "Doe" {
		t.Errorf("Expected LastName 'Doe', got '%s'", user.LastName)
	}
	if user.Username != "johndoe" {
		t.Errorf("Expected Username 'johndoe', got '%s'", user.Username)
	}
}

func TestTelegramChannel_Receive_NotConfigured(t *testing.T) {
	channel := NewTelegramChannel()

	handler := func(ctx context.Context, msg IncomingMessage) error {
		return nil
	}

	err := channel.Receive(context.Background(), handler)
	if err == nil {
		t.Error("Receive should fail when not configured")
	}
}
