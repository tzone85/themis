package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/ledger"
)

func writePolicy(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "themis.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeWorkdir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for path, body := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDecide_DocOnlyAllow(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	pol := writePolicy(t, t.TempDir(), validPolicyYAML)
	change := aichange.AIChange{
		PRID:  "gh:test#decide-1",
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	cp := writeAIChange(t, t.TempDir(), change)

	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"decide", "--id", id, "--base", base, "--aichange", cp, "--policy", pol})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("decide: %v", err)
	}
	var d map[string]any
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if d["verdict"] != "ALLOW" {
		t.Fatalf("verdict = %v, want ALLOW", d["verdict"])
	}

	// Ledger should have: TENANT_INITIALISED, CATALOGUE_SYNCED, DECISION_ISSUED.
	events, err := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].Kind != "DECISION_ISSUED" {
		t.Fatalf("last event = %q, want DECISION_ISSUED", events[len(events)-1].Kind)
	}
}

const policyWithSecretRule = `version: 1
default: REQUIRE_APPROVAL
rules:
  - name: doc-only allowed
    when:
      impact.kind: [DOC_ONLY]
    then:
      verdict: ALLOW
  - name: secrets block
    when:
      findings.kind: secret
    then:
      verdict: DENY
      reason: secret detected by scanner
`

func TestDecide_SecretFindingDenies(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	pol := writePolicy(t, t.TempDir(), policyWithSecretRule)
	change := aichange.AIChange{
		PRID:  "gh:test#decide-2",
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "src/leak.go", ChangeKind: aichange.FileAdded, AfterHash: "h"},
		},
	}
	cp := writeAIChange(t, t.TempDir(), change)
	work := writeWorkdir(t, map[string]string{
		"src/leak.go": "aws_id = AKIAIOSFODNN7EXAMPLE\n",
	})

	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"decide", "--id", id, "--base", base, "--aichange", cp, "--policy", pol, "--workdir", work})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("decide: %v", err)
	}
	var d map[string]any
	_ = json.Unmarshal(out.Bytes(), &d)
	if d["verdict"] != "DENY" {
		t.Fatalf("verdict = %v, want DENY (secret detected)", d["verdict"])
	}

	// Ledger must include at least one SCAN_FINDING + a DECISION_ISSUED.
	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	var sawFinding, sawDecision bool
	for _, e := range events {
		switch e.Kind {
		case "SCAN_FINDING":
			sawFinding = true
		case "DECISION_ISSUED":
			sawDecision = true
		}
	}
	if !sawFinding {
		t.Error("ledger missing SCAN_FINDING event")
	}
	if !sawDecision {
		t.Error("ledger missing DECISION_ISSUED event")
	}
}

func TestDecide_RejectsInvalidPolicyAndLogs(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	pol := writePolicy(t, t.TempDir(), "default: SHRUG\n") // no version → invalid
	change := aichange.AIChange{PRID: "x", Actor: "y", TouchedFiles: []aichange.FileTouch{{Path: "README.md", ChangeKind: aichange.FileModified}}}
	cp := writeAIChange(t, t.TempDir(), change)

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"decide", "--id", id, "--base", base, "--aichange", cp, "--policy", pol})
	if err := cmd.Execute(); err == nil {
		t.Fatal("decide should refuse to issue verdict on invalid policy")
	}

	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	var sawPolicyInvalid bool
	for _, e := range events {
		if e.Kind == "POLICY_INVALID" {
			sawPolicyInvalid = true
		}
	}
	if !sawPolicyInvalid {
		t.Error("ledger should contain POLICY_INVALID event after parse failure")
	}
}
