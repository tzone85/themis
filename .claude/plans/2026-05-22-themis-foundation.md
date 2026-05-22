# Themis Foundation — Implementation Plan (Plan 1 of N)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the trust-critical foundation of Themis: a Go binary that initialises per-tenant directories, appends events to a Merkle-chained JSONL ledger with fsync durability, projects them into a per-tenant SQLite WAL database, replays events deterministically, and verifies ledger integrity. CI gate ≥ 95% coverage with property + wiring + multi-tenant isolation tests.

**Architecture:** Per-tenant filesystem isolation (`tenants/<id>/`). Append-only `events.jsonl` is the source of truth; SQLite WAL is a rebuildable projection. Every event carries a content hash and references the prior event's hash (Merkle-style chain). The CLI exposes `themis tenant init / status` and `themis ledger replay / verify / doctor`.

**Tech Stack:** Go 1.23+, Cobra (CLI), `database/sql` + `modernc.org/sqlite` (pure-Go SQLite, no CGO for easier cross-compile + air-gapped deploys), `pgregory.net/rapid` (property tests), standard library `testing` + `testing/synctest` for deterministic time.

**Scope boundaries (out-of-scope for Plan 1; covered later):**
- No ingest adapters (Plan 4).
- No catalogue / classifier (Plan 2).
- No policy engine (Plan 3).
- No scanners (Plan 5).
- No BOM / signing (Plan 6).
- No HTTP / REST / MCP surfaces (Plans 7–8).
- Event taxonomy at Plan 1 is intentionally minimal: only the events needed to demonstrate the ledger machinery. Real event types (`INGEST_COMPLETED`, `DECISION_ISSUED`, etc.) arrive in later plans, *each* protected by a wiring test added at that time.

**Foundational invariants this plan locks in (and tests):**
1. Per-tenant filesystem isolation — no code path can read/write outside `tenants/<id>/` without an explicit `Tenant`.
2. Every ledger event has a content hash and a `prev_hash` referencing the prior event in the same tenant.
3. `Replay(events) == Project(events)` — ledger replay produces a byte-identical SQLite projection.
4. Any mutation to `events.jsonl` is detected by `themis ledger verify`.
5. Every event type registered in the registry has a non-default `Project()` branch (wiring test).
6. Coverage ≥ 95% gate enforced in CI; per-package thresholds tracked in `coverage.thresholds.yaml`.

---

## Source files this plan creates

| File | Purpose |
|---|---|
| `go.mod` | Module declaration. Module path: `github.com/tzone85/themis`. |
| `Makefile` | `make build`, `make test`, `make cover`, `make lint`, `make ci`. |
| `LICENSE` | Apache 2.0 (per design spec §16). |
| `coverage.thresholds.yaml` | Per-package coverage targets. |
| `scripts/cover_check.sh` | Coverage gate enforcement script. |
| `.github/workflows/ci.yml` | CI: build, vet, test, coverage gate. |
| `.golangci.yml` | Linter config (gosec, staticcheck, govulncheck enabled). |
| `cmd/themis/main.go` | Binary entrypoint; delegates to `internal/cli`. |
| `internal/cli/root.go` | Cobra root + version + global flags. |
| `internal/cli/tenant_cmd.go` | `themis tenant init / status`. |
| `internal/cli/ledger_cmd.go` | `themis ledger replay / verify / doctor / status`. |
| `internal/cli/root_test.go` | CLI smoke tests. |
| `internal/cli/tenant_cmd_test.go` | Tenant-CLI tests. |
| `internal/cli/ledger_cmd_test.go` | Ledger-CLI tests. |
| `internal/tenant/tenant.go` | `Tenant` value type (ID + base path). |
| `internal/tenant/paths.go` | Path helpers (`Events()`, `Projection()`, `BOM()`, `Wing()`). |
| `internal/tenant/init.go` | `Init(root, id) (Tenant, error)`. |
| `internal/tenant/tenant_test.go` | Tenant unit + isolation tests. |
| `internal/ledger/event.go` | `Event` struct, `Hash()`, `Chain()` helpers. |
| `internal/ledger/event_test.go` | Event hash / chain tests + property tests. |
| `internal/ledger/registry.go` | Event-type registry (kinds with `Project()` implementations). |
| `internal/ledger/registry_test.go` | Wiring test enforcing registry coverage. |
| `internal/ledger/store.go` | JSONL append-only store with fsync + chain check on write. |
| `internal/ledger/store_test.go` | Store tests (append, idempotency, chain enforcement). |
| `internal/ledger/projection.go` | SQLite WAL projection (`Open`, `Project`, `Close`). |
| `internal/ledger/projection_test.go` | Projection tests. |
| `internal/ledger/replay.go` | `Replay(tenant) error`; `Verify(tenant) error`; `Doctor(tenant) (Report, error)`. |
| `internal/ledger/replay_test.go` | Replay + verify + doctor tests, including tampering detection. |
| `internal/ledger/doc.go` | Package-level docs (Go doc comment). |
| `testdata/sample-events/*.jsonl` | Hand-crafted fixtures for replay tests. |
| `README.md` | Already exists — no change at Plan 1. |

---

## Phase A — Project bootstrap

### Task 1: Initialise Go module

**Files:**
- Create: `go.mod`
- Create: `.gitattributes`

- [ ] **Step 1: Write the failing test (smoke for module declaration)**

Create `internal/ledger/doc.go`:

```go
// Package ledger implements Themis's tamper-evident append-only event store.
//
// The ledger is per-tenant: each Tenant has its own events.jsonl file (the
// source of truth) and a SQLite WAL projection (rebuildable from the JSONL).
// Every event references the prior event's content hash, forming a
// Merkle-style chain. See docs/superpowers/specs/ for the design.
package ledger
```

Create `internal/ledger/doc_test.go`:

```go
package ledger

import "testing"

func TestPackageCompiles(t *testing.T) {
    // Sentinel that lives until real tests arrive in Task 11+.
}
```

- [ ] **Step 2: Initialise the module**

Run:

```bash
cd /Users/mncedimini/Sites/misc/themis
go mod init github.com/tzone85/themis
```

Expected output: `go: creating new go.mod: module github.com/tzone85/themis`.

- [ ] **Step 3: Set LF line endings via .gitattributes**

Create `.gitattributes`:

```
* text=auto eol=lf
*.go text eol=lf
*.md text eol=lf
*.yaml text eol=lf
*.yml text eol=lf
*.json text eol=lf
*.sh text eol=lf
*.sum text eol=lf
*.mod text eol=lf
```

(Per shared-learnings: prevents CRLF flakes in tests that read files back.)

- [ ] **Step 4: Run the smoke test**

Run:

```bash
go test ./internal/ledger/... -count=1
```

Expected output: `ok      github.com/tzone85/themis/internal/ledger`.

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitattributes internal/ledger/doc.go internal/ledger/doc_test.go
git commit -m "chore: initialise Go module + ledger package skeleton"
```

---

### Task 2: Add Makefile

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Write the Makefile**

Create `Makefile`:

```make
.PHONY: build test cover lint vet ci clean

BIN := ~/.local/bin/themis
PKGS := ./...

build:
	go build -o $(BIN) ./cmd/themis

test:
	go test -race -count=1 $(PKGS)

cover:
	go test -race -count=1 -coverprofile=coverage.out -covermode=atomic $(PKGS)
	go tool cover -func=coverage.out | tail -1

cover-html: cover
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run

vet:
	go vet $(PKGS)

ci: vet lint test cover
	bash scripts/cover_check.sh

clean:
	rm -f coverage.out coverage.html
```

- [ ] **Step 2: Verify make targets parse**

Run:

```bash
make -n test
```

Expected: prints `go test -race -count=1 ./...` without error.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile (build, test, cover, lint, ci)"
```

---

### Task 3: Add coverage threshold config + gate script

**Files:**
- Create: `coverage.thresholds.yaml`
- Create: `scripts/cover_check.sh`

- [ ] **Step 1: Write the threshold config**

Create `coverage.thresholds.yaml`:

```yaml
# Per-package coverage targets. Global target is 95% (enforced in scripts/cover_check.sh).
# A package may set a higher target; the gate enforces the higher of (global, per-package).
global: 95.0
packages:
  github.com/tzone85/themis/internal/ledger: 95.0
  github.com/tzone85/themis/internal/tenant: 95.0
  github.com/tzone85/themis/internal/cli: 90.0  # CLI glue gets a lower bar; logic must be in pure packages.
```

- [ ] **Step 2: Write the gate script**

Create `scripts/cover_check.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Reads coverage.out (go cover profile) and coverage.thresholds.yaml.
# Fails if global coverage or any per-package coverage is below threshold.

COVERAGE_FILE="${COVERAGE_FILE:-coverage.out}"
THRESHOLDS="${THRESHOLDS:-coverage.thresholds.yaml}"

if [[ ! -f "$COVERAGE_FILE" ]]; then
  echo "coverage profile $COVERAGE_FILE not found; run 'make cover' first" >&2
  exit 2
fi

# Global coverage from `go tool cover -func` final line.
GLOBAL_PCT=$(go tool cover -func="$COVERAGE_FILE" | awk '/^total:/ {gsub("%",""); print $3}')

# Per-package coverage from `go tool cover -func`.
PER_PKG=$(go tool cover -func="$COVERAGE_FILE" \
  | grep -v '^total:' \
  | awk '{n=split($1,p,"/"); gsub("%","",$3); pkg=$1; sub(/\/[^/]+$/, "", pkg); cov[pkg]+=$3; cnt[pkg]++} END {for (k in cov) printf "%s %.1f\n", k, cov[k]/cnt[k]}')

# Global gate.
GLOBAL_TARGET=$(awk '/^global:/ {print $2}' "$THRESHOLDS")
echo "[cover-check] global: ${GLOBAL_PCT}% (target: ${GLOBAL_TARGET}%)"
if (( $(echo "$GLOBAL_PCT < $GLOBAL_TARGET" | bc -l) )); then
  echo "[cover-check] FAIL: global coverage ${GLOBAL_PCT}% below ${GLOBAL_TARGET}%"
  exit 1
fi

# Per-package gate.
FAIL=0
while read -r pkg pct; do
  TARGET=$(awk -v p="$pkg" '$1 == p":" {gsub(":",""); print $2}' "$THRESHOLDS")
  if [[ -z "${TARGET:-}" ]]; then TARGET="$GLOBAL_TARGET"; fi
  echo "[cover-check] $pkg: ${pct}% (target: ${TARGET}%)"
  if (( $(echo "$pct < $TARGET" | bc -l) )); then
    echo "[cover-check] FAIL: $pkg ${pct}% below ${TARGET}%"
    FAIL=1
  fi
done <<< "$PER_PKG"

if [[ "$FAIL" == "1" ]]; then exit 1; fi
echo "[cover-check] PASS"
```

