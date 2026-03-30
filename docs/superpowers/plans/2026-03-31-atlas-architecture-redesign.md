# Atlas Architecture Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor go-atlas from a monolithic service locator into a three-domain architecture (Atlas Core / Pillar / Artifact) with pluggable Pillar interface.

**Architecture:** Atlas Core provides config, logging, HTTP server, and lifecycle management. Pillars are infrastructure components (database, cache, auth, etc.) registered via `atlas.Option` and accessed via generic `Use[T]`/`Of()`. Artifacts are stateless utility packages. All existing functionality is preserved, reorganized into `atlas/`, `pillar/`, `artifact/` namespaces.

**Tech Stack:** Go 1.25, Gin, Viper, GORM, go-redis, OpenTelemetry

**Spec:** `docs/superpowers/specs/2026-03-30-atlas-architecture-redesign.md`

---

## File Structure

### New Files (atlas/ core)

| File | Responsibility |
|------|---------------|
| `atlas/atlas.go` | Atlas struct, New(), Run(), Route(), Unmarshal() |
| `atlas/pillar.go` | Pillar interface, Starter, HealthChecker, MiddlewareProvider, Use[T], TryUse[T], Register |
| `atlas/core.go` | Core struct (Pillar's limited view of Atlas) |
| `atlas/option.go` | Option type, With* functions |
| `atlas/config.go` | Config loading (absorbs config/ package) |
| `atlas/server.go` | HTTP server (absorbs server/ package) |
| `atlas/middleware.go` | Default middleware chain (absorbs middleware/ package) |
| `atlas/lifecycle.go` | Startup, signal handling, graceful shutdown (absorbs app/ package) |
| `atlas/health.go` | Health check endpoint aggregation |
| `atlas/atlas_test.go` | Core integration tests |
| `atlas/pillar_test.go` | Pillar interface tests |

### Moved Files (pillar/)

| New Path | Old Path |
|----------|----------|
| `pillar/database/` | `database/` |
| `pillar/cache/` | `cache/` + `lock/` |
| `pillar/auth/` | `auth/` |
| `pillar/oauth/` | `oauth/` |
| `pillar/storage/` | `storage/` |
| `pillar/sms/` | `sms/` |
| `pillar/tracing/` | `tracing/` |
| `pillar/httpclient/` | `httpclient/` |
| `pillar/serviceclient/` | `serviceclient/` |

### Moved Files (artifact/)

| New Path | Old Path |
|----------|----------|
| `artifact/crypto/` | `crypto/` |
| `artifact/id/` | `id/` |
| `artifact/pagination/` | `pagination/` |
| `artifact/validate/` | `validate/` |
| `artifact/jsonutil/` | `jsonutil/` |

### Moved Files (atlas/ subpackages)

| New Path | Old Path |
|----------|----------|
| `atlas/errors/` | `errors/` |
| `atlas/response/` | `response/` |
| `atlas/log/` | `log/` |
| `atlas/i18n/` | `i18n/` |

### Deleted After Migration

`app/`, `config/`, `server/`, `lock/`, `middleware/`, old `atlas/arche.go`, old `atlas/config.go`

---

## Task 1: Pillar Interface & Core Types

**Files:**
- Create: `atlas/pillar.go`
- Create: `atlas/core.go`
- Create: `atlas/pillar_test.go`

- [ ] **Step 1: Write tests for Pillar registration and Use[T]**

```go
// atlas/pillar_test.go
package atlas

import (
    "context"
    "testing"
)

// testPillar is a minimal Pillar implementation for testing.
type testPillar struct {
    name    string
    inited  bool
    stopped bool
}

func (p *testPillar) Name() string                    { return p.name }
func (p *testPillar) Init(core *Core) error           { p.inited = true; return nil }
func (p *testPillar) Stop(ctx context.Context) error  { p.stopped = true; return nil }

type testHealthPillar struct {
    testPillar
    healthy bool
}

func (p *testHealthPillar) Health(ctx context.Context) error {
    if !p.healthy {
        return fmt.Errorf("unhealthy")
    }
    return nil
}

func TestRegister(t *testing.T) {
    a := &Atlas{pillars: make(map[string]Pillar), pillarOrder: []string{}}
    p := &testPillar{name: "test"}
    a.Register(p)

    if _, ok := a.pillars["test"]; !ok {
        t.Fatal("pillar not registered")
    }
}

func TestRegisterDuplicate(t *testing.T) {
    a := &Atlas{pillars: make(map[string]Pillar), pillarOrder: []string{}}
    a.Register(&testPillar{name: "test"})

    defer func() {
        if r := recover(); r == nil {
            t.Fatal("expected panic on duplicate registration")
        }
    }()
    a.Register(&testPillar{name: "test"})
}

func TestUse(t *testing.T) {
    a := &Atlas{pillars: make(map[string]Pillar), pillarOrder: []string{}}
    p := &testPillar{name: "test"}
    a.Register(p)

    got := Use[*testPillar](a)
    if got != p {
        t.Fatal("Use returned wrong pillar")
    }
}

func TestUsePanic(t *testing.T) {
    a := &Atlas{pillars: make(map[string]Pillar), pillarOrder: []string{}}

    defer func() {
        if r := recover(); r == nil {
            t.Fatal("expected panic on missing pillar")
        }
    }()
    Use[*testPillar](a)
}

func TestTryUse(t *testing.T) {
    a := &Atlas{pillars: make(map[string]Pillar), pillarOrder: []string{}}
    a.Register(&testPillar{name: "test"})

    got, ok := TryUse[*testPillar](a)
    if !ok || got == nil {
        t.Fatal("TryUse should find registered pillar")
    }

    _, ok = TryUse[*testHealthPillar](a)
    if ok {
        t.Fatal("TryUse should return false for unregistered type")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/nullkey/laoshen/go-atlas && go test ./atlas/ -run TestRegister -v`
Expected: Compilation error — `Pillar`, `Core`, `Use`, `TryUse` not defined.

- [ ] **Step 3: Implement Pillar interface and Core**

```go
// atlas/pillar.go
package atlas

import (
    "context"
    "fmt"

    "github.com/gin-gonic/gin"
)

// Pillar is the unified protocol for all infrastructure components.
type Pillar interface {
    Name() string
    Init(core *Core) error
    Stop(ctx context.Context) error
}

// Starter is implemented by Pillars that need background goroutines.
type Starter interface {
    Start(ctx context.Context) error
}

// HealthChecker is implemented by Pillars that support health checks.
type HealthChecker interface {
    Health(ctx context.Context) error
}

// MiddlewareProvider is implemented by Pillars that inject global middleware.
type MiddlewareProvider interface {
    Middleware() []gin.HandlerFunc
}

// Register adds a Pillar to the Atlas registry. Panics on duplicate names.
func (a *Atlas) Register(p Pillar) {
    if _, exists := a.pillars[p.Name()]; exists {
        panic(fmt.Sprintf("atlas: duplicate pillar name %q", p.Name()))
    }
    a.pillars[p.Name()] = p
}

// Use retrieves a registered Pillar by type. Panics if not found.
func Use[T Pillar](a *Atlas) T {
    for _, p := range a.pillars {
        if t, ok := p.(T); ok {
            return t
        }
    }
    var zero T
    panic(fmt.Sprintf("atlas: pillar %T not registered", zero))
}

// TryUse retrieves a registered Pillar by type without panicking.
func TryUse[T Pillar](a *Atlas) (T, bool) {
    for _, p := range a.pillars {
        if t, ok := p.(T); ok {
            return t, true
        }
    }
    var zero T
    return zero, false
}
```

```go
// atlas/core.go
package atlas

import (
    "github.com/shiliu-ai/go-atlas/log"
    "github.com/spf13/viper"
)

// Core is the limited view of Atlas exposed to Pillars.
type Core struct {
    config *viper.Viper
    logger log.Logger
}

// Unmarshal deserializes the config section at key into target.
func (c *Core) Unmarshal(key string, target any) error {
    sub := c.config.Sub(key)
    if sub == nil {
        return fmt.Errorf("atlas: config section %q not found", key)
    }
    return sub.Unmarshal(target)
}

// Logger returns a sub-logger with the given name prefix.
func (c *Core) Logger(name string) log.Logger {
    return c.logger.WithFields(log.F("pillar", name))
}
```

Note: The `Atlas` struct is defined minimally here for tests to compile. The full struct is built in Task 4.

Add a temporary minimal Atlas struct at the bottom of `pillar.go` for now:

```go
// Atlas is the central framework instance. Full definition in atlas.go.
type Atlas struct {
    pillars     map[string]Pillar
    pillarOrder []string
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/nullkey/laoshen/go-atlas && go test ./atlas/ -run "TestRegister|TestUse|TestTryUse" -v`
Expected: All 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add atlas/pillar.go atlas/core.go atlas/pillar_test.go
git commit -m "feat(atlas): add Pillar interface, Core, Use[T], TryUse[T]"
```

---

## Task 2: Option Type & With* Functions

**Files:**
- Create: `atlas/option.go`

- [ ] **Step 1: Implement Option type and With* functions**

```go
// atlas/option.go
package atlas

import "github.com/gin-gonic/gin"

// Option configures an Atlas instance.
type Option func(*Atlas)

// WithConfigName sets the config file name (default: "config").
func WithConfigName(name string) Option {
    return func(a *Atlas) { a.configName = name }
}

// WithConfigPaths sets directories to search for config files.
func WithConfigPaths(paths ...string) Option {
    return func(a *Atlas) { a.configPaths = paths }
}

// WithEnvPrefix sets the environment variable prefix (default: "APP").
func WithEnvPrefix(prefix string) Option {
    return func(a *Atlas) { a.envPrefix = prefix }
}

// WithMiddleware appends custom middleware to the chain.
func WithMiddleware(mw ...gin.HandlerFunc) Option {
    return func(a *Atlas) { a.extraMiddleware = append(a.extraMiddleware, mw...) }
}

// WithoutDefaultMiddleware disables all default middleware.
func WithoutDefaultMiddleware() Option {
    return func(a *Atlas) { a.skipDefaultMW = true }
}
```

- [ ] **Step 2: Commit**

```bash
git add atlas/option.go
git commit -m "feat(atlas): add Option type and With* functions"
```

---

## Task 3: Move Core Subpackages (errors, response, log, i18n)

This task moves user-facing Core packages under `atlas/`. Each move involves: copy files, update package declarations and internal imports, update all references across the codebase.

**Files:**
- Move: `errors/` → `atlas/errors/`
- Move: `response/` → `atlas/response/`
- Move: `log/` → `atlas/log/`
- Move: `i18n/` → `atlas/i18n/`

- [ ] **Step 1: Move errors package**

```bash
cp -r errors/ atlas/errors/
```

Update all files in `atlas/errors/` — package name stays `errors`. Update import paths in all files across the project:

Old: `"github.com/shiliu-ai/go-atlas/errors"`
New: `"github.com/shiliu-ai/go-atlas/atlas/errors"`

Use IDE or `sed` to find and replace across all `.go` files.

- [ ] **Step 2: Move response package**

```bash
cp -r response/ atlas/response/
```

Update import paths in all files:

Old: `"github.com/shiliu-ai/go-atlas/response"`
New: `"github.com/shiliu-ai/go-atlas/atlas/response"`

Note: `atlas/response` imports `atlas/errors` — update that internal reference too.

- [ ] **Step 3: Move log package**

```bash
cp -r log/ atlas/log/
```

Update import paths in all files:

Old: `"github.com/shiliu-ai/go-atlas/log"`
New: `"github.com/shiliu-ai/go-atlas/atlas/log"`

Note: `log` is used extensively — `atlas/arche.go`, `app/app.go`, `middleware/logging.go`, `middleware/recovery.go`, `database/logger.go`, `cache/cache.go`, etc. Update all.

- [ ] **Step 4: Move i18n package**

```bash
cp -r i18n/ atlas/i18n/
```

Update import paths in all files:

Old: `"github.com/shiliu-ai/go-atlas/i18n"`
New: `"github.com/shiliu-ai/go-atlas/atlas/i18n"`

- [ ] **Step 5: Verify compilation**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./...`
Expected: Compiles successfully.

- [ ] **Step 6: Run all existing tests**

Run: `cd /Users/nullkey/laoshen/go-atlas && go test ./...`
Expected: All tests pass.

- [ ] **Step 7: Delete old directories**

```bash
rm -rf errors/ response/ log/ i18n/
```

- [ ] **Step 8: Verify again**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./... && go test ./...`
Expected: Compiles and all tests pass.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "refactor: move errors, response, log, i18n under atlas/"
```

---

## Task 4: Move Artifact Packages

**Files:**
- Move: `crypto/` → `artifact/crypto/`
- Move: `id/` → `artifact/id/`
- Move: `pagination/` → `artifact/pagination/`
- Move: `validate/` → `artifact/validate/`
- Move: `jsonutil/` → `artifact/jsonutil/`

- [ ] **Step 1: Create artifact directory and copy packages**

```bash
mkdir -p artifact
cp -r crypto/ artifact/crypto/
cp -r id/ artifact/id/
cp -r pagination/ artifact/pagination/
cp -r validate/ artifact/validate/
cp -r jsonutil/ artifact/jsonutil/
```

- [ ] **Step 2: Update all import paths across the project**

For each package, find and replace:

| Old Import | New Import |
|-----------|-----------|
| `"github.com/shiliu-ai/go-atlas/crypto"` | `"github.com/shiliu-ai/go-atlas/artifact/crypto"` |
| `"github.com/shiliu-ai/go-atlas/id"` | `"github.com/shiliu-ai/go-atlas/artifact/id"` |
| `"github.com/shiliu-ai/go-atlas/pagination"` | `"github.com/shiliu-ai/go-atlas/artifact/pagination"` |
| `"github.com/shiliu-ai/go-atlas/validate"` | `"github.com/shiliu-ai/go-atlas/artifact/validate"` |
| `"github.com/shiliu-ai/go-atlas/jsonutil"` | `"github.com/shiliu-ai/go-atlas/artifact/jsonutil"` |

- [ ] **Step 3: Verify and delete old directories**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./... && go test ./...`

```bash
rm -rf crypto/ id/ pagination/ validate/ jsonutil/
```

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./... && go test ./...`
Expected: All pass.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: move crypto, id, pagination, validate, jsonutil to artifact/"
```

---

## Task 5: Atlas Config (absorb config/)

**Files:**
- Rewrite: `atlas/config.go`
- Delete (later): `config/config.go`

- [ ] **Step 1: Rewrite atlas/config.go**

Replace the current `atlas/config.go` (which defines the Config struct) with config loading logic. The old Config struct is no longer needed — each Pillar defines its own config.

```go
// atlas/config.go
package atlas

import (
    "strings"

    "github.com/spf13/viper"
)

// serverConfig holds Atlas Core's own configuration.
type serverConfig struct {
    Port            int    `mapstructure:"port"`
    Name            string `mapstructure:"name"`
    Mode            string `mapstructure:"mode"`
    ReadTimeout     string `mapstructure:"read_timeout"`
    WriteTimeout    string `mapstructure:"write_timeout"`
    ShutdownTimeout string `mapstructure:"shutdown_timeout"`
}

type logConfig struct {
    Level  string `mapstructure:"level"`
    Format string `mapstructure:"format"`
}

type i18nConfig struct {
    Default string `mapstructure:"default"`
}

type corsConfig struct {
    AllowOrigins []string `mapstructure:"allow_origins"`
    AllowMethods []string `mapstructure:"allow_methods"`
    AllowHeaders []string `mapstructure:"allow_headers"`
    MaxAge       int      `mapstructure:"max_age"`
}

type rateLimitConfig struct {
    Rate   int    `mapstructure:"rate"`
    Window string `mapstructure:"window"`
}

type middlewareConfig struct {
    CORS      corsConfig      `mapstructure:"cors"`
    RateLimit rateLimitConfig `mapstructure:"rate_limit"`
}

// coreConfig is the internal config read by Atlas Core.
type coreConfig struct {
    Server     serverConfig     `mapstructure:"server"`
    Log        logConfig        `mapstructure:"log"`
    I18n       i18nConfig       `mapstructure:"i18n"`
    Middleware middlewareConfig  `mapstructure:"middleware"`
}

// loadConfig loads configuration from file and environment variables.
func loadConfig(name string, paths []string, envPrefix string) (*viper.Viper, error) {
    v := viper.New()
    v.SetConfigName(name)
    for _, p := range paths {
        v.AddConfigPath(p)
    }
    v.SetEnvPrefix(envPrefix)
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    v.AutomaticEnv()

    if err := v.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            return nil, err
        }
    }
    return v, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./atlas/`
Expected: Compiles. (Other packages may still reference old config — that's fine for now.)

- [ ] **Step 3: Commit**

```bash
git add atlas/config.go
git commit -m "feat(atlas): rewrite config.go with internal config loading"
```

---

## Task 6: Atlas Server & Lifecycle (absorb server/, app/)

**Files:**
- Create: `atlas/server.go`
- Create: `atlas/lifecycle.go`

- [ ] **Step 1: Implement server.go**

Absorb `server/server.go` functionality into Atlas core. The server is an internal implementation detail — not exported.

```go
// atlas/server.go
package atlas

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
)

