# Email Pillar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an email sending Pillar to go-atlas with SMTP and Tencent Cloud SES drivers, supporting direct send, template mode, and attachments.

**Architecture:** Manager + Factory pattern aligned with SMS Pillar. Manager handles multi-instance lifecycle with lazy initialization; Factory dispatches to drivers by config `driver` field. Two drivers: SMTP (universal, direct mode only) and Tencent Cloud SES (direct + template modes).

**Tech Stack:** Go 1.25, `net/smtp` + `crypto/tls` + `mime/multipart` (SMTP driver), `tencentcloud-sdk-go/ses` (Tencent driver), existing Atlas Pillar framework.

**Spec:** `docs/superpowers/specs/2026-03-31-email-pillar-design.md`

---

## File Structure

```
pillar/email/
├── email.go     # Email interface, SendRequest, Attachment, ContentType, errors
├── factory.go   # Config structs (Config, SMTPConfig, TencentConfig), DefaultName, newEmail() factory
├── pillar.go    # Manager struct, Pillar()/Of(), Get/Default/Names, Option, Init/Stop/Health
├── smtp.go      # smtpClient implementing Email via net/smtp
└── tencent.go   # TencentEmail implementing Email via tencentcloud-sdk-go/ses
```

---

### Task 1: Interface and Types (`email.go`)

**Files:**
- Create: `pillar/email/email.go`

- [ ] **Step 1: Create `email.go` with interface, types, and errors**

```go
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
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./pillar/email/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add pillar/email/email.go
git commit -m "feat(email): add Email interface, SendRequest, and error definitions"
```

---

### Task 2: Config and Factory (`factory.go`)

**Files:**
- Create: `pillar/email/factory.go`

- [ ] **Step 1: Create `factory.go` with config structs and factory function**

```go
package email

import "fmt"

// DefaultName is the key used by Default().
const DefaultName = "default"

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

// newEmail creates an Email instance based on the driver specified in Config.
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

Note: This will not compile yet because `NewSMTP` and `NewTencent` are not defined. That is expected — they will be added in Tasks 4 and 5.

- [ ] **Step 2: Commit**

```bash
git add pillar/email/factory.go
git commit -m "feat(email): add Config structs and driver factory"
```

---

### Task 3: Manager and Pillar Lifecycle (`pillar.go`)

**Files:**
- Create: `pillar/email/pillar.go`

- [ ] **Step 1: Create `pillar.go` with Manager, Pillar(), Of(), and lifecycle methods**

```go
package email

import (
	"context"
	"fmt"
	"sync"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Manager manages multiple named Email instances with lazy initialization.
type Manager struct {
	configs map[string]Config

	mu       sync.RWMutex
	services map[string]Email
}

// Get returns the named Email instance, initializing it on first access.
func (m *Manager) Get(name string) (Email, error) {
	// Fast path: already initialized.
	m.mu.RLock()
	if s, ok := m.services[name]; ok {
		m.mu.RUnlock()
		return s, nil
	}
	m.mu.RUnlock()

	// Slow path: initialize.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock.
	if s, ok := m.services[name]; ok {
		return s, nil
	}

	cfg, ok := m.configs[name]
	if !ok {
		return nil, fmt.Errorf("email: unknown instance %q", name)
	}

	s, err := newEmail(cfg)
	if err != nil {
		return nil, fmt.Errorf("email: init %q: %w", name, err)
	}

	m.services[name] = s
	return s, nil
}

// Default returns the "default" Email instance.
func (m *Manager) Default() (Email, error) {
	return m.Get(DefaultName)
}

// Names returns all configured Email instance names.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// Pillar returns an atlas.Option that registers the Email Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		mgr := &Manager{}
		for _, opt := range opts {
			opt(mgr)
		}
		a.Register(mgr)
	}
}

// Of retrieves the Email Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager {
	return atlas.Use[*Manager](a)
}

// Option configures the Email Pillar.
type Option func(*Manager)

