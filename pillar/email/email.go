package email

import (
	"context"
	"errors"
)

// Standard email errors for unified error handling across drivers.
var (
	ErrTemplateNotSupported = errors.New("email: template mode not supported by this driver")
	ErrProviderError        = errors.New("email: provider error")
	ErrSendFailed           = errors.New("email: send failed")
	ErrInvalidRecipient     = errors.New("email: at least one recipient required")
)

// ContentType defines email content type.
type ContentType string

const (
	ContentTypePlain ContentType = "text/plain"
	ContentTypeHTML  ContentType = "text/html"
)

// SendRequest represents an email send request.
type SendRequest struct {
	From         string            // Sender address (optional, defaults to config "from")
	To           []string          // Recipients
	Cc           []string          // Carbon copy
	Bcc          []string          // Blind carbon copy
	ReplyTo      string            // Reply-to address
	Subject      string            // Subject (direct mode)
	Body         string            // Body content, supports HTML (direct mode)
	ContentType  ContentType       // "text/plain" or "text/html"; zero value treated as "text/html" by drivers
	TemplateID   string            // Template ID (template mode, cloud provider drivers)
	TemplateData map[string]string // Template variables (template mode)
	Attachments  []Attachment      // Attachment list
}

// Attachment represents an email attachment.
type Attachment struct {
	Filename string // File name
	Content  []byte // File content
	MIMEType string // MIME type, e.g. "application/pdf"
}

// Email defines the email sending capability.
type Email interface {
	// Send sends an email. Direct mode uses Subject+Body; template mode uses TemplateID+TemplateData.
	Send(ctx context.Context, req *SendRequest) error

	// Ping checks connectivity (SMTP attempts connection; cloud providers call API health).
	Ping(ctx context.Context) error
}
