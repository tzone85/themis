package heartbeat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tzone85/themis/internal/ledger"
)

func setupTenant(t *testing.T) (base, id string) {
	t.Helper()
	base = t.TempDir()
	id = "acme"
	if err := os.MkdirAll(filepath.Join(base, "tenants", id), 0o700); err != nil {
		t.Fatal(err)
	}
	// Seed an empty events.jsonl so the store opens cleanly.
	if err := os.WriteFile(filepath.Join(base, "tenants", id, "events.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return
}

func TestLoadConfig_MissingFileReturnsEmpty(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir(), "no-such")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Targets) != 0 {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestLoadConfig_ParsesYAML(t *testing.T) {
	base, id := setupTenant(t)
	WriteConfig(base, id, Config{Targets: []Target{
		{Repo: "gh:org/a", ExpectedCheck: "themis-check"},
		{Repo: "gh:org/b", ExpectedCheck: "themis-check"},
	}})
	cfg, err := LoadConfig(base, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %+v", cfg)
	}
}

func TestStubChecker_AllowList(t *testing.T) {
	c := NewStubChecker([]string{"gh:org/ok"}, []string{"gh:org/bad"})
	if present, _, _ := c.Check(context.Background(), Target{Repo: "gh:org/ok"}); !present {
		t.Fatal("allowed repo should be present")
	}
	if present, _, _ := c.Check(context.Background(), Target{Repo: "gh:org/bad"}); present {
		t.Fatal("rejected repo should be missing")
	}
	if present, _, _ := c.Check(context.Background(), Target{Repo: "gh:org/unknown"}); present {
		t.Fatal("unknown repo should default to missing")
	}
}

func TestStubChecker_Name(t *testing.T) {
	if NewStubChecker(nil, nil).Name() != "stub" {
		t.Fatal("Name should be stub")
	}
}

func TestRunOnce_EmitsMissingEvents(t *testing.T) {
	base, id := setupTenant(t)
	WriteConfig(base, id, Config{Targets: []Target{
		{Repo: "gh:org/ok", ExpectedCheck: "themis-check"},
		{Repo: "gh:org/missing-1", ExpectedCheck: "themis-check"},
		{Repo: "gh:org/missing-2", ExpectedCheck: "themis-check"},
	}})
	checker := NewStubChecker([]string{"gh:org/ok"}, nil)

	misses, err := RunOnce(context.Background(), base, id, checker)
	if err != nil {
		t.Fatal(err)
	}
	if misses != 2 {
		t.Fatalf("misses = %d, want 2", misses)
	}

	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	missingByRepo := map[string]bool{}
	for _, e := range events {
		if e.Kind != "ENFORCEMENT_MISSING" {
			continue
		}
		var p struct {
			Repo       string `json:"repo"`
			ReportedBy string `json:"reported_by"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		missingByRepo[p.Repo] = true
		if p.ReportedBy != "heartbeat:stub" {
			t.Errorf("reported_by = %q", p.ReportedBy)
		}
	}
	if !missingByRepo["gh:org/missing-1"] || !missingByRepo["gh:org/missing-2"] {
		t.Fatalf("missing events not emitted for both repos: %+v", missingByRepo)
	}
	if missingByRepo["gh:org/ok"] {
		t.Fatal("present repo should not emit ENFORCEMENT_MISSING")
	}
}

func TestRunOnce_RequiresChecker(t *testing.T) {
	base, id := setupTenant(t)
	WriteConfig(base, id, Config{Targets: []Target{{Repo: "x", ExpectedCheck: "y"}}})
	_, err := RunOnce(context.Background(), base, id, nil)
	if !errors.Is(err, ErrCheckerNil) {
		t.Fatalf("expected ErrCheckerNil, got %v", err)
	}
}

func TestRunOnce_EmptyConfigErrors(t *testing.T) {
	base, id := setupTenant(t)
	_, err := RunOnce(context.Background(), base, id, NewStubChecker(nil, nil))
	if !errors.Is(err, ErrNoTargets) {
		t.Fatalf("expected ErrNoTargets, got %v", err)
	}
}

// errorChecker returns an error for every check — should still record a miss
// (silence equals problem).
type errorChecker struct{}

func (errorChecker) Name() string                                       { return "broken" }
func (errorChecker) Check(_ context.Context, _ Target) (bool, time.Time, error) {
	return false, time.Time{}, errors.New("checker exploded")
}

func TestRunOnce_CheckerErrorsCountAsMiss(t *testing.T) {
	base, id := setupTenant(t)
	WriteConfig(base, id, Config{Targets: []Target{{Repo: "gh:org/x", ExpectedCheck: "themis-check"}}})
	misses, err := RunOnce(context.Background(), base, id, errorChecker{})
	if err != nil {
		t.Fatal(err)
	}
	if misses != 1 {
		t.Fatalf("expected 1 miss, got %d", misses)
	}
}

func TestRunOnce_ContextCancelStopsLoop(t *testing.T) {
	base, id := setupTenant(t)
	WriteConfig(base, id, Config{Targets: []Target{
		{Repo: "a"}, {Repo: "b"}, {Repo: "c"},
	}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before we start
	_, err := RunOnce(ctx, base, id, NewStubChecker(nil, nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWatch_TerminatesOnCtxCancel(t *testing.T) {
	base, id := setupTenant(t)
	WriteConfig(base, id, Config{Targets: []Target{{Repo: "gh:org/x", ExpectedCheck: "y"}}})
	ctx, cancel := context.WithCancel(context.Background())

	logs := make(chan string, 8)
	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, base, id, NewStubChecker(nil, []string{"gh:org/x"}), 5*time.Millisecond, func(s string) {
			select {
			case logs <- s:
			default:
			}
		})
	}()

	// Wait for at least one pass.
	select {
	case <-logs:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("Watch never logged a pass")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Watch returned non-nil on cancel: %v", err)
	}
}
