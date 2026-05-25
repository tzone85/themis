// Package approvals computes the approval state of a pull request by
// walking the ledger. Approvals are themselves ledger events
// (APPROVAL_GRANTED / APPROVAL_DENIED); once every required role for a
// REQUIRE_APPROVAL decision has signed off, the CLI/API emits
// DECISION_FINALISED carrying the final verdict.
//
// All functions in this package are pure: same ledger slice + same PR id
// → same result, always. This is what lets compliance officers replay the
// approval history and prove the verdict was inevitable given the inputs.
package approvals

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/policy"
)

// GrantPayload is the JSON body of an APPROVAL_GRANTED event.
type GrantPayload struct {
	PRID      string    `json:"pr_id"`
	Approver  string    `json:"approver"`
	Role      string    `json:"role"`
	Comment   string    `json:"comment,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
}

// DenyPayload is the JSON body of an APPROVAL_DENIED event.
type DenyPayload struct {
	PRID     string    `json:"pr_id"`
	Approver string    `json:"approver"`
	Role     string    `json:"role"`
	Reason   string    `json:"reason"`
	DeniedAt time.Time `json:"denied_at"`
}

// FinalisedPayload is the JSON body of a DECISION_FINALISED event.
type FinalisedPayload struct {
	PRID         string          `json:"pr_id"`
	FinalVerdict policy.Verdict  `json:"final_verdict"`
	Decision     policy.Decision `json:"decision"`
	Grants       []GrantPayload  `json:"grants,omitempty"`
	FinalisedAt  time.Time       `json:"finalised_at"`
}

// Status describes the current approval state for a PR.
type Status struct {
	// Decision is the most-recent DECISION_ISSUED payload for the PR. When
	// no decision has been issued yet, Decision.Verdict is "".
	Decision policy.Decision `json:"decision"`

	// GrantedRoles is the set of roles that have at least one APPROVAL_GRANTED
	// for the PR after the last DECISION_ISSUED. Sorted for deterministic output.
	GrantedRoles []string `json:"granted_roles,omitempty"`

	// DenialsByRole records the most recent denial per role, if any.
	DenialsByRole map[string]DenyPayload `json:"denials_by_role,omitempty"`

	// Denied is true when at least one APPROVAL_DENIED has landed for the PR
	// since the latest DECISION_ISSUED. Denials are sticky for the current
	// decision — a later grant doesn't clear them; only a fresh DECISION_ISSUED
	// (re-decide) does.
	Denied bool `json:"denied"`

	// Finalised is true when a DECISION_FINALISED event already exists for the PR.
	Finalised bool `json:"finalised"`

	// FinalVerdict is the verdict written into DECISION_FINALISED (when Finalised),
	// or the verdict we *would* finalise to right now given the current grants/denials.
	FinalVerdict policy.Verdict `json:"final_verdict,omitempty"`

	// Grants captures the chronological list of APPROVAL_GRANTED payloads
	// for this PR after the latest DECISION_ISSUED.
	Grants []GrantPayload `json:"grants,omitempty"`
}

// Compute reads ledger events for prID and returns the approval status.
// The function tolerates missing fields gracefully — events whose payloads
// don't parse are skipped rather than failing the whole computation.
func Compute(events []ledger.Event, prID string) Status {
	st := Status{}

	// Find the most recent DECISION_ISSUED for prID; only approvals AFTER it
	// count toward the current decision.
	decisionIdx := -1
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "DECISION_ISSUED" {
			continue
		}
		var probe struct {
			PRID     string          `json:"pr_id"`
			Decision policy.Decision `json:"decision"`
		}
		if err := json.Unmarshal(events[i].Payload, &probe); err != nil {
			continue
		}
		if probe.PRID == prID {
			decisionIdx = i
			st.Decision = probe.Decision
			break
		}
	}
	if decisionIdx < 0 {
		return st
	}

	grantedSet := map[string]struct{}{}
	st.DenialsByRole = map[string]DenyPayload{}

	for i := decisionIdx + 1; i < len(events); i++ {
		e := events[i]
		switch e.Kind {
		case "APPROVAL_GRANTED":
			var p GrantPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil || p.PRID != prID {
				continue
			}
			grantedSet[p.Role] = struct{}{}
			st.Grants = append(st.Grants, p)
		case "APPROVAL_DENIED":
			var p DenyPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil || p.PRID != prID {
				continue
			}
			st.DenialsByRole[p.Role] = p
			st.Denied = true
		case "DECISION_FINALISED":
			var p FinalisedPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil || p.PRID != prID {
				continue
			}
			st.Finalised = true
			st.FinalVerdict = p.FinalVerdict
		}
	}

	for role := range grantedSet {
		st.GrantedRoles = append(st.GrantedRoles, role)
	}
	sort.Strings(st.GrantedRoles)

	// If not already finalised, compute the *would-be* final verdict given
	// the current state. This is the same logic Finalise() uses below.
	if !st.Finalised {
		st.FinalVerdict = projectedVerdict(st)
	}

	return st
}

// requiredRoles returns the set of roles the decision needs grants from
// before it can finalise to ALLOW. Empty slice means "no specific roles
// required" — any single grant satisfies (the conservative default).
func requiredRoles(d policy.Decision) []string {
	out := make([]string, 0, len(d.RequiredApprovers))
	for _, a := range d.RequiredApprovers {
		if a.Role != "" {
			out = append(out, a.Role)
		}
	}
	sort.Strings(out)
	return out
}

// projectedVerdict computes what the final verdict WOULD be right now given
// the current grants/denials. Used both by Compute (for the live view) and
// by Finalise (as the source of truth on emission).
func projectedVerdict(st Status) policy.Verdict {
	// Any denial is sticky → DENY.
	if st.Denied {
		return policy.VerdictDeny
	}
	// If the underlying decision was already ALLOW or DENY, no approvals
	// change it; finalisation just locks it in.
	switch st.Decision.Verdict {
	case policy.VerdictAllow:
		return policy.VerdictAllow
	case policy.VerdictDeny:
		return policy.VerdictDeny
	case policy.VerdictRequireApproval:
		needed := requiredRoles(st.Decision)
		granted := map[string]struct{}{}
		for _, r := range st.GrantedRoles {
			granted[r] = struct{}{}
		}
		if len(needed) == 0 {
			// No specific roles required; any single grant finalises ALLOW.
			if len(st.GrantedRoles) > 0 {
				return policy.VerdictAllow
			}
			return policy.VerdictRequireApproval
		}
		for _, role := range needed {
			if _, ok := granted[role]; !ok {
				return policy.VerdictRequireApproval
			}
		}
		return policy.VerdictAllow
	}
	return ""
}

// CanFinalise reports whether the current Status is ripe to emit a
// DECISION_FINALISED event. Returns (verdict, true) when the projected
// verdict is no longer REQUIRE_APPROVAL and DECISION_FINALISED hasn't
// already been emitted.
func CanFinalise(st Status) (policy.Verdict, bool) {
	if st.Finalised {
		return "", false
	}
	v := projectedVerdict(st)
	if v == "" || v == policy.VerdictRequireApproval {
		return "", false
	}
	return v, true
}

// BuildFinalised constructs the FinalisedPayload that should accompany the
// DECISION_FINALISED ledger event.
func BuildFinalised(st Status, prID string, now time.Time) FinalisedPayload {
	return FinalisedPayload{
		PRID:         prID,
		FinalVerdict: projectedVerdict(st),
		Decision:     st.Decision,
		Grants:       append([]GrantPayload(nil), st.Grants...),
		FinalisedAt:  now.UTC(),
	}
}
