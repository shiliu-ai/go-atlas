# Four-Domain Architecture Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure go-atlas from 3-directory layout to a 4-domain architecture (Atlas/Aether/Pillar/Artifact) with core promoted to root package.

**Architecture:** Move `atlas/*.go` to project root (still `package atlas`), move `atlas/{errors,log,i18n,response}` into new `aether/` directory, update all import paths. No API or behavior changes — purely structural.

**Tech Stack:** Go 1.25, Gin, Viper, slog, GORM, OpenTelemetry

**Spec:** `docs/superpowers/specs/2026-03-31-four-domain-architecture-redesign.md`

---

## File Map

### Files to move (atlas/ → root)
- `atlas/atlas.go` → `atlas.go`
- `atlas/config.go` → `config.go`
- `atlas/core.go` → `core.go`
- `atlas/health.go` → `health.go`
- `atlas/lifecycle.go` → `lifecycle.go`
- `atlas/middleware.go` → `middleware.go`
- `atlas/option.go` → `option.go`
- `atlas/pillar.go` → `pillar.go`
- `atlas/server.go` → `server.go`
- `atlas/pillar_test.go` → `pillar_test.go`
- `atlas/integration_test.go` → `integration_test.go`

### Directories to move (atlas/sub → aether/sub)
- `atlas/errors/` → `aether/errors/`
- `atlas/log/` → `aether/log/`
- `atlas/i18n/` → `aether/i18n/`
- `atlas/response/` → `aether/response/`

### Files needing import updates (after move)

**Root package (formerly atlas/):**
- `atlas.go` — `atlas/i18n` → `aether/i18n`, `atlas/log` → `aether/log`
- `core.go` — `atlas/log` → `aether/log`
- `lifecycle.go` — `atlas/log` → `aether/log`
- `middleware.go` — `atlas/errors` → `aether/errors`, `atlas/i18n` → `aether/i18n`, `atlas/log` → `aether/log`, `atlas/response` → `aether/response`
- `option.go` — `atlas/log` → `aether/log`
- `pillar_test.go` — `atlas/log` → `aether/log`
- `integration_test.go` — `go-atlas/atlas` → `go-atlas`

**Aether packages:**
- `aether/response/response.go` — `atlas/errors` → `aether/errors`, `atlas/i18n` → `aether/i18n`, `atlas/log` → `aether/log`
- `aether/response/response_test.go` — `atlas/errors` → `aether/errors`

**Pillar packages:**
- `pillar/auth/pillar.go` — `go-atlas/atlas` → `go-atlas`
- `pillar/auth/middleware.go` — `atlas/errors` → `aether/errors`, `atlas/response` → `aether/response`
- `pillar/cache/pillar.go` — `go-atlas/atlas` → `go-atlas`
- `pillar/database/pillar.go` — `go-atlas/atlas` → `go-atlas`
- `pillar/database/database.go` — `atlas/log` → `aether/log`
- `pillar/database/logger.go` — `atlas/log` → `aether/log`
- `pillar/httpclient/pillar.go` — `go-atlas/atlas` → `go-atlas`
- `pillar/httpclient/client.go` — `atlas/log` → `aether/log`
- `pillar/oauth/pillar.go` — `go-atlas/atlas` → `go-atlas`
- `pillar/serviceclient/pillar.go` — `go-atlas/atlas` → `go-atlas`, `atlas/log` → `aether/log`
- `pillar/serviceclient/manager.go` — `atlas/log` → `aether/log`
- `pillar/serviceclient/client.go` — `atlas/errors` → `aether/errors`, `atlas/log` → `aether/log`
- `pillar/sms/pillar.go` — `go-atlas/atlas` → `go-atlas`
- `pillar/storage/pillar.go` — `go-atlas/atlas` → `go-atlas`
- `pillar/tracing/pillar.go` — `go-atlas/atlas` → `go-atlas`

**Artifact packages:**
- `artifact/validate/validate.go` — `atlas/errors` → `aether/errors`, `atlas/i18n` → `aether/i18n`, `atlas/response` → `aether/response`

**Example:**
- `example/main.go` — `go-atlas/atlas` → `go-atlas`, `atlas/errors` → `aether/errors`, `atlas/response` → `aether/response`

**Docs:**
- `README.md` — update import paths in code examples
- `README_CN.md` — update import paths in code examples

---

### Task 1: Create aether/ directory and move sub-packages

- [ ] **Step 1: Create aether/ directory and move the four sub-packages**

