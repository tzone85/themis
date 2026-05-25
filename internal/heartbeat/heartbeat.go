// Package heartbeat implements the polling-daemon side of design spec
// §9.1.2: actively probe each tenant repo to confirm the required Themis
// check is still installed, and emit ENFORCEMENT_MISSING when it's not.
//
// The daemon depends on a Checker abstraction so the GitHub (or
// GitLab/Bitbucket) integration is pluggable. Plan 15 ships an offline
// StubChecker that lets tests + air-gapped deployments validate the loop
// logic; a real GitHub adapter is a drop-in extension.
package heartbeat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tzone85/themis/internal/ledger"
)

// Target is a single (repo, expected_check) pair the daemon monitors.
type Target struct {
	Repo          string `yaml:"repo" json:"repo"`
	ExpectedCheck string `yaml:"expected_check" json:"expected_check"`
}

// Config is the per-tenant YAML file read from tenants/<id>/heartbeat.yaml.
type Config struct {
	Targets []Target `yaml:"targets"`
}

// Checker decides whether a Target's required check is still installed.
type Checker interface {
	// Name identifies the source (e.g. "github", "stub"). Embedded in
	// emitted events so audit trail records which observer reported.
	Name() string
	// Check returns (presentAndEnforced, lastSeen, error). lastSeen is
	// optional and is recorded in the ENFORCEMENT_MISSING event when not
	// the zero time.
	Check(ctx context.Context, t Target) (present bool, lastSeen time.Time, err error)
}

// StubChecker is a deterministic Checker used by tests + air-gapped
// deployments. Targets present in the Allow set are reported as installed;
// targets in the Reject set are reported missing.
type StubChecker struct {
	Allow  map[string]struct{}
	Reject map[string]struct{}
}

// NewStubChecker constructs a stub from two repo lists. A repo not in
// either set is treated as "missing" (the conservative default — silence
// equals a problem).
func NewStubChecker(allow, reject []string) *StubChecker {
	a := map[string]struct{}{}
	for _, r := range allow {
		a[r] = struct{}{}
	}
	rj := map[string]struct{}{}
	for _, r := range reject {
		rj[r] = struct{}{}
	}
	return &StubChecker{Allow: a, Reject: rj}
}

// Name implements Checker.
func (s *StubChecker) Name() string { return "stub" }

// Check implements Checker.
func (s *StubChecker) Check(_ context.Context, t Target) (bool, time.Time, error) {
	if _, ok := s.Allow[t.Repo]; ok {
		return true, time.Now().UTC(), nil
	}
	if _, ok := s.Reject[t.Repo]; ok {
		return false, time.Time{}, nil
	}
	// Default to "missing" so silence is treated as a problem.
	return false, time.Time{}, nil
}

// LoadConfig reads the per-tenant heartbeat targets file. Missing file
// returns an empty config with no error.
func LoadConfig(base, tenantID string) (Config, error) {
	path := filepath.Join(base, "tenants", tenantID, "heartbeat.yaml")
	raw, err := os.ReadFile(path) // #nosec G304 -- tenant-scoped path.
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}

// Errors surfaced by the daemon.
var (
	ErrNoTargets   = errors.New("heartbeat: no targets configured")
	ErrCheckerNil  = errors.New("heartbeat: checker is nil")
)

// RunOnce performs one pass over every target, emitting ENFORCEMENT_MISSING
// for any target reported as missing. Returns the number of misses found.
//
// This is the building block both `themis heartbeat watch` (loops with a
// configurable interval) and CI-style one-shot integrations use.
func RunOnce(ctx context.Context, base, tenantID string, checker Checker) (int, error) {
	if checker == nil {
		return 0, ErrCheckerNil
	}
	cfg, err := LoadConfig(base, tenantID)
	if err != nil {
		return 0, err
	}
	if len(cfg.Targets) == 0 {
		return 0, ErrNoTargets
	}

	eventsPath := filepath.Join(base, "tenants", tenantID, "events.jsonl")
	store, err := ledger.OpenStore(eventsPath)
	if err != nil {
		return 0, fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	misses := 0
	for _, t := range cfg.Targets {
		select {
		case <-ctx.Done():
			return misses, ctx.Err()
		default:
		}
		present, lastSeen, err := checker.Check(ctx, t)
		if err != nil {
			// Checker errors are themselves a missing signal — record
			// them as misses so silence-of-the-watchdog doesn't blind us.
			present = false
		}
		if present {
			continue
		}
		misses++
		payload := map[string]string{
			"repo":           t.Repo,
			"expected_check": t.ExpectedCheck,
			"reported_by":    "heartbeat:" + checker.Name(),
			"reported_at":    time.Now().UTC().Format(time.RFC3339Nano),
		}
		if !lastSeen.IsZero() {
			payload["last_seen"] = lastSeen.Format(time.RFC3339Nano)
		}
		raw, _ := json.Marshal(payload)
		if _, err := store.Append(ledger.Event{
			Kind:      "ENFORCEMENT_MISSING",
			Tenant:    tenantID,
			Timestamp: time.Now().UTC(),
			Payload:   raw,
			PrevHash:  store.LastHash(),
		}); err != nil {
			return misses, fmt.Errorf("append ENFORCEMENT_MISSING: %w", err)
		}
	}
	return misses, nil
}

// Watch is the long-running daemon loop. It calls RunOnce every interval
// and returns when ctx is cancelled. Errors from individual passes are
// logged via the supplied logFn (called with the formatted line, no newline);
// a nil logFn discards them.
func Watch(ctx context.Context, base, tenantID string, checker Checker, interval time.Duration, logFn func(string)) error {
	if interval <= 0 {
		interval = time.Minute
	}
	if logFn == nil {
		logFn = func(string) {}
	}
	var mu sync.Mutex
	tick := time.NewTicker(interval)
	defer tick.Stop()

	// Fire one pass immediately so the first detection doesn't wait for
	// the first tick.
	for {
		mu.Lock()
		misses, err := RunOnce(ctx, base, tenantID, checker)
		mu.Unlock()
		switch {
		case err == nil:
			logFn(fmt.Sprintf("heartbeat: %d miss(es) recorded\n", misses))
		case errors.Is(err, ErrNoTargets):
			logFn("heartbeat: no targets configured; sleeping\n")
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil
		default:
			logFn(fmt.Sprintf("heartbeat: %v\n", err))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// WriteConfig persists a Config to disk. Used by tests + the operator-facing
// `themis heartbeat configure` (out-of-scope at Plan 15; operators hand-edit
// the YAML for now).
func WriteConfig(base, tenantID string, c Config) error {
	if err := os.MkdirAll(filepath.Join(base, "tenants", tenantID), 0o700); err != nil {
		return err
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(base, "tenants", tenantID, "heartbeat.yaml"), raw, 0o600)
}

// ensureFmtImported keeps the linter quiet during refactors that
// temporarily drop fmt usage.
var _ = strings.TrimSpace
