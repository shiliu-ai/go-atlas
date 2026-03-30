# SMS Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an `sms/` module to go-atlas with unified SMS interface, Tencent Cloud provider, and Manager pattern for multi-instance support.

**Architecture:** Follows the established Manager + Factory + Interface pattern (same as `storage/`). Core interface in `sms.go`, config/factory/manager in `factory.go`, Tencent Cloud provider in `tencent.go`. Atlas integration via `Config` struct and `initComponents()`.

**Tech Stack:** Go 1.25+, `github.com/tencentcloud/tencentcloud-sdk-go` (Tencent Cloud SMS SDK v20210111)

**Spec:** `docs/superpowers/specs/2026-03-30-sms-module-design.md`

---

### Task 1: SMS Interface and Types (`sms/sms.go`)

**Files:**
- Create: `sms/sms.go`

- [ ] **Step 1: Create `sms/sms.go` with interface, types, and sentinel errors**

```go
package sms

import (
	"context"
	"errors"
)

// Standard SMS errors for unified error handling across providers.
var (
	ErrInvalidPhone  = errors.New("sms: invalid phone number")
	ErrSendFailed    = errors.New("sms: send failed")
	ErrProviderError = errors.New("sms: provider error")
)

// SendRequest contains the parameters for sending an SMS.
type SendRequest struct {
	Phone      string   // E.164 format, e.g. "+8613800138000"
	TemplateID string   // SMS template ID
	Params     []string // Ordered template parameters (e.g. ["1234", "5"])
	Sign       string   // Optional; overrides default sign from config if non-empty
}

// SMS defines the SMS sending capability.
type SMS interface {
	// Send sends an SMS message to a single recipient.
	Send(ctx context.Context, req *SendRequest) error

	// Ping checks provider availability.
	Ping(ctx context.Context) error
}
```

- [ ] **Step 2: Verify the file compiles**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./sms/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add sms/sms.go
git commit -m "feat(sms): add SMS interface, SendRequest, and sentinel errors"
```

---

### Task 2: Config, Factory, and Manager (`sms/factory.go`)

**Files:**
- Create: `sms/factory.go`

- [ ] **Step 1: Create `sms/factory.go` with Config, Factory, and Manager**

```go
package sms

import (
	"fmt"
	"sync"
)

const DefaultName = "default"

// Config is the unified SMS configuration.
type Config struct {
	Driver  string        `mapstructure:"driver"` // "tencentcloud"
	Tencent TencentConfig `mapstructure:"tencent"`
}

// TencentConfig holds Tencent Cloud SMS configuration.
type TencentConfig struct {
	SecretID  string `mapstructure:"secret_id"`
	SecretKey string `mapstructure:"secret_key"`
	AppID     string `mapstructure:"app_id"` // SmsSdkAppId
	Sign      string `mapstructure:"sign"`   // Default SMS signature
	Region    string `mapstructure:"region"` // Default: "ap-guangzhou"
}

// New creates an SMS instance based on the driver specified in Config.
func New(cfg Config) (SMS, error) {
	switch cfg.Driver {
	case "tencentcloud":
		return NewTencent(cfg.Tencent)
	default:
		return nil, fmt.Errorf("sms: unsupported driver %q", cfg.Driver)
	}
}

// Manager manages multiple named SMS instances with lazy initialization.
type Manager struct {
	configs map[string]Config

	mu       sync.RWMutex
	services map[string]SMS
}

// NewManager creates a Manager from a map of named configs.
func NewManager(configs map[string]Config) *Manager {
	return &Manager{
		configs:  configs,
		services: make(map[string]SMS, len(configs)),
	}
}

// Get returns the named SMS instance, initializing it on first access.
func (m *Manager) Get(name string) (SMS, error) {
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
		return nil, fmt.Errorf("sms: unknown instance %q", name)
	}

	s, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("sms: init %q: %w", name, err)
	}

	m.services[name] = s
	return s, nil
}

// Default returns the "default" SMS instance.
func (m *Manager) Default() (SMS, error) {
	return m.Get(DefaultName)
}

// Names returns all configured SMS instance names.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// Component wraps an SMS instance for app lifecycle integration.
type Component struct {
	SMS SMS
}

func (c *Component) Name() string { return "sms" }
```

- [ ] **Step 2: Verify the file compiles**

This will fail because `NewTencent` doesn't exist yet. Create a temporary stub to verify the factory/manager code compiles. We'll replace it in Task 3.

Create a temporary `sms/tencent.go`:
```go
package sms

// NewTencent creates a new Tencent Cloud SMS client.
// Stub — replaced in Task 3.
func NewTencent(cfg TencentConfig) (*TencentSMS, error) {
	return nil, nil
}

// TencentSMS implements SMS using Tencent Cloud.
type TencentSMS struct{}

func (t *TencentSMS) Send(_ context.Context, _ *SendRequest) error { return nil }
func (t *TencentSMS) Ping(_ context.Context) error                 { return nil }
```

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./sms/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add sms/factory.go sms/tencent.go
git commit -m "feat(sms): add Config, Factory, and Manager with tencent stub"
```

