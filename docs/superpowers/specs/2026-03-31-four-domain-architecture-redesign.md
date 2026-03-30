# Atlas Four-Domain Architecture Redesign

**Date:** 2026-03-31
**Status:** Approved

## Overview

Restructure the go-atlas project from the current three-directory layout (`atlas/`, `pillar/`, `artifact/`) to a four-domain architecture with the core atlas package promoted to the module root.

### Current Structure

```
go-atlas/
├── atlas/                    # package atlas (core + sub-packages)
│   ├── *.go                  # 10 source files
│   ├── errors/               # error types
│   ├── i18n/                 # internationalization
│   ├── log/                  # logging
│   └── response/             # HTTP response helpers
├── pillar/                   # lifecycle components
├── artifact/                 # standalone toolkit
├── example/
├── go.mod
└── README.md
```

### Problems

1. **Redundant import path**: `github.com/shiliu-ai/go-atlas/atlas` — `go-atlas/atlas` repeats itself.
2. **Cluttered core directory**: `atlas/` has 10 .go files + 4 sub-directories, visually messy.
3. **Orphaned sub-packages**: `errors/`, `log/`, `i18n/`, `response/` don't belong to any of the three named domains (Atlas/Pillar/Artifact), creating ambiguity.

## Design

### Four-Domain Taxonomy

| Domain | Name | Meaning | Role |
|--------|------|---------|------|
| Core | **Atlas** | 阿特拉斯 | Framework orchestrator |
| Built-in | **Aether** | 以太 | Essential, omnipresent components |
| Extension | **Pillar** | 立柱 | Optional lifecycle components |
| Toolkit | **Artifact** | 神器 | Standalone utilities |

All four names are rooted in Greek mythology, forming a cohesive thematic system.

### Target Structure

```
go-atlas/
│
│  # Atlas — 阿特拉斯 (package atlas)
├── atlas.go              # Atlas struct, New(), Run(), Route(), MustRun()
├── config.go             # Config loading (Viper)
├── core.go               # Core struct (Pillar init context)
├── health.go             # Health check routes (/healthz, /livez, /readyz)
├── lifecycle.go          # Signal handling, graceful shutdown
├── middleware.go          # Middleware chain (recovery, requestID, CORS, rate limit, logging)
├── option.go             # Option functions (WithConfigName, WithLogger, etc.)
├── pillar.go             # Pillar interface, registry, Use[T], TryUse[T]
├── server.go             # Internal HTTP server (Gin wrapper)
│
│  # Aether — 以太 (essential components)
├── aether/
│   ├── errors/           # Error types, codes, sentinels (Is/As/Unwrap re-exports)
│   ├── i18n/             # Bundle, context locale, middleware, default messages
│   ├── log/              # Logger interface, Field, global logger, context extractors, slog default impl
│   └── response/         # OK(), Err(), Fail(), FailT(), AbortErr()
│
│  # Pillar — 立柱 (optional lifecycle components)
├── pillar/
│   ├── auth/             # JWT authentication + middleware
│   ├── cache/            # Redis cache + distributed lock
│   ├── database/         # GORM database + session management
│   ├── httpclient/       # HTTP client wrapper
│   ├── oauth/            # OAuth provider
│   ├── serviceclient/    # Inter-service RPC client
│   ├── sms/              # SMS (Tencent Cloud)
│   ├── storage/          # Object storage (S3/COS/OSS/TOS)
│   └── tracing/          # OpenTelemetry tracing
│
│  # Artifact — 神器 (standalone toolkit)
├── artifact/
│   ├── crypto/           # AES encryption, password hashing
│   ├── id/               # Snowflake, UUID, NanoID generation
│   ├── jsonutil/         # JSON helpers
│   ├── pagination/       # Pagination structs and helpers
│   └── validate/         # Gin binding + i18n validation
│
├── example/
│   ├── main.go
│   └── config.yaml
├── docs/
├── go.mod
├── go.sum
├── LICENSE
├── README.md
└── README_CN.md
```

### Import Path Changes

| Before | After |
|--------|-------|
| `github.com/shiliu-ai/go-atlas/atlas` | `github.com/shiliu-ai/go-atlas` |
| `github.com/shiliu-ai/go-atlas/atlas/errors` | `github.com/shiliu-ai/go-atlas/aether/errors` |
| `github.com/shiliu-ai/go-atlas/atlas/log` | `github.com/shiliu-ai/go-atlas/aether/log` |
| `github.com/shiliu-ai/go-atlas/atlas/i18n` | `github.com/shiliu-ai/go-atlas/aether/i18n` |
| `github.com/shiliu-ai/go-atlas/atlas/response` | `github.com/shiliu-ai/go-atlas/aether/response` |
| `github.com/shiliu-ai/go-atlas/pillar/*` | No change |
| `github.com/shiliu-ai/go-atlas/artifact/*` | No change |

### User-Facing API