// Ensure interface compliance at compile time.
var _ atlas.Pillar = (*Manager)(nil)
var _ atlas.HealthChecker = (*Manager)(nil)

func (m *Manager) Name() string { return "email" }

func (m *Manager) Init(core *atlas.Core) error {
	var cfg map[string]Config
	if err := core.Unmarshal("email", &cfg); err != nil {
		return fmt.Errorf("email: %w", err)
	}
	m.configs = cfg
	m.services = make(map[string]Email, len(cfg))
	return nil
}

func (m *Manager) Stop(_ context.Context) error {
	return nil
}

func (m *Manager) Health(ctx context.Context) error {
	svc, err := m.Default()
	if err != nil {
		return fmt.Errorf("email: health: %w", err)
	}
	return svc.Ping(ctx)
}
```

Note: This will not compile yet because `NewSMTP` and `NewTencent` (referenced by `newEmail` in factory.go) are not defined. They will be added in Tasks 4 and 5.

- [ ] **Step 2: Commit**

```bash
git add pillar/email/pillar.go
git commit -m "feat(email): add Manager with Pillar lifecycle and multi-instance support"
```

---

### Task 4: SMTP Driver (`smtp.go`)

**Files:**
- Create: `pillar/email/smtp.go`

- [ ] **Step 1: Create `smtp.go` with SMTP driver implementation**

```go
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// smtpClient implements Email using SMTP.
type smtpClient struct {
	cfg SMTPConfig
}

// NewSMTP creates a new SMTP email client.
func NewSMTP(cfg SMTPConfig) (Email, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("email/smtp: host is required")
	}
	if cfg.From == "" {
		return nil, fmt.Errorf("email/smtp: from is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return &smtpClient{cfg: cfg}, nil
}

func (s *smtpClient) Send(_ context.Context, req *SendRequest) error {
	if req.TemplateID != "" {
		return ErrTemplateNotSupported
	}
	if len(req.To) == 0 {
		return ErrInvalidRecipient
	}
	if req.Subject == "" {
		return fmt.Errorf("email/smtp: subject is required for direct mode")
	}

	from := s.cfg.From
	if req.From != "" {
		from = req.From
	}

	msg := s.buildMessage(from, req)
	recipients := s.collectRecipients(req)
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	if s.cfg.TLS {
		return s.sendWithTLS(addr, auth, from, recipients, msg)
	}
	return smtp.SendMail(addr, auth, from, recipients, msg)
}

func (s *smtpClient) Ping(_ context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("email/smtp: ping: %w", err)
	}
	return conn.Close()
}

// sendWithTLS sends email over a direct TLS connection (port 465).
func (s *smtpClient) sendWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	tlsConfig := &tls.Config{ServerName: s.cfg.Host}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("email/smtp: tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("email/smtp: new client: %w", err)
	}
	defer client.Close()

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("email/smtp: auth: %w", err)
	}
	if err = client.Mail(from); err != nil {
		return fmt.Errorf("email/smtp: mail from: %w", err)
	}
	for _, rcpt := range to {
		if err = client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("email/smtp: rcpt %q: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email/smtp: data: %w", err)
	}
	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("email/smtp: write: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("email/smtp: close data: %w", err)
	}

	return client.Quit()
}

// collectRecipients merges To, Cc, and Bcc into a single slice.
func (s *smtpClient) collectRecipients(req *SendRequest) []string {
	recipients := make([]string, 0, len(req.To)+len(req.Cc)+len(req.Bcc))
	recipients = append(recipients, req.To...)
	recipients = append(recipients, req.Cc...)
	recipients = append(recipients, req.Bcc...)
	return recipients
}

// encodeHeader encodes a header value using RFC 2047 if it contains non-ASCII characters.
func encodeHeader(value string) string {
	encoder := mime.BEncoding
	return encoder.Encode("UTF-8", value)
}