---

### Task 3: Tencent Cloud Provider (`sms/tencent.go`)

**Files:**
- Modify: `sms/tencent.go` (replace stub with full implementation)

- [ ] **Step 1: Add Tencent Cloud SMS SDK dependency**

Run: `cd /Users/nullkey/laoshen/go-atlas && go get github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111`

- [ ] **Step 2: Replace `sms/tencent.go` with full implementation**

```go
package sms

import (
	"context"
	"fmt"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tcsms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
)

// TencentSMS implements SMS using Tencent Cloud SMS service.
type TencentSMS struct {
	client *tcsms.Client
	cfg    TencentConfig
}

// NewTencent creates a new Tencent Cloud SMS client.
func NewTencent(cfg TencentConfig) (*TencentSMS, error) {
	if cfg.Region == "" {
		cfg.Region = "ap-guangzhou"
	}

	credential := common.NewCredential(cfg.SecretID, cfg.SecretKey)
	cpf := profile.NewClientProfile()

	client, err := tcsms.NewClient(credential, cfg.Region, cpf)
	if err != nil {
		return nil, fmt.Errorf("sms: create tencent client: %w", err)
	}

	return &TencentSMS{client: client, cfg: cfg}, nil
}

func (t *TencentSMS) Send(ctx context.Context, req *SendRequest) error {
	if req.Phone == "" {
		return ErrInvalidPhone
	}

	sign := req.Sign
	if sign == "" {
		sign = t.cfg.Sign
	}

	request := tcsms.NewSendSmsRequest()
	request.SmsSdkAppId = common.StringPtr(t.cfg.AppID)
	request.SignName = common.StringPtr(sign)
	request.TemplateId = common.StringPtr(req.TemplateID)
	request.PhoneNumberSet = common.StringPtrs([]string{req.Phone})
	if len(req.Params) > 0 {
		request.TemplateParamSet = common.StringPtrs(req.Params)
	}

	response, err := t.client.SendSmsWithContext(ctx, request)
	if err != nil {
		if sdkErr, ok := err.(*tcerrors.TencentCloudSDKError); ok {
			return fmt.Errorf("%w: [%s] %s", ErrProviderError, sdkErr.GetCode(), sdkErr.GetMessage())
		}
		return fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	// Check per-number send status.
	if response.Response != nil && len(response.Response.SendStatusSet) > 0 {
		status := response.Response.SendStatusSet[0]
		if status.Code != nil && *status.Code != "Ok" {
			msg := ""
			if status.Message != nil {
				msg = *status.Message
			}
			return fmt.Errorf("%w: [%s] %s", ErrSendFailed, *status.Code, msg)
		}
	}

	return nil
}

func (t *TencentSMS) Ping(_ context.Context) error {
	return nil
}
```

- [ ] **Step 3: Verify the file compiles**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./sms/`
Expected: no errors

- [ ] **Step 4: Run `go mod tidy`**

Run: `cd /Users/nullkey/laoshen/go-atlas && go mod tidy`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add sms/tencent.go go.mod go.sum
git commit -m "feat(sms): implement Tencent Cloud SMS provider"
```

---

### Task 4: Unit Tests (`sms/tencent_test.go`)

**Files:**
- Create: `sms/tencent_test.go`

- [ ] **Step 1: Write tests for Manager and factory logic**