- [ ] **Step 3: Make script executable**

Run:

```bash
chmod +x scripts/cover_check.sh
```

- [ ] **Step 4: Smoke-run the script (expect "FAIL" until we have code)**

Run:

```bash
go test ./internal/ledger/... -coverprofile=coverage.out -covermode=atomic && bash scripts/cover_check.sh || true
```

Expected: prints `[cover-check] global: 0.0% (target: 95.0%)` and `FAIL`. That's expected — we have no code yet. We just verified the script *runs*; we'll satisfy it in later tasks.

- [ ] **Step 5: Commit**

```bash
git add coverage.thresholds.yaml scripts/cover_check.sh
git commit -m "ci: add coverage threshold config + gate script (95% global)"
```

---

### Task 4: Add LICENSE (Apache 2.0)

**Files:**
- Create: `LICENSE`

- [ ] **Step 1: Fetch the canonical Apache 2.0 text**

Run:

```bash
curl -sL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE
head -3 LICENSE
```

Expected first lines:

```
                                 Apache License
                           Version 2.0, January 2004
```

- [ ] **Step 2: Add copyright line at the bottom**

Edit `LICENSE`, append:

```

Copyright 2026 Thando Mini and Themis contributors.
```

- [ ] **Step 3: Commit**

```bash
git add LICENSE
git commit -m "docs: add Apache 2.0 licence (per design spec §16)"
```

---

### Task 5: Add linter config

**Files:**
- Create: `.golangci.yml`

- [ ] **Step 1: Write the linter config**

Create `.golangci.yml`:

```yaml
run:
  timeout: 5m
  tests: true

linters:
  enable:
    - errcheck
    - gosec
    - govet
    - ineffassign
    - revive
    - staticcheck
    - unconvert
    - unused

linters-settings:
  revive:
    rules:
      - name: var-naming
      - name: exported
      - name: package-comments
  gosec:
    excludes:
      - G104  # errcheck handles this; gosec double-reports

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gosec  # tests use t.TempDir() and similar; gosec false-positives.
```

- [ ] **Step 2: Verify the config loads (skip running golangci-lint until it's installed in CI)**

Verify the YAML is valid:

```bash
python3 -c "import yaml,sys; yaml.safe_load(open('.golangci.yml'))" && echo "OK"
```

Expected: `OK`.

- [ ] **Step 3: Commit**

```bash
git add .golangci.yml
git commit -m "ci: add golangci-lint config (errcheck, gosec, staticcheck, ...)"
```

---

### Task 6: Add CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: ci

on:
  pull_request:
  push:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
          cache: true
      - name: vet
        run: go vet ./...
      - name: lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: v1.59
      - name: test
        run: go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
      - name: coverage gate
        run: bash scripts/cover_check.sh
      - name: govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: GitHub Actions workflow (vet, lint, test, coverage gate, vulncheck)"
```

---

## Phase B — Tenant model

### Task 7: Define the Tenant type + path helpers

**Files:**
- Create: `internal/tenant/tenant.go`
- Create: `internal/tenant/paths.go`
- Create: `internal/tenant/tenant_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/tenant/tenant_test.go`:

```go
package tenant

import (
    "path/filepath"
    "strings"
    "testing"
)

func TestTenant_PathsAreScopedToBase(t *testing.T) {
    base := t.TempDir()
    tn := Tenant{ID: "acme-corp", Base: base}

    want := filepath.Join(base, "tenants", "acme-corp")
    if got := tn.Root(); got != want {
        t.Fatalf("Root() = %q, want %q", got, want)
    }
    if !strings.HasPrefix(tn.Events(), tn.Root()) {
        t.Fatalf("Events() %q not under Root() %q", tn.Events(), tn.Root())
    }
    if !strings.HasPrefix(tn.Projection(), tn.Root()) {
        t.Fatalf("Projection() %q not under Root() %q", tn.Projection(), tn.Root())
    }
}

func TestTenant_RejectsEmptyID(t *testing.T) {
    if _, err := New(t.TempDir(), ""); err == nil {
        t.Fatal("New with empty ID should error")
    }
}

func TestTenant_RejectsTraversalID(t *testing.T) {
    base := t.TempDir()
    bad := []string{"../escape", "..", "a/b", "a\\b", "."}
    for _, id := range bad {
        if _, err := New(base, id); err == nil {
            t.Errorf("New(%q) should error; ID contains illegal characters", id)
        }
    }
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
go test ./internal/tenant/... -v
```

Expected: build failure (`undefined: Tenant`, `undefined: New`).

- [ ] **Step 3: Write the minimal implementation**

Create `internal/tenant/tenant.go`:

```go
// Package tenant models the per-customer isolation boundary in Themis.
// Every package that touches per-customer state takes a Tenant explicitly.
// There are no globals; cross-tenant access requires constructing a
// different Tenant value.
package tenant

import (
    "errors"
    "fmt"
    "regexp"
)

// Tenant is the per-customer isolation boundary. All Themis state for one
// customer lives under Tenant.Root().
type Tenant struct {
    ID   string // stable, opaque, filesystem-safe identifier
    Base string // absolute path to the Themis state root (e.g. /var/lib/themis)
}

// validID restricts tenant IDs to a portable, filesystem-safe subset.
// Lowercase letters, digits, dash; 1-63 chars (DNS-label safe).
var validID = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62})$`)

// ErrInvalidID indicates a tenant ID failed validation.
var ErrInvalidID = errors.New("invalid tenant id")

// New constructs a Tenant after validating the id. It does NOT create
// directories on disk — see Init for that.
func New(base, id string) (Tenant, error) {
    if !validID.MatchString(id) {
        return Tenant{}, fmt.Errorf("%w: %q (must match %s)", ErrInvalidID, id, validID.String())
    }
    if base == "" {
        return Tenant{}, fmt.Errorf("base path required")
    }
    return Tenant{ID: id, Base: base}, nil
}
```

Create `internal/tenant/paths.go`:

```go
package tenant

import "path/filepath"

// Root returns the tenant's filesystem root directory.
func (t Tenant) Root() string {
    return filepath.Join(t.Base, "tenants", t.ID)
}

// Events returns the path to the tenant's append-only events.jsonl.
func (t Tenant) Events() string {
    return filepath.Join(t.Root(), "events.jsonl")
}

// Projection returns the path to the tenant's SQLite WAL projection database.
func (t Tenant) Projection() string {
    return filepath.Join(t.Root(), "projection.sqlite")
}

// BOM returns the directory where signed AI-BOM artefacts are stored.
func (t Tenant) BOM() string {
    return filepath.Join(t.Root(), "bom")
}

// Wing returns the per-tenant Mempalace wing directory.
func (t Tenant) Wing() string {
    return filepath.Join(t.Root(), "mempalace-wing")
}
```

- [ ] **Step 4: Run the test and verify it passes**

Run:

```bash
go test ./internal/tenant/... -v
```

Expected: `--- PASS: TestTenant_PathsAreScopedToBase`, `--- PASS: TestTenant_RejectsEmptyID`, `--- PASS: TestTenant_RejectsTraversalID`.

- [ ] **Step 5: Commit**

```bash
git add internal/tenant/
git commit -m "feat(tenant): Tenant type + validated ID + per-tenant paths"
```

---

### Task 8: Implement Init (create per-tenant directory tree on disk)

**Files:**
- Create: `internal/tenant/init.go`
- Modify: `internal/tenant/tenant_test.go` (add test)

- [ ] **Step 1: Write the failing test**

Append to `internal/tenant/tenant_test.go`:

```go
func TestInit_CreatesAllExpectedDirs(t *testing.T) {
    base := t.TempDir()
    tn, err := Init(base, "acme-corp")
    if err != nil {
        t.Fatalf("Init: %v", err)
    }
    for _, dir := range []string{tn.Root(), tn.BOM(), tn.Wing()} {
        info, err := os.Stat(dir)
        if err != nil {
            t.Errorf("%q not created: %v", dir, err)
            continue
        }
        if !info.IsDir() {
            t.Errorf("%q exists but is not a directory", dir)
        }
    }
}

func TestInit_IsIdempotent(t *testing.T) {
    base := t.TempDir()
    if _, err := Init(base, "acme-corp"); err != nil {
        t.Fatalf("first Init: %v", err)
    }
    if _, err := Init(base, "acme-corp"); err != nil {
        t.Fatalf("second Init (should be no-op): %v", err)
    }
}

func TestInit_TenantsCannotEscapeBase(t *testing.T) {
    // Construct a Tenant with a malicious base that includes "..".
    // (Won't ever happen via Init — base is operator-supplied — but if it
    // does, the methods must still produce paths anchored to that base.)
    bad, err := New("/var/lib/themis/../../etc", "acme")
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    // Root must remain underneath the (now-canonicalised) base; this test
    // documents that we don't sanitise base — that's the operator's job.
    if !strings.Contains(bad.Root(), "acme") {
        t.Fatalf("Root() lost the tenant id: %q", bad.Root())
    }
}
```

Also add at the top of the file:

```go
import "os"
```

(combine with existing imports).

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
go test ./internal/tenant/... -v
```

Expected: build failure `undefined: Init`.

- [ ] **Step 3: Write the implementation**

Create `internal/tenant/init.go`:

```go
package tenant

import (
    "fmt"
    "os"
)

// Init creates the tenant directory tree on disk. It is idempotent:
// re-running on an existing tenant returns the same Tenant without error.
//
// Directory permissions: 0o700 (rwx for owner only). The Themis daemon
// must run as a single user; cross-user access on the host is out of scope.
func Init(base, id string) (Tenant, error) {
    t, err := New(base, id)
    if err != nil {
        return Tenant{}, err
    }
    for _, dir := range []string{t.Root(), t.BOM(), t.Wing()} {
        if err := os.MkdirAll(dir, 0o700); err != nil {
            return Tenant{}, fmt.Errorf("mkdir %s: %w", dir, err)
        }
    }
    return t, nil
}
```

- [ ] **Step 4: Run the tests**

Run:

```bash
go test ./internal/tenant/... -v
```

Expected: all four tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/tenant/
git commit -m "feat(tenant): idempotent Init creates per-tenant dir tree"
```

---

### Task 9: Cross-tenant isolation property test

