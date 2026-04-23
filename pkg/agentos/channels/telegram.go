package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// TelegramChannel implements Telegram Bot API for messaging
type TelegramChannel struct {
	name    string
	botToken string
	apiURL   string
	client   *http.Client
	webhook  string
	configured bool
}

// NewTelegramChannel creates a new Telegram channel
func NewTelegramChannel() *TelegramChannel {
	return &TelegramChannel{
		name:    "telegram",
		apiURL:  "https://api.telegram.org/bot",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Type returns the channel type
func (t *TelegramChannel) Type() ChannelType {
	return ChannelTypeTelegram
}

// Name returns the channel name
func (t *TelegramChannel) Name() string {
	return t.name
}

// Configure sets up the Telegram channel
func (t *TelegramChannel) Configure(config ChannelConfig) error {
	if config.Type != ChannelTypeTelegram {
		return fmt.Errorf("invalid channel type: %s", config.Type)
	}

	token, ok := config.Config["bot_token"].(string)
	if !ok || token == "" {
		return fmt.Errorf("bot_token is required for Telegram channel")
	}

	t.botToken = token
	t.configured = true

	if webhook, ok := config.Config["webhook_url"].(string); ok {
		t.webhook = webhook
	}

	return nil
}

// IsConfigured returns true if the channel is configured
func (t *TelegramChannel) IsConfigured() bool {
	return t.configured && t.botToken != ""
}

// Send sends a message via Telegram
func (t *TelegramChannel) Send(ctx context.Context, message Message) error {
	if !t.IsConfigured() {
		return fmt.Errorf("telegram channel not configured")
	}

	for _, recipient := range message.Recipients {
		chatID, err := t.resolveRecipient(recipient)
		if err != nil {
			return fmt.Errorf("failed to resolve recipient %s: %w", recipient, err)
		}

		if err := t.sendMessage(ctx, chatID, message); err != nil {
			return fmt.Errorf("failed to send message to %s: %w", recipient, err)
		}
	}

	return nil
}

// SendTemplate sends a message using a template
func (t *TelegramChannel) SendTemplate(ctx context.Context, templateName string, data map[string]interface{}, recipients []string) error {
	// Template handling would be implemented here
	// For now, just format the message
	message := Message{
		Body:       fmt.Sprintf("Template: %s with data: %v", templateName, data),
		Recipients: recipients,
	}
	return t.Send(ctx, message)
}

// Receive starts polling for messages (for bot mode)
func (t *TelegramChannel) Receive(ctx context.Context, handler MessageHandler) error {
	if !t.IsConfigured() {
		return fmt.Errorf("telegram channel not configured")
	}

	// If webhook is configured, use webhook mode
	if t.webhook != "" {
		return t.receiveWebhook(ctx, handler)
	}

	// Otherwise use polling
	return t.receivePolling(ctx, handler)
}

// Close closes the channel
func (t *TelegramChannel) Close() error {
	return nil
}

// sendMessage sends a text message
func (t *TelegramChannel) sendMessage(ctx context.Context, chatID int64, message Message) error {
	apiURL := fmt.Sprintf("%s%s/sendMessage", t.apiURL, t.botToken)

	params := url.Values{}
	params.Set("chat_id", strconv.FormatInt(chatID, 10))
	params.Set("text", t.formatMessage(message))
	params.Set("parse_mode", "HTML")

	if message.ReplyTo != "" {
		params.Set("reply_to_message_id", message.ReplyTo)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return err
	}

	req.URL.RawQuery = params.Encode()

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API error: %s", string(body))
	}

	return nil
}

// receivePolling polls for updates
func (t *TelegramChannel) receivePolling(ctx context.Context, handler MessageHandler) error {
	offset := int64(0)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			updates, err := t.getUpdates(ctx, offset)
			if err != nil {
				continue
			}

			for _, update := range updates {
				if update.UpdateID >= offset {
					offset = update.UpdateID + 1
				}

				if update.Message != nil {
					msg := IncomingMessage{
						ID:         strconv.FormatInt(update.Message.MessageID, 10),
						Channel:    ChannelTypeTelegram,
						From:       strconv.FormatInt(update.Message.From.ID, 10),
						Body:       update.Message.Text,
						RawData:    nil,
						ReceivedAt: time.Now(),
						Metadata: map[string]interface{}{
							"chat_id":    update.Message.Chat.ID,
							"username":   update.Message.From.Username,
							"first_name": update.Message.From.FirstName,
							"last_name":  update.Message.From.LastName,
						},
					}

					if err := handler(ctx, msg); err != nil {
						// Log error
					}
				}
			}
		}
	}
}

// receiveWebhook handles webhook mode
func (t *TelegramChannel) receiveWebhook(ctx context.Context, handler MessageHandler) error {
	// Webhook mode requires an HTTP server to be set up externally
	// This is a placeholder for webhook handling
	return fmt.Errorf("webhook mode not implemented in this channel")
}

// getUpdates fetches updates from Telegram API
func (t *TelegramChannel) getUpdates(ctx context.Context, offset int64) ([]Update, error) {
	apiURL := fmt.Sprintf("%s%s/getUpdates", t.apiURL, t.botToken)

	params := url.Values{}
	params.Set("offset", strconv.FormatInt(offset, 10))
	params.Set("limit", "100")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Ok     bool     `json:"ok"`
		Result []Update `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Ok {
		return nil, fmt.Errorf("telegram API returned not ok")
	}

	return result.Result, nil
}

// resolveRecipient resolves a recipient to a chat ID
func (t *TelegramChannel) resolveRecipient(recipient string) (int64, error) {
	// Try to parse as integer (chat ID)
	if chatID, err := strconv.ParseInt(recipient, 10, 64); err == nil {
		return chatID, nil
	}

	// Otherwise, try to get chat by username
	return t.getChatIDByUsername(recipient)
}

// getChatIDByUsername gets chat ID by username
func (t *TelegramChannel) getChatIDByUsername(username string) (int64, error) {
	// This would require storing username -> chat_id mappings
	// For now, return an error
	return 0, fmt.Errorf("username resolution not implemented")
}

// formatMessage formats a message for Telegram
func (t *TelegramChannel) formatMessage(message Message) string {
	if message.Subject != "" {
		return fmt.Sprintf("<b>%s</b>\n\n%s", escapeHTML(message.Subject), escapeHTML(message.Body))
	}
	return escapeHTML(message.Body)
}

// escapeHTML escapes HTML characters
func escapeHTML(s string) string {
	replacer := map[string]string{
		"&":  "&amp;",
		"<":  "&lt;",
		">":  "&gt;",
		"\"": "&quot;",
		"'":  "&#x27;",
	}

	result := s
	for old, new := range replacer {
		result = replaceAllString(result, old, new)
	}
	return result
}

// Update represents a Telegram update
type Update struct {
	UpdateID int64             `json:"update_id"`
	Message *TelegramMessage  `json:"message,omitempty"`
}

// TelegramMessage represents a Telegram message
type TelegramMessage struct {
	MessageID int64       `json:"message_id"`
	From      *TelegramUser `json:"from,omitempty"`
	Chat      *TelegramChat `json:"chat,omitempty"`
	Date      int64       `json:"date"`
	Text      string      `json:"text,omitempty"`
}

// TelegramUser represents a Telegram user
type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// TelegramChat represents a Telegram chat
type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