```go
import (
    "github.com/shiliu-ai/go-atlas"
    "github.com/shiliu-ai/go-atlas/aether/errors"
    "github.com/shiliu-ai/go-atlas/aether/log"
    "github.com/shiliu-ai/go-atlas/aether/response"
    "github.com/shiliu-ai/go-atlas/pillar/database"
    "github.com/shiliu-ai/go-atlas/pillar/cache"
    "github.com/shiliu-ai/go-atlas/artifact/crypto"
)

func main() {
    app := atlas.New("myapp",
        atlas.WithConfigPaths(".", "./config"),
        database.Pillar(),
        cache.Pillar(),
    )

    app.Route(func(r *gin.RouterGroup) {
        r.GET("/users/:id", GetUser)
    })

    app.MustRun()
}

func GetUser(c *gin.Context) {
    user, err := userService.Get(c.Param("id"))
    if err != nil {
        response.Err(c, err)
        return
    }
    response.OK(c, user)
}
```

## Detailed Changes

### 1. Promote atlas core to root package

Move all `.go` files from `atlas/` to the project root. Change `package atlas` declaration — this already matches since the package name stays `atlas` (Go uses the `package` declaration, not the directory name, so `import "github.com/shiliu-ai/go-atlas"` will use the identifier `atlas`).

**Files to move (atlas/ → root):**
- `atlas.go`
- `config.go`
- `core.go`
- `health.go`
- `lifecycle.go`
- `middleware.go`
- `option.go`
- `pillar.go`
- `server.go`

**Test files to move:**
- `pillar_test.go`
- `integration_test.go` (package `atlas_test` — stays as-is)

### 2. Move sub-packages into aether/

Move four sub-packages from `atlas/` into the new `aether/` directory:

- `atlas/errors/` → `aether/errors/`
- `atlas/log/` → `aether/log/`
- `atlas/i18n/` → `aether/i18n/`
- `atlas/response/` → `aether/response/`

No code changes within these packages other than updating their internal imports (where they reference each other).

### 3. Update all import paths

Global search-and-replace across all files:

| Search | Replace |
|--------|---------|
| `"github.com/shiliu-ai/go-atlas/atlas/errors"` | `"github.com/shiliu-ai/go-atlas/aether/errors"` |
| `"github.com/shiliu-ai/go-atlas/atlas/log"` | `"github.com/shiliu-ai/go-atlas/aether/log"` |
| `"github.com/shiliu-ai/go-atlas/atlas/i18n"` | `"github.com/shiliu-ai/go-atlas/aether/i18n"` |
| `"github.com/shiliu-ai/go-atlas/atlas/response"` | `"github.com/shiliu-ai/go-atlas/aether/response"` |
| `"github.com/shiliu-ai/go-atlas/atlas"` | `"github.com/shiliu-ai/go-atlas"` |

**Files that need import updates:**

Root package (formerly `atlas/`):
- `atlas.go` — imports `aether/i18n`, `aether/log`
- `core.go` — imports `aether/log`
- `lifecycle.go` — imports `aether/log`
- `middleware.go` — imports `aether/errors`, `aether/i18n`, `aether/log`, `aether/response`
- `option.go` — imports `aether/log`

Aether packages (internal cross-references):
- `aether/response/response.go` — imports `aether/errors`, `aether/i18n`, `aether/log`
- `aether/response/response_test.go` — imports `aether/errors`

Pillar packages (that reference atlas or aether packages):
- All `pillar/*/pillar.go` files — change `atlas` import to root module path
- Any pillar files that import `atlas/log`, `atlas/errors`, etc. → `aether/log`, `aether/errors`

Example:
- `example/main.go` — update all atlas/aether imports

Integration test:
- `integration_test.go` — update `atlas` import to root module path

### 4. Delete old atlas/ directory

After all files are moved and imports are updated, remove the now-empty `atlas/` directory.

### 5. Update README files

Update `README.md` and `README_CN.md` to reflect:
- New four-domain architecture description
- Updated import paths in all code examples
- Updated directory structure diagrams

## Constraints

- **No API changes**: All public types, functions, and interfaces remain identical. Only import paths change.
- **No behavior changes**: All functionality, middleware ordering, lifecycle management, etc. remain the same.
- **Package name stays `atlas`**: The root package declares `package atlas`, matching the current package name. User code still writes `atlas.New(...)`, `atlas.Use[T](...)`, etc.
- **pillar/ and artifact/ untouched structurally**: Only import paths within their files need updating.

## Testing

- All existing tests must pass after the migration (`go test ./...`)
- The `example/` app must compile and run correctly
- Verify import paths resolve correctly with `go build ./...`

## Risk

- **Low risk**: This is purely a file move + import path rewrite. No logic, API, or behavior changes.
- **Breaking change for external consumers**: Anyone importing `go-atlas/atlas` will need to update imports. Since this is pre-1.0, this is acceptable.
