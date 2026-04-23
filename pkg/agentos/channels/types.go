package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ChannelType represents the type of communication channel
type ChannelType string

const (
	ChannelTypeTelegram  ChannelType = "telegram"
	ChannelTypeEmail     ChannelType = "email"
	ChannelTypeWebhook   ChannelType = "webhook"
	ChannelTypeSMS       ChannelType = "sms"
	ChannelTypeWhatsApp  ChannelType = "whatsapp"
)

// Channel is the interface for all communication channels
type Channel interface {
	// Type returns the channel type
	Type() ChannelType

	// Name returns the channel name
	Name() string

	// Configure sets up the channel with configuration
	Configure(config ChannelConfig) error

	// Send sends a message to the channel
	Send(ctx context.Context, message Message) error

	// SendTemplate sends a message using a template
	SendTemplate(ctx context.Context, templateName string, data map[string]interface{}, recipients []string) error

	// Receive starts receiving messages (for channels that support it)
	Receive(ctx context.Context, handler MessageHandler) error

	// IsConfigured returns true if the channel is properly configured
	IsConfigured() bool

	// Close closes the channel connection
	Close() error
}

// MessageHandler is called when a message is received from a channel
type MessageHandler func(ctx context.Context, message IncomingMessage) error

// Message represents a message to be sent
type Message struct {
	ID          string                 `json:"id"`
	Subject     string                 `json:"subject,omitempty"`
	Body        string                 `json:"body"`
	HTMLBody    string                 `json:"html_body,omitempty"`
	Recipients  []string               `json:"recipients"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Priority    MessagePriority        `json:"priority"`
	ScheduledAt *time.Time             `json:"scheduled_at,omitempty"`
	ReplyTo     string                 `json:"reply_to,omitempty"`
}

// IncomingMessage represents a message received from a channel
type IncomingMessage struct {
	ID         string                 `json:"id"`
	Channel    ChannelType            `json:"channel"`
	From       string                 `json:"from"`
	Body       string                 `json:"body"`
	RawData    []byte                 `json:"raw_data"`
	ReceivedAt time.Time              `json:"received_at"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// MessagePriority represents the priority of a message
type MessagePriority string

const (
	PriorityLow      MessagePriority = "low"
	PriorityNormal   MessagePriority = "normal"
	PriorityHigh     MessagePriority = "high"
	PriorityUrgent   MessagePriority = "urgent"
)

// Attachment represents a file attachment
type Attachment struct {
	Name     string `json:"name"`
	Content  []byte `json:"content"`
	MIMEType string `json:"mime_type"`
}

// ChannelConfig represents configuration for a channel
type ChannelConfig struct {
	Type    ChannelType            `json:"type"`
	Name    string                 `json:"name"`
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config"`
}

// Manager manages multiple channels
type Manager struct {
	channels map[ChannelType]Channel
	handlers map[ChannelType][]MessageHandler
}

// NewManager creates a new channel manager
func NewManager() *Manager {
	return &Manager{
		channels: make(map[ChannelType]Channel),
		handlers: make(map[ChannelType][]MessageHandler),
	}
}

// Register registers a channel with the manager
func (m *Manager) Register(channel Channel) error {
	if channel == nil {
		return fmt.Errorf("channel cannot be nil")
	}

	if !channel.IsConfigured() {
		return fmt.Errorf("channel %s is not configured", channel.Name())
	}

	m.channels[channel.Type()] = channel
	return nil
}

// Get retrieves a channel by type
func (m *Manager) Get(channelType ChannelType) (Channel, error) {
	channel, ok := m.channels[channelType]
	if !ok {
		return nil, fmt.Errorf("channel type %s not found", channelType)
	}
	return channel, nil
}

// GetAll returns all registered channels
func (m *Manager) GetAll() []Channel {
	channels := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		channels = append(channels, ch)
	}
	return channels
}

// Send sends a message through the specified channel type
func (m *Manager) Send(ctx context.Context, channelType ChannelType, message Message) error {
	channel, err := m.Get(channelType)
	if err != nil {
		return err
	}
	return channel.Send(ctx, message)
}

// SendAll sends a message through all configured channels
func (m *Manager) SendAll(ctx context.Context, message Message) []error {
	var errors []error
	for _, channel := range m.channels {
		if err := channel.Send(ctx, message); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", channel.Type(), err))
		}
	}
	return errors
}

