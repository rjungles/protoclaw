package channels

import (
	"bytes"
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"text/template"
	"time"
)

// EmailChannel implements SMTP email sending
type EmailChannel struct {
	name       string
	host       string
	port       int
	username   string
	password   string
	from       string
	fromName   string
	useTLS     bool
	configured bool
}

// NewEmailChannel creates a new email channel
func NewEmailChannel() *EmailChannel {
	return &EmailChannel{
		name: "email",
		port: 587,
	}
}

// Type returns the channel type
func (e *EmailChannel) Type() ChannelType {
	return ChannelTypeEmail
}

// Name returns the channel name
func (e *EmailChannel) Name() string {
	return e.name
}

// Configure sets up the email channel
func (e *EmailChannel) Configure(config ChannelConfig) error {
	if config.Type != ChannelTypeEmail {
		return fmt.Errorf("invalid channel type: %s", config.Type)
	}

	// Required: SMTP host
	host, ok := config.Config["host"].(string)
	if !ok || host == "" {
		return fmt.Errorf("host is required for Email channel")
	}
	e.host = host

	// Required: SMTP port
	if port, ok := config.Config["port"].(float64); ok {
		e.port = int(port)
	} else if port, ok := config.Config["port"].(int); ok {
		e.port = port
	}

	// Required: username
	username, ok := config.Config["username"].(string)
	if !ok || username == "" {
		return fmt.Errorf("username is required for Email channel")
	}
	e.username = username

	// Required: password
	password, ok := config.Config["password"].(string)
	if !ok || password == "" {
		return fmt.Errorf("password is required for Email channel")
	}
	e.password = password

	// Required: from address
	from, ok := config.Config["from"].(string)
	if !ok || from == "" {
		return fmt.Errorf("from is required for Email channel")
	}
	e.from = from

	// Optional: from name
	if fromName, ok := config.Config["from_name"].(string); ok {
		e.fromName = fromName
	}

	// Optional: use TLS
	if useTLS, ok := config.Config["use_tls"].(bool); ok {
		e.useTLS = useTLS
	} else {
		e.useTLS = true
	}

	e.configured = true
	return nil
}

// IsConfigured returns true if the channel is configured
func (e *EmailChannel) IsConfigured() bool {
	return e.configured && e.host != "" && e.username != ""
}

// Send sends an email
func (e *EmailChannel) Send(ctx context.Context, message Message) error {
	if !e.IsConfigured() {
		return fmt.Errorf("email channel not configured")
	}

	for _, recipient := range message.Recipients {
		if err := e.sendEmail(ctx, recipient, message); err != nil {
			return fmt.Errorf("failed to send email to %s: %w", recipient, err)
		}
	}

	return nil
}

// SendTemplate sends an email using a template
func (e *EmailChannel) SendTemplate(ctx context.Context, templateName string, data map[string]interface{}, recipients []string) error {
	// Template handling would format the message
	subject := fmt.Sprintf("Template: %s", templateName)
	body := fmt.Sprintf("Data: %v", data)

	message := Message{
		Subject:    subject,
		Body:       body,
		Recipients: recipients,
	}

	return e.Send(ctx, message)
}

// Receive starts receiving emails (requires IMAP/POP3 server)
func (e *EmailChannel) Receive(ctx context.Context, handler MessageHandler) error {
	// Email receiving requires IMAP or POP3 implementation
	// This is a placeholder
	return fmt.Errorf("email receiving not implemented in this channel")
}

// Close closes the channel
func (e *EmailChannel) Close() error {
	return nil
}

// sendEmail sends a single email
func (e *EmailChannel) sendEmail(ctx context.Context, recipient string, message Message) error {
	// Build email headers
	var buf bytes.Buffer

	from := e.from
	if e.fromName != "" {
		from = fmt.Sprintf("%s <%s>", e.fromName, e.from)
	}

	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", recipient))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", message.Subject))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123)))
	buf.WriteString("MIME-Version: 1.0\r\n")

	// Content-Type
	if message.HTMLBody != "" {
		// Multipart message
		boundary := "boundary-" + fmt.Sprintf("%d", time.Now().Unix())
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
		buf.WriteString(message.Body)
		buf.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
		buf.WriteString(message.HTMLBody)
		buf.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))
	} else {
		buf.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
		buf.WriteString(message.Body)
	}

	// Send via SMTP
	addr := fmt.Sprintf("%s:%d", e.host, e.port)

	auth := smtp.PlainAuth("", e.username, e.password, e.host)

	err := smtp.SendMail(
		addr,
		auth,
		e.from,
		[]string{recipient},
		buf.Bytes(),
	)

	if err != nil {
		return fmt.Errorf("SMTP error: %w", err)
	}

	return nil
}

// parseTemplate parses an email template
func (e *EmailChannel) parseTemplate(name, content string) (*template.Template, error) {
	return template.New(name).Parse(content)
}

// replaceTemplateVars replaces template variables
func replaceTemplateVars(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return result
}