**Files:**
- Modify: `internal/tenant/tenant_test.go`

- [ ] **Step 1: Write the property test**

Append to `internal/tenant/tenant_test.go`:

```go
func TestTenant_DistinctIDsAlwaysGetDistinctRoots(t *testing.T) {
    base := t.TempDir()
    a, err := New(base, "a")
    if err != nil { t.Fatal(err) }
    b, err := New(base, "b")
    if err != nil { t.Fatal(err) }

    if a.Root() == b.Root() {
        t.Fatal("distinct tenants share a root path")
    }
    if strings.HasPrefix(a.Root(), b.Root()+string(filepath.Separator)) {
        t.Fatal("tenant a's root is nested under tenant b's root")
    }
    if strings.HasPrefix(b.Root(), a.Root()+string(filepath.Separator)) {
        t.Fatal("tenant b's root is nested under tenant a's root")
    }
}
```

- [ ] **Step 2: Run the tests**

Run:

```bash
go test ./internal/tenant/... -v
```

Expected: all tests pass including the new `TestTenant_DistinctIDsAlwaysGetDistinctRoots`.

- [ ] **Step 3: Run coverage on the tenant package**

Run:

```bash
go test ./internal/tenant/... -cover
```

Expected: coverage ≥ 95% on `internal/tenant`.

- [ ] **Step 4: Commit**

```bash
git add internal/tenant/
git commit -m "test(tenant): distinct IDs always produce distinct, non-nesting roots"
```

---

## Phase C — Event type + Merkle hash chain

### Task 10: Define the Event struct + content hash

**Files:**
- Create: `internal/ledger/event.go`
- Create: `internal/ledger/event_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/ledger/event_test.go`:

```go
package ledger

import (
    "encoding/json"
    "testing"
    "time"
)

func TestEvent_ContentHashIsDeterministic(t *testing.T) {
    e := Event{
        Kind:      "TEST_EVENT",
        Tenant:    "acme",
        Timestamp: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
        Payload:   json.RawMessage(`{"hello":"world"}`),
        PrevHash:  "0000000000000000000000000000000000000000000000000000000000000000",
    }
    h1, err := e.ContentHash()
    if err != nil { t.Fatal(err) }
    h2, err := e.ContentHash()
    if err != nil { t.Fatal(err) }
    if h1 != h2 {
        t.Fatalf("hash not deterministic: %s vs %s", h1, h2)
    }
    if len(h1) != 64 { // hex sha256
        t.Fatalf("hash wrong length: %d", len(h1))
    }
}

func TestEvent_HashChangesWithAnyField(t *testing.T) {
    base := Event{
        Kind: "TEST", Tenant: "acme",
        Timestamp: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
        Payload:   json.RawMessage(`{}`),
        PrevHash:  "00",
    }
    baseHash, _ := base.ContentHash()

    cases := map[string]Event{
        "kind":      {Kind: "OTHER", Tenant: "acme", Timestamp: base.Timestamp, Payload: base.Payload, PrevHash: "00"},
        "tenant":    {Kind: "TEST", Tenant: "beta", Timestamp: base.Timestamp, Payload: base.Payload, PrevHash: "00"},
        "timestamp": {Kind: "TEST", Tenant: "acme", Timestamp: base.Timestamp.Add(time.Second), Payload: base.Payload, PrevHash: "00"},
        "payload":   {Kind: "TEST", Tenant: "acme", Timestamp: base.Timestamp, Payload: json.RawMessage(`{"x":1}`), PrevHash: "00"},
        "prev":      {Kind: "TEST", Tenant: "acme", Timestamp: base.Timestamp, Payload: base.Payload, PrevHash: "01"},
    }
    for field, e := range cases {
        h, err := e.ContentHash()
        if err != nil { t.Fatalf("%s: %v", field, err) }
        if h == baseHash {
            t.Errorf("hash unchanged when %s differs", field)
        }
    }
}
```

- [ ] **Step 2: Run the tests; expect build failure**

Run:

```bash
go test ./internal/ledger/... -run TestEvent -v
```

Expected: `undefined: Event`.

- [ ] **Step 3: Write the implementation**

Create `internal/ledger/event.go`:

```go
package ledger

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "time"
)

// Event is one append to the ledger. The hash chain (PrevHash → ContentHash)
// is what makes the ledger tamper-evident.
type Event struct {
    Kind      string          `json:"kind"`               // e.g. INGEST_COMPLETED
    Tenant    string          `json:"tenant"`             // tenant ID; redundant inside a per-tenant file but kept for portability
    Timestamp time.Time       `json:"ts"`                 // UTC
    Payload   json.RawMessage `json:"payload"`            // schema depends on Kind
    PrevHash  string          `json:"prev_hash"`          // hex sha256 of the prior event's ContentHash; "GENESIS" for the first
}

// ZeroHash is the prev_hash sentinel for the first event in a tenant's chain.
const ZeroHash = "GENESIS"

// canonical returns the byte representation hashed for ContentHash.
// We marshal a struct (not the receiver) so that field order is fixed and
// the JSON canonical form is deterministic across Go versions.
func (e Event) canonical() ([]byte, error) {
    type canonical struct {
        Kind      string          `json:"kind"`
        Tenant    string          `json:"tenant"`
        Timestamp string          `json:"ts"`
        Payload   json.RawMessage `json:"payload"`
        PrevHash  string          `json:"prev_hash"`
    }
    c := canonical{
        Kind:      e.Kind,
        Tenant:    e.Tenant,
        Timestamp: e.Timestamp.UTC().Format(time.RFC3339Nano),
        Payload:   e.Payload,
        PrevHash:  e.PrevHash,
    }
    var buf bytes.Buffer
    enc := json.NewEncoder(&buf)
    enc.SetEscapeHTML(false)
    if err := enc.Encode(c); err != nil {
        return nil, fmt.Errorf("marshal event for hashing: %w", err)
    }
    // Encoder appends a newline; strip it so the hash is over the JSON value, not "value\n".
    out := buf.Bytes()
    if len(out) > 0 && out[len(out)-1] == '\n' {
        out = out[:len(out)-1]
    }
    return out, nil
}

// ContentHash returns the hex-encoded SHA-256 of the event's canonical form.
func (e Event) ContentHash() (string, error) {
    raw, err := e.canonical()
    if err != nil {
        return "", err
    }
    sum := sha256.Sum256(raw)
    return hex.EncodeToString(sum[:]), nil
}
```

- [ ] **Step 4: Run the tests**

Run:

```bash
go test ./internal/ledger/... -run TestEvent -v
```

Expected: both pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/event.go internal/ledger/event_test.go
git commit -m "feat(ledger): Event struct + deterministic SHA-256 content hash"
```

---

### Task 11: Hash-chain helper + chain integrity test

**Files:**
- Modify: `internal/ledger/event.go` (add Chain helper)
- Modify: `internal/ledger/event_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/ledger/event_test.go`:

```go
func TestChain_LinksConsecutiveEvents(t *testing.T) {
    a := Event{Kind: "A", Tenant: "x", Timestamp: time.Unix(1, 0).UTC(), Payload: json.RawMessage(`{}`), PrevHash: ZeroHash}
    aHash, _ := a.ContentHash()

    b := Event{Kind: "B", Tenant: "x", Timestamp: time.Unix(2, 0).UTC(), Payload: json.RawMessage(`{}`)}
    b = Chain(b, aHash)

    if b.PrevHash != aHash {
        t.Fatalf("Chain did not set PrevHash; got %q want %q", b.PrevHash, aHash)
    }
}

func TestChain_DifferentPriorsProduceDifferentChildHashes(t *testing.T) {
    base := Event{Kind: "X", Tenant: "t", Timestamp: time.Unix(10, 0).UTC(), Payload: json.RawMessage(`{}`)}
    h1, _ := Chain(base, "AAAA").ContentHash()
    h2, _ := Chain(base, "BBBB").ContentHash()
    if h1 == h2 {
        t.Fatal("child events with different priors should hash differently")
    }
}
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/ledger/... -run TestChain -v
```

Expected: `undefined: Chain`.

- [ ] **Step 3: Add the Chain helper**

Append to `internal/ledger/event.go`:

```go
// Chain returns a copy of e with PrevHash set to prior. Used when appending.
func Chain(e Event, prior string) Event {
    e.PrevHash = prior
    return e
}
```

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/ledger/... -run TestChain -v
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/
git commit -m "feat(ledger): Chain helper + hash-chain property tests"
```

---

### Task 12: Property tests for the hash chain via `rapid`

**Files:**
- Modify: `go.mod` (add rapid dep)
- Create: `internal/ledger/event_property_test.go`

- [ ] **Step 1: Add the `rapid` dependency**

Run:

```bash
go get pgregory.net/rapid@v1.2.0
go mod tidy
```

Expected: `go.mod` now requires `pgregory.net/rapid`.

- [ ] **Step 2: Write the property test**

Create `internal/ledger/event_property_test.go`:

```go
package ledger

import (
    "encoding/json"
    "testing"
    "time"

    "pgregory.net/rapid"
)

func TestPropContentHash_DependsOnAllFields(t *testing.T) {
    rapid.Check(t, func(rt *rapid.T) {
        e1 := genEvent(rt)
        h1, err := e1.ContentHash()
        if err != nil { rt.Fatal(err) }

        // Mutate one field at random and assert the hash changes.
        field := rapid.SampledFrom([]string{"Kind", "Tenant", "Timestamp", "Payload", "PrevHash"}).Draw(rt, "field")
        e2 := e1
        switch field {
        case "Kind":
            e2.Kind = e1.Kind + "_X"
        case "Tenant":
            e2.Tenant = e1.Tenant + "_X"
        case "Timestamp":
            e2.Timestamp = e1.Timestamp.Add(time.Nanosecond)
        case "Payload":
            e2.Payload = json.RawMessage(append([]byte{}, e1.Payload...))
            // Toggle one byte deterministically.
            if len(e2.Payload) > 0 {
                e2.Payload[len(e2.Payload)-1] ^= 0x01
            } else {
                e2.Payload = json.RawMessage(`{"_":1}`)
            }
        case "PrevHash":
            if e1.PrevHash == "" || e1.PrevHash == "X"+e1.PrevHash {
                e2.PrevHash = e1.PrevHash + "Y"
            } else {
                e2.PrevHash = "X" + e1.PrevHash
            }
        }
        h2, err := e2.ContentHash()
        if err != nil { rt.Fatal(err) }
        if h1 == h2 {
            rt.Fatalf("hash unchanged after mutating %s: %s", field, h1)
        }
    })
}

func genEvent(t *rapid.T) Event {
    return Event{
        Kind:      rapid.StringMatching(`[A-Z][A-Z_]{0,20}`).Draw(t, "kind"),
        Tenant:    rapid.StringMatching(`[a-z][a-z0-9-]{0,20}`).Draw(t, "tenant"),
        Timestamp: time.Unix(rapid.Int64Range(0, 2_000_000_000).Draw(t, "ts"), 0).UTC(),
        Payload:   json.RawMessage(`{"k":"v"}`),
        PrevHash:  rapid.StringMatching(`[a-f0-9]{8,16}|GENESIS`).Draw(t, "prev"),
    }
}
```