// buildMessage constructs the full MIME message.
func (s *smtpClient) buildMessage(from string, req *SendRequest) []byte {
	var buf bytes.Buffer

	// Headers
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(req.To, ", ")))
	if len(req.Cc) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(req.Cc, ", ")))
	}
	if req.ReplyTo != "" {
		buf.WriteString(fmt.Sprintf("Reply-To: %s\r\n", req.ReplyTo))
	}
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", encodeHeader(req.Subject)))
	buf.WriteString("MIME-Version: 1.0\r\n")

	contentType := string(req.ContentType)
	if contentType == "" {
		contentType = string(ContentTypeHTML)
	}

	if len(req.Attachments) == 0 {
		// Simple message without attachments.
		buf.WriteString(fmt.Sprintf("Content-Type: %s; charset=UTF-8\r\n", contentType))
		buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		buf.WriteString(base64.StdEncoding.EncodeToString([]byte(req.Body)))
	} else {
		// Multipart message with attachments.
		writer := multipart.NewWriter(&buf)
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n\r\n", writer.Boundary()))

		// Body part
		bodyHeader := make(textproto.MIMEHeader)
		bodyHeader.Set("Content-Type", fmt.Sprintf("%s; charset=UTF-8", contentType))
		bodyHeader.Set("Content-Transfer-Encoding", "base64")
		bodyPart, _ := writer.CreatePart(bodyHeader)
		bodyPart.Write([]byte(base64.StdEncoding.EncodeToString([]byte(req.Body))))

		// Attachment parts
		for _, att := range req.Attachments {
			attHeader := make(textproto.MIMEHeader)
			mimeType := att.MIMEType
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			attHeader.Set("Content-Type", mimeType)
			attHeader.Set("Content-Transfer-Encoding", "base64")
			attHeader.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Filename))
			attPart, _ := writer.CreatePart(attHeader)
			attPart.Write([]byte(base64.StdEncoding.EncodeToString(att.Content)))
		}

		writer.Close()
	}

	return buf.Bytes()
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./pillar/email/`
Expected: may fail because `NewTencent` is not yet defined. If so, temporarily comment out the `tencentcloud` case in `factory.go` to verify SMTP code compiles, then revert.

Alternative: proceed to Task 5 first, then compile together.

- [ ] **Step 3: Commit**

```bash
git add pillar/email/smtp.go
git commit -m "feat(email): add SMTP driver with TLS, MIME encoding, and attachments"
```

---

### Task 5: Tencent Cloud SES Driver (`tencent.go`)

**Files:**
- Create: `pillar/email/tencent.go`

- [ ] **Step 1: Add `tencentcloud-sdk-go/ses` dependency**

Run: `cd /Users/nullkey/laoshen/go-atlas && go get github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ses@v1.3.57`

- [ ] **Step 2: Create `tencent.go` with Tencent Cloud SES driver implementation**

```go
package email

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tcses "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ses/v20201002"
)

// TencentEmail implements Email using Tencent Cloud SES.
type TencentEmail struct {
	client *tcses.Client
	cfg    TencentConfig
}

// NewTencent creates a new Tencent Cloud SES email client.
func NewTencent(cfg TencentConfig) (Email, error) {
	if cfg.Region == "" {
		cfg.Region = "ap-hongkong"
	}

	credential := common.NewCredential(cfg.SecretID, cfg.SecretKey)
	cpf := profile.NewClientProfile()

	client, err := tcses.NewClient(credential, cfg.Region, cpf)
	if err != nil {
		return nil, fmt.Errorf("email: create tencent client: %w", err)
	}

	return &TencentEmail{client: client, cfg: cfg}, nil
}

