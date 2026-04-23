package channels

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// WebhookChannel implements webhook communication channel
type WebhookChannel struct {
	name       string
	url        string
	method     string
	headers    map[string]string
	secret     string
	timeout    time.Duration
	httpClient *http.Client

	// Delivery tracking
	pendingDeliveries map[string]*WebhookDelivery
	deliveriesMu      sync.RWMutex

	// Configuration
	configured bool
	configMu   sync.RWMutex
}

// WebhookPayload represents a webhook payload
type WebhookPayload struct {
	ID          string                 `json:"id"`
	Timestamp   int64                  `json:"timestamp"`
	Event       string                 `json:"event"`
	Subject     string                 `json:"subject,omitempty"`
	Body        string                 `json:"body"`
	HTMLBody    string                 `json:"html_body,omitempty"`
	Recipients  []string               `json:"recipients"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Attachments []Attachment           `json:"attachments,omitempty"`
}

// WebhookMessage represents a received webhook message
type WebhookMessage struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
	Signature string    `json:"signature,omitempty"`
}

// WebhookResponse represents a webhook response
type WebhookResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code"`
}

// WebhookEndpoint represents a webhook endpoint configuration
type WebhookEndpoint struct {
	ID        string            `json:"id"`
	URL       string            `json:"url"`
	Secret    string            `json:"secret,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Events    []string          `json:"events,omitempty"`
	Active    bool              `json:"active"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// WebhookDelivery represents a webhook delivery attempt
type WebhookDelivery struct {
	ID          string    `json:"id"`
	MessageID   string    `json:"message_id"`
	EndpointID  string    `json:"endpoint_id"`
	URL         string    `json:"url"`
	Status      string    `json:"status"` // pending, delivered, failed
	Attempts    int       `json:"attempts"`
	LastAttempt time.Time `json:"last_attempt,omitempty"`
	NextRetry   time.Time `json:"next_retry,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// WebhookEventPayload represents a webhook event payload
type WebhookEventPayload struct {
	Event     string                 `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// WebhookManager manages webhook endpoints and deliveries
type WebhookManager struct {
	endpoints  map[string]*WebhookEndpoint
	deliveries map[string]*WebhookDelivery
	mu         sync.RWMutex
}

// NewWebhookChannel creates a new webhook channel
func NewWebhookChannel(name, url string) *WebhookChannel {
	return &WebhookChannel{
		name:              name,
		url:               url,
		method:            "POST",
		headers:           make(map[string]string),
		timeout:           30 * time.Second,
		httpClient:        &http.Client{Timeout: 30 * time.Second},
		pendingDeliveries: make(map[string]*WebhookDelivery),
	}
}

// NewWebhookManager creates a new webhook manager
func NewWebhookManager() *WebhookManager {
	return &WebhookManager{
		endpoints:  make(map[string]*WebhookEndpoint),
		deliveries: make(map[string]*WebhookDelivery),
	}
}

// Type returns the channel type
func (w *WebhookChannel) Type() ChannelType {
	return ChannelTypeWebhook
}

// Name returns the channel name
func (w *WebhookChannel) Name() string {
	return w.name
}

// Configure sets up the webhook channel
func (w *WebhookChannel) Configure(config ChannelConfig) error {
	if config.Type != ChannelTypeWebhook {
		return fmt.Errorf("invalid channel type: %s", config.Type)
	}

	w.configMu.Lock()
	defer w.configMu.Unlock()

	// Parse configuration
	if url, ok := config.Config["url"].(string); ok && url != "" {
		w.url = url
	} else {
		return fmt.Errorf("webhook URL is required")
	}

	if method, ok := config.Config["method"].(string); ok && method != "" {
		w.method = method
	} else {
		w.method = "POST"
	}

	if headers, ok := config.Config["headers"].(map[string]interface{}); ok {
		w.headers = make(map[string]string)
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				w.headers[key] = strValue
			}
		}
	}

	if secret, ok := config.Config["secret"].(string); ok {
		w.secret = secret
	}

	// Parse timeout - accept int, float64, or use default
	if timeout, ok := config.Config["timeout"].(float64); ok && timeout > 0 {
		w.timeout = time.Duration(timeout) * time.Second
	} else if timeout, ok := config.Config["timeout"].(int); ok && timeout > 0 {
		w.timeout = time.Duration(timeout) * time.Second
	}

	w.configured = true
	return nil
}

// IsConfigured returns true if the channel is configured
func (w *WebhookChannel) IsConfigured() bool {
	w.configMu.RLock()
	defer w.configMu.RUnlock()
	return w.configured && w.url != ""
}