// RegisterHandler registers a message handler for a channel type
func (m *Manager) RegisterHandler(channelType ChannelType, handler MessageHandler) {
	m.handlers[channelType] = append(m.handlers[channelType], handler)
}

// StartReceiving starts receiving messages from all channels that support it
func (m *Manager) StartReceiving(ctx context.Context) error {
	for channelType, channel := range m.channels {
		handlers := m.handlers[channelType]
		if len(handlers) == 0 {
			continue
		}

		// Wrap handlers
		handler := func(ctx context.Context, msg IncomingMessage) error {
			for _, h := range handlers {
				if err := h(ctx, msg); err != nil {
					return err
				}
			}
			return nil
		}

		go func(ch Channel) {
			if err := ch.Receive(ctx, handler); err != nil {
				// Log error but don't stop
			}
		}(channel)
	}
	return nil
}

// Close closes all channels
func (m *Manager) Close() error {
	var errors []error
	for _, channel := range m.channels {
		if err := channel.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to close %d channels", len(errors))
	}
	return nil
}

// MessageFormatter formats messages with templates
type MessageFormatter struct {
	templates map[string]string
}

// NewMessageFormatter creates a new message formatter
func NewMessageFormatter() *MessageFormatter {
	return &MessageFormatter{
		templates: make(map[string]string),
	}
}

// RegisterTemplate registers a template
func (f *MessageFormatter) RegisterTemplate(name, template string) {
	f.templates[name] = template
}

// Format formats a message using a template
func (f *MessageFormatter) Format(templateName string, data map[string]interface{}) (string, error) {
	template, ok := f.templates[templateName]
	if !ok {
		return "", fmt.Errorf("template %s not found", templateName)
	}

	// Simple placeholder replacement
	result := template
	for key, value := range data {
		placeholder := fmt.Sprintf("{{%s}}", key)
		result = replaceString(result, placeholder, fmt.Sprintf("%v", value))
	}

	return result, nil
}

// replaceString replaces all occurrences of old with new in s
func replaceString(s, old, new string) string {
	return replaceAllString(s, old, new)
}

// replaceAllString replaces all occurrences of old with new in s
func replaceAllString(s, old, new string) string {
	// Simple implementation
	result := ""
	for {
		found := false
		for i := 0; i <= len(s)-len(old); i++ {
			if s[i:i+len(old)] == old {
				result += s[:i] + new
				s = s[i+len(old):]
				found = true
				break
			}
		}
		if !found {
			result += s
			break
		}
	}
	return result
}

// NotificationService provides high-level notification capabilities
type NotificationService struct {
	manager   *Manager
	formatter *MessageFormatter
}

// NewNotificationService creates a new notification service
func NewNotificationService(manager *Manager) *NotificationService {
	return &NotificationService{
		manager:   manager,
		formatter: NewMessageFormatter(),
	}
}

// NotifyUser sends a notification to a user through their preferred channels
func (s *NotificationService) NotifyUser(ctx context.Context, userID string, notification Notification) error {
	// Get user's preferred channels
	// This would typically come from a user preferences store

	// Send through all channels for now
	message := Message{
		Subject:    notification.Title,
		Body:       notification.Message,
		Recipients: []string{userID},
		Priority:   notification.Priority,
	}

	errors := s.manager.SendAll(ctx, message)
	if len(errors) > 0 {
		return fmt.Errorf("failed to send notification: %v", errors)
	}
	return nil
}

// NotifyBroadcast sends a notification to multiple users
func (s *NotificationService) NotifyBroadcast(ctx context.Context, userIDs []string, notification Notification) error {
	message := Message{
		Subject:    notification.Title,
		Body:       notification.Message,
		Recipients: userIDs,
		Priority:   notification.Priority,
	}

	errors := s.manager.SendAll(ctx, message)
	if len(errors) > 0 {
		return fmt.Errorf("failed to broadcast notification: %v", errors)
	}
	return nil
}

// Notification represents a notification to be sent
type Notification struct {
	Title      string                 `json:"title"`
	Message    string                 `json:"message"`
	Priority   MessagePriority        `json:"priority"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Template   string                 `json:"template,omitempty"`
	Channel    ChannelType            `json:"channel,omitempty"`
}

// ToJSON converts notification to JSON
func (n Notification) ToJSON() ([]byte, error) {
	return json.Marshal(n)
}
