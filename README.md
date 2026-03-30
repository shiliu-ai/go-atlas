# Atlas

[中文文档](README_CN.md)

A refined Go framework for building production-grade backend services. Built on Gin, designed for teams who value clarity over ceremony.

Atlas provides a cohesive set of building blocks — authentication, storage, caching, tracing, inter-service communication, and more — wired together through a **four-domain architecture** with sensible defaults and zero boilerplate.

## Quick Start

```go
package main

import (
    "github.com/gin-gonic/gin"
    atlas "github.com/shiliu-ai/go-atlas"
    "github.com/shiliu-ai/go-atlas/aether/response"
)

func main() {
    a := atlas.New("my-service")

    a.Route(func(r *gin.RouterGroup) {
        r.GET("/health", func(c *gin.Context) {
            response.OK(c, gin.H{"status": "ok"})
        })
    })

    a.MustRun()
}
```

That's it. You get structured logging, request IDs, panic recovery, CORS, i18n, and graceful shutdown — out of the box.

## Install

```bash
go get github.com/shiliu-ai/go-atlas
```

Requires Go 1.25+.

## Architecture

Atlas is organized into four domains, all rooted in Greek mythology:

| Domain | Name | Meaning | Role |
|--------|------|---------|------|
| Core | **Atlas** | The Titan | Framework orchestrator |
| Built-in | **Aether** | The divine air | Essential, omnipresent components |
| Extension | **Pillar** | The columns | Optional lifecycle components |
| Toolkit | **Artifact** | Divine tools | Standalone utilities |

```
*.go                Core (package atlas) — config, server, lifecycle, middleware, Pillar interface
aether/             Built-in essentials
  ├── errors        Structured error codes
  ├── i18n          Internationalization
  ├── log           Structured logging
  └── response      Unified API responses
pillar/             Infrastructure — pluggable components registered via Pillar()
  ├── auth          JWT authentication
  ├── cache         Redis cache + distributed locks
  ├── database      GORM (MySQL/PostgreSQL, multiple named connections)
  ├── httpclient    Production-ready HTTP client
  ├── oauth         OAuth2 providers
  ├── serviceclient Typed inter-service RPC
  ├── sms           SMS sending (Tencent Cloud)
  ├── storage       Object storage (S3/COS/OSS/TOS)
  └── tracing       OpenTelemetry distributed tracing
artifact/           Utilities — standalone helpers (crypto, ID generation, pagination, validation)
```

### Pillar Pattern

Every infrastructure component follows the same pattern:

```go
// 1. Register Pillars as options in atlas.New()
a := atlas.New("my-service",
    auth.Pillar(),
    database.Pillar(),
    cache.Pillar(),
)

// 2. Retrieve initialized instances with Of()
jwt := auth.Of(a)
dbm := database.Of(a)
redis := cache.Of(a)
```

All Pillars implement the `atlas.Pillar` interface:

```go
type Pillar interface {
    Name() string
    Init(core *Core) error
    Stop(ctx context.Context) error
}
```

Pillars can optionally implement `Starter` (background goroutines), `HealthChecker` (health checks), or `MiddlewareProvider` (auto-injected middleware).

## Configuration