// Send sends a message via webhook
func (w *WebhookChannel) Send(ctx context.Context, message Message) error {
	w.configMu.RLock()
	defer w.configMu.RUnlock()

	if !w.IsConfigured() {
		return fmt.Errorf("webhook channel not configured")
	}

	if len(message.Recipients) == 0 {
		return fmt.Errorf("no recipients specified")
	}

	// Build payload
	payload := WebhookPayload{
		ID:         message.ID,
		Timestamp:  time.Now().Unix(),
		Event:      "message",
		Subject:    message.Subject,
		Body:       message.Body,
		HTMLBody:   message.HTMLBody,
		Recipients: message.Recipients,
		Metadata:   message.Metadata,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Send to each recipient (which is a webhook URL)
	for _, recipient := range message.Recipients {
		if err := w.sendWebhook(ctx, recipient, payloadJSON, message); err != nil {
			return fmt.Errorf("failed to send to %s: %w", recipient, err)
		}
	}

	return nil
}

// SendTemplate sends a message using a template via webhook
func (w *WebhookChannel) SendTemplate(ctx context.Context, templateName string, data map[string]interface{}, recipients []string) error {
	w.configMu.RLock()
	defer w.configMu.RUnlock()

	if !w.IsConfigured() {
		return fmt.Errorf("webhook channel not configured")
	}

	// Build template payload
	payload := WebhookPayload{
		ID:         generateWebhookID(),
		Timestamp:  time.Now().Unix(),
		Event:      "template",
		Recipients: recipients,
		Metadata: map[string]interface{}{
			"template": templateName,
			"data":     data,
		},
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Send to each recipient
	for _, recipient := range recipients {
		if err := w.sendWebhook(ctx, recipient, payloadJSON, Message{}); err != nil {
			return fmt.Errorf("failed to send to %s: %w", recipient, err)
		}
	}

	return nil
}

// Receive starts receiving messages (not supported for webhook)
func (w *WebhookChannel) Receive(ctx context.Context, handler MessageHandler) error {
	return fmt.Errorf("webhook channel does not support receiving")
}

// Close closes the webhook channel
func (w *WebhookChannel) Close() error {
	// Nothing to close for webhook
	return nil
}

// sendWebhook sends a webhook request
func (w *WebhookChannel) sendWebhook(ctx context.Context, url string, payload []byte, message Message) error {
	req, err := http.NewRequestWithContext(ctx, w.method, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AgentOS-Webhook/1.0")
	req.Header.Set("X-Webhook-ID", generateWebhookID())

	for key, value := range w.headers {
		req.Header.Set(key, value)
	}

	// Add signature if secret is configured
	if w.secret != "" {
		signature := w.signPayload(payload)
		req.Header.Set("X-Webhook-Signature", signature)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// signPayload creates HMAC signature for payload
func (w *WebhookChannel) signPayload(payload []byte) string {
	if w.secret == "" {
		return ""
	}

	h := hmac.New(sha256.New, []byte(w.secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// RegisterEndpoint registers a webhook endpoint
func (m *WebhookManager) RegisterEndpoint(endpoint WebhookEndpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if endpoint.ID == "" {
		endpoint.ID = generateWebhookID()
	}

	if endpoint.CreatedAt.IsZero() {
		endpoint.CreatedAt = time.Now()
	}
	endpoint.UpdatedAt = time.Now()

	if endpoint.Active {
		endpoint.Active = true
	}

	m.endpoints[endpoint.ID] = &endpoint
	return nil
}

// UnregisterEndpoint removes a webhook endpoint
func (m *WebhookManager) UnregisterEndpoint(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.endpoints, id)
	return nil
}

// GetEndpoint retrieves an endpoint by ID
func (m *WebhookManager) GetEndpoint(id string) *WebhookEndpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.endpoints[id]
}

// GetEndpoints returns all registered endpoints
func (m *WebhookManager) GetEndpoints() []*WebhookEndpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	endpoints := make([]*WebhookEndpoint, 0, len(m.endpoints))
	for _, endpoint := range m.endpoints {
		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}

// GetEndpointsForEvent returns endpoints subscribed to an event
func (m *WebhookManager) GetEndpointsForEvent(event string) []*WebhookEndpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*WebhookEndpoint
	for _, endpoint := range m.endpoints {
		if !endpoint.Active {
			continue
		}
		for _, e := range endpoint.Events {
			if e == event || e == "*" {
				result = append(result, endpoint)
				break
			}
		}
		// If no specific events, endpoint receives all
		if len(endpoint.Events) == 0 {
			result = append(result, endpoint)
		}
	}
	return result
}

// EnableEndpoint enables a webhook endpoint
func (m *WebhookManager) EnableEndpoint(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	endpoint, ok := m.endpoints[id]
	if !ok {
		return fmt.Errorf("endpoint not found")
	}

	endpoint.Active = true
	endpoint.UpdatedAt = time.Now()
	return nil
}

// DisableEndpoint disables a webhook endpoint
func (m *WebhookManager) DisableEndpoint(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	endpoint, ok := m.endpoints[id]
	if !ok {
		return fmt.Errorf("endpoint not found")
	}

	endpoint.Active = false
	endpoint.UpdatedAt = time.Now()
	return nil
}

// VerifySignature verifies webhook signature
func VerifySignature(payload []byte, signature, secret string) bool {
	if secret == "" || signature == "" {
		return false
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expectedSig := "sha256=" + hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(signature))
}

// generateWebhookID generates a unique webhook ID
func generateWebhookID() string {
	return fmt.Sprintf("wh_%d_%d", time.Now().UnixNano(), time.Now().Unix())
}
