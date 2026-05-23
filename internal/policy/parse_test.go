package policy

import (
	"errors"
	"testing"
)

const validPolicy = `
version: 1
default: REQUIRE_APPROVAL
required_approvers_for_default:
  - role: senior
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
      reason: secret detected
  - name: schema breaking
    when:
      impact.kind: [SCHEMA_BREAKING]
    then:
      verdict: REQUIRE_APPROVAL
      required_approvers:
        - role: senior
        - role: compliance
`

func TestParse_LoadsValidPolicy(t *testing.T) {
	p, err := Parse([]byte(validPolicy))
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.Default != VerdictRequireApproval || len(p.Rules) != 3 {
		t.Fatalf("policy parsed wrong: %+v", p)
	}
	if p.Rules[1].Then.Verdict != VerdictDeny {
		t.Errorf("rule[1] verdict = %q", p.Rules[1].Then.Verdict)
	}
}

func TestParse_RejectsMissingVersion(t *testing.T) {
	_, err := Parse([]byte("default: ALLOW\n"))
	if err == nil {
		t.Fatal("missing version should fail")
	}
	if !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("error %v should wrap ErrPolicyInvalid", err)
	}
}

func TestParse_RejectsUnknownVersion(t *testing.T) {
	_, err := Parse([]byte("version: 99\ndefault: ALLOW\n"))
	if err == nil || !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("unknown version should fail: %v", err)
	}
}

func TestParse_RejectsMissingDefault(t *testing.T) {
	_, err := Parse([]byte("version: 1\nrules: []\n"))
	if err == nil || !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("missing default should fail: %v", err)
	}
}

func TestParse_RejectsBadDefaultVerdict(t *testing.T) {
	_, err := Parse([]byte("version: 1\ndefault: WHATEVER\n"))
	if err == nil || !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("bad default should fail: %v", err)
	}
}

func TestParse_RejectsNamelessRule(t *testing.T) {
	body := "version: 1\ndefault: ALLOW\nrules:\n  - then:\n      verdict: ALLOW\n"
	_, err := Parse([]byte(body))
	if err == nil || !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("nameless rule should fail: %v", err)
	}
}

func TestParse_RejectsBadRuleVerdict(t *testing.T) {
	body := "version: 1\ndefault: ALLOW\nrules:\n  - name: r\n    then:\n      verdict: SHRUG\n"
	_, err := Parse([]byte(body))
	if err == nil || !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("bad rule verdict should fail: %v", err)
	}
}

func TestParse_RejectsBadSeverityClause(t *testing.T) {
	body := "version: 1\ndefault: ALLOW\nrules:\n  - name: r\n    when:\n      findings.severity: \"=high\"\n    then:\n      verdict: DENY\n"
	_, err := Parse([]byte(body))
	if err == nil || !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("bad severity clause should fail: %v", err)
	}
}

func TestParse_RejectsMalformedYAML(t *testing.T) {
	_, err := Parse([]byte("::not yaml::"))
	if err == nil || !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("garbled yaml should fail: %v", err)
	}
}