```bash
mkdir -p aether
mv atlas/errors aether/errors
mv atlas/log aether/log
mv atlas/i18n aether/i18n
mv atlas/response aether/response
```

- [ ] **Step 2: Verify directory structure**

```bash
ls aether/
```

Expected: `errors  i18n  log  response`

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "refactor: move atlas sub-packages into aether/ directory

Move errors/, log/, i18n/, response/ from atlas/ to aether/ as the
first step of the four-domain architecture redesign."
```

---

### Task 2: Move atlas core files to project root

- [ ] **Step 1: Move all .go source and test files from atlas/ to root**

```bash
mv atlas/atlas.go atlas.go
mv atlas/config.go config.go
mv atlas/core.go core.go
mv atlas/health.go health.go
mv atlas/lifecycle.go lifecycle.go
mv atlas/middleware.go middleware.go
mv atlas/option.go option.go
mv atlas/pillar.go pillar.go
mv atlas/server.go server.go
mv atlas/pillar_test.go pillar_test.go
mv atlas/integration_test.go integration_test.go
```

- [ ] **Step 2: Remove the now-empty atlas/ directory**

```bash
rmdir atlas
```

If `rmdir` fails because atlas/ is not empty, check what's left and remove it:
```bash
ls atlas/
rm -rf atlas/
```

- [ ] **Step 3: Verify files are at root**

```bash
ls *.go
```

Expected: `atlas.go  config.go  core.go  health.go  integration_test.go  lifecycle.go  middleware.go  option.go  pillar.go  pillar_test.go  server.go`

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: promote atlas core to root package

Move all atlas/*.go files to the project root, eliminating the
redundant go-atlas/atlas import path."
```

---

### Task 3: Update import paths in root package files

These files were moved from `atlas/` to root. Their internal imports reference `atlas/log`, `atlas/i18n`, etc. which must become `aether/log`, `aether/i18n`, etc.

- [ ] **Step 1: Update atlas.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/i18n"
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/i18n"
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 2: Update core.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 3: Update lifecycle.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 4: Update middleware.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/errors"
"github.com/shiliu-ai/go-atlas/atlas/i18n"
"github.com/shiliu-ai/go-atlas/atlas/log"
"github.com/shiliu-ai/go-atlas/atlas/response"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/errors"
"github.com/shiliu-ai/go-atlas/aether/i18n"
"github.com/shiliu-ai/go-atlas/aether/log"
"github.com/shiliu-ai/go-atlas/aether/response"
```

- [ ] **Step 5: Update option.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 6: Update pillar_test.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 7: Update integration_test.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
```
With:
```go
"github.com/shiliu-ai/go-atlas"
```

Note: The test package declaration `package atlas_test` stays unchanged — it refers to the package name, not the import path.

- [ ] **Step 8: Commit**

```bash
git add atlas.go core.go lifecycle.go middleware.go option.go pillar_test.go integration_test.go
git commit -m "refactor: update import paths in root package files

Update all atlas/* imports to aether/* and atlas import to root module."
```

---

### Task 4: Update import paths in aether packages

- [ ] **Step 1: Update aether/response/response.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/errors"
"github.com/shiliu-ai/go-atlas/atlas/i18n"
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/errors"
"github.com/shiliu-ai/go-atlas/aether/i18n"
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 2: Update aether/response/response_test.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/errors"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/errors"
```

- [ ] **Step 3: Commit**

```bash
git add aether/response/response.go aether/response/response_test.go
git commit -m "refactor: update internal imports in aether/response"
```

---

### Task 5: Update import paths in pillar packages

- [ ] **Step 1: Update pillar/auth/pillar.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
```

Note: Since the module path `go-atlas` doesn't match the package name `atlas`, we use a named import alias. This applies to ALL pillar files that import the atlas core package.

- [ ] **Step 2: Update pillar/auth/middleware.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/errors"
"github.com/shiliu-ai/go-atlas/atlas/response"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/errors"
"github.com/shiliu-ai/go-atlas/aether/response"
```

- [ ] **Step 3: Update pillar/cache/pillar.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
```

- [ ] **Step 4: Update pillar/database/pillar.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
```

- [ ] **Step 5: Update pillar/database/database.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 6: Update pillar/database/logger.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 7: Update pillar/httpclient/pillar.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
```

- [ ] **Step 8: Update pillar/httpclient/client.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 9: Update pillar/oauth/pillar.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
```

- [ ] **Step 10: Update pillar/serviceclient/pillar.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 11: Update pillar/serviceclient/manager.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 12: Update pillar/serviceclient/client.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/errors"
"github.com/shiliu-ai/go-atlas/atlas/log"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/errors"
"github.com/shiliu-ai/go-atlas/aether/log"
```