type server struct {
    engine *gin.Engine
    srv    *http.Server
    port   int
    shutdownTimeout time.Duration
}

func newServer(cfg serverConfig) *server {
    mode := cfg.Mode
    if mode == "" {
        mode = "release"
    }
    gin.SetMode(mode)

    engine := gin.New()

    port := cfg.Port
    if port == 0 {
        port = 8080
    }

    readTimeout := parseDuration(cfg.ReadTimeout, 30*time.Second)
    writeTimeout := parseDuration(cfg.WriteTimeout, 30*time.Second)
    shutdownTimeout := parseDuration(cfg.ShutdownTimeout, 10*time.Second)

    return &server{
        engine: engine,
        port:   port,
        shutdownTimeout: shutdownTimeout,
        srv: &http.Server{
            Addr:         fmt.Sprintf(":%d", port),
            Handler:      engine,
            ReadTimeout:  readTimeout,
            WriteTimeout: writeTimeout,
        },
    }
}

func (s *server) start() error {
    if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        return err
    }
    return nil
}

func (s *server) stop() error {
    ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
    defer cancel()
    return s.srv.Shutdown(ctx)
}

func parseDuration(s string, fallback time.Duration) time.Duration {
    if s == "" {
        return fallback
    }
    d, err := time.ParseDuration(s)
    if err != nil {
        return fallback
    }
    return d
}
```

- [ ] **Step 2: Implement lifecycle.go**

```go
// atlas/lifecycle.go
package atlas