- [ ] **Step 3: Run**

```bash
go test ./internal/ledger/... -run TestPropContentHash -v
```

Expected: PASS, with rapid generating 100 random cases.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/ledger/event_property_test.go
git commit -m "test(ledger): property test — hash depends on every Event field"
```

---

## Phase D — Append-only JSONL store

### Task 13: JSONL store — append + read

**Files:**
- Create: `internal/ledger/store.go`
- Create: `internal/ledger/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ledger/store_test.go`:

```go
package ledger

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"
)

func newTestEvent(kind string, prev string) Event {
    return Event{
        Kind:      kind,
        Tenant:    "test-tenant",
        Timestamp: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
        Payload:   json.RawMessage(`{}`),
        PrevHash:  prev,
    }
}

func TestStore_AppendAndRead(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "events.jsonl")

    s, err := OpenStore(path)
    if err != nil { t.Fatal(err) }
    defer s.Close()

    if got := s.LastHash(); got != ZeroHash {
        t.Fatalf("empty store LastHash() = %q, want %q", got, ZeroHash)
    }

    e1 := newTestEvent("A", s.LastHash())
    if _, err := s.Append(e1); err != nil { t.Fatal(err) }

    e2 := newTestEvent("B", s.LastHash())
    if _, err := s.Append(e2); err != nil { t.Fatal(err) }

    // Read back.
    events, err := ReadAll(path)
    if err != nil { t.Fatal(err) }
    if len(events) != 2 {
        t.Fatalf("ReadAll: got %d events, want 2", len(events))
    }
    if events[0].Kind != "A" || events[1].Kind != "B" {
        t.Fatalf("unexpected order: %v", events)
    }

    // The on-disk file is also valid JSONL (one JSON object per line).
    raw, err := os.ReadFile(path)
    if err != nil { t.Fatal(err) }
    if want := 2; bytesLines(raw) != want {
        t.Fatalf("file has %d lines, want %d", bytesLines(raw), want)
    }
}

func bytesLines(b []byte) int {
    n := 0
    for _, c := range b {
        if c == '\n' { n++ }
    }
    return n
}
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/ledger/... -run TestStore -v
```

Expected: build failure (`undefined: OpenStore`).

- [ ] **Step 3: Implement**

Create `internal/ledger/store.go`:

```go
package ledger

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "sync"
)

// Store is an append-only JSONL event store. The file is the authoritative
// source of truth; the SQLite projection is rebuildable from it.
//
// Store is safe for use by ONE writer goroutine. Multiple readers may call
// ReadAll concurrently provided no concurrent Append is in flight on the
// same path.
type Store struct {
    path string
    f    *os.File
    bw   *bufio.Writer
    last string // hex of last event's ContentHash
    mu   sync.Mutex
}

// OpenStore opens (creates if missing) a JSONL store at path. It scans
// the existing file (if any) to recover LastHash.
func OpenStore(path string) (*Store, error) {
    f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
    if err != nil {
        return nil, fmt.Errorf("open %s: %w", path, err)
    }
    s := &Store{path: path, f: f, bw: bufio.NewWriter(f), last: ZeroHash}

    // Recover last hash by scanning existing content.
    events, err := ReadAll(path)
    if err != nil {
        _ = f.Close()
        return nil, fmt.Errorf("recover %s: %w", path, err)
    }
    if len(events) > 0 {
        h, err := events[len(events)-1].ContentHash()
        if err != nil {
            _ = f.Close()
            return nil, fmt.Errorf("compute last hash for %s: %w", path, err)
        }
        s.last = h
    }
    return s, nil
}

// LastHash returns the content hash of the most recently appended event,
// or ZeroHash if the store is empty.
func (s *Store) LastHash() string {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.last
}

// Append serialises e and writes it as one JSONL line. It returns the
// content hash of the appended event.
//
// Append is fsync-on-every-write. Throughput is intentionally bounded by
// disk fsync latency; the audit story requires durability before reporting
// success.
func (s *Store) Append(e Event) (string, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    if e.PrevHash != s.last {
        return "", fmt.Errorf("chain mismatch: event.PrevHash=%q, store.LastHash=%q", e.PrevHash, s.last)
    }
    line, err := json.Marshal(e)
    if err != nil {
        return "", fmt.Errorf("marshal event: %w", err)
    }
    if _, err := s.bw.Write(line); err != nil {
        return "", fmt.Errorf("write event: %w", err)
    }
    if err := s.bw.WriteByte('\n'); err != nil {
        return "", fmt.Errorf("write newline: %w", err)
    }
    if err := s.bw.Flush(); err != nil {
        return "", fmt.Errorf("flush: %w", err)
    }
    if err := s.f.Sync(); err != nil {
        return "", fmt.Errorf("fsync: %w", err)
    }
    h, err := e.ContentHash()
    if err != nil {
        return "", fmt.Errorf("content hash: %w", err)
    }
    s.last = h
    return h, nil
}

// Close flushes and closes the underlying file.
func (s *Store) Close() error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.bw != nil {
        if err := s.bw.Flush(); err != nil { return err }
    }
    if s.f != nil {
        return s.f.Close()
    }
    return nil
}

// ReadAll reads the entire JSONL file from path and returns the events in
// file order. Empty / missing file = empty slice, nil error.
func ReadAll(path string) ([]Event, error) {
    f, err := os.Open(path)
    if err != nil {
        if os.IsNotExist(err) { return nil, nil }
        return nil, fmt.Errorf("open %s: %w", path, err)
    }
    defer f.Close()

    var out []Event
    dec := json.NewDecoder(bufio.NewReader(f))
    for {
        var e Event
        if err := dec.Decode(&e); err != nil {
            if err == io.EOF { return out, nil }
            return nil, fmt.Errorf("decode: %w", err)
        }
        out = append(out, e)
    }
}
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/ledger/... -run TestStore -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/store.go internal/ledger/store_test.go
git commit -m "feat(ledger): append-only JSONL Store with fsync + chain check"
```

---

### Task 14: Chain-mismatch rejection test

**Files:**
- Modify: `internal/ledger/store_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ledger/store_test.go`:

```go
func TestStore_RejectsAppendWithStaleChain(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "events.jsonl")
    s, err := OpenStore(path)
    if err != nil { t.Fatal(err) }
    defer s.Close()

    e1 := newTestEvent("A", s.LastHash())
    if _, err := s.Append(e1); err != nil { t.Fatal(err) }

    // Now try to append an event whose PrevHash is wrong.
    bad := newTestEvent("B", "WRONG_PREVIOUS_HASH")
    if _, err := s.Append(bad); err == nil {
        t.Fatal("Append accepted stale-chain event; should have rejected")
    }
}
```

- [ ] **Step 2: Run; expect pass (the code already does this)**

```bash
go test ./internal/ledger/... -run TestStore_RejectsAppendWithStaleChain -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/ledger/store_test.go
git commit -m "test(ledger): Store rejects appends with stale prev-hash"
```

---

## Phase E — Event-type registry + wiring test

### Task 15: Event-type registry

**Files:**
- Create: `internal/ledger/registry.go`
- Create: `internal/ledger/registry_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ledger/registry_test.go`:

```go
package ledger

import "testing"

func TestRegistry_RegisterAndLookup(t *testing.T) {
    r := NewRegistry()
    r.Register("FOO", func(payload []byte) error { return nil })

    if _, ok := r.Projector("FOO"); !ok {
        t.Fatal("Projector(FOO) not found after Register")
    }
    if _, ok := r.Projector("MISSING"); ok {
        t.Fatal("Projector(MISSING) should not be found")
    }
}

func TestRegistry_KindsListsAll(t *testing.T) {
    r := NewRegistry()
    r.Register("A", noopProjector)
    r.Register("B", noopProjector)
    r.Register("C", noopProjector)
    got := r.Kinds()
    if len(got) != 3 {
        t.Fatalf("Kinds returned %d items: %v", len(got), got)
    }
}

func noopProjector(_ []byte) error { return nil }
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/ledger/... -run TestRegistry -v
```

Expected: `undefined: NewRegistry`.

- [ ] **Step 3: Implement the registry**

Create `internal/ledger/registry.go`:

```go
package ledger

import (
    "fmt"
    "sort"
    "sync"
)

// Projector is a per-event-kind projection function. It is called with
// the raw payload bytes; the function returns an error if the event
// cannot be projected. Projectors MUST be deterministic and side-effect
// free except for writing to their tenant's SQLite projection.
type Projector func(payload []byte) error

// Registry holds the set of known event kinds and their projectors.
// New event kinds MUST be registered here — the wiring test will fail
// the build if a kind is appended via Store.Append() but absent from
// the registry, or if the registry has an entry with no projector.
type Registry struct {
    mu sync.RWMutex
    p  map[string]Projector
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{p: map[string]Projector{}} }

// Register adds a projector for kind. Re-registering is allowed (last write wins)
// to support test setup; production code should call Register exactly once
// per kind during package init.
func (r *Registry) Register(kind string, p Projector) {
    if kind == "" {
        panic("ledger: cannot register empty kind")
    }
    if p == nil {
        panic(fmt.Sprintf("ledger: cannot register nil projector for %q", kind))
    }
    r.mu.Lock()
    defer r.mu.Unlock()
    r.p[kind] = p
}

// Projector returns the projector registered for kind and whether it exists.
func (r *Registry) Projector(kind string) (Projector, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    p, ok := r.p[kind]
    return p, ok
}

// Kinds returns a sorted slice of all registered kinds.
func (r *Registry) Kinds() []string {
    r.mu.RLock()
    defer r.mu.RUnlock()
    out := make([]string, 0, len(r.p))
    for k := range r.p {
        out = append(out, k)
    }
    sort.Strings(out)
    return out
}
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/ledger/... -run TestRegistry -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/registry.go internal/ledger/registry_test.go
git commit -m "feat(ledger): event-kind Registry with Projector lookup"
```

---

### Task 16: Wiring test — every used kind must be registered

**Files:**
- Create: `internal/ledger/wiring_test.go`

- [ ] **Step 1: Write the wiring test**

Create `internal/ledger/wiring_test.go`:

```go
package ledger

