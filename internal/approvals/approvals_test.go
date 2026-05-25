package approvals

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/policy"
)

// mustMarshal panics on JSON marshal failure — tests only.
func mustMarshal(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

// decisionEvent constructs a synthetic DECISION_ISSUED for tests.
func decisionEvent(prID string, d policy.Decision) ledger.Event {
	return ledger.Event{
		Kind: "DECISION_ISSUED",
		Payload: mustMarshal(map[string]any{
			"pr_id":    prID,
			"actor":    "claude_code",
			"decision": d,
		}),
	}
}

func grantEvent(prID, approver, role string) ledger.Event {
	return ledger.Event{
		Kind: "APPROVAL_GRANTED",
		Payload: mustMarshal(GrantPayload{
			PRID:      prID,
			Approver:  approver,
			Role:      role,
			GrantedAt: time.Unix(0, 0).UTC(),
		}),
	}
}

func denyEvent(prID, approver, role, reason string) ledger.Event {
	return ledger.Event{
		Kind: "APPROVAL_DENIED",
		Payload: mustMarshal(DenyPayload{
			PRID:     prID,
			Approver: approver,
			Role:     role,
			Reason:   reason,
			DeniedAt: time.Unix(0, 0).UTC(),
		}),
	}
}

func TestCompute_NoDecisionYet(t *testing.T) {
	st := Compute(nil, "missing#1")
	if st.Decision.Verdict != "" {
		t.Fatalf("expected zero Decision, got %+v", st.Decision)
	}
}

func TestCompute_AllowPassesThrough(t *testing.T) {
	events := []ledger.Event{
		decisionEvent("x#1", policy.Decision{Verdict: policy.VerdictAllow}),
	}
	st := Compute(events, "x#1")
	if v, ok := CanFinalise(st); !ok || v != policy.VerdictAllow {
		t.Fatalf("ALLOW should finalise immediately: v=%q ok=%v", v, ok)
	}
}

func TestCompute_RequireApproval_GrantSatisfiesSingleRole(t *testing.T) {
	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}},
	}
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		grantEvent("x#1", "alice", "senior"),
	}
	st := Compute(events, "x#1")
	if len(st.GrantedRoles) != 1 || st.GrantedRoles[0] != "senior" {
		t.Fatalf("granted roles = %v", st.GrantedRoles)
	}
	if v, ok := CanFinalise(st); !ok || v != policy.VerdictAllow {
		t.Fatalf("single-role grant should finalise ALLOW: v=%q ok=%v", v, ok)
	}
}

func TestCompute_RequireApproval_TwoRolesPartialDoesNotFinalise(t *testing.T) {
	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}, {Role: "compliance"}},
	}
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		grantEvent("x#1", "alice", "senior"),
	}
	st := Compute(events, "x#1")
	if _, ok := CanFinalise(st); ok {
		t.Fatal("partial grants should NOT finalise")
	}
	if st.FinalVerdict != policy.VerdictRequireApproval {
		t.Fatalf("projected = %q, want REQUIRE_APPROVAL", st.FinalVerdict)
	}
}

func TestCompute_RequireApproval_BothRolesFinalisesAllow(t *testing.T) {
	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}, {Role: "compliance"}},
	}
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		grantEvent("x#1", "alice", "senior"),
		grantEvent("x#1", "bob", "compliance"),
	}
	st := Compute(events, "x#1")
	v, ok := CanFinalise(st)
	if !ok || v != policy.VerdictAllow {
		t.Fatalf("both roles granted should finalise ALLOW: v=%q ok=%v", v, ok)
	}
}

func TestCompute_DenialFinalisesDeny(t *testing.T) {
	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}},
	}
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		denyEvent("x#1", "alice", "senior", "not safe"),
	}
	st := Compute(events, "x#1")
	if !st.Denied {
		t.Fatal("Denied should be true")
	}
	if v, ok := CanFinalise(st); !ok || v != policy.VerdictDeny {
		t.Fatalf("denial should finalise DENY: v=%q ok=%v", v, ok)
	}
}

func TestCompute_DenialStickyAcrossLaterGrant(t *testing.T) {
	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}},
	}
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		denyEvent("x#1", "alice", "senior", "not safe"),
		grantEvent("x#1", "alice", "senior"),
	}
	st := Compute(events, "x#1")
	if v, ok := CanFinalise(st); !ok || v != policy.VerdictDeny {
		t.Fatalf("denial sticky: v=%q ok=%v", v, ok)
	}
}

func TestCompute_NewDecisionResetsApprovals(t *testing.T) {
	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}},
	}
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		denyEvent("x#1", "alice", "senior", "first round"),
		decisionEvent("x#1", dec), // re-decide → resets
		grantEvent("x#1", "alice", "senior"),
	}
	st := Compute(events, "x#1")
	if st.Denied {
		t.Fatal("new decision should reset denial")
	}
	if v, ok := CanFinalise(st); !ok || v != policy.VerdictAllow {
		t.Fatalf("post-reset grant should finalise ALLOW: v=%q ok=%v", v, ok)
	}
}

func TestCompute_AlreadyFinalisedReportsFinalised(t *testing.T) {
	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}},
	}
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		grantEvent("x#1", "alice", "senior"),
		{
			Kind: "DECISION_FINALISED",
			Payload: mustMarshal(FinalisedPayload{
				PRID:         "x#1",
				FinalVerdict: policy.VerdictAllow,
			}),
		},
	}
	st := Compute(events, "x#1")
	if !st.Finalised {
		t.Fatal("Finalised should be true")
	}
	if _, ok := CanFinalise(st); ok {
		t.Fatal("already finalised → CanFinalise should be false")
	}
}

func TestCompute_NoRequiredRolesAnyGrantFinalises(t *testing.T) {
	dec := policy.Decision{Verdict: policy.VerdictRequireApproval} // no RequiredApprovers
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		grantEvent("x#1", "alice", "any-role"),
	}
	st := Compute(events, "x#1")
	if v, ok := CanFinalise(st); !ok || v != policy.VerdictAllow {
		t.Fatalf("any grant should finalise ALLOW when no roles required: v=%q ok=%v", v, ok)
	}
}

func TestBuildFinalised_CapturesGrants(t *testing.T) {
	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}},
	}
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		grantEvent("x#1", "alice", "senior"),
	}
	st := Compute(events, "x#1")
	payload := BuildFinalised(st, "x#1", time.Unix(123, 0))
	if payload.PRID != "x#1" {
		t.Fatalf("PRID = %q", payload.PRID)
	}
	if payload.FinalVerdict != policy.VerdictAllow {
		t.Fatalf("FinalVerdict = %q", payload.FinalVerdict)
	}
	if len(payload.Grants) != 1 || payload.Grants[0].Approver != "alice" {
		t.Fatalf("Grants = %+v", payload.Grants)
	}
}

func TestCompute_IgnoresOtherPRs(t *testing.T) {
	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}},
	}
	events := []ledger.Event{
		decisionEvent("x#1", dec),
		grantEvent("OTHER#99", "alice", "senior"),
	}
	st := Compute(events, "x#1")
	if len(st.GrantedRoles) != 0 {
		t.Fatalf("grants from other PR leaked: %+v", st.GrantedRoles)
	}
}