import (
    "context"
    "os"
    "os/signal"
    "syscall"
)

// run starts the application, waits for shutdown signal, then gracefully stops.
func (a *Atlas) run() error {
    // Start Pillars that implement Starter
    for _, name := range a.pillarOrder {
        p := a.pillars[name]
        if s, ok := p.(Starter); ok {
            go func(name string, s Starter) {
                if err := s.Start(context.Background()); err != nil {
                    a.logger.Error(context.Background(), "pillar start failed",
                        log.F("pillar", name), log.F("error", err.Error()))
                }
            }(name, s)
        }
    }

    // Start HTTP server
    errCh := make(chan error, 1)
    go func() {
        a.logger.Info(context.Background(), "server starting",
            log.F("port", a.server.port))
        errCh <- a.server.start()
    }()

    // Wait for signal or error
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

    select {
    case sig := <-quit:
        a.logger.Info(context.Background(), "shutdown signal received",
            log.F("signal", sig.String()))
    case err := <-errCh:
        if err != nil {
            return err
        }
    }

    // Graceful shutdown
    return a.shutdown()
}

func (a *Atlas) shutdown() error {
    ctx := context.Background()

    // 1. Stop HTTP server first (drain in-flight requests)
    a.logger.Info(ctx, "stopping http server")
    if err := a.server.stop(); err != nil {
        a.logger.Error(ctx, "http server stop error", log.F("error", err.Error()))
    }

    // 2. Stop Pillars in reverse registration order
    for i := len(a.pillarOrder) - 1; i >= 0; i-- {
        name := a.pillarOrder[i]
        p := a.pillars[name]
        a.logger.Info(ctx, "stopping pillar", log.F("pillar", name))
        if err := p.Stop(ctx); err != nil {
            a.logger.Error(ctx, "pillar stop error",
                log.F("pillar", name), log.F("error", err.Error()))
        }
    }

    a.logger.Info(ctx, "shutdown complete")
    return nil
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./atlas/`
Expected: Compiles.

- [ ] **Step 4: Commit**

```bash
git add atlas/server.go atlas/lifecycle.go
git commit -m "feat(atlas): add server and lifecycle management"
```

---

## Task 7: Atlas Middleware (absorb middleware/)

**Files:**
- Create: `atlas/middleware.go`

- [ ] **Step 1: Implement middleware.go**

Move the middleware assembly logic into Atlas core. The individual middleware implementations stay in their current files for now but will be referenced from the new location.

Note: The middleware implementations (Recovery, RequestID, Logging, CORS, RateLimit, Tracing) are absorbed from the old `middleware/` package into `atlas/middleware.go`. Copy all function implementations from `middleware/*.go` into this file, updating their package declaration to `atlas`.

```go
// atlas/middleware.go
package atlas

import (
    "github.com/gin-gonic/gin"
)

// setupMiddleware assembles the three-layer middleware chain.
// Individual middleware functions (recovery, requestID, logging, cors, rateLimit)
// are copied from the old middleware/ package into this file.
func (a *Atlas) setupMiddleware() {
    if a.skipDefaultMW {
        a.applyPillarMiddleware()
        a.server.engine.Use(a.extraMiddleware...)
        return
    }

    // Layer 1: Core defaults
    a.server.engine.Use(
        recovery(a.logger),
        requestID(),
    )

    if a.i18nBundle != nil {
        a.server.engine.Use(a.i18nBundle.Middleware())
    }

    a.server.engine.Use(logging(a.logger))

    a.server.engine.Use(cors(corsConfig{
        AllowOrigins: a.coreCfg.Middleware.CORS.AllowOrigins,
        AllowMethods: a.coreCfg.Middleware.CORS.AllowMethods,
        AllowHeaders: a.coreCfg.Middleware.CORS.AllowHeaders,
        MaxAge:       a.coreCfg.Middleware.CORS.MaxAge,
    }))

    if a.coreCfg.Middleware.RateLimit.Rate > 0 {
        window := parseDuration(a.coreCfg.Middleware.RateLimit.Window, 0)
        a.server.engine.Use(rateLimit(a.coreCfg.Middleware.RateLimit.Rate, window))
    }

    // Layer 2: Pillar middleware
    a.applyPillarMiddleware()

    // Layer 3: User custom middleware
    if len(a.extraMiddleware) > 0 {
        a.server.engine.Use(a.extraMiddleware...)
    }
}

func (a *Atlas) applyPillarMiddleware() {
    for _, name := range a.pillarOrder {
        p := a.pillars[name]
        if mp, ok := p.(MiddlewareProvider); ok {
            a.server.engine.Use(mp.Middleware()...)
        }
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add atlas/middleware.go
git commit -m "feat(atlas): add middleware chain assembly"
```

---

## Task 8: Atlas Health Check

**Files:**
- Create: `atlas/health.go`

- [ ] **Step 1: Implement health.go**

```go
// atlas/health.go
package atlas

import (
    "context"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
)

type pillarHealth struct {
    Status  string `json:"status"`
    Latency string `json:"latency,omitempty"`
    Error   string `json:"error,omitempty"`
}

type healthResponse struct {
    Status  string                    `json:"status"`
    Pillars map[string]pillarHealth   `json:"pillars,omitempty"`
}

func (a *Atlas) registerHealthRoutes() {
    a.server.engine.GET("/livez", a.handleLive)
    a.server.engine.GET("/healthz", a.handleHealth)
    a.server.engine.GET("/readyz", a.handleHealth)
}

func (a *Atlas) handleLive(c *gin.Context) {
    c.JSON(http.StatusOK, healthResponse{Status: "healthy"})
}

func (a *Atlas) handleHealth(c *gin.Context) {
    resp := healthResponse{
        Status:  "healthy",
        Pillars: make(map[string]pillarHealth),
    }
    allHealthy := true

    for _, name := range a.pillarOrder {
        p := a.pillars[name]
        hc, ok := p.(HealthChecker)
        if !ok {
            continue
        }

        start := time.Now()
        err := hc.Health(context.Background())
        latency := time.Since(start)

        ph := pillarHealth{
            Status:  "healthy",
            Latency: latency.String(),
        }
        if err != nil {
            ph.Status = "unhealthy"
            ph.Error = err.Error()
            allHealthy = false
        }
        resp.Pillars[name] = ph
    }

    if !allHealthy {
        resp.Status = "unhealthy"
        c.JSON(http.StatusServiceUnavailable, resp)
        return
    }
    c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 2: Commit**

```bash
git add atlas/health.go
git commit -m "feat(atlas): add health check endpoint aggregation"
```

---

## Task 9: Atlas Main (atlas.go rewrite)

**Files:**
- Rewrite: `atlas/atlas.go` (replace old `atlas/arche.go`)

- [ ] **Step 1: Write atlas_test.go for the full New/Run flow**

```go
// atlas/atlas_test.go
package atlas

import (
    "context"
    "testing"
)

func TestNewInitsPillars(t *testing.T) {
    p := &testPillar{name: "test"}

    // We can't call real New() without config file,
    // so test the init flow directly.
    a := &Atlas{
        pillars:     make(map[string]Pillar),
        pillarOrder: []string{},
    }
    a.Register(p)
    a.pillarOrder = append(a.pillarOrder, p.Name())

    // Simulate init
    core := &Core{logger: /* need a logger */}
    // This test will be expanded once the full Atlas is wired.
}