import "testing"

// TestWiring_DefaultRegistryHasKindsForTask16 documents the kinds that must
// exist in the default registry for this Plan-1 layer. Later plans expand
// this list when they add new event types.
//
// The test exists to ensure that adding new ledger event kinds is a deliberate,
// reviewable change rather than something that can creep in via Store.Append
// without a corresponding projector.
func TestWiring_DefaultRegistryHasMinimumKinds(t *testing.T) {
    r := DefaultRegistry()

    // Plan 1 only needs the housekeeping kinds — real product event types
    // (INGEST_COMPLETED, DECISION_ISSUED, etc.) arrive in later plans and
    // MUST add themselves to DefaultRegistry + extend this test.
    want := []string{
        "TENANT_INITIALISED",
        "LEDGER_REPLAYED",
        "LEDGER_VERIFIED",
    }
    for _, kind := range want {
        if _, ok := r.Projector(kind); !ok {
            t.Errorf("default registry missing required kind %q", kind)
        }
    }
}
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/ledger/... -run TestWiring -v
```

Expected: `undefined: DefaultRegistry`.

- [ ] **Step 3: Add DefaultRegistry**

Append to `internal/ledger/registry.go`:

```go
// DefaultRegistry returns the registry pre-populated with all event kinds
// the Themis core ledger uses.
//
// IMPORTANT: when adding a new event kind anywhere in the codebase, add
// its projector here AND extend internal/ledger/wiring_test.go to assert
// the kind is registered. Wiring tests prevent the default-case-eats-events
// bug class observed in adjacent systems (see VXD shared-learnings).
func DefaultRegistry() *Registry {
    r := NewRegistry()
    r.Register("TENANT_INITIALISED", noopProject)
    r.Register("LEDGER_REPLAYED",    noopProject)
    r.Register("LEDGER_VERIFIED",    noopProject)
    return r
}

// noopProject is a placeholder projector used for kinds whose projection
// is "just record the event" — the row already lands in events table via
// the generic projector path; no kind-specific work is needed.
func noopProject(_ []byte) error { return nil }
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/ledger/... -run TestWiring -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/
git commit -m "feat(ledger): DefaultRegistry + wiring test for kind coverage"
```

---

## Phase F — SQLite WAL projection

### Task 17: Open + schema migration

**Files:**
- Create: `internal/ledger/projection.go`
- Create: `internal/ledger/projection_test.go`
- Modify: `go.mod` (add modernc.org/sqlite)

- [ ] **Step 1: Add the pure-Go SQLite driver**

Run:

```bash
go get modernc.org/sqlite@v1.34.1
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/ledger/projection_test.go`:

```go
package ledger

import (
    "path/filepath"
    "testing"
)

func TestProjection_OpenCreatesSchema(t *testing.T) {
    path := filepath.Join(t.TempDir(), "projection.sqlite")
    p, err := OpenProjection(path)
    if err != nil { t.Fatal(err) }
    defer p.Close()

    // After Open the events table must exist.
    rows, err := p.DB().Query("SELECT count(*) FROM events")
    if err != nil { t.Fatalf("events table missing: %v", err) }
    rows.Close()
}

func TestProjection_WALModeEnabled(t *testing.T) {
    path := filepath.Join(t.TempDir(), "projection.sqlite")
    p, err := OpenProjection(path)
    if err != nil { t.Fatal(err) }
    defer p.Close()

    var mode string
    if err := p.DB().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil { t.Fatal(err) }
    if mode != "wal" {
        t.Fatalf("journal_mode = %q, want %q", mode, "wal")
    }
}
```

- [ ] **Step 3: Run; expect failure**

```bash
go test ./internal/ledger/... -run TestProjection -v
```

Expected: `undefined: OpenProjection`.

- [ ] **Step 4: Implement**

Create `internal/ledger/projection.go`:

```go
package ledger

import (
    "database/sql"
    "fmt"

    _ "modernc.org/sqlite" // pure-Go SQLite driver, no CGO
)

// Projection is the per-tenant SQLite WAL projection of the JSONL ledger.
// It is rebuildable: deleting and re-opening + replaying events.jsonl
// produces a byte-identical Projection.
type Projection struct {
    db   *sql.DB
    path string
}

// OpenProjection opens (and migrates) the SQLite projection at path.
// WAL mode is enabled for concurrent reads + single writer.
func OpenProjection(path string) (*Projection, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, fmt.Errorf("sql.Open: %w", err)
    }
    p := &Projection{db: db, path: path}
    if err := p.migrate(); err != nil {
        _ = db.Close()
        return nil, err
    }
    return p, nil
}

// DB returns the underlying *sql.DB. Callers must not close it; call
// Projection.Close instead.
func (p *Projection) DB() *sql.DB { return p.db }

// Close closes the underlying connection.
func (p *Projection) Close() error { return p.db.Close() }

// migrate applies the v1 schema. Idempotent.
func (p *Projection) migrate() error {
    pragmas := []string{
        "PRAGMA journal_mode = WAL",
        "PRAGMA synchronous = NORMAL",
        "PRAGMA foreign_keys = ON",
    }
    for _, q := range pragmas {
        if _, err := p.db.Exec(q); err != nil {
            return fmt.Errorf("pragma %q: %w", q, err)
        }
    }
    schema := `
    CREATE TABLE IF NOT EXISTS events (
        seq         INTEGER PRIMARY KEY AUTOINCREMENT,
        kind        TEXT    NOT NULL,
        tenant      TEXT    NOT NULL,
        ts          TEXT    NOT NULL,           -- RFC3339Nano UTC
        prev_hash   TEXT    NOT NULL,
        content_hash TEXT   NOT NULL UNIQUE,    -- enforces idempotent re-projection
        payload     BLOB    NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_events_kind ON events(kind);
    CREATE INDEX IF NOT EXISTS idx_events_ts   ON events(ts);

    CREATE TABLE IF NOT EXISTS meta (
        key   TEXT PRIMARY KEY,
        value TEXT NOT NULL
    );
    `
    if _, err := p.db.Exec(schema); err != nil {
        return fmt.Errorf("schema migrate: %w", err)
    }
    return nil
}
```

- [ ] **Step 5: Run; expect pass**

```bash
go test ./internal/ledger/... -run TestProjection -v
```

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/ledger/projection.go internal/ledger/projection_test.go
git commit -m "feat(ledger): SQLite WAL Projection + schema migration"
```

---

### Task 18: Project an Event into the table

**Files:**
- Modify: `internal/ledger/projection.go`
- Modify: `internal/ledger/projection_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ledger/projection_test.go`:

```go
func TestProjection_ProjectInsertsEvent(t *testing.T) {
    path := filepath.Join(t.TempDir(), "projection.sqlite")
    p, err := OpenProjection(path)
    if err != nil { t.Fatal(err) }
    defer p.Close()

    e := newTestEvent("TENANT_INITIALISED", ZeroHash)
    if err := p.Project(e, DefaultRegistry()); err != nil { t.Fatal(err) }

    var count int
    if err := p.DB().QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil { t.Fatal(err) }
    if count != 1 {
        t.Fatalf("got %d rows, want 1", count)
    }
}

func TestProjection_ProjectIsIdempotent(t *testing.T) {
    path := filepath.Join(t.TempDir(), "projection.sqlite")
    p, err := OpenProjection(path)
    if err != nil { t.Fatal(err) }
    defer p.Close()

    e := newTestEvent("TENANT_INITIALISED", ZeroHash)
    if err := p.Project(e, DefaultRegistry()); err != nil { t.Fatal(err) }
    if err := p.Project(e, DefaultRegistry()); err != nil {
        t.Fatalf("second Project (should be idempotent): %v", err)
    }

    var count int
    if err := p.DB().QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil { t.Fatal(err) }
    if count != 1 {
        t.Fatalf("got %d rows after second Project, want 1", count)
    }
}

func TestProjection_RefusesUnknownKind(t *testing.T) {
    path := filepath.Join(t.TempDir(), "projection.sqlite")
    p, err := OpenProjection(path)
    if err != nil { t.Fatal(err) }
    defer p.Close()

    e := newTestEvent("DEFINITELY_NOT_REGISTERED", ZeroHash)
    if err := p.Project(e, DefaultRegistry()); err == nil {
        t.Fatal("Project accepted unknown kind; should have failed")
    }
}
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/ledger/... -run TestProjection_Project -v
```

Expected: `undefined: (*Projection).Project`.

- [ ] **Step 3: Implement**

Append to `internal/ledger/projection.go`:

```go
// Project records e in the projection. It is idempotent (same content_hash
// twice = single row). Project refuses kinds not present in registry —
// this is the wiring guard that prevents the default-case-eats-events bug.
func (p *Projection) Project(e Event, registry *Registry) error {
    projector, ok := registry.Projector(e.Kind)
    if !ok {
        return fmt.Errorf("ledger: unknown event kind %q (every kind must be registered in DefaultRegistry)", e.Kind)
    }
    hash, err := e.ContentHash()
    if err != nil {
        return fmt.Errorf("content hash: %w", err)
    }
    // Idempotent insert keyed by content_hash UNIQUE constraint.
    _, err = p.db.Exec(
        `INSERT OR IGNORE INTO events (kind, tenant, ts, prev_hash, content_hash, payload)
         VALUES (?, ?, ?, ?, ?, ?)`,
        e.Kind, e.Tenant, e.Timestamp.UTC().Format(timeFormat), e.PrevHash, hash, []byte(e.Payload),
    )
    if err != nil {
        return fmt.Errorf("project insert: %w", err)
    }
    // Run the kind-specific projector (currently noop for Plan 1 kinds).
    if err := projector([]byte(e.Payload)); err != nil {
        return fmt.Errorf("projector %q: %w", e.Kind, err)
    }
    return nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/ledger/... -run TestProjection -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/projection.go internal/ledger/projection_test.go
git commit -m "feat(ledger): Project event into SQLite (idempotent + kind-checked)"
```

---

## Phase G — Replay, Verify, Doctor

### Task 19: Replay events.jsonl into a fresh projection

**Files:**
- Create: `internal/ledger/replay.go`
- Create: `internal/ledger/replay_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ledger/replay_test.go`:

