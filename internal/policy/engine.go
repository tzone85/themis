package policy

import (
	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/scan"
)

// Decide runs the policy engine. It is a pure function — same inputs always
// return the same Decision — which is what makes audit-replay credible:
// re-running Decide against the historical inputs reproduces the historical
// verdict bit-for-bit.
//
// Semantics:
//   - Rules are evaluated in YAML order; the first matching rule wins.
//   - If no rule matches, the policy's `default` verdict applies (carrying
//     `required_approvers_for_default` when the default is REQUIRE_APPROVAL).
//   - If the policy has no default (which Parse rejects, but defensive code
//     handles the gap), the engine returns a DENY with a fail-closed reason.
func Decide(_ aichange.AIChange, imp classify.Impact, findings []scan.Finding, p Policy) Decision {
	for _, r := range p.Rules {
		if ruleMatches(r.When, imp, findings) {
			return Decision{
				Verdict:           r.Then.Verdict,
				Reason:            r.Then.Reason,
				RuleName:          r.Name,
				RequiredApprovers: append([]Approver(nil), r.Then.RequiredApprovers...),
			}
		}
	}
	if p.Default == "" {
		return Decision{
			Verdict:  VerdictDeny,
			Reason:   "policy has no default verdict (fail-closed)",
			RuleName: "<default>",
		}
	}
	return Decision{
		Verdict:           p.Default,
		Reason:            "no rule matched; fell through to default",
		RuleName:          "<default>",
		RequiredApprovers: append([]Approver(nil), p.RequiredApproversForDefault...),
	}
}

// ruleMatches returns true iff every populated clause field matches.
// Empty fields are wildcards.
func ruleMatches(when MatchClause, imp classify.Impact, findings []scan.Finding) bool {
	if len(when.ImpactKind) > 0 && !containsString(when.ImpactKind, string(imp.Kind)) {
		return false
	}
	if when.ImpactDomain != "" && when.ImpactDomain != imp.Domain {
		return false
	}
	if when.FindingKind != "" {
		match := false
		for _, f := range findings {
			if f.Kind == when.FindingKind {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	if when.FindingSeverityMin != "" {
		minSev := scan.Severity(when.FindingSeverityMin[2:]) // ">=high" → "high"
		minRank := minSev.Rank()
		match := false
		for _, f := range findings {
			if f.Severity.Rank() >= minRank {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

func containsString(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