func TestUnmarshal(t *testing.T) {
    v := viper.New()
    v.Set("app.name", "test-app")
    v.Set("app.port", 3000)

    a := &Atlas{config: v}

    type appCfg struct {
        Name string `mapstructure:"name"`
        Port int    `mapstructure:"port"`
    }
    var cfg appCfg
    if err := a.Unmarshal("app", &cfg); err != nil {
        t.Fatalf("Unmarshal failed: %v", err)
    }
    if cfg.Name != "test-app" || cfg.Port != 3000 {
        t.Fatalf("unexpected config: %+v", cfg)
    }
}
```

- [ ] **Step 2: Implement the new atlas.go**

Delete `atlas/arche.go`. Create `atlas/atlas.go`:

```go
// atlas/atlas.go
package atlas

import (
    "context"
    "fmt"

    "github.com/gin-gonic/gin"
    "github.com/shiliu-ai/go-atlas/atlas/i18n"
    "github.com/shiliu-ai/go-atlas/atlas/log"
    "github.com/spf13/viper"
)

// Atlas is the central framework instance.
type Atlas struct {
    name        string
    configName  string
    configPaths []string
    envPrefix   string

    config  *viper.Viper
    coreCfg coreConfig
    logger  log.Logger
    server  *server

    pillars     map[string]Pillar
    pillarOrder []string

    extraMiddleware []gin.HandlerFunc
    skipDefaultMW   bool
    i18nBundle      *i18n.Bundle
}

// New creates and initializes an Atlas instance.
func New(name string, opts ...Option) *Atlas {
    a := &Atlas{
        name:        name,
        configName:  "config",
        configPaths: []string{"."},
        envPrefix:   "APP",
        pillars:     make(map[string]Pillar),
    }

    // 1. Apply options (collects Pillars and settings)
    for _, opt := range opts {
        opt(a)
    }

    // 2. Load config
    v, err := loadConfig(a.configName, a.configPaths, a.envPrefix)
    if err != nil {
        panic(fmt.Sprintf("atlas: failed to load config: %v", err))
    }
    a.config = v

    // Unmarshal core config
    if err := v.Unmarshal(&a.coreCfg); err != nil {
        panic(fmt.Sprintf("atlas: failed to parse core config: %v", err))
    }

    // 3. Init Core (logger, server)
    a.logger = initLogger(a.coreCfg.Log)
    log.SetGlobal(a.logger)
    a.server = newServer(a.coreCfg.Server)

    // i18n
    if a.coreCfg.I18n.Default != "" {
        a.i18nBundle = i18n.NewBundle(a.coreCfg.I18n.Default)
    }

    // 4. Init Pillars
    core := &Core{config: a.config, logger: a.logger}
    for _, name := range a.pillarOrder {
        p := a.pillars[name]
        if err := p.Init(core); err != nil {
            panic(fmt.Sprintf("atlas: pillar %q init failed: %v", name, err))
        }
        a.logger.Info(context.Background(), "pillar initialized", log.F("pillar", name))
    }

    // 5. Setup middleware
    a.setupMiddleware()

    // 6. Register health routes
    a.registerHealthRoutes()

    return a
}

