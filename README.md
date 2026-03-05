# Atlas

[中文文档](README_CN.md)

A refined Go framework for building production-grade backend services. Built on Gin, designed for teams who value clarity over ceremony.

Atlas provides a cohesive set of building blocks — authentication, storage, caching, tracing, and more — wired together through a single entry point with sensible defaults and zero boilerplate.

## Quick Start

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/shiliu-ai/go-atlas/atlas"
    "github.com/shiliu-ai/go-atlas/response"
)

func main() {
    a := atlas.New("my-service")

    v1 := a.Group().Group("/v1")
    v1.GET("/health", func(c *gin.Context) {
        response.OK(c, gin.H{"status": "ok"})
    })

    a.MustRun()
}
```

That's it. You get structured logging, request IDs, panic recovery, CORS, and graceful shutdown — out of the box.

## Install

```bash
go get github.com/shiliu-ai/go-atlas
```

Requires Go 1.25+.

## Configuration

Atlas uses [Viper](https://github.com/spf13/viper) under the hood. Drop a `config.yaml` alongside your binary:

```yaml
server:
  addr: ":8080"
  name: "my-service"
  mode: "release"

log:
  level: "info"          # debug | info | warn | error

auth:
  secret: "change-me"
  issuer: "my-service"
  access_expire: 2h
  refresh_expire: 168h

database:
  driver: "mysql"        # mysql | postgres
  dsn: "user:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True"
  max_open_conns: 50
  max_idle_conns: 10

redis:
  addr: "127.0.0.1:6379"

storage:
  driver: "s3"           # s3 | cos | oss | tos
  s3:
    endpoint: "https://s3.amazonaws.com"
    region: "us-east-1"
    bucket: "my-bucket"
    access_key_id: ""
    secret_access_key: ""

tracing:
  endpoint: "localhost:4318"
  sample_rate: 1.0

httpclient:
  timeout: 5s
  max_retries: 2

middleware:
  cors:
    allow_origins: ["*"]
  rate_limit:
    rate: 100
    window: 1m
```

All sections are optional. Components initialize lazily on first access — only pay for what you use.

## Features

### Authentication

JWT-based auth with access/refresh token pairs. HS256/384/512 signing.

```go
// Generate token pair
pair, err := a.Auth().GeneratePair(userID, map[string]any{"role": "admin"})

// Protect routes
authorized := v1.Group("/api", auth.Middleware(a.Auth()))

// Extract claims
authorized.GET("/me", func(c *gin.Context) {
    claims := auth.ClaimsFromContext(c.Request.Context())
    response.OK(c, gin.H{"user_id": claims.UserID})
})
```

### Database

xorm-backed ORM with connection pooling, MySQL/PostgreSQL support, and context-aware transactions.

```go
db := a.Database()

// Transaction with automatic rollback
err := database.Transaction(db, func(session *xorm.Session) error {
    _, err := session.Insert(&user)
    return err
})
```

### Cache

Unified cache interface backed by Redis.

```go
cache := a.Cache()
cache.Set(ctx, "key", "value", 5*time.Minute)

val, err := cache.Get(ctx, "key")
```

### Object Storage

One interface, four cloud providers. Switch backends by changing a config line.

| Driver | Provider |
|--------|----------|
| `s3`   | AWS S3, MinIO, S3-compatible |
| `cos`  | Tencent Cloud COS |
| `oss`  | Alibaba Cloud OSS |
| `tos`  | Volcengine TOS |

```go
store := a.Storage()
err := store.Put(ctx, "path/to/file.png", reader, size, "image/png")
url, err := store.SignURL(ctx, "path/to/file.png", 15*time.Minute)
```

### Distributed Locking

Redis-based distributed locks with owner tokens and auto-expiry.

```go
l := lock.NewRedisLock(redisClient, "my-lock", 10*time.Second)
acquired, err := l.Acquire(ctx)
defer l.Release(ctx)
```

### HTTP Client

Production-ready HTTP client with retries, exponential backoff, and trace propagation.

```go
resp, err := a.HTTPClient().Get(ctx, "https://api.example.com/data")
body := resp.String()

resp, err := a.HTTPClient().PostJSON(ctx, url, payload)
```

### ID Generation

Four strategies for different needs:

```go
id.UUID()                       // "550e8400-e29b-41d4-a716-446655440000"
id.NanoID()                     // "V1StGXR8_Z5jdHi6B-myT"
id.ShortID()                    // "0h7a8sK2x9pL3mN1"

sf, _ := id.NewSnowflake(1)
sf.MustGenerate()               // 182439823049723904
```

### Structured Errors

Code-based errors that map cleanly to HTTP status codes:

```go
errors.New(errors.CodeNotFound, "user not found")
errors.Wrap(err, errors.CodeInternal, "database query failed")

// In handlers
response.Fail(c, errors.CodeBadRequest, "invalid email format")
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
```

### Pagination

```go
authorized.GET("/users", func(c *gin.Context) {
    pg := pagination.FromContext(c)   // auto-bind ?page=1&size=20
    users, total := fetchUsers(pg.Offset(), pg.Size)
    response.OK(c, pagination.NewResponse(users, total, pg))
})
```

### Cryptography

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
provider := oauth.NewProvider("github", oauth.ProviderConfig{
    ClientID:     "...",
    ClientSecret: "...",
    AuthURL:      "https://github.com/login/oauth/authorize",
    TokenURL:     "https://github.com/login/oauth/access_token",
    Scopes:       []string{"user:email"},
})
url := provider.AuthCodeURL("state-token")
```

## Middleware

Atlas registers these middleware by default:

| Middleware | Description |
|------------|-------------|
| **Recovery** | Catches panics, logs stack traces, returns 500 |
| **Request ID** | Generates/propagates `X-Request-ID` header |
| **Tracing** | OpenTelemetry span extraction, `X-Trace-ID` header |
| **Logging** | Structured request logs with latency, status, path |
| **CORS** | Configurable cross-origin resource sharing |
| **Rate Limit** | Token bucket (in-memory) or sliding window (Redis) |

Logging is context-aware — trace IDs and request IDs flow through automatically.

## Observability

Atlas integrates OpenTelemetry for distributed tracing:

```yaml
tracing:
  endpoint: "localhost:4318"
  sample_rate: 1.0
  insecure: true
```

Traces propagate across HTTP client calls. Every response includes a `trace_id` for end-to-end debugging.

## Architecture

```
atlas.New()
  ├── Config        (Viper)
  ├── Logger        (slog with color output)
  ├── Server        (Gin + graceful shutdown)
  ├── Auth          (JWT)
  ├── Database      (xorm)
  ├── Cache         (Redis)
  ├── Storage       (S3/COS/OSS/TOS)
  ├── HTTPClient    (retries + tracing)
  ├── Tracing       (OpenTelemetry)
  └── Middleware    (recovery, logging, CORS, rate limit, ...)
```

Components are lazy-initialized via `sync.Once` — they're created only when you first call them. The lifecycle manager handles startup ordering and reverse-order graceful shutdown on SIGINT/SIGTERM.

## Example

See [example/main.go](example/main.go) for a complete working example with auth, pagination, proxying, and ID generation.

## License

MIT
