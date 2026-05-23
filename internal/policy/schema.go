// Package policy is the YAML-driven rule engine. The engine itself is a
// pure function — see engine.go for `Decide(...)`. This file defines the
// shapes the YAML parser produces.
package policy

// Verdict is the policy engine's output category.
type Verdict string

const (
	// VerdictAllow lets the PR through without further approval.
	VerdictAllow Verdict = "ALLOW"
	// VerdictRequireApproval forces a human (or named role) sign-off before merge.
	VerdictRequireApproval Verdict = "REQUIRE_APPROVAL"
	// VerdictDeny blocks the PR; only an emergency override may clear it.
	VerdictDeny Verdict = "DENY"
)

// Approver names a role required to sign off on REQUIRE_APPROVAL verdicts.
type Approver struct {
	Role string `yaml:"role" json:"role"`
}

// MatchClause is the `when:` portion of a rule. Every field is optional;
// an empty MatchClause matches anything.
type MatchClause struct {
	// ImpactKind matches if the Impact's Kind appears in this list. Empty = wildcard.
	ImpactKind []string `yaml:"impact.kind" json:"impact_kind,omitempty"`
	// ImpactDomain matches against Impact.Domain. Empty = wildcard.
	ImpactDomain string `yaml:"impact.domain" json:"impact_domain,omitempty"`
	// FindingKind matches if ANY finding has this kind.
	FindingKind string `yaml:"findings.kind" json:"findings_kind,omitempty"`
	// FindingSeverityMin (e.g. ">=high") requires at least one finding at or above
	// that severity. Format: ">=<severity>". An empty string disables the check.
	FindingSeverityMin string `yaml:"findings.severity" json:"findings_severity,omitempty"`
}

// ThenClause is the `then:` portion of a rule.
type ThenClause struct {
	Verdict           Verdict    `yaml:"verdict" json:"verdict"`
	RequiredApprovers []Approver `yaml:"required_approvers" json:"required_approvers,omitempty"`
	Reason            string     `yaml:"reason" json:"reason,omitempty"`
}

// Rule is one named rule in the policy.
type Rule struct {
	Name string      `yaml:"name" json:"name"`
	When MatchClause `yaml:"when" json:"when"`
	Then ThenClause  `yaml:"then" json:"then"`
}

// Policy is the parsed YAML document.
type Policy struct {
	Version                    int        `yaml:"version" json:"version"`
	Default                    Verdict    `yaml:"default" json:"default"`
	RequiredApproversForDefault []Approver `yaml:"required_approvers_for_default" json:"required_approvers_for_default,omitempty"`
	Rules                      []Rule     `yaml:"rules" json:"rules"`
}
