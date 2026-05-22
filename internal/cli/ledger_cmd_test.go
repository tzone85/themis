package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tzone85/themis/internal/ledger"
)

// appendExtraEvent adds a second event to the tenant ledger so that
// chain-break-based tamper detection has something to detect against.
func appendExtraEvent(t *testing.T, base, id string) {
	t.Helper()
	eventsPath := filepath.Join(base, "tenants", id, "events.jsonl")
	s, err := ledger.OpenStore(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e := ledger.Event{
		Kind:      "LEDGER_VERIFIED",
		Tenant:    id,
		Timestamp: time.Unix(1700000000, 0).UTC(),
		Payload:   json.RawMessage(`{"src":"test"}`),
		PrevHash:  s.LastHash(),
	}
	if _, err := s.Append(e); err != nil {
		t.Fatal(err)
	}
}

func setupTenant(t *testing.T) (base, id string) {
	t.Helper()
	base = t.TempDir()
	id = "acme"
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"tenant", "init", "--id", id, "--base", base})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	return
}

func TestLedgerDoctor_ReportsHealthy(t *testing.T) {
	base, id := setupTenant(t)
	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"ledger", "doctor", "--id", id, "--base", base})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor: %v", err)
	}
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
	appendExtraEvent(t, base, id)

	eventsPath := filepath.Join(base, "tenants", id, "events.jsonl")
	raw, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte inside the first event so its ContentHash changes;
	// the second event's PrevHash then no longer matches → chain break.
	raw[10] ^= 0x01
	if err := os.WriteFile(eventsPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

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
	if err := cmd.Execute(); err != nil {
		t.Fatalf("replay: %v", err)
	}

	fi, err := os.Stat(projPath)
	if err != nil {
		t.Fatalf("projection not created: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("projection file is empty after replay")
	}
}