// Register adds a Pillar and tracks registration order.
// Overrides the simple Register in pillar.go.
func (a *Atlas) Register(p Pillar) {
    name := p.Name()
    if _, exists := a.pillars[name]; exists {
        panic(fmt.Sprintf("atlas: duplicate pillar name %q", name))
    }
    a.pillars[name] = p
    a.pillarOrder = append(a.pillarOrder, name)
}

// Unmarshal deserializes a config section into target.
func (a *Atlas) Unmarshal(key string, target any) error {
    sub := a.config.Sub(key)
    if sub == nil {
        return fmt.Errorf("atlas: config section %q not found", key)
    }
    return sub.Unmarshal(target)
}

// Engine returns the underlying Gin engine for advanced usage.
func (a *Atlas) Engine() *gin.Engine {
    return a.server.engine
}

// Route registers routes on the service's base route group.
func (a *Atlas) Route(fn func(*gin.RouterGroup)) *Atlas {
    prefix := ""
    if a.coreCfg.Server.Name != "" {
        prefix = "/" + a.coreCfg.Server.Name
    }
    group := a.server.engine.Group(prefix)
    fn(group)
    return a
}

// Run starts the HTTP server and blocks until shutdown.
func (a *Atlas) Run() error {
    return a.run()
}

// MustRun calls Run and panics on error.
func (a *Atlas) MustRun() {
    if err := a.Run(); err != nil {
        panic(fmt.Sprintf("atlas: %v", err))
    }
}

// Logger returns the framework logger.
func (a *Atlas) Logger() log.Logger {
    return a.logger
}

// initLogger creates a logger from config.
func initLogger(cfg logConfig) log.Logger {
    level := log.LevelInfo
    switch cfg.Level {
    case "debug":
        level = log.LevelDebug
    case "warn":
        level = log.LevelWarn
    case "error":
        level = log.LevelError
    }

    var opts []log.Option
    if cfg.Format == "json" {
        opts = append(opts, log.WithJSON())
    }
    return log.NewDefault(level, opts...)
}
```

Note: Remove the temporary `Atlas` struct from `pillar.go` since it's now defined in `atlas.go`. Also remove the duplicate `Register` from `pillar.go` — the one in `atlas.go` tracks order.

Update `pillar.go`: remove the `Atlas` struct and `Register` method, keep only interfaces and `Use`/`TryUse` functions.

- [ ] **Step 3: Delete old arche.go and old config.go**

```bash
rm atlas/arche.go
```

The old `atlas/config.go` has already been replaced in Task 5.

- [ ] **Step 4: Verify compilation**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./atlas/`

Fix any compilation errors. At this point, other packages still reference the old Atlas API — that's expected. The old packages (`app/`, `server/`, `config/`) still exist and other code uses them.

- [ ] **Step 5: Run atlas package tests**

Run: `cd /Users/nullkey/laoshen/go-atlas && go test ./atlas/ -v`
Expected: Tests pass.

- [ ] **Step 6: Commit**

```bash
git add atlas/
git commit -m "feat(atlas): rewrite atlas.go with Pillar-based architecture"
```

---

## Task 10: Database Pillar (first migration, validates pattern)

**Files:**
- Create: `pillar/database/database.go` (Manager + Pillar interface)
- Create: `pillar/database/pillar.go` (Pillar(), Of(), options)
- Create: `pillar/database/logger.go` (GORM logger adapter)
- Create: `pillar/database/session.go` (session helpers)
- Create: `pillar/database/pillar_test.go`

- [ ] **Step 1: Write test for database Pillar registration**

```go
// pillar/database/pillar_test.go
package database

import (
    "testing"

    "github.com/shiliu-ai/go-atlas/atlas"
    "github.com/spf13/viper"
)

func TestPillarReturnsOption(t *testing.T) {
    opt := Pillar()
    if opt == nil {
        t.Fatal("Pillar() returned nil")
    }
}

func TestPillarName(t *testing.T) {
    m := &Manager{}
    if m.Name() != "databases" {
        t.Fatalf("expected name 'databases', got %q", m.Name())
    }
}
```

- [ ] **Step 2: Copy and adapt existing database code**

```bash
mkdir -p pillar/database
cp database/database.go pillar/database/database.go
cp database/logger.go pillar/database/logger.go
cp database/session.go pillar/database/session.go
```

Update package declarations to `package database`.
Update import paths: `log` → `atlas/log`.

- [ ] **Step 3: Create pillar.go with Pillar(), Of(), and interface implementation**

```go
// pillar/database/pillar.go
package database

import (
    "context"
    "fmt"

    "github.com/shiliu-ai/go-atlas/atlas"
)

// Pillar returns an atlas.Option that registers the database Pillar.
func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        m := &Manager{}
        for _, opt := range opts {
            opt(m)
        }
        a.Register(m)
    }
}

// Of retrieves the database Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager {
    return atlas.Use[*Manager](a)
}

// Option configures the database Manager.
type Option func(*Manager)

// Ensure Manager implements atlas.Pillar and atlas.HealthChecker.
var (
    _ atlas.Pillar       = (*Manager)(nil)
    _ atlas.HealthChecker = (*Manager)(nil)
)

func (m *Manager) Name() string { return "databases" }

func (m *Manager) Init(core *atlas.Core) error {
    var cfgs map[string]Config
    if err := core.Unmarshal("databases", &cfgs); err != nil {
        return fmt.Errorf("database: %w", err)
    }
    m.logger = core.Logger("database")
    m.configs = cfgs
    // Initialize default connection eagerly
    if _, ok := cfgs["default"]; ok {
        if _, err := m.Get("default"); err != nil {
            return fmt.Errorf("database: default connection: %w", err)
        }
    }
    return nil
}

func (m *Manager) Stop(ctx context.Context) error {
    m.mu.RLock()
    defer m.mu.RUnlock()
    for name, db := range m.instances {
        sqlDB, err := db.DB()
        if err != nil {
            m.logger.Error(ctx, "failed to get sql.DB", log.F("name", name))
            continue
        }
        if err := sqlDB.Close(); err != nil {
            m.logger.Error(ctx, "failed to close db", log.F("name", name))
        }
    }
    return nil
}

func (m *Manager) Health(ctx context.Context) error {
    m.mu.RLock()
    defer m.mu.RUnlock()
    for name, db := range m.instances {
        sqlDB, err := db.DB()
        if err != nil {
            return fmt.Errorf("database %s: %w", name, err)
        }
        if err := sqlDB.PingContext(ctx); err != nil {
            return fmt.Errorf("database %s: %w", name, err)
        }
    }
    return nil
}
```

Note: The `Manager` struct, `Config` struct, `Get()`, and other methods remain from the copied `database.go`. Adapt them to use `atlas/log` and remove dependency on the old config structs.

- [ ] **Step 4: Run tests**

Run: `cd /Users/nullkey/laoshen/go-atlas && go test ./pillar/database/ -v`
Expected: Tests pass.

- [ ] **Step 5: Commit**

```bash
git add pillar/database/
git commit -m "feat(pillar): migrate database to Pillar interface"
```

---

## Task 11: Cache Pillar (including lock absorption)

**Files:**
- Create: `pillar/cache/cache.go` (RedisCache + Pillar interface)
- Create: `pillar/cache/pillar.go` (Pillar(), Of())
- Create: `pillar/cache/lock.go` (absorbed from lock/)

- [ ] **Step 1: Copy and adapt existing cache and lock code**