func (t *TencentEmail) Send(ctx context.Context, req *SendRequest) error {
	if len(req.To) == 0 {
		return ErrInvalidRecipient
	}

	from := t.cfg.From
	if req.From != "" {
		from = req.From
	}

	request := tcses.NewSendEmailRequest()
	request.FromEmailAddress = common.StringPtr(from)
	request.Destination = &tcses.Destination{
		ToEmailAddress:  common.StringPtrs(req.To),
		CcEmailAddress:  common.StringPtrs(req.Cc),
		BccEmailAddress: common.StringPtrs(req.Bcc),
	}

	if req.ReplyTo != "" {
		request.ReplyToAddresses = common.StringPtr(req.ReplyTo)
	}

	if req.TemplateID != "" {
		// Template mode.
		templateID, err := strconv.ParseUint(req.TemplateID, 10, 64)
		if err != nil {
			return fmt.Errorf("email/tencent: invalid template ID %q: %w", req.TemplateID, err)
		}
		templateData, err := json.Marshal(req.TemplateData)
		if err != nil {
			return fmt.Errorf("email/tencent: marshal template data: %w", err)
		}
		request.Template = &tcses.Template{
			TemplateID:   common.Uint64Ptr(templateID),
			TemplateData: common.StringPtr(string(templateData)),
		}
		if req.Subject != "" {
			request.Subject = common.StringPtr(req.Subject)
		}
	} else {
		// Direct mode.
		request.Subject = common.StringPtr(req.Subject)
		request.Simple = &tcses.Simple{
			Html: common.StringPtr(req.Body),
		}
		if req.ContentType == ContentTypePlain {
			request.Simple = &tcses.Simple{
				Text: common.StringPtr(req.Body),
			}
		}
	}

	// Attachments.
	if len(req.Attachments) > 0 {
		attachments := make([]*tcses.Attachment, 0, len(req.Attachments))
		for _, att := range req.Attachments {
			attachments = append(attachments, &tcses.Attachment{
				FileName: common.StringPtr(att.Filename),
				Content:  common.StringPtr(base64.StdEncoding.EncodeToString(att.Content)),
			})
		}
		request.Attachments = attachments
	}

	_, err := t.client.SendEmailWithContext(ctx, request)
	if err != nil {
		if sdkErr, ok := err.(*tcerrors.TencentCloudSDKError); ok {
			return fmt.Errorf("%w: [%s] %s", ErrProviderError, sdkErr.GetCode(), sdkErr.GetMessage())
		}
		return fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	return nil
}

func (t *TencentEmail) Ping(_ context.Context) error {
	return nil
}
```

- [ ] **Step 3: Verify the full package compiles**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./pillar/email/`
Expected: no errors

- [ ] **Step 4: Run `go mod tidy`**

Run: `cd /Users/nullkey/laoshen/go-atlas && go mod tidy`
Expected: clean up unused deps, no errors

- [ ] **Step 5: Commit**

```bash
git add pillar/email/tencent.go go.mod go.sum
git commit -m "feat(email): add Tencent Cloud SES driver with template and attachment support"
```

---

### Task 6: Update Example Config

**Files:**
- Modify: `example/config.yaml`

- [ ] **Step 1: Add email config section to `example/config.yaml`**

Add the following after the existing `sms` section (around line 69):

```yaml
email:
  default:
    driver: "tencentcloud"
    tencent:
      secret_id: ""
      secret_key: ""
      from: ""
      region: "ap-hongkong"
  # Example: SMTP instance for verification codes
  # verify:
  #   driver: "smtp"
  #   smtp:
  #     host: "smtp.example.com"
  #     port: 465
  #     username: ""
  #     password: ""
  #     from: "verify@example.com"
  #     tls: true
```

- [ ] **Step 2: Verify config parses**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./example/`
Expected: no errors (email Pillar is not registered in example/main.go, so config is just data)

- [ ] **Step 3: Commit**

```bash
git add example/config.yaml
git commit -m "feat(email): add email config section to example config"
```

---

### Task 7: Verify Full Build

**Files:** None (verification only)

- [ ] **Step 1: Build all packages**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./...`
Expected: no errors

- [ ] **Step 2: Run all existing tests**

Run: `cd /Users/nullkey/laoshen/go-atlas && go test ./...`
Expected: all existing tests pass, no regressions

- [ ] **Step 3: Run go vet**

Run: `cd /Users/nullkey/laoshen/go-atlas && go vet ./pillar/email/...`
Expected: no issues