Atlas uses [Viper](https://github.com/spf13/viper) under the hood. Drop a `config.yaml` alongside your binary:

```yaml
server:
  port: 8080
  name: "my-service"
  mode: "release"           # debug | release | test
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 10s

log:
  level: "info"             # debug | info | warn | error
  format: "text"            # text (default) | json

i18n:
  default: "en"             # default language tag, e.g. "en", "zh-Hans"

auth:
  secret: "change-me"
  issuer: "my-service"
  access_expire: 2h
  refresh_expire: 168h

databases:
  default:
    driver: "mysql"         # mysql | postgres
    dsn: "user:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True"
    max_open_conns: 50
    max_idle_conns: 10
    max_lifetime: 1h
    log_level: "info"
  # readonly:
  #   driver: "mysql"
  #   dsn: "user:pass@tcp(127.0.0.1:3307)/mydb?charset=utf8mb4&parseTime=True"

redis:
  addr: "127.0.0.1:6379"

storages:
  default:
    driver: "s3"            # s3 | cos | oss | tos
    s3:
      endpoint: "https://s3.amazonaws.com"
      region: "us-east-1"
      bucket: "my-bucket"
      access_key_id: ""
      secret_access_key: ""
  # backup:
  #   driver: "cos"
  #   cos:
  #     bucket_url: "https://<bucket>-<appid>.cos.<region>.myqcloud.com"

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

tracing:
  service_name: "my-service"
  endpoint: "localhost:4318"
  sample_rate: 1.0
  insecure: true

httpclient:
  timeout: 5s
  max_retries: 2
  retry_wait: 500ms

services:
  user-service:
    base_url: "http://user-service:8080/user-service"
    timeout: 5s             # override global httpclient timeout
    max_retries: 3          # override global httpclient retries

middleware:
  cors:
    allow_origins: ["*"]
    allow_methods: ["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"]
    allow_headers: ["Origin", "Content-Type", "Authorization", "X-Request-ID"]
    max_age: 86400
  rate_limit:
    rate: 100
    window: 1m
```

All sections are optional. Pillars read their own config section during `Init()` — misconfiguration triggers a fail-fast panic at startup.

Environment variables are also supported with the `APP_` prefix (configurable via `WithEnvPrefix`). Nested keys use underscores: `APP_SERVER_PORT=9090`.

## Features

### Authentication

JWT-based auth with access/refresh token pairs. HS256/384/512 signing.

```go
a := atlas.New("my-service", auth.Pillar())

jwt := auth.Of(a)

// Generate token pair
pair, err := jwt.GeneratePair(userID, map[string]any{"role": "admin"})

// Protect routes
a.Route(func(r *gin.RouterGroup) {
    authorized := r.Group("/api", jwt.Middleware())

    // Extract claims
    authorized.GET("/me", func(c *gin.Context) {
        claims := auth.ClaimsFromContext(c.Request.Context())
        response.OK(c, gin.H{"user_id": claims.UserID})
    })
})
```

### Database

GORM-based ORM with multiple named connections, lazy initialization, connection pooling, and MySQL/PostgreSQL support.

```go
a := atlas.New("my-service", database.Pillar())

dbm := database.Of(a)

// Default connection
db, err := dbm.Default()

// Named connection (e.g. read-only replica)
db, err := dbm.Get("readonly")
```

### Cache

Redis client with unified cache interface and distributed locking.

```go
a := atlas.New("my-service", cache.Pillar())

redis := cache.Of(a)
redis.Set(ctx, "key", "value", 5*time.Minute)
val, err := redis.Get(ctx, "key")

// Distributed lock
l := redis.NewLock("my-lock", 10*time.Second)
acquired, err := l.Acquire(ctx)
defer l.Release(ctx)
```

### Object Storage

One interface, four cloud providers. Switch backends by changing a config line. Supports multiple named storage instances.

| Driver | Provider |
|--------|----------|
| `s3`   | AWS S3, MinIO, S3-compatible |
| `cos`  | Tencent Cloud COS |
| `oss`  | Alibaba Cloud OSS |
| `tos`  | Volcengine TOS |

```go
a := atlas.New("my-service", storage.Pillar())

stm := storage.Of(a)

// Default storage
store, err := stm.Get("default")
err = store.Put(ctx, "path/to/file.png", reader, size, "image/png")
url, err := store.SignURL(ctx, "path/to/file.png", 15*time.Minute)

// Named storage
store, err := stm.Get("backup")
```

### SMS

Unified SMS sending with multi-provider support (Tencent Cloud); named instances for multi-tenant/OEM scenarios.

```go
a := atlas.New("my-service", sms.Pillar())

smsMgr := sms.Of(a)
s, err := smsMgr.Default()
err = s.Send(ctx, &sms.SendRequest{
    Phone:      "+8613800138000",
    TemplateID: "123456",
    Params:     []string{"1234", "5"},
})
```

### Inter-Service Communication

Typed HTTP clients for calling other atlas-based services. Automatically unwraps the standard `R{code, message, data}` response envelope, forwards request headers (Authorization, X-Request-ID, X-Trace-ID), and supports per-service timeout/retry overrides.

```go
a := atlas.New("my-service",
    httpclient.Pillar(),
    serviceclient.Pillar(),
)

svcm := serviceclient.Of(a)
userSvc := svcm.MustGet("user-service")

// Typed call — response.data is unmarshalled into the target
var user User
err := serviceclient.Get(ctx, userSvc, "/v1/users/123", &user)

// With query parameters
var users []User
err := serviceclient.Get(ctx, userSvc, "/v1/users", &users,
    serviceclient.WithQuery(url.Values{"page": {"1"}, "size": {"20"}}),
)

// POST with body
var created User
err := serviceclient.Post(ctx, userSvc, "/v1/users", createReq, &created)
```

### HTTP Client

Production-ready HTTP client with retries, exponential backoff, and trace propagation.

```go
a := atlas.New("my-service", httpclient.Pillar())

hc := httpclient.Of(a)
resp, err := hc.Get(ctx, "https://api.example.com/data")
body := resp.String()

resp, err := hc.PostJSON(ctx, url, payload)
```

### ID Generation

Four strategies for different needs (standalone utilities, no Pillar needed):

```go
id.UUID()                       // "550e8400-e29b-41d4-a716-446655440000"
id.NanoID()                     // "V1StGXR8_Z5jdHi6B-myT"
id.ShortID()                    // "0h7a8sK2x9pL3mN1"

sf, _ := id.NewSnowflake(1)
sf.MustGenerate()               // 182439823049723904
```

### Structured Errors

Code-based errors that map cleanly to HTTP status codes. Supports i18n message keys.

```go
errors.New(errors.CodeNotFound, "user not found")
errors.NewT(errors.CodeNotFound, "error.user_not_found")  // i18n key
errors.Wrap(errors.CodeInternal, "database query failed", err)

// Predefined sentinel errors
errors.ErrNotFound          // 404
errors.ErrUnauthorized      // 401
errors.ErrBadRequest        // 400

// Fluent API
errors.ErrNotFound.WithMessage("user not found")
errors.ErrNotFound.WithMsgKey("error.user_not_found")

// In handlers
response.Fail(c, errors.CodeBadRequest, "invalid email format")
response.FailT(c, errors.CodeBadRequest, "error.invalid_email")  // i18n
response.Err(c, err)  // auto-detect *errors.Error
```

### Request Validation

Bind and validate in one step with human-readable error messages:

```go
type CreateUserReq struct {
    Email string `json:"email" binding:"required,email"`
    Name  string `json:"name"  binding:"required,min=2,max=50"`
}

var req CreateUserReq
if !validate.BindJSON(c, &req) {
    return // error response already sent
}
```

### Unified Response Format

Consistent JSON responses across your entire API:

```go
response.OK(c, data)
// {"code": 0, "message": "ok", "data": {...}, "trace_id": "..."}

response.Fail(c, errors.CodeNotFound, "user not found")
// {"code": 404, "message": "user not found", "trace_id": "..."}

response.Err(c, err)        // derive response from *errors.Error
response.AbortErr(c, err)   // same as Err but aborts middleware chain
```

### Pagination

```go
authorized.GET("/users", func(c *gin.Context) {
    pg := pagination.FromContext(c)   // auto-bind ?page=1&size=20
    users, total := fetchUsers(pg.Offset(), pg.Size)
    response.OK(c, pagination.NewResponse(users, total, pg))
})
```

### Internationalization (i18n)

Built-in i18n support with per-request locale detection via `Accept-Language` header.

```go
// Register custom translations
bundle := a.I18nBundle()
bundle.Register(language.English, map[string]string{
    "error.user_not_found": "User not found",
})
bundle.Register(language.SimplifiedChinese, map[string]string{
    "error.user_not_found": "用户不存在",
})

// Use i18n in responses
response.FailT(c, errors.CodeNotFound, "error.user_not_found")
```

### Cryptography

Standalone utilities, no Pillar needed:

```go
// Password hashing (bcrypt)
hash, _ := crypto.HashPassword("secret")
ok := crypto.CheckPassword(hash, "secret")

// AES-GCM encryption
cipher, _ := crypto.NewAES(key)
encrypted, _ := cipher.EncryptString("sensitive data")
decrypted, _ := cipher.DecryptString(encrypted)
```

### OAuth2

```go
a := atlas.New("my-service", oauth.Pillar())

oauthMgr := oauth.Of(a)
github := oauthMgr.MustGet("github")
url := github.AuthCodeURL("state-token")
```

### Custom Configuration

Use `a.Unmarshal()` to read custom config sections from the same config file:

```go
type BusinessConfig struct {
    MaxItems int    `mapstructure:"max_items"`
    Region   string `mapstructure:"region"`
}

var bizCfg BusinessConfig
if err := a.Unmarshal("business", &bizCfg); err != nil {
    panic(err)
}
```

## Middleware

Atlas registers these middleware by default:

| Middleware | Description |
|------------|-------------|
| **Recovery** | Catches panics, logs stack traces, returns 500 |
| **Request ID** | Generates/propagates `X-Request-ID` header |
| **I18n** | Detects locale from `Accept-Language` header |
| **Logging** | Structured request logs with latency, status, path |
| **CORS** | Configurable cross-origin resource sharing |
| **Rate Limit** | Sliding window rate limiter (if configured) |

Pillars that implement `MiddlewareProvider` (e.g. tracing, serviceclient) automatically inject their middleware between core defaults and user middleware.

Logging is context-aware — trace IDs and request IDs flow through automatically.

Disable all defaults with `WithoutDefaultMiddleware()`, or add custom middleware with `WithMiddleware(...)`.

## Observability

Register the tracing Pillar for OpenTelemetry distributed tracing:

```go
a := atlas.New("my-service", tracing.Pillar())
```

```yaml
tracing:
  service_name: "my-service"
  endpoint: "localhost:4318"
  sample_rate: 1.0
  insecure: true
```

Traces propagate across HTTP client calls and inter-service communication. Every response includes a `trace_id` for end-to-end debugging.

## Example

See [example/main.go](example/main.go) for a complete working example with auth, pagination, inter-service calls, proxying, and ID generation.

## License

MIT