```bash
mkdir -p pillar/cache
cp cache/cache.go pillar/cache/cache.go
cp cache/redis.go pillar/cache/redis.go
cp lock/lock.go pillar/cache/lock.go
cp lock/redis.go pillar/cache/lock_redis.go
```

Update package declarations and import paths.

- [ ] **Step 2: Create pillar.go**

```go
// pillar/cache/pillar.go
package cache

import (
    "context"
    "fmt"

    "github.com/shiliu-ai/go-atlas/atlas"
)

func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        c := &RedisCache{}
        for _, opt := range opts {
            opt(c)
        }
        a.Register(c)
    }
}

func Of(a *atlas.Atlas) *RedisCache {
    return atlas.Use[*RedisCache](a)
}

type Option func(*RedisCache)

var (
    _ atlas.Pillar        = (*RedisCache)(nil)
    _ atlas.HealthChecker = (*RedisCache)(nil)
)

func (c *RedisCache) Name() string { return "redis" }

func (c *RedisCache) Init(core *atlas.Core) error {
    var cfg RedisConfig
    if err := core.Unmarshal("redis", &cfg); err != nil {
        return fmt.Errorf("cache: %w", err)
    }
    c.logger = core.Logger("cache")
    return c.connect(cfg)
}

func (c *RedisCache) Stop(ctx context.Context) error {
    if c.client != nil {
        return c.client.Close()
    }
    return nil
}

func (c *RedisCache) Health(ctx context.Context) error {
    return c.client.Ping(ctx).Err()
}

// NewLock creates a distributed lock using this cache's Redis connection.
func (c *RedisCache) NewLock(key string, ttl time.Duration) *RedisLock {
    return NewRedisLock(c.client, key, ttl)
}
```

- [ ] **Step 3: Verify and commit**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./pillar/cache/`

```bash
git add pillar/cache/
git commit -m "feat(pillar): migrate cache to Pillar interface, absorb lock"
```

---

## Task 12: Auth & OAuth Pillars

**Files:**
- Create: `pillar/auth/` (copy from auth/, add pillar.go)
- Create: `pillar/oauth/` (copy from oauth/, add pillar.go)

- [ ] **Step 1: Migrate auth**

```bash
mkdir -p pillar/auth
cp auth/jwt.go pillar/auth/jwt.go
cp auth/middleware.go pillar/auth/middleware.go
```

```go
// pillar/auth/pillar.go
package auth

import (
    "context"
    "fmt"

    "github.com/shiliu-ai/go-atlas/atlas"
)

func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        j := &JWT{}
        for _, opt := range opts {
            opt(j)
        }
        a.Register(j)
    }
}

func Of(a *atlas.Atlas) *JWT {
    return atlas.Use[*JWT](a)
}

type Option func(*JWT)

var _ atlas.Pillar = (*JWT)(nil)

func (j *JWT) Name() string { return "auth" }

func (j *JWT) Init(core *atlas.Core) error {
    var cfg Config
    if err := core.Unmarshal("auth", &cfg); err != nil {
        return fmt.Errorf("auth: %w", err)
    }
    return j.configure(cfg)
}

func (j *JWT) Stop(ctx context.Context) error { return nil }
```

- [ ] **Step 2: Migrate oauth**

```bash
mkdir -p pillar/oauth
cp oauth/oauth.go pillar/oauth/oauth.go
```

```go
// pillar/oauth/pillar.go
package oauth

import (
    "context"
    "fmt"

    "github.com/shiliu-ai/go-atlas/atlas"
)

func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        p := &Provider{}
        for _, opt := range opts {
            opt(p)
        }
        a.Register(p)
    }
}

func Of(a *atlas.Atlas) *Provider {
    return atlas.Use[*Provider](a)
}

type Option func(*Provider)

var _ atlas.Pillar = (*Provider)(nil)

func (p *Provider) Name() string { return "oauth" }

func (p *Provider) Init(core *atlas.Core) error {
    var cfg Config
    if err := core.Unmarshal("oauth", &cfg); err != nil {
        return fmt.Errorf("oauth: %w", err)
    }
    return p.configure(cfg)
}

func (p *Provider) Stop(ctx context.Context) error { return nil }
```

- [ ] **Step 3: Verify and commit**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./pillar/auth/ ./pillar/oauth/`

```bash
git add pillar/auth/ pillar/oauth/
git commit -m "feat(pillar): migrate auth and oauth to Pillar interface"
```

---

## Task 13: Storage & SMS Pillars

**Files:**
- Create: `pillar/storage/` (copy from storage/, add pillar.go)
- Create: `pillar/sms/` (copy from sms/, add pillar.go)

- [ ] **Step 1: Migrate storage**

```bash
mkdir -p pillar/storage
cp storage/*.go pillar/storage/
```

```go
// pillar/storage/pillar.go
package storage

import (
    "context"
    "fmt"

    "github.com/shiliu-ai/go-atlas/atlas"
)

func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        m := &Manager{}
        for _, opt := range opts {
            opt(m)
        }
        a.Register(m)
    }
}

func Of(a *atlas.Atlas) *Manager {
    return atlas.Use[*Manager](a)
}

type Option func(*Manager)

var _ atlas.Pillar = (*Manager)(nil)

func (m *Manager) Name() string { return "storages" }

func (m *Manager) Init(core *atlas.Core) error {
    var cfgs map[string]Config
    if err := core.Unmarshal("storages", &cfgs); err != nil {
        return fmt.Errorf("storage: %w", err)
    }
    m.logger = core.Logger("storage")
    m.configs = cfgs
    return nil
}

func (m *Manager) Stop(ctx context.Context) error { return nil }
```

- [ ] **Step 2: Migrate sms**

```bash
mkdir -p pillar/sms
cp sms/*.go pillar/sms/
```

```go
// pillar/sms/pillar.go
package sms

import (
    "context"
    "fmt"

    "github.com/shiliu-ai/go-atlas/atlas"
)

func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        m := &Manager{}
        for _, opt := range opts {
            opt(m)
        }
        a.Register(m)
    }
}

func Of(a *atlas.Atlas) *Manager {
    return atlas.Use[*Manager](a)
}

type Option func(*Manager)

var (
    _ atlas.Pillar       = (*Manager)(nil)
    _ atlas.HealthChecker = (*Manager)(nil)
)

func (m *Manager) Name() string { return "sms" }

func (m *Manager) Init(core *atlas.Core) error {
    var cfgs map[string]Config
    if err := core.Unmarshal("sms", &cfgs); err != nil {
        return fmt.Errorf("sms: %w", err)
    }
    m.logger = core.Logger("sms")
    m.configs = cfgs
    return nil
}

func (m *Manager) Stop(ctx context.Context) error { return nil }

func (m *Manager) Health(ctx context.Context) error {
    sms, err := m.Get("default")
    if err != nil {
        return err
    }
    return sms.Ping(ctx)
}
```

- [ ] **Step 3: Verify and commit**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./pillar/storage/ ./pillar/sms/`

```bash
git add pillar/storage/ pillar/sms/
git commit -m "feat(pillar): migrate storage and sms to Pillar interface"
```

---

## Task 14: Tracing, HTTPClient, ServiceClient Pillars

**Files:**
- Create: `pillar/tracing/` (copy from tracing/, add pillar.go)
- Create: `pillar/httpclient/` (copy from httpclient/, add pillar.go)
- Create: `pillar/serviceclient/` (copy from serviceclient/, add pillar.go)

- [ ] **Step 1: Migrate tracing**

```bash
mkdir -p pillar/tracing
cp tracing/tracing.go pillar/tracing/tracing.go
```

```go
// pillar/tracing/pillar.go
package tracing

