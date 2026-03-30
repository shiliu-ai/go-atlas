# SMS Module Design Spec

## Overview

Add an `sms/` module to go-atlas, providing a unified SMS sending interface with multi-provider support. First provider: Tencent Cloud SMS. Follows the established Manager + Factory + Interface pattern (same as `storage/`).

## Motivation

Atlas-based user services need SMS verification code capability. The module must support named instances for OEM / private deployment scenarios where different clients configure their own SMS providers.

## Interface

```go
package sms

import "context"

// SMS defines the SMS sending capability.
type SMS interface {
    // Send sends an SMS message.
    Send(ctx context.Context, req *SendRequest) error

    // Ping checks provider availability.
    Ping(ctx context.Context) error
}

// SendRequest contains the parameters for sending an SMS.
type SendRequest struct {
    Phone      string   // E.164 format, e.g. "+8613800138000"
    TemplateID string   // SMS template ID
    Params     []string // Ordered template parameters (e.g. ["1234", "5"])
    Sign       string   // Optional; overrides default sign from config if non-empty
}
```

### Design decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| `Phone` as string | E.164 format | Matches all cloud provider SDK conventions |
| `Params` as `[]string` | Ordered slice | Tencent/Aliyun templates use positional params (`{1}`, `{2}`) |
| `TemplateID` per-request | Not in config | One SMS instance sends different message types (verification, notification) |
| `Sign` optional override | Default from config | Tencent Cloud `SignName` is per-request; config provides default, caller can override |
| `SendRequest` struct | Not individual args | Extensible without breaking interface; >3 params favors struct in Go |
| No `BatchSend` | Omitted | Current scope is verification codes (single send); add later if needed |

### Sentinel errors

```go
var (
    ErrInvalidPhone    = errors.New("sms: invalid phone number")
    ErrSendFailed      = errors.New("sms: send failed")
    ErrProviderError   = errors.New("sms: provider error")
)
```

## Configuration

### Config structs

```go
type Config struct {
    Driver  string        `mapstructure:"driver"`  // "tencentcloud"
    Tencent TencentConfig `mapstructure:"tencent"`
}

type TencentConfig struct {
    SecretID  string `mapstructure:"secret_id"`
    SecretKey string `mapstructure:"secret_key"`
    AppID     string `mapstructure:"app_id"`   // SmsSdkAppId
    Sign      string `mapstructure:"sign"`      // Default signature
    Region    string `mapstructure:"region"`     // Default: "ap-guangzhou"
}
```

### YAML example

```yaml
sms:
  default:
    driver: "tencentcloud"
    tencent:
      secret_id: "${SMS_SECRET_ID}"
      secret_key: "${SMS_SECRET_KEY}"
      app_id: "1400000000"
      sign: "MyApp"
      region: "ap-guangzhou"
  oem-client-a:
    driver: "tencentcloud"
    tencent:
      secret_id: "..."
      secret_key: "..."
      app_id: "1400000001"
      sign: "ClientA"
```

## Factory

```go
func New(cfg Config) (SMS, error) {
    switch cfg.Driver {
    case "tencentcloud":
        return NewTencent(cfg.Tencent)
    default:
        return nil, fmt.Errorf("sms: unsupported driver %q", cfg.Driver)
    }
}
```

## Manager

Replicates the `storage.Manager` pattern exactly:

- `NewManager(configs map[string]Config) *Manager`
- `Get(name string) (SMS, error)` — lazy init with double-check locking (`sync.RWMutex`)
- `Default() (SMS, error)` — shortcut for `Get("default")`
- `Names() []string` — list configured instance names

Thread-safe, lazy initialization on first access per name.

## Tencent Cloud Provider

### Dependencies

```
github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common
github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors
github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile
github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111
```

### Implementation

```go
type TencentSMS struct {
    client *tcsms.Client
    cfg    TencentConfig
}

func NewTencent(cfg TencentConfig) (*TencentSMS, error) {
    // Default region
    if cfg.Region == "" {
        cfg.Region = "ap-guangzhou"
    }
    credential := common.NewCredential(cfg.SecretID, cfg.SecretKey)
    cpf := profile.NewClientProfile()
    client, err := tcsms.NewClient(credential, cfg.Region, cpf)
    if err != nil {
        return nil, fmt.Errorf("sms: failed to create tencent client: %w", err)
    }
    return &TencentSMS{client: client, cfg: cfg}, nil
}
```

### Send logic

1. Build `SendSmsRequest` with `PhoneNumberSet`, `SmsSdkAppId`, `TemplateId`, `SignName`, `TemplateParamSet`
2. `SignName`: use `req.Sign` if non-empty, otherwise fall back to `cfg.Sign`
3. Call `client.SendSmsWithContext(ctx, request)`
4. **Two-layer error check:**
   - Layer 1: SDK/transport error from the API call itself
   - Layer 2: Per-number status in `SendStatusSet` — success is `Code == "Ok"`
5. Wrap errors with sentinel types for caller handling

### Ping

Return `nil` — Tencent Cloud SMS SDK validates credentials at request time, not at client creation. There is no lightweight health-check API. Credential issues will surface as errors on the first `Send` call, which is acceptable for this use case.

## Atlas Integration

### Config registration

```go
// atlas/config.go — add to Config struct
type Config struct {
    // ... existing fields
    SMS map[string]sms.Config `mapstructure:"sms"`
}
```

### Lifecycle initialization

```go
// atlas/arche.go
type Atlas struct {
    // ... existing fields
    smsm *sms.Manager
}

// In initialization block (same pattern as storage):
if len(a.cfg.SMS) > 0 {
    a.smsm = sms.NewManager(a.cfg.SMS)
}
```

### Public accessors

```go
func (a *Atlas) SMS() (sms.SMS, error) {
    return a.SMSManager().Default()
}

func (a *Atlas) SMSManager() *sms.Manager {
    if a.smsm == nil {
        panic("atlas: no sms configured")
    }
    return a.smsm
}
```

## File Structure

```
sms/
├── sms.go          # SMS interface, SendRequest, sentinel errors
├── factory.go      # Config, TencentConfig, New(), Manager, Component
├── tencent.go      # Tencent Cloud provider implementation
└── tencent_test.go # Unit tests
```

## Testing

- **Unit tests** for the Tencent provider: mock the SDK client, verify:
  - `SendRequest` → `SendSmsRequest` parameter mapping
  - Sign default/override behavior
  - Two-layer error handling (SDK error + per-number status)
  - `ErrInvalidPhone` for bad phone formats
- No integration tests (depends on external service)
- Pattern follows existing `response/response_test.go` style

## README Update

Add SMS to the Features list and configuration example in README.md.

## Out of Scope

- Batch sending (群发)
- SMS template/signature management APIs
- Delivery status callbacks
- Rate limiting / retry logic (leave to caller)
- Providers other than Tencent Cloud (add when needed)
