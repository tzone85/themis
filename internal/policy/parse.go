package policy

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ErrPolicyInvalid is wrapped by every parse error so callers can route
// to a POLICY_INVALID ledger event regardless of which check failed.
var ErrPolicyInvalid = errors.New("policy: invalid")

// supportedVersions lists the policy schema versions Themis can parse. New
// versions are added explicitly so old tenants don't silently load policies
// with semantics they don't understand.
var supportedVersions = map[int]bool{1: true}

// Parse decodes a YAML policy document and validates its top-level shape.
//
// Fail-closed semantics: any structural error returns ErrPolicyInvalid so
// the calling CLI emits POLICY_INVALID and refuses to issue decisions for
// the tenant until the file is fixed. (Design spec §8.2.)
func Parse(raw []byte) (Policy, error) {
	var p Policy
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return Policy{}, fmt.Errorf("%w: yaml unmarshal: %v", ErrPolicyInvalid, err)
	}
	if p.Version == 0 {
		return Policy{}, fmt.Errorf("%w: missing 'version' field", ErrPolicyInvalid)
	}
	if !supportedVersions[p.Version] {
		return Policy{}, fmt.Errorf("%w: unsupported version %d", ErrPolicyInvalid, p.Version)
	}
	if p.Default == "" {
		return Policy{}, fmt.Errorf("%w: missing 'default' verdict (fail-closed; declare ALLOW/REQUIRE_APPROVAL/DENY explicitly)", ErrPolicyInvalid)
	}
	if !validVerdict(p.Default) {
		return Policy{}, fmt.Errorf("%w: default %q is not a valid verdict", ErrPolicyInvalid, p.Default)
	}
	for i, r := range p.Rules {
		if r.Name == "" {
			return Policy{}, fmt.Errorf("%w: rules[%d] missing name", ErrPolicyInvalid, i)
		}
		if !validVerdict(r.Then.Verdict) {
			return Policy{}, fmt.Errorf("%w: rules[%d] (%s) verdict %q invalid", ErrPolicyInvalid, i, r.Name, r.Then.Verdict)
		}
		if r.When.FindingSeverityMin != "" && !validSeverityClause(r.When.FindingSeverityMin) {
			return Policy{}, fmt.Errorf("%w: rules[%d] (%s) findings.severity %q must be '>=info|low|med|high|critical'",
				ErrPolicyInvalid, i, r.Name, r.When.FindingSeverityMin)
		}
	}
	return p, nil
}

func validVerdict(v Verdict) bool {
	switch v {
	case VerdictAllow, VerdictRequireApproval, VerdictDeny:
		return true
	}
	return false
}

// validSeverityClause accepts ">=info", ">=low", ">=med", ">=high", ">=critical".
func validSeverityClause(s string) bool {
	if len(s) < 3 || s[:2] != ">=" {
		return false
	}
	switch s[2:] {
	case "info", "low", "med", "high", "critical":
		return true
	}
	return false
}
