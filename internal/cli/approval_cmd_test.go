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

// approvalRequirePolicy forces a REQUIRE_APPROVAL verdict with a single
// "senior" role required so the test can grant/deny deterministically.
const approvalRequirePolicy = `version: 1
default: REQUIRE_APPROVAL
required_approvers_for_default:
  - role: senior
`

// setupTenantWithPendingDecision drives the pipeline up to DECISION_ISSUED
// so subsequent approval CLI calls have something to act on.
func setupTenantWithPendingDecision(t *testing.T, prID string) (base, id string) {
	t.Helper()
	base, id = setupTenantWithSyncedCatalogue(t)
	pol := writePolicy(t, t.TempDir(), approvalRequirePolicy)
	change := aichange.AIChange{
		PRID:  prID,
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "services/collector/handler.go", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	cp := writeAIChange(t, t.TempDir(), change)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"decide", "--id", id, "--base", base, "--aichange", cp, "--policy", pol})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("decide: %v", err)
	}
	return
}

func runApprovalCLI(t *testing.T, args ...string) string {
	t.Helper()
	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("`themis %v` failed: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func TestApprovalGrant_FinalisesAllow(t *testing.T) {
	base, id := setupTenantWithPendingDecision(t, "gh:test#approve-1")

	out := runApprovalCLI(t,
		"approval", "grant",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#approve-1",
		"--approver", "human:alice",
		"--role", "senior",
		"--comment", "looks fine",
	)
	var st map[string]any
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if st["finalised"] != true {
		t.Fatalf("not finalised: %+v", st)
	}
	if st["final_verdict"] != "ALLOW" {
		t.Fatalf("final_verdict = %v", st["final_verdict"])
	}

	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	kinds := []string{}
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	sawGrant, sawFinal := false, false
	for _, k := range kinds {
		if k == "APPROVAL_GRANTED" {
			sawGrant = true
		}
		if k == "DECISION_FINALISED" {
			sawFinal = true
		}
	}
	if !sawGrant || !sawFinal {
		t.Fatalf("expected APPROVAL_GRANTED + DECISION_FINALISED in ledger: %v", kinds)
	}
}

func TestApprovalDeny_FinalisesDeny(t *testing.T) {
	base, id := setupTenantWithPendingDecision(t, "gh:test#approve-2")

	out := runApprovalCLI(t,
		"approval", "deny",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#approve-2",
		"--approver", "human:alice",
		"--role", "senior",
		"--reason", "schema break not safe",
	)
	var st map[string]any
	_ = json.Unmarshal([]byte(out), &st)
	if st["finalised"] != true {
		t.Fatalf("expected finalised after deny: %+v", st)
	}
	if st["final_verdict"] != "DENY" {
		t.Fatalf("final_verdict = %v", st["final_verdict"])
	}
}

func TestApprovalStatus_NotFinalisedYet(t *testing.T) {
	base, id := setupTenantWithPendingDecision(t, "gh:test#approve-3")
	out := runApprovalCLI(t, "approval", "status",
		"--id", id, "--base", base, "--pr-id", "gh:test#approve-3")
	var st map[string]any
	_ = json.Unmarshal([]byte(out), &st)
	if st["finalised"] == true {
		t.Fatalf("status pre-grant should not be finalised: %+v", st)
	}
	if st["final_verdict"] != "REQUIRE_APPROVAL" {
		t.Fatalf("projected = %v, want REQUIRE_APPROVAL", st["final_verdict"])
	}
}

func TestApprovalGrant_RejectsUnknownPR(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"approval", "grant",
		"--id", id, "--base", base,
		"--pr-id", "no-such",
		"--approver", "human:alice",
		"--role", "senior",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("approval grant should reject pr without a DECISION_ISSUED")
	}
}

func TestApprovalGrant_RejectsAlreadyFinalised(t *testing.T) {
	base, id := setupTenantWithPendingDecision(t, "gh:test#approve-4")
	// First grant finalises.
	runApprovalCLI(t,
		"approval", "grant",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#approve-4",
		"--approver", "human:alice", "--role", "senior")
	// Second grant should fail (already finalised).
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"approval", "grant",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#approve-4",
		"--approver", "human:bob", "--role", "senior"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("second grant after finalisation should error")
	}
}

func TestApprovalStatus_RequiresPRID(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"approval", "status", "--id", id, "--base", base})
	if err := cmd.Execute(); err == nil {
		t.Fatal("status without --pr-id should error")
	}
	_ = os.Stdout // keep imports tidy
}