import (
    "context"
    "fmt"

    "github.com/gin-gonic/gin"
    "github.com/shiliu-ai/go-atlas/atlas"
    "github.com/shiliu-ai/go-atlas/middleware"
)

func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        t := &Tracer{}
        for _, opt := range opts {
            opt(t)
        }
        a.Register(t)
    }
}

func Of(a *atlas.Atlas) *Tracer {
    return atlas.Use[*Tracer](a)
}

type Option func(*Tracer)

type Tracer struct {
    shutdown func(context.Context) error
}

var (
    _ atlas.Pillar             = (*Tracer)(nil)
    _ atlas.MiddlewareProvider = (*Tracer)(nil)
)

func (t *Tracer) Name() string { return "tracing" }

func (t *Tracer) Init(core *atlas.Core) error {
    var cfg Config
    if err := core.Unmarshal("tracing", &cfg); err != nil {
        return fmt.Errorf("tracing: %w", err)
    }
    shutdown, err := Init(cfg)
    if err != nil {
        return fmt.Errorf("tracing: %w", err)
    }
    t.shutdown = shutdown
    return nil
}

func (t *Tracer) Stop(ctx context.Context) error {
    if t.shutdown != nil {
        return t.shutdown(ctx)
    }
    return nil
}

func (t *Tracer) Middleware() []gin.HandlerFunc {
    return []gin.HandlerFunc{middleware.Tracing()}
}
```

- [ ] **Step 2: Migrate httpclient**

```bash
mkdir -p pillar/httpclient
cp httpclient/client.go pillar/httpclient/client.go
```

```go
// pillar/httpclient/pillar.go
package httpclient

import (
    "context"
    "fmt"

    "github.com/shiliu-ai/go-atlas/atlas"
)

func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        c := &Client{}
        for _, opt := range opts {
            opt(c)
        }
        a.Register(c)
    }
}

func Of(a *atlas.Atlas) *Client {
    return atlas.Use[*Client](a)
}

type Option func(*Client)

var _ atlas.Pillar = (*Client)(nil)

func (c *Client) Name() string { return "httpclient" }

func (c *Client) Init(core *atlas.Core) error {
    var cfg Config
    if err := core.Unmarshal("httpclient", &cfg); err != nil {
        // httpclient config is optional — use defaults
        cfg = DefaultConfig()
    }
    c.configure(cfg)
    return nil
}