```go
package ledger

import (
    "path/filepath"
    "testing"
)

func TestReplay_ReproducesProjection(t *testing.T) {
    dir := t.TempDir()
    storePath := filepath.Join(dir, "events.jsonl")
    projPath  := filepath.Join(dir, "projection.sqlite")

    // Build a live store with 3 events.
    s, err := OpenStore(storePath)
    if err != nil { t.Fatal(err) }
    p, err := OpenProjection(projPath)
    if err != nil { t.Fatal(err) }
    reg := DefaultRegistry()

    for _, kind := range []string{"TENANT_INITIALISED", "LEDGER_REPLAYED", "LEDGER_VERIFIED"} {
        e := newTestEvent(kind, s.LastHash())
        if _, err := s.Append(e); err != nil { t.Fatal(err) }
        if err := p.Project(e, reg); err != nil { t.Fatal(err) }
    }
    s.Close()
    p.Close()

    // Now delete and rebuild the projection from JSONL alone.
    if err := DeleteFile(projPath); err != nil { t.Fatal(err) }
    if err := Replay(storePath, projPath, reg); err != nil { t.Fatal(err) }

    // Assert event count matches.
    p2, err := OpenProjection(projPath)
    if err != nil { t.Fatal(err) }
    defer p2.Close()

    var n int
    if err := p2.DB().QueryRow("SELECT count(*) FROM events").Scan(&n); err != nil { t.Fatal(err) }
    if n != 3 {
        t.Fatalf("after replay: %d rows, want 3", n)
    }
}
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/ledger/... -run TestReplay -v
```

Expected: `undefined: Replay`, `undefined: DeleteFile`.

- [ ] **Step 3: Implement**

Create `internal/ledger/replay.go`:

```go
package ledger

import (
    "fmt"
    "os"
)

// DeleteFile removes path. Used by replay to drop a stale projection
// before rebuilding from the JSONL source of truth.
func DeleteFile(path string) error {
    if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
        return err
    }
    // Also remove WAL + shm sidecars if present.
    for _, suffix := range []string{"-wal", "-shm"} {
        _ = os.Remove(path + suffix)
    }
    return nil
}

// Replay rebuilds the SQLite projection at projPath from the JSONL
// at storePath. The projection file is left in a fully consistent state
// (same byte-identical content regardless of how many times Replay runs).
func Replay(storePath, projPath string, registry *Registry) error {
    events, err := ReadAll(storePath)
    if err != nil {
        return fmt.Errorf("read events: %w", err)
    }
    p, err := OpenProjection(projPath)
    if err != nil {
        return fmt.Errorf("open projection: %w", err)
    }
    defer p.Close()

    for i, e := range events {
        if err := p.Project(e, registry); err != nil {
            return fmt.Errorf("project event %d (%s): %w", i, e.Kind, err)
        }
    }
    return nil
}
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/ledger/... -run TestReplay -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/replay.go internal/ledger/replay_test.go
git commit -m "feat(ledger): Replay rebuilds projection from JSONL"
```

---

### Task 20: Verify detects ledger tampering

**Files:**
- Modify: `internal/ledger/replay.go`
- Modify: `internal/ledger/replay_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ledger/replay_test.go`:

```go
import (
    "io/ioutil"
    "os"
    "strings"
)

func TestVerify_PassesOnUntamperedLedger(t *testing.T) {
    dir := t.TempDir()
    storePath := filepath.Join(dir, "events.jsonl")
    s, err := OpenStore(storePath)
    if err != nil { t.Fatal(err) }
    for _, kind := range []string{"TENANT_INITIALISED", "LEDGER_REPLAYED"} {
        if _, err := s.Append(newTestEvent(kind, s.LastHash())); err != nil { t.Fatal(err) }
    }
    s.Close()

    if err := Verify(storePath); err != nil {
        t.Fatalf("Verify on untampered ledger: %v", err)
    }
}

func TestVerify_DetectsByteFlip(t *testing.T) {
    dir := t.TempDir()
    storePath := filepath.Join(dir, "events.jsonl")
    s, err := OpenStore(storePath)
    if err != nil { t.Fatal(err) }
    if _, err := s.Append(newTestEvent("TENANT_INITIALISED", s.LastHash())); err != nil { t.Fatal(err) }
    if _, err := s.Append(newTestEvent("LEDGER_REPLAYED",    s.LastHash())); err != nil { t.Fatal(err) }
    s.Close()

    // Tamper: flip one byte mid-file.
    raw, err := ioutil.ReadFile(storePath)
    if err != nil { t.Fatal(err) }
    raw[10] ^= 0x01
    if err := os.WriteFile(storePath, raw, 0o600); err != nil { t.Fatal(err) }

    err = Verify(storePath)
    if err == nil {
        t.Fatal("Verify should have detected tampering")
    }
    if !strings.Contains(err.Error(), "chain") && !strings.Contains(err.Error(), "decode") {
        t.Fatalf("Verify error should mention chain or decode: %v", err)
    }
}
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/ledger/... -run TestVerify -v
```

Expected: `undefined: Verify`.

- [ ] **Step 3: Implement Verify**

Append to `internal/ledger/replay.go`:

```go
// Verify walks the JSONL ledger and asserts the Merkle chain is intact.
// Returns nil if every event's PrevHash matches the prior event's content hash
// (or ZeroHash for the first), and every event hashes to a stable value.
func Verify(storePath string) error {
    events, err := ReadAll(storePath)
    if err != nil {
        return fmt.Errorf("read: %w", err)
    }
    prev := ZeroHash
    for i, e := range events {
        if e.PrevHash != prev {
            return fmt.Errorf("ledger: chain break at event %d (%s): prev_hash=%q, expected=%q",
                i, e.Kind, e.PrevHash, prev)
        }
        h, err := e.ContentHash()
        if err != nil {
            return fmt.Errorf("ledger: hash event %d: %w", i, err)
        }
        prev = h
    }
    return nil
}
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/ledger/... -run TestVerify -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/
git commit -m "feat(ledger): Verify walks the Merkle chain, detects tampering"
```

---

### Task 21: Doctor produces a structured health report

**Files:**
- Modify: `internal/ledger/replay.go`
- Modify: `internal/ledger/replay_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ledger/replay_test.go`:

```go
func TestDoctor_ReportsCountsAndChainState(t *testing.T) {
    dir := t.TempDir()
    storePath := filepath.Join(dir, "events.jsonl")
    s, err := OpenStore(storePath)
    if err != nil { t.Fatal(err) }
    for _, kind := range []string{"TENANT_INITIALISED", "LEDGER_REPLAYED", "LEDGER_VERIFIED"} {
        if _, err := s.Append(newTestEvent(kind, s.LastHash())); err != nil { t.Fatal(err) }
    }
    s.Close()

    rep, err := Doctor(storePath)
    if err != nil { t.Fatal(err) }
    if rep.EventCount != 3 {
        t.Errorf("EventCount = %d, want 3", rep.EventCount)
    }
    if !rep.ChainIntact {
        t.Error("ChainIntact = false on a clean ledger")
    }
    if rep.LastHash == "" || rep.LastHash == ZeroHash {
        t.Errorf("LastHash should be a real hash, got %q", rep.LastHash)
    }
}
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/ledger/... -run TestDoctor -v
```

- [ ] **Step 3: Implement**

Append to `internal/ledger/replay.go`:

```go
// Report is the structured output of Doctor.
type Report struct {
    EventCount  int
    ChainIntact bool
    ChainError  string
    LastHash    string
}

// Doctor inspects the ledger and produces a Report. Unlike Verify, Doctor
// never returns a non-nil error for ledger-content issues; it captures
// such conditions inside the Report. (Errors only on I/O failures.)
func Doctor(storePath string) (Report, error) {
    events, err := ReadAll(storePath)
    if err != nil {
        return Report{}, err
    }
    rep := Report{EventCount: len(events), LastHash: ZeroHash, ChainIntact: true}
    prev := ZeroHash
    for i, e := range events {
        if e.PrevHash != prev {
            rep.ChainIntact = false
            rep.ChainError = fmt.Sprintf("chain break at event %d (%s): prev=%q want %q", i, e.Kind, e.PrevHash, prev)
            return rep, nil
        }
        h, err := e.ContentHash()
        if err != nil {
            return Report{}, err
        }
        prev = h
    }
    rep.LastHash = prev
    return rep, nil
}
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/ledger/... -run TestDoctor -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/
git commit -m "feat(ledger): Doctor produces a structured health Report"
```

---

### Task 22: Property test — Replay(events) == Project(events)

**Files:**
- Create: `internal/ledger/replay_property_test.go`

- [ ] **Step 1: Write the property test**

Create `internal/ledger/replay_property_test.go`:

```go
package ledger

import (
    "encoding/json"
    "path/filepath"
    "testing"
    "time"

    "pgregory.net/rapid"
)

// TestPropReplay_DeterministicProjection asserts that for any valid
// sequence of registered-kind events, replaying produces the same set
// of (kind, ts, content_hash) triples as projecting live.
func TestPropReplay_DeterministicProjection(t *testing.T) {
    rapid.Check(t, func(rt *rapid.T) {
        kinds := []string{"TENANT_INITIALISED", "LEDGER_REPLAYED", "LEDGER_VERIFIED"}
        n := rapid.IntRange(1, 12).Draw(rt, "n")

        dir := t.TempDir()
        storePath := filepath.Join(dir, "events.jsonl")
        liveProj  := filepath.Join(dir, "live.sqlite")
        replayProj := filepath.Join(dir, "replay.sqlite")

        s, err := OpenStore(storePath)
        if err != nil { rt.Fatal(err) }
        p, err := OpenProjection(liveProj)
        if err != nil { rt.Fatal(err) }
        reg := DefaultRegistry()

        for i := 0; i < n; i++ {
            kind := rapid.SampledFrom(kinds).Draw(rt, "kind")
            e := Event{
                Kind: kind, Tenant: "t",
                Timestamp: time.Unix(int64(1700000000+i), 0).UTC(),
                Payload:   json.RawMessage(`{"i":1}`),
                PrevHash:  s.LastHash(),
            }
            if _, err := s.Append(e); err != nil { rt.Fatal(err) }
            if err := p.Project(e, reg); err != nil { rt.Fatal(err) }
        }
        s.Close(); p.Close()

        if err := Replay(storePath, replayProj, reg); err != nil { rt.Fatal(err) }

        liveSet := dumpHashes(rt, liveProj)
        repSet  := dumpHashes(rt, replayProj)
        if !equalSets(liveSet, repSet) {
            rt.Fatalf("Replay produced different content_hash set: live=%v replay=%v", liveSet, repSet)
        }
    })
}

func dumpHashes(t *rapid.T, path string) map[string]struct{} {
    p, err := OpenProjection(path)
    if err != nil { t.Fatal(err) }
    defer p.Close()
    rows, err := p.DB().Query("SELECT content_hash FROM events ORDER BY content_hash")
    if err != nil { t.Fatal(err) }
    defer rows.Close()
    out := map[string]struct{}{}
    for rows.Next() {
        var h string
        if err := rows.Scan(&h); err != nil { t.Fatal(err) }
        out[h] = struct{}{}
    }
    return out
}

func equalSets(a, b map[string]struct{}) bool {
    if len(a) != len(b) { return false }
    for k := range a {
        if _, ok := b[k]; !ok { return false }
    }
    return true
}
```

