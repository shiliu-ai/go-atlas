# Email Pillar Design

## Overview

Add an email sending component to go-atlas as a Pillar (extension plugin), following the established Manager + Factory pattern used by the SMS Pillar. Supports multiple named instances, dual drivers (SMTP + Tencent Cloud SES), direct send and template modes, and attachments.

## Motivation

Users need email capabilities for:
- Email login (verification codes)
- Notifications (system alerts, account activity)
- Transactional emails (reports with attachments)

No email Pillar exists in go-atlas today. SMS Pillar provides a proven pattern for message-sending components.

## Architecture

### Pattern: Manager + Factory (aligned with SMS Pillar)

- **Manager**: Multi-instance management with lazy initialization and double-check locking
- **Factory**: Driver-based dispatch (`smtp`, `tencentcloud`)
- **Pillar lifecycle**: `Pillar()` register -> `Of(a)` retrieve -> `Get(name)` / `Default()` use

### File Structure

```
pillar/email/
├── pillar.go    # Manager + Pillar()/Of() entry points + Option
├── email.go     # Email interface + SendRequest + Attachment + errors
├── factory.go   # Config definitions + newEmail() factory
├── smtp.go      # SMTP driver
└── tencent.go   # Tencent Cloud SES driver
```

## Core Interface

```go
package email

import "context"

// Email defines the email sending capability.
type Email interface {
    // Send sends an email. Direct mode uses Subject+Body; template mode uses TemplateID+TemplateData.
    Send(ctx context.Context, req *SendRequest) error
    // Ping checks connectivity (SMTP attempts connection; cloud providers call API health).
    Ping(ctx context.Context) error
}

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

// ContentType defines email content type.
type ContentType string

const (
    ContentTypePlain ContentType = "text/plain"
    ContentTypeHTML  ContentType = "text/html"
)
```

## Configuration

### Config Structures

```go
// Config is the unified email configuration for a single instance.
type Config struct {
    Driver  string        `mapstructure:"driver"` // "smtp" or "tencentcloud"
    SMTP    SMTPConfig    `mapstructure:"smtp"`
    Tencent TencentConfig `mapstructure:"tencent"`
}

// SMTPConfig holds SMTP driver configuration.
type SMTPConfig struct {
    Host     string `mapstructure:"host"`     // SMTP server address
    Port     int    `mapstructure:"port"`     // Port (25/465/587), defaults to 587
    Username string `mapstructure:"username"` // Auth username
    Password string `mapstructure:"password"` // Auth password
    From     string `mapstructure:"from"`     // Default sender address
    TLS      bool   `mapstructure:"tls"`      // Enable direct TLS (for port 465)
}

// TencentConfig holds Tencent Cloud SES configuration.
type TencentConfig struct {
    SecretID  string `mapstructure:"secret_id"`
    SecretKey string `mapstructure:"secret_key"`
    From      string `mapstructure:"from"`   // Sender address (must be verified in SES console)
    Region    string `mapstructure:"region"` // Default: "ap-hongkong"
}
```

### YAML Example

```yaml
email:
  default:                      # Notification emails - Tencent Cloud SES
    driver: "tencentcloud"
    tencent:
      secret_id: "AKIDxxx"
      secret_key: "xxx"
      from: "noreply@example.com"
      region: "ap-hongkong"
  verify:                       # Verification codes - SMTP
    driver: "smtp"
    smtp:
      host: "smtp.example.com"
      port: 465
      username: "verify@example.com"
      password: "xxx"
      from: "verify@example.com"
      tls: true
```

## Manager

```go
const DefaultName = "default"

type Manager struct {
    configs  map[string]Config
    services map[string]Email
    mu       sync.RWMutex
}

// Pillar registers the email component with Atlas.
func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        m := &Manager{}
        for _, opt := range opts {
            opt(m)
        }
        a.Register(m)
    }
}

// Of retrieves the Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager {
    return atlas.Use[*Manager](a)
}

// Option configures the Email Pillar.
type Option func(*Manager)

// Ensure interface compliance at compile time.
var _ atlas.Pillar = (*Manager)(nil)
var _ atlas.HealthChecker = (*Manager)(nil)
```

### Manager Methods

- `Get(name string) (Email, error)` — Lazy init with double-check locking
- `Default() (Email, error)` — Shorthand for `Get("default")`
- `Names() []string` — List all configured instance names

### Pillar Interface