func (c *Client) Stop(ctx context.Context) error { return nil }
```

- [ ] **Step 3: Migrate serviceclient**

```bash
mkdir -p pillar/serviceclient
cp serviceclient/*.go pillar/serviceclient/
```

```go
// pillar/serviceclient/pillar.go
package serviceclient

import (
    "context"
    "fmt"

    "github.com/gin-gonic/gin"
    "github.com/shiliu-ai/go-atlas/atlas"
)

func Pillar(opts ...Option) atlas.Option {
    return func(a *atlas.Atlas) {
        m := &Manager{}
        for _, opt := range opts {
            opt(m)
        }
        a.Register(m)
    }
}

func Of(a *atlas.Atlas) *Manager {
    return atlas.Use[*Manager](a)
}

type Option func(*Manager)

var (
    _ atlas.Pillar             = (*Manager)(nil)
    _ atlas.MiddlewareProvider = (*Manager)(nil)
)

func (m *Manager) Name() string { return "services" }

func (m *Manager) Init(core *atlas.Core) error {
    var cfgs map[string]ServiceConfig
    if err := core.Unmarshal("services", &cfgs); err != nil {
        return fmt.Errorf("serviceclient: %w", err)
    }
    m.logger = core.Logger("serviceclient")
    m.configs = cfgs
    return nil
}

func (m *Manager) Stop(ctx context.Context) error { return nil }

func (m *Manager) Middleware() []gin.HandlerFunc {
    return []gin.HandlerFunc{ForwardHeaders()}
}
```

- [ ] **Step 4: Verify and commit**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./pillar/tracing/ ./pillar/httpclient/ ./pillar/serviceclient/`

```bash
git add pillar/tracing/ pillar/httpclient/ pillar/serviceclient/
git commit -m "feat(pillar): migrate tracing, httpclient, serviceclient"
```

---

## Task 15: Update Example Application

**Files:**
- Rewrite: `example/main.go`

- [ ] **Step 1: Rewrite example with new architecture**

```go
// example/main.go
package main

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/shiliu-ai/go-atlas/atlas"
    "github.com/shiliu-ai/go-atlas/atlas/errors"
    "github.com/shiliu-ai/go-atlas/atlas/response"
    "github.com/shiliu-ai/go-atlas/artifact/crypto"
    "github.com/shiliu-ai/go-atlas/artifact/id"
    "github.com/shiliu-ai/go-atlas/artifact/pagination"
    "github.com/shiliu-ai/go-atlas/artifact/validate"
    "github.com/shiliu-ai/go-atlas/pillar/auth"
    "github.com/shiliu-ai/go-atlas/pillar/cache"
    "github.com/shiliu-ai/go-atlas/pillar/database"
    "github.com/shiliu-ai/go-atlas/pillar/httpclient"
    "github.com/shiliu-ai/go-atlas/pillar/serviceclient"
)

func main() {
    // 立柱
    a := atlas.New("example-service",
        atlas.WithConfigPaths(".", "./example"),
        database.Pillar(),
        cache.Pillar(),
        auth.Pillar(),
        httpclient.Pillar(),
        serviceclient.Pillar(),
    )

    // 连线
    authMW := auth.Of(a).Middleware()
    hc := httpclient.Of(a)
    svcm := serviceclient.Of(a)
    snowflake, _ := id.NewSnowflake(1)

    // 路由
    a.Route(func(r *gin.RouterGroup) {
        v1 := r.Group("/v1")

        // Public
        v1.GET("/health", func(c *gin.Context) {
            response.OK(c, gin.H{"status": "ok"})
        })
        v1.POST("/login", handleLogin(auth.Of(a)))
        v1.POST("/refresh", handleRefresh(auth.Of(a)))

        // Protected
        api := v1.Group("/api", authMW)
        api.GET("/me", handleMe)
        api.GET("/items", handleItems(snowflake))
        api.GET("/proxy", handleProxy(hc))
        api.GET("/user/:id", handleGetUser(svcm))
        api.GET("/id", handleID(snowflake))
    })

    // 启动
    a.MustRun()
}

// Handler functions follow the same logic as the current example,
// but receive dependencies as parameters instead of using atlas accessors.

func handleLogin(jwt *auth.JWT) gin.HandlerFunc {
    return func(c *gin.Context) {
        var req struct {
            Username string `json:"username" binding:"required"`
            Password string `json:"password" binding:"required"`
        }
        if err := validate.BindJSON(c, &req); err != nil {
            response.Err(c, err)
            return
        }
        // Demo: hardcoded check
        if req.Username != "admin" || !crypto.CheckPassword("password123", req.Password) {
            response.Fail(c, errors.CodeUnauthorized, "invalid credentials")
            return
        }
        pair, err := jwt.GeneratePair(req.Username, nil)
        if err != nil {
            response.Err(c, err)
            return
        }
        response.OK(c, pair)
    }
}

func handleRefresh(jwt *auth.JWT) gin.HandlerFunc {
    return func(c *gin.Context) {
        var req struct {
            RefreshToken string `json:"refresh_token" binding:"required"`
        }
        if err := validate.BindJSON(c, &req); err != nil {
            response.Err(c, err)
            return
        }
        pair, err := jwt.Refresh(req.RefreshToken)
        if err != nil {
            response.Err(c, err)
            return
        }
        response.OK(c, pair)
    }
}

func handleMe(c *gin.Context) {
    claims := auth.ClaimsFromContext(c)
    response.OK(c, claims)
}

func handleItems(sf *id.Snowflake) gin.HandlerFunc {
    return func(c *gin.Context) {
        page := pagination.FromContext(c)
        items := make([]gin.H, 0, page.Size)
        for i := 0; i < page.Size; i++ {
            items = append(items, gin.H{"id": sf.Generate(), "name": "item"})
        }
        response.OK(c, pagination.NewResponse(page, int64(100), items))
    }
}

func handleProxy(hc *httpclient.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        url := c.Query("url")
        if url == "" {
            response.Fail(c, errors.CodeBadRequest, "url required")
            return
        }
        resp, err := hc.Get(c.Request.Context(), url)
        if err != nil {
            response.Err(c, errors.Wrap(errors.CodeBadGateway, "proxy failed", err))
            return
        }
        response.OK(c, resp.String())
    }
}

func handleGetUser(svcm *serviceclient.Manager) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.Param("id")
        svc := svcm.MustGet("user-service")
        var user map[string]any
        if err := serviceclient.Get(c, svc, "/v1/users/"+userID, &user); err != nil {
            response.Err(c, err)
            return
        }
        response.OK(c, user)
    }
}

func handleID(sf *id.Snowflake) gin.HandlerFunc {
    return func(c *gin.Context) {
        response.OK(c, gin.H{
            "snowflake": sf.Generate(),
            "uuid":      id.UUID(),
            "nanoid":    id.NanoID(),
            "short":     id.ShortID(),
        })
    }
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./example/`
Expected: Compiles.

- [ ] **Step 3: Commit**

```bash
git add example/main.go
git commit -m "refactor(example): update to new Atlas architecture"
```

---

## Task 16: Delete Old Packages & Final Cleanup

**Files:**
- Delete: `app/`, `config/`, `server/`, `lock/`, `middleware/` (if fully absorbed)
- Delete: `auth/`, `cache/`, `database/`, `oauth/`, `storage/`, `sms/`, `tracing/`, `httpclient/`, `serviceclient/`
- Delete: `crypto/`, `id/`, `pagination/`, `validate/`, `jsonutil/`

- [ ] **Step 1: Verify no remaining references to old import paths**

Run: `cd /Users/nullkey/laoshen/go-atlas && grep -r '"github.com/shiliu-ai/go-atlas/auth"' --include='*.go' .`

Repeat for each old package path. Fix any remaining references.

- [ ] **Step 2: Delete old directories**

```bash
rm -rf app/ config/ server/ lock/
rm -rf auth/ cache/ database/ oauth/ storage/ sms/ tracing/ httpclient/ serviceclient/
rm -rf crypto/ id/ pagination/ validate/ jsonutil/
```

- [ ] **Step 3: Final verification**

Run: `cd /Users/nullkey/laoshen/go-atlas && go build ./... && go test ./...`
Expected: Everything compiles and all tests pass.

- [ ] **Step 4: Run go mod tidy**

Run: `cd /Users/nullkey/laoshen/go-atlas && go mod tidy`

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: remove old packages, complete architecture migration"
```

---

## Task 17: Integration Test

**Files:**
- Create: `atlas/integration_test.go`

- [ ] **Step 1: Write integration test verifying the full lifecycle**

```go
// atlas/integration_test.go
package atlas_test

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/shiliu-ai/go-atlas/atlas"
)

type mockPillar struct {
    name    string
    initErr error
    inited  bool
    stopped bool
}

func (p *mockPillar) Name() string                    { return p.name }
func (p *mockPillar) Init(core *atlas.Core) error     { p.inited = true; return p.initErr }
func (p *mockPillar) Stop(ctx context.Context) error  { p.stopped = true; return nil }

func (p *mockPillar) Health(ctx context.Context) error {
    if !p.inited {
        return fmt.Errorf("not initialized")
    }
    return nil
}

func writeTestConfig(t *testing.T, dir string) {
    t.Helper()
    cfg := `
server:
  port: 0
  mode: test
log:
  level: error
  format: text
`
    os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0644)
}

func TestAtlasNewInitsPillarsInOrder(t *testing.T) {
    dir := t.TempDir()
    writeTestConfig(t, dir)

    p1 := &mockPillar{name: "first"}
    p2 := &mockPillar{name: "second"}

    pillar1 := atlas.Option(func(a *atlas.Atlas) { a.Register(p1) })
    pillar2 := atlas.Option(func(a *atlas.Atlas) { a.Register(p2) })

    _ = atlas.New("test-svc",
        atlas.WithConfigPaths(dir),
        pillar1,
        pillar2,
    )

    if !p1.inited || !p2.inited {
        t.Fatal("pillars not initialized")
    }
}

func TestAtlasNewPanicsOnInitFailure(t *testing.T) {
    dir := t.TempDir()
    writeTestConfig(t, dir)

    p := &mockPillar{name: "bad", initErr: fmt.Errorf("connection refused")}
    opt := atlas.Option(func(a *atlas.Atlas) { a.Register(p) })

    defer func() {
        r := recover()
        if r == nil {
            t.Fatal("expected panic on pillar init failure")
        }
    }()
    atlas.New("test-svc", atlas.WithConfigPaths(dir), opt)
}

func TestAtlasUnmarshal(t *testing.T) {
    dir := t.TempDir()
    cfg := `
server:
  port: 0
app:
  name: "my-app"
  workers: 4
`
    os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0644)

    a := atlas.New("test-svc", atlas.WithConfigPaths(dir))

    var appCfg struct {
        Name    string `mapstructure:"name"`
        Workers int    `mapstructure:"workers"`
    }
    if err := a.Unmarshal("app", &appCfg); err != nil {
        t.Fatalf("Unmarshal failed: %v", err)
    }
    if appCfg.Name != "my-app" || appCfg.Workers != 4 {
        t.Fatalf("unexpected: %+v", appCfg)
    }
}
```

- [ ] **Step 2: Run integration tests**

Run: `cd /Users/nullkey/laoshen/go-atlas && go test ./atlas/ -v`
Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add atlas/integration_test.go
git commit -m "test(atlas): add integration tests for Pillar lifecycle"
```

---

## Verification Checklist

After all tasks are complete, verify:

- [ ] `go build ./...` — all packages compile
- [ ] `go test ./...` — all tests pass
- [ ] `go vet ./...` — no vet issues
- [ ] Import paths follow three-domain pattern (`atlas/`, `pillar/`, `artifact/`)
- [ ] No references to old package paths remain
- [ ] `example/main.go` demonstrates the new API correctly
- [ ] Health endpoints (`/healthz`, `/livez`, `/readyz`) respond correctly
- [ ] Old directories (`app/`, `config/`, `server/`, `lock/`, etc.) are deleted