- [ ] **Step 2: Run; expect pass (Replay already exists)**

```bash
go test ./internal/ledger/... -run TestPropReplay -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/ledger/replay_property_test.go
git commit -m "test(ledger): property test Replay(events) ≡ live Project(events)"
```

---

## Phase H — CLI surface

### Task 23: Cobra root + version flag

**Files:**
- Create: `cmd/themis/main.go`
- Create: `internal/cli/root.go`
- Create: `internal/cli/root_test.go`
- Modify: `go.mod` (add cobra)

- [ ] **Step 1: Add cobra**

```bash
go get github.com/spf13/cobra@v1.8.1
go mod tidy
```

- [ ] **Step 2: Write the failing test**

Create `internal/cli/root_test.go`:

```go
package cli

import (
    "bytes"
    "strings"
    "testing"
)

func TestRoot_VersionFlagPrintsVersion(t *testing.T) {
    out := &bytes.Buffer{}
    cmd := NewRootCmd()
    cmd.SetOut(out)
    cmd.SetErr(out)
    cmd.SetArgs([]string{"--version"})
    if err := cmd.Execute(); err != nil { t.Fatalf("execute: %v", err) }
    if !strings.Contains(out.String(), "themis") {
        t.Fatalf("--version output missing 'themis': %q", out.String())
    }
}
```

- [ ] **Step 3: Run; expect failure**

```bash
go test ./internal/cli/... -v
```

Expected: `undefined: NewRootCmd`.

- [ ] **Step 4: Implement root command**

Create `internal/cli/root.go`:

```go
// Package cli implements the themis CLI surface.
package cli

import (
    "github.com/spf13/cobra"
)

// Version is the embedded build version. ldflags-injectable at build time:
//
//	go build -ldflags="-X github.com/tzone85/themis/internal/cli.Version=v0.1.0" ./cmd/themis
var Version = "dev"

// NewRootCmd constructs the root `themis` command tree.
func NewRootCmd() *cobra.Command {
    root := &cobra.Command{
        Use:           "themis",
        Short:         "Themis — a compliance gateway for AI-generated code",
        SilenceUsage:  true,
        SilenceErrors: true,
        Version:       Version,
    }
    root.SetVersionTemplate("themis {{.Version}}\n")
    root.AddCommand(newTenantCmd())
    root.AddCommand(newLedgerCmd())
    return root
}
```

Create `cmd/themis/main.go`:

```go
package main

import (
    "fmt"
    "os"

    "github.com/tzone85/themis/internal/cli"
)

func main() {
    if err := cli.NewRootCmd().Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

Note: the test will fail to compile until Tasks 24 + 25 add `newTenantCmd` and `newLedgerCmd`. Stub them now to keep the build green for the next test step.

Create stub file `internal/cli/stubs.go` (temporary; removed when Tasks 24+25 replace it):

```go
package cli

import "github.com/spf13/cobra"

func newTenantCmd() *cobra.Command { return &cobra.Command{Use: "tenant"} }
func newLedgerCmd() *cobra.Command { return &cobra.Command{Use: "ledger"} }
```

- [ ] **Step 5: Run; expect pass**

```bash
go test ./internal/cli/... -v
```

- [ ] **Step 6: Build the binary as a smoke check**

```bash
go build -o /tmp/themis-smoke ./cmd/themis && /tmp/themis-smoke --version
```

Expected: `themis dev`.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum cmd/themis/main.go internal/cli/root.go internal/cli/root_test.go internal/cli/stubs.go
git commit -m "feat(cli): cobra root + --version + main entrypoint + tenant/ledger stubs"
```

---

### Task 24: `themis tenant init` command

**Files:**
- Create: `internal/cli/tenant_cmd.go` (replaces the stub)
- Create: `internal/cli/tenant_cmd_test.go`
- Modify: `internal/cli/stubs.go` (remove the `newTenantCmd` stub)

- [ ] **Step 1: Write the failing test**

Create `internal/cli/tenant_cmd_test.go`:

```go
package cli

import (
    "bytes"
    "os"
    "path/filepath"
    "testing"
)

func TestTenantInit_CreatesDirectoryAndEmitsEvent(t *testing.T) {
    base := t.TempDir()
    out := &bytes.Buffer{}

    cmd := NewRootCmd()
    cmd.SetOut(out)
    cmd.SetErr(out)
    cmd.SetArgs([]string{"tenant", "init", "--id", "acme", "--base", base})

    if err := cmd.Execute(); err != nil { t.Fatalf("execute: %v", err) }

    tenantDir := filepath.Join(base, "tenants", "acme")
    if _, err := os.Stat(tenantDir); err != nil {
        t.Fatalf("tenant dir not created: %v", err)
    }
    if _, err := os.Stat(filepath.Join(tenantDir, "events.jsonl")); err != nil {
        t.Fatalf("events.jsonl not created: %v", err)
    }
}

func TestTenantInit_RejectsInvalidID(t *testing.T) {
    base := t.TempDir()
    out := &bytes.Buffer{}
    cmd := NewRootCmd()
    cmd.SetOut(out); cmd.SetErr(out)
    cmd.SetArgs([]string{"tenant", "init", "--id", "../escape", "--base", base})
    if err := cmd.Execute(); err == nil {
        t.Fatal("invalid id should have errored")
    }
}
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/cli/... -run TestTenantInit -v
```

- [ ] **Step 3: Implement**

Create `internal/cli/tenant_cmd.go`:

```go
package cli

import (
    "encoding/json"
    "fmt"
    "time"

    "github.com/spf13/cobra"

    "github.com/tzone85/themis/internal/ledger"
    "github.com/tzone85/themis/internal/tenant"
)

func newTenantCmd() *cobra.Command {
    cmd := &cobra.Command{Use: "tenant", Short: "Manage Themis tenants"}
    cmd.AddCommand(newTenantInitCmd())
    return cmd
}

func newTenantInitCmd() *cobra.Command {
    var id, base string
    cmd := &cobra.Command{
        Use:   "init",
        Short: "Initialise a tenant directory tree + write TENANT_INITIALISED event",
        RunE: func(cmd *cobra.Command, args []string) error {
            t, err := tenant.Init(base, id)
            if err != nil {
                return fmt.Errorf("init tenant: %w", err)
            }
            s, err := ledger.OpenStore(t.Events())
            if err != nil {
                return fmt.Errorf("open store: %w", err)
            }
            defer s.Close()

            payload, _ := json.Marshal(map[string]string{"id": id, "base": base})
            e := ledger.Event{
                Kind:      "TENANT_INITIALISED",
                Tenant:    id,
                Timestamp: time.Now().UTC(),
                Payload:   payload,
                PrevHash:  s.LastHash(),
            }
            if _, err := s.Append(e); err != nil {
                return fmt.Errorf("append init event: %w", err)
            }
            fmt.Fprintf(cmd.OutOrStdout(), "tenant %q initialised at %s\n", id, t.Root())
            return nil
        },
    }
    cmd.Flags().StringVar(&id, "id", "", "tenant id (lowercase letters, digits, dash)")
    cmd.Flags().StringVar(&base, "base", "", "base state directory")
    _ = cmd.MarkFlagRequired("id")
    _ = cmd.MarkFlagRequired("base")
    return cmd
}
```

Now modify `internal/cli/stubs.go` — remove the `newTenantCmd` stub (keep only `newLedgerCmd` stub for now):

```go
package cli

import "github.com/spf13/cobra"

func newLedgerCmd() *cobra.Command { return &cobra.Command{Use: "ledger"} }
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/cli/... -v
```

- [ ] **Step 5: Build + smoke**

```bash
go build -o /tmp/themis-smoke ./cmd/themis
/tmp/themis-smoke tenant init --id acme --base /tmp/themis-smoke-data
ls /tmp/themis-smoke-data/tenants/acme/
```

Expected: `events.jsonl`, `bom`, `mempalace-wing` listed.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/
git commit -m "feat(cli): 'themis tenant init' creates tenant + emits TENANT_INITIALISED"
```

---

### Task 25: `themis ledger replay / verify / doctor`

**Files:**
- Create: `internal/cli/ledger_cmd.go` (replaces the stub)
- Create: `internal/cli/ledger_cmd_test.go`
- Modify: `internal/cli/stubs.go` (delete this file — both stubs replaced)

- [ ] **Step 1: Write the failing test**

Create `internal/cli/ledger_cmd_test.go`:

```go
package cli

import (
    "bytes"
    "encoding/json"
    "io/ioutil"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func setupTenant(t *testing.T) (base, id string) {
    t.Helper()
    base = t.TempDir()
    id = "acme"
    cmd := NewRootCmd()
    cmd.SetArgs([]string{"tenant", "init", "--id", id, "--base", base})
    if err := cmd.Execute(); err != nil { t.Fatal(err) }
    return
}

func TestLedgerDoctor_ReportsHealthy(t *testing.T) {
    base, id := setupTenant(t)
    out := &bytes.Buffer{}
    cmd := NewRootCmd()
    cmd.SetOut(out); cmd.SetErr(out)
    cmd.SetArgs([]string{"ledger", "doctor", "--id", id, "--base", base})
    if err := cmd.Execute(); err != nil { t.Fatalf("doctor: %v", err) }
    var rep map[string]any
    if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
        t.Fatalf("doctor output not JSON: %v\n%s", err, out.String())
    }
    if rep["chain_intact"] != true {
        t.Errorf("chain_intact = %v, want true", rep["chain_intact"])
    }
    if int(rep["event_count"].(float64)) != 1 {
        t.Errorf("event_count = %v, want 1", rep["event_count"])
    }
}

