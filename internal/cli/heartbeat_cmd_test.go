package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/ledger"
)

func TestHeartbeatReport_AppendsEnforcementMissing(t *testing.T) {
	base, id := setupTenant(t)
	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{
		"heartbeat", "report",
		"--id", id, "--base", base,
		"--repo", "gh:tzone85/svc",
		"--expected-check", "themis-check",
		"--reported-by", "gh-action-watchdog",
		"--last-seen", "2026-05-24T10:00:00Z",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("heartbeat report: %v\n%s", err, out.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if payload["repo"] != "gh:tzone85/svc" {
		t.Errorf("repo = %q", payload["repo"])
	}

	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if events[len(events)-1].Kind != "ENFORCEMENT_MISSING" {
		t.Fatalf("last event = %q, want ENFORCEMENT_MISSING", events[len(events)-1].Kind)
	}
}

func TestHeartbeatReport_RequiresMandatoryFields(t *testing.T) {
	base, id := setupTenant(t)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"heartbeat", "report",
		"--id", id, "--base", base,
		"--repo", "gh:x/y",
		"--expected-check", "themis-check",
		// --reported-by missing
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("missing --reported-by should error")
	}
}

func TestHeartbeatReport_LastSeenOptional(t *testing.T) {
	base, id := setupTenant(t)
	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{
		"heartbeat", "report",
		"--id", id, "--base", base,
		"--repo", "gh:x/y",
		"--expected-check", "themis-check",
		"--reported-by", "watchdog",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	_ = json.Unmarshal(out.Bytes(), &payload)
	if _, has := payload["last_seen"]; has {
		t.Errorf("last_seen should be absent when not supplied: %+v", payload)
	}
}