- `Name() string` — returns `"email"`
- `Init(core *atlas.Core) error` — reads `email` config section via `core.Unmarshal`
- `Stop(ctx context.Context) error` — no-op (stateless connections)
- `Health(ctx context.Context) error` — pings default instance

## Factory

```go
func newEmail(cfg Config) (Email, error) {
    switch cfg.Driver {
    case "smtp":
        return NewSMTP(cfg.SMTP)
    case "tencentcloud":
        return NewTencent(cfg.Tencent)
    default:
        return nil, fmt.Errorf("email: unsupported driver %q", cfg.Driver)
    }
}
```

## Drivers

### SMTP Driver (`smtp.go`)

- Validates config on construction (host and from required, port defaults to 587)
- Direct send mode only; returns `ErrTemplateNotSupported` for template mode
- **Input validation**: `To` must not be empty (returns `ErrInvalidRecipient`); direct mode requires non-empty `Subject`
- TLS dual-mode: port 465 uses direct TLS connection; port 587/25 uses STARTTLS
- **MIME header encoding**: Subject and other headers containing non-ASCII characters (e.g. Chinese) must use RFC 2047 encoding (`=?UTF-8?B?...?=`), using Go's `mime.QEncoding` or `mime.BEncoding`
- MIME message construction: simple message for no attachments, multipart/mixed for attachments
- Ping: TCP dial to verify server reachability

### Tencent Cloud SES Driver (`tencent.go`)

- Uses `tencentcloud-sdk-go/ses` SDK, same auth pattern as SMS tencent driver
- **Input validation**: `To` must not be empty (returns `ErrInvalidRecipient`)
- Supports both direct mode (Subject + Simple.Html) and template mode (Template.TemplateID + TemplateData)
- **TemplateData serialization**: `map[string]string` must be serialized to JSON string via `json.Marshal` before passing to SDK's `Template.TemplateData` field
- Attachments via base64-encoded content
- Region defaults to `"ap-hongkong"`
- Error handling follows SMS tencent driver style (SDK error wrapping)
- Ping: no-op (same as SMS tencent driver)

## Errors

Uses `errors.New()` (not `fmt.Errorf`) to support `errors.Is` comparison, consistent with SMS Pillar.

```go
var (
    ErrTemplateNotSupported = errors.New("email: template mode not supported by this driver")
    ErrProviderError        = errors.New("email: provider error")
    ErrSendFailed           = errors.New("email: send failed")
    ErrInvalidRecipient     = errors.New("email: at least one recipient required")
)
```

## Usage Examples

### Register and Retrieve

```go
a := atlas.New("my-service",
    atlas.WithConfigPaths(".", "./config"),
    email.Pillar(),
)

em := email.Of(a)
```

### Send Verification Code (Direct Mode)

```go
mailer, err := em.Get("verify")
if err != nil {
    return err
}

err = mailer.Send(ctx, &email.SendRequest{
    To:      []string{"user@example.com"},
    Subject: "Your Verification Code",
    Body:    "<h1>Code: 123456</h1><p>Valid for 5 minutes.</p>",
})
```

### Send Notification (Template Mode, Tencent Cloud SES)

```go
mailer, err := em.Default()
if err != nil {
    return err
}

err = mailer.Send(ctx, &email.SendRequest{
    To:         []string{"user@example.com"},
    TemplateID: "12345",
    TemplateData: map[string]string{
        "username": "Alice",
        "action":   "logged into your account",
    },
})
```

### Send with Attachment

```go
err = mailer.Send(ctx, &email.SendRequest{
    To:      []string{"admin@example.com"},
    Subject: "Monthly Report",
    Body:    "<p>Please find the report attached.</p>",
    Attachments: []email.Attachment{
        {
            Filename: "report.pdf",
            Content:  pdfBytes,
            MIMEType: "application/pdf",
        },
    },
})
```

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Manager + Factory pattern | Alignment with SMS Pillar; proven pattern in go-atlas |
| SMTP + Tencent Cloud SES dual drivers | SMTP for universality; Tencent Cloud for production-grade delivery and template support |
| Direct + Template send modes | Direct mode works everywhere; template mode leverages cloud provider template management |
| Lazy initialization | Consistent with SMS; only init instances when first used |
| Attachments in interface | Low cost to support; framework would feel incomplete without it |
| SMTP rejects template mode | Clear error rather than silent failure; guides users to cloud drivers for template needs |
| HealthChecker on default instance | Follows SMS pattern; default instance is the critical path |

## Future Extensions

- Additional cloud drivers: AWS SES, Alibaba Cloud DirectMail
- Batch sending support
- Send rate limiting