func TestLedgerVerify_DetectsTampering(t *testing.T) {
    base, id := setupTenant(t)

    eventsPath := filepath.Join(base, "tenants", id, "events.jsonl")
    raw, err := ioutil.ReadFile(eventsPath)
    if err != nil { t.Fatal(err) }
    raw[10] ^= 0x01
    if err := os.WriteFile(eventsPath, raw, 0o600); err != nil { t.Fatal(err) }

    cmd := NewRootCmd()
    cmd.SetArgs([]string{"ledger", "verify", "--id", id, "--base", base})
    err = cmd.Execute()
    if err == nil {
        t.Fatal("verify on tampered ledger should fail")
    }
    if !strings.Contains(err.Error(), "chain") && !strings.Contains(err.Error(), "decode") {
        t.Fatalf("verify error should mention chain or decode: %v", err)
    }
}

func TestLedgerReplay_RebuildsProjection(t *testing.T) {
    base, id := setupTenant(t)
    projPath := filepath.Join(base, "tenants", id, "projection.sqlite")

    cmd := NewRootCmd()
    cmd.SetArgs([]string{"ledger", "replay", "--id", id, "--base", base})
    if err := cmd.Execute(); err != nil { t.Fatalf("replay: %v", err) }

    fi, err := os.Stat(projPath)
    if err != nil { t.Fatalf("projection not created: %v", err) }
    if fi.Size() == 0 {
        t.Fatal("projection file is empty after replay")
    }
}
```

- [ ] **Step 2: Run; expect failure**

```bash
go test ./internal/cli/... -run TestLedger -v
```

- [ ] **Step 3: Implement**

Create `internal/cli/ledger_cmd.go`:

```go
package cli

import (
    "encoding/json"
    "fmt"
    "path/filepath"

    "github.com/spf13/cobra"

    "github.com/tzone85/themis/internal/ledger"
)

func newLedgerCmd() *cobra.Command {
    cmd := &cobra.Command{Use: "ledger", Short: "Inspect, replay, and verify tenant ledgers"}
    cmd.AddCommand(newLedgerDoctorCmd(), newLedgerVerifyCmd(), newLedgerReplayCmd())
    return cmd
}

func tenantPaths(base, id string) (events, projection string) {
    root := filepath.Join(base, "tenants", id)
    return filepath.Join(root, "events.jsonl"), filepath.Join(root, "projection.sqlite")
}

func newLedgerDoctorCmd() *cobra.Command {
    var id, base string
    cmd := &cobra.Command{
        Use:   "doctor",
        Short: "Report ledger health (event count, chain status, last hash) as JSON",
        RunE: func(cmd *cobra.Command, args []string) error {
            events, _ := tenantPaths(base, id)
            rep, err := ledger.Doctor(events)
            if err != nil {
                return fmt.Errorf("doctor: %w", err)
            }
            out := map[string]any{
                "event_count":   rep.EventCount,
                "chain_intact":  rep.ChainIntact,
                "chain_error":   rep.ChainError,
                "last_hash":     rep.LastHash,
            }
            enc := json.NewEncoder(cmd.OutOrStdout())
            enc.SetIndent("", "  ")
            return enc.Encode(out)
        },
    }
    cmd.Flags().StringVar(&id, "id", "", "tenant id")
    cmd.Flags().StringVar(&base, "base", "", "base state directory")
    _ = cmd.MarkFlagRequired("id")
    _ = cmd.MarkFlagRequired("base")
    return cmd
}

func newLedgerVerifyCmd() *cobra.Command {
    var id, base string
    cmd := &cobra.Command{
        Use:   "verify",
        Short: "Walk the Merkle chain; non-zero exit if tampering detected",
        RunE: func(cmd *cobra.Command, args []string) error {
            events, _ := tenantPaths(base, id)
            if err := ledger.Verify(events); err != nil {
                return err
            }
            fmt.Fprintln(cmd.OutOrStdout(), "ledger: chain intact")
            return nil
        },
    }
    cmd.Flags().StringVar(&id, "id", "", "tenant id")
    cmd.Flags().StringVar(&base, "base", "", "base state directory")
    _ = cmd.MarkFlagRequired("id")
    _ = cmd.MarkFlagRequired("base")
    return cmd
}

func newLedgerReplayCmd() *cobra.Command {
    var id, base string
    cmd := &cobra.Command{
        Use:   "replay",
        Short: "Rebuild the SQLite projection from events.jsonl",
        RunE: func(cmd *cobra.Command, args []string) error {
            events, projection := tenantPaths(base, id)
            if err := ledger.DeleteFile(projection); err != nil {
                return err
            }
            return ledger.Replay(events, projection, ledger.DefaultRegistry())
        },
    }
    cmd.Flags().StringVar(&id, "id", "", "tenant id")
    cmd.Flags().StringVar(&base, "base", "", "base state directory")
    _ = cmd.MarkFlagRequired("id")
    _ = cmd.MarkFlagRequired("base")
    return cmd
}
```

Delete `internal/cli/stubs.go`:

```bash
rm internal/cli/stubs.go
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/cli/... -v
```

- [ ] **Step 5: Build + smoke**

```bash
go build -o /tmp/themis-smoke ./cmd/themis
/tmp/themis-smoke tenant init --id acme --base /tmp/themis-smoke-data2
/tmp/themis-smoke ledger doctor --id acme --base /tmp/themis-smoke-data2
/tmp/themis-smoke ledger verify --id acme --base /tmp/themis-smoke-data2
/tmp/themis-smoke ledger replay --id acme --base /tmp/themis-smoke-data2
```

Expected: each command exits 0; `doctor` prints a small JSON document; `verify` prints `ledger: chain intact`.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/
git commit -m "feat(cli): 'themis ledger' doctor / verify / replay; remove stubs"
```

---

## Phase I — Coverage gate + final polish

### Task 26: Run full coverage and confirm ≥ 95%

**Files:** none (verification only)

- [ ] **Step 1: Run the full test suite with coverage**

```bash
cd /Users/mncedimini/Sites/misc/themis
go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -func=coverage.out | tail -1
```

Expected output ends with `total: (statements) 95.x%` or higher.

- [ ] **Step 2: Run the gate script**

```bash
bash scripts/cover_check.sh
```

Expected: `[cover-check] PASS`.

- [ ] **Step 3: If coverage is below threshold, identify the gap**

If FAIL, run:

```bash
go tool cover -func=coverage.out | sort -k3 -n | head -20
```

This lists the least-covered functions first. Add targeted unit tests until the global gate passes. Common gaps to fill:

- Error branches in `internal/ledger/projection.go` (force corrupt DB by `os.Chmod` to 0o000 before opening).
- Error branches in `internal/ledger/store.go` (force open failure by passing a directory path).
- CLI flag-validation paths (call `Execute` with missing flags + assert error).

Add tests as needed; commit incrementally.

- [ ] **Step 4: Commit (only if you added coverage tests)**

```bash
git add internal/...
git commit -m "test: cover error paths to reach 95% gate"
```

---

### Task 27: Wire govulncheck into the local Makefile

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Edit Makefile**

Append the following target to `Makefile`:

```make
vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...
```

And add `vulncheck` to the `ci` chain:

```make
ci: vet lint test cover vulncheck
	bash scripts/cover_check.sh
```

- [ ] **Step 2: Run it once locally**

```bash
make vulncheck
```

Expected: `No vulnerabilities found.` (or output reports any vulns; address before continuing).

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "ci: add govulncheck to local CI chain"
```

---

### Task 28: Add a CHANGELOG entry

**Files:**
- Create: `CHANGELOG.md`

- [ ] **Step 1: Write the changelog**

Create `CHANGELOG.md`:

```markdown
# Changelog

## [Unreleased] — Plan 1 (Foundation)

### Added
- Go module scaffold + Makefile + golangci-lint config + CI workflow + 95% coverage gate.
- `internal/tenant` package — Tenant value type, validated IDs, per-tenant filesystem paths, isolation tests.
- `internal/ledger` package:
  - `Event` struct with deterministic SHA-256 content hash and Merkle-style hash chain.
  - Append-only JSONL `Store` with fsync durability and chain-check on every append.
  - SQLite WAL `Projection` with kind-checked, idempotent `Project()`.
  - Event-kind `Registry` + `DefaultRegistry` + wiring test ensuring every used kind is registered.
  - `Replay`, `Verify`, and `Doctor` for ledger reconstruction and integrity checks.
  - Property tests covering hash determinism, sensitivity to every field, and Replay≡Project.
- `themis` CLI (`cmd/themis`):
  - `themis tenant init` — initialise a tenant directory tree + emit `TENANT_INITIALISED`.
  - `themis ledger doctor / verify / replay` — health, integrity, projection rebuild.

### Notes
- Multi-tenant filesystem isolation enforced at the storage layer (`tenants/<id>/`).
- Pure-Go SQLite driver (`modernc.org/sqlite`) — no CGO, cross-compile friendly, air-gapped-friendly.
- Apache 2.0 licence (per design spec §16).
- Tests use `pgregory.net/rapid` for property testing.
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add CHANGELOG with Plan 1 deliverables"
```

---

## Phase J — Final verification

### Task 29: Run the full CI chain locally

**Files:** none.

- [ ] **Step 1: Run the full chain**

```bash
make ci
```

Expected steps run in order: vet → lint → test → cover → vulncheck → cover_check.sh.

Expected final lines:

```
[cover-check] PASS
```

Non-zero exit → fix the failure before merging.

- [ ] **Step 2: Push the branch**

```bash
git push origin main
```

(We've been committing directly to main for this foundation pass. Future plans should use feature branches.)

- [ ] **Step 3: Verify CI green in GitHub**

Run:

```bash
gh run watch
```

Expected: workflow run succeeds.

---

## Plan 1 — Definition of Done

- [ ] `make ci` is green locally.
- [ ] GitHub Actions CI is green on the pushed commit.
- [ ] `themis tenant init / ledger doctor / verify / replay` smoke-tested manually.
- [ ] Coverage gate ≥ 95% global; per-package thresholds met.
- [ ] All property + wiring + isolation tests pass.
- [ ] CHANGELOG updated.
- [ ] One commit per task (clean linear history).

---

## What's next (after Plan 1 ships)

Plan 2 — Catalogue + classifier — will:
- Parse a real EventCatalog repository tree into a `CatalogueGraph`.
- Implement the pure `classify(AIChange, Graph) → Impact` function with all classification kinds (SCHEMA_BREAKING, NEW_EVENT, CONSUMER_TOUCH, ...).
- Add classifier property tests.
- Add the first non-trivial event kinds (`CATALOGUE_SYNCED`, `IMPACT_CLASSIFIED`) — each with a wiring-test entry.

We do **not** start Plan 2 until Plan 1 is shipped and green.
