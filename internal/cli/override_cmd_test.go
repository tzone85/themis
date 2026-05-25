package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tzone85/themis/internal/ledger"
)

const longReason = "Catalogue server outage blocking the on-call rotation; need to merge a logging fix before the next scheduled deploy."

func runOverrideCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func setupTenantOverride(t *testing.T, prID string) (base, id string) {
	t.Helper()
	return setupTenantWithPendingDecision(t, prID)
}

func TestOverrideInvoke_AppendsBothEvents(t *testing.T) {
	base, id := setupTenantOverride(t, "gh:test#override-1")
	out, err := runOverrideCLI(t,
		"override", "invoke",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-1",
		"--actor", "human:alice",
		"--co-signer", "human:bob",
		"--reason", longReason,
		"--scope", "one-pr",
	)
	if err != nil {
		t.Fatalf("override invoke: %v\n%s", err, out)
	}
	var p map[string]any
	_ = json.Unmarshal([]byte(out), &p)
	if p["actor"] != "human:alice" {
		t.Fatalf("payload actor = %v", p["actor"])
	}

	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	sawInvoke, sawDue := false, false
	for _, e := range events {
		if e.Kind == "EMERGENCY_OVERRIDE_INVOKED" {
			sawInvoke = true
		}
		if e.Kind == "OVERRIDE_POSTMORTEM_DUE" {
			sawDue = true
		}
	}
	if !sawInvoke || !sawDue {
		t.Fatalf("expected both EMERGENCY_OVERRIDE_INVOKED + OVERRIDE_POSTMORTEM_DUE; saw invoke=%v due=%v", sawInvoke, sawDue)
	}
}

func TestOverrideInvoke_RejectsShortReason(t *testing.T) {
	base, id := setupTenantOverride(t, "gh:test#override-2")
	_, err := runOverrideCLI(t,
		"override", "invoke",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-2",
		"--actor", "human:alice", "--co-signer", "human:bob",
		"--reason", "too short",
	)
	if err == nil {
		t.Fatal("short reason should error")
	}
}

func TestOverrideInvoke_RejectsActorEqualsCoSigner(t *testing.T) {
	base, id := setupTenantOverride(t, "gh:test#override-3")
	_, err := runOverrideCLI(t,
		"override", "invoke",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-3",
		"--actor", "human:alice", "--co-signer", "human:alice",
		"--reason", longReason,
	)
	if err == nil {
		t.Fatal("actor==co_signer should error")
	}
}

func TestOverride_ClosePostmortemFlow(t *testing.T) {
	base, id := setupTenantOverride(t, "gh:test#override-4")
	if _, err := runOverrideCLI(t,
		"override", "invoke",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-4",
		"--actor", "human:alice", "--co-signer", "human:bob",
		"--reason", longReason,
	); err != nil {
		t.Fatal(err)
	}
	out, err := runOverrideCLI(t,
		"override", "close-postmortem",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-4",
		"--closer", "human:compliance",
		"--notes", "post-mortem complete: root cause identified",
	)
	if err != nil {
		t.Fatalf("close-postmortem: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"postmortem_closed": true`) {
		t.Fatalf("expected postmortem_closed=true in output: %s", out)
	}
}

func TestOverride_ClosePostmortemRejectsTwice(t *testing.T) {
	base, id := setupTenantOverride(t, "gh:test#override-5")
	if _, err := runOverrideCLI(t,
		"override", "invoke",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-5",
		"--actor", "human:a", "--co-signer", "human:b",
		"--reason", longReason,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := runOverrideCLI(t,
		"override", "close-postmortem",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-5",
		"--closer", "human:c", "--notes", "first close",
	); err != nil {
		t.Fatal(err)
	}
	_, err := runOverrideCLI(t,
		"override", "close-postmortem",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-5",
		"--closer", "human:c", "--notes", "second close",
	)
	if err == nil {
		t.Fatal("second close should error")
	}
}

func TestOverride_ClosePostmortemRejectsUnknownPR(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)
	_, err := runOverrideCLI(t,
		"override", "close-postmortem",
		"--id", id, "--base", base,
		"--pr-id", "nope",
		"--closer", "human:c", "--notes", "x",
	)
	if err == nil {
		t.Fatal("unknown pr should error")
	}
}

func TestOverride_Status(t *testing.T) {
	base, id := setupTenantOverride(t, "gh:test#override-6")
	if _, err := runOverrideCLI(t,
		"override", "invoke",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-6",
		"--actor", "human:a", "--co-signer", "human:b",
		"--reason", longReason,
	); err != nil {
		t.Fatal(err)
	}
	out, err := runOverrideCLI(t,
		"override", "status",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#override-6",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"active": true`) {
		t.Fatalf("expected active=true: %s", out)
	}
}