- [ ] **Step 13: Update pillar/sms/pillar.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
```

- [ ] **Step 14: Update pillar/storage/pillar.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
```

- [ ] **Step 15: Update pillar/tracing/pillar.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
```

- [ ] **Step 16: Commit**

```bash
git add pillar/
git commit -m "refactor: update import paths in all pillar packages"
```

---

### Task 6: Update import paths in artifact and example

- [ ] **Step 1: Update artifact/validate/validate.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas/errors"
"github.com/shiliu-ai/go-atlas/atlas/i18n"
"github.com/shiliu-ai/go-atlas/atlas/response"
```
With:
```go
"github.com/shiliu-ai/go-atlas/aether/errors"
"github.com/shiliu-ai/go-atlas/aether/i18n"
"github.com/shiliu-ai/go-atlas/aether/response"
```

- [ ] **Step 2: Update example/main.go**

Replace:
```go
"github.com/shiliu-ai/go-atlas/atlas"
"github.com/shiliu-ai/go-atlas/atlas/errors"
"github.com/shiliu-ai/go-atlas/atlas/response"
```
With:
```go
atlas "github.com/shiliu-ai/go-atlas"
"github.com/shiliu-ai/go-atlas/aether/errors"
"github.com/shiliu-ai/go-atlas/aether/response"
```

- [ ] **Step 3: Commit**

```bash
git add artifact/validate/validate.go example/main.go
git commit -m "refactor: update import paths in artifact and example"
```

---

### Task 7: Build and test

- [ ] **Step 1: Run go build to verify all imports resolve**

```bash
go build ./...
```

Expected: No errors.

- [ ] **Step 2: Run all tests**

```bash
go test ./...
```

Expected: All tests pass.

- [ ] **Step 3: If there are build errors, fix the broken import paths**

Common issues:
- Missing named import alias `atlas "github.com/shiliu-ai/go-atlas"` where the package name doesn't match the path
- Stale imports that weren't caught in the grep

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: resolve remaining import path issues after restructure"
```

---

### Task 8: Update README files

- [ ] **Step 1: Update README.md import paths**

Replace all occurrences:
- `"github.com/shiliu-ai/go-atlas/atlas"` → `atlas "github.com/shiliu-ai/go-atlas"`
- `"github.com/shiliu-ai/go-atlas/atlas/errors"` → `"github.com/shiliu-ai/go-atlas/aether/errors"`
- `"github.com/shiliu-ai/go-atlas/atlas/response"` → `"github.com/shiliu-ai/go-atlas/aether/response"`
- `"github.com/shiliu-ai/go-atlas/atlas/log"` → `"github.com/shiliu-ai/go-atlas/aether/log"`
- `"github.com/shiliu-ai/go-atlas/atlas/i18n"` → `"github.com/shiliu-ai/go-atlas/aether/i18n"`

Also update any directory structure diagrams and architecture descriptions to reflect the four-domain model (Atlas/Aether/Pillar/Artifact).

- [ ] **Step 2: Update README_CN.md with the same changes**

Apply identical import path replacements. Update the architecture description to mention the four domains with their Chinese names (阿特拉斯/以太/立柱/神器).

- [ ] **Step 3: Commit**

```bash
git add README.md README_CN.md
git commit -m "docs: update READMEs for four-domain architecture"
```

---

### Task 9: Final verification and cleanup

- [ ] **Step 1: Verify no stale references remain**

```bash
grep -r "go-atlas/atlas" --include="*.go" .
```

Expected: No matches (only docs/ files may still reference old paths in historical context).

- [ ] **Step 2: Verify final directory structure**

```bash
ls -la *.go
ls aether/
ls pillar/
ls artifact/
```

Expected:
- Root has ~11 .go files
- `aether/` has 4 subdirs: `errors/`, `i18n/`, `log/`, `response/`
- `pillar/` and `artifact/` unchanged structurally

- [ ] **Step 3: Run full build and test one final time**

```bash
go build ./...
go test ./...
```

Expected: All pass.

- [ ] **Step 4: Final commit if any cleanup was needed**

```bash
git add -A
git commit -m "refactor: complete four-domain architecture migration

Atlas (阿特拉斯) — framework core at root package
Aether (以太) — built-in essential components
Pillar (立柱) — optional lifecycle components
Artifact (神器) — standalone toolkit"
```