```go
package sms

import (
	"testing"
)

func TestNewUnsupportedDriver(t *testing.T) {
	_, err := New(Config{Driver: "unknown"})
	if err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

func TestManagerGetUnknownInstance(t *testing.T) {
	m := NewManager(map[string]Config{})
	_, err := m.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown instance")
	}
}

func TestManagerNames(t *testing.T) {
	m := NewManager(map[string]Config{
		"default": {Driver: "tencentcloud"},
		"oem":     {Driver: "tencentcloud"},
	})
	names := m.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

func TestNewTencentDefaultRegion(t *testing.T) {
	sms, err := NewTencent(TencentConfig{
		SecretID:  "test-id",
		SecretKey: "test-key",
		AppID:     "1400000000",
		Sign:      "TestSign",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sms.cfg.Region != "ap-guangzhou" {
		t.Errorf("expected default region ap-guangzhou, got %s", sms.cfg.Region)
	}
}

func TestNewTencentCustomRegion(t *testing.T) {
	sms, err := NewTencent(TencentConfig{
		SecretID:  "test-id",
		SecretKey: "test-key",
		AppID:     "1400000000",
		Sign:      "TestSign",
		Region:    "ap-beijing",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sms.cfg.Region != "ap-beijing" {
		t.Errorf("expected region ap-beijing, got %s", sms.cfg.Region)
	}
}

func TestSendInvalidPhone(t *testing.T) {
	sms, err := NewTencent(TencentConfig{
		SecretID:  "test-id",
		SecretKey: "test-key",
		AppID:     "1400000000",
		Sign:      "TestSign",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = sms.Send(t.Context(), &SendRequest{
		Phone:      "",
		TemplateID: "123",
		Params:     []string{"1234"},
	})
	if err != ErrInvalidPhone {
		t.Errorf("expected ErrInvalidPhone, got %v", err)
	}
}

func TestPingReturnsNil(t *testing.T) {
	sms, err := NewTencent(TencentConfig{
		SecretID:  "test-id",
		SecretKey: "test-key",
		AppID:     "1400000000",
		Sign:      "TestSign",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := sms.Ping(t.Context()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `cd /Users/nullkey/laoshen/go-atlas && go test ./sms/ -v`
Expected: all tests PASS

- [ ] **Step 3: Commit**

```bash
git add sms/tencent_test.go
git commit -m "test(sms): add unit tests for Manager, factory, and Tencent provider"
```

---

### Task 5: Atlas Integration

**Files:**
- Modify: `atlas/config.go:10` (add import)
- Modify: `atlas/config.go:24-36` (add SMS to Config struct)
- Modify: `atlas/arche.go:14` (add import)
- Modify: `atlas/arche.go:79-106` (add smsm field to Atlas struct)
- Modify: `atlas/arche.go:180-206` (add SMS init to initComponents)
- Modify: `atlas/arche.go:257-270` (add SMS/SMSManager accessors after StorageManager)

- [ ] **Step 1: Add SMS field to `atlas/config.go`**

Add the import:
```go
import (
	// ... existing imports
	"github.com/shiliu-ai/go-atlas/sms"
)
```

Add SMS to the Config struct (after `Storages`):
```go
SMS      map[string]sms.Config `mapstructure:"sms"`
```

- [ ] **Step 2: Add SMS manager to `atlas/arche.go`**

Add the import:
```go
import (
	// ... existing imports
	"github.com/shiliu-ai/go-atlas/sms"
)
```

Add `smsm` field to the Atlas struct (after `stm`):
```go
smsm       *sms.Manager
```

Add SMS initialization in `initComponents()` (after the `Storages` block):
```go
if len(a.cfg.SMS) > 0 {
	a.smsm = sms.NewManager(a.cfg.SMS)
}
```

Add public accessor methods (after `StorageManager()`):
```go
// SMS returns the default SMS instance.
// This is a convenience shortcut for SMSManager().Default().
func (a *Atlas) SMS() (sms.SMS, error) {
	return a.SMSManager().Default()
}

// SMSManager returns the SMS manager for accessing named instances.
// Panics if no SMS providers are configured.
func (a *Atlas) SMSManager() *sms.Manager {
	if a.smsm == nil {
		panic("atlas: no sms configured")
	}
	return a.smsm
}
```

- [ ] **Step 3: Verify everything compiles**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./...`
Expected: no errors

- [ ] **Step 4: Run all tests**

Run: `cd /Users/nullkey/laoshen/go-atlas && go test ./...`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add atlas/config.go atlas/arche.go
git commit -m "feat(sms): integrate SMS module into Atlas lifecycle"
```

---

### Task 6: Update README

**Files:**
- Modify: `README.md`
- Modify: `README_CN.md` (if exists)

- [ ] **Step 1: Add SMS to Features list in README.md**

Find the Features section and add SMS. For example, after the Storage bullet:
```
- **SMS** — Unified SMS sending with multi-provider support (Tencent Cloud); named instances for multi-tenant/OEM scenarios
```

- [ ] **Step 2: Add SMS configuration example to README.md**

Add an SMS section to the configuration example area:
```yaml
# SMS (Tencent Cloud)
sms:
  default:
    driver: "tencentcloud"
    tencent:
      secret_id: "${SMS_SECRET_ID}"
      secret_key: "${SMS_SECRET_KEY}"
      app_id: "1400000000"
      sign: "YourAppName"
      region: "ap-guangzhou"
```

- [ ] **Step 3: Add SMS usage example to README.md**

```go
// Send verification code
s, err := a.SMS()
if err != nil {
    panic(err)
}
err = s.Send(ctx, &sms.SendRequest{
    Phone:      "+8613800138000",
    TemplateID: "123456",
    Params:     []string{"1234", "5"},
})
```

- [ ] **Step 4: Update README_CN.md with the same changes (if it exists)**

Mirror the same additions in the Chinese README.

- [ ] **Step 5: Verify build still passes**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./...`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add README.md README_CN.md
git commit -m "docs: add SMS module to README"
```

---

### Task 7: Update Example Config

**Files:**
- Modify: `example/config.yaml` (if exists)

- [ ] **Step 1: Check if example config exists and add SMS section**

If `example/config.yaml` exists, add the SMS config block:
```yaml
sms:
  default:
    driver: "tencentcloud"
    tencent:
      secret_id: ""
      secret_key: ""
      app_id: ""
      sign: ""
      region: "ap-guangzhou"
```

- [ ] **Step 2: Commit**

```bash
git add example/
git commit -m "docs: add SMS config to example"
```
