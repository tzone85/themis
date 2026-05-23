package policy

import (
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/scan"
)

func mustParse(t *testing.T, raw string) Policy {
	t.Helper()
	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDecide_DocOnlyAllowed(t *testing.T) {
	p := mustParse(t, validPolicy)
	d := Decide(aichange.AIChange{}, classify.Impact{Kind: classify.KindDocOnly}, nil, p)
	if d.Verdict != VerdictAllow {
		t.Fatalf("DOC_ONLY → %q, want ALLOW", d.Verdict)
	}
}

func TestDecide_SecretFindingDenies(t *testing.T) {
	p := mustParse(t, validPolicy)
	findings := []scan.Finding{{Kind: "secret", Severity: scan.SeverityHigh}}
	d := Decide(aichange.AIChange{}, classify.Impact{Kind: classify.KindNonContract}, findings, p)
	if d.Verdict != VerdictDeny {
		t.Fatalf("secret → %q, want DENY", d.Verdict)
	}
}

func TestDecide_SchemaBreakingRequiresApproval(t *testing.T) {
	p := mustParse(t, validPolicy)
	d := Decide(aichange.AIChange{}, classify.Impact{Kind: classify.KindSchemaBreaking, Domain: "Collections"}, nil, p)
	if d.Verdict != VerdictRequireApproval {
		t.Fatalf("SCHEMA_BREAKING → %q, want REQUIRE_APPROVAL", d.Verdict)
	}
	if len(d.RequiredApprovers) != 2 {
		t.Fatalf("expected 2 required approvers, got %d", len(d.RequiredApprovers))
	}
}

func TestDecide_FallsThroughToDefault(t *testing.T) {
	p := mustParse(t, validPolicy)
	d := Decide(aichange.AIChange{}, classify.Impact{Kind: classify.KindOffCatalogue}, nil, p)
	if d.Verdict != VerdictRequireApproval {
		t.Fatalf("default → %q, want REQUIRE_APPROVAL", d.Verdict)
	}
	if d.RuleName != "<default>" {
		t.Errorf("RuleName = %q, want <default>", d.RuleName)
	}
	if len(d.RequiredApprovers) != 1 {
		t.Fatalf("default approvers = %d, want 1 (senior)", len(d.RequiredApprovers))
	}
}

func TestDecide_NoDefaultFailsClosed(t *testing.T) {
	// Construct a policy struct directly that bypasses Parse's validation
	// to simulate a corrupted-then-loaded policy in memory.
	p := Policy{Version: 1, Default: "", Rules: nil}
	d := Decide(aichange.AIChange{}, classify.Impact{Kind: classify.KindDocOnly}, nil, p)
	if d.Verdict != VerdictDeny {
		t.Fatalf("no-default → %q, want DENY (fail-closed)", d.Verdict)
	}
}

func TestDecide_SeverityThresholdGate(t *testing.T) {
	policyYAML := `
version: 1
default: ALLOW
rules:
  - name: high-severity blocks
    when:
      findings.severity: ">=high"
    then:
      verdict: DENY
      reason: high-severity finding
`
	p := mustParse(t, policyYAML)
	// low-severity finding → falls through.
	d := Decide(aichange.AIChange{}, classify.Impact{}, []scan.Finding{{Kind: "x", Severity: scan.SeverityLow}}, p)
	if d.Verdict != VerdictAllow {
		t.Fatalf("low severity → %q, want ALLOW", d.Verdict)
	}
	// high-severity finding → rule fires.
	d = Decide(aichange.AIChange{}, classify.Impact{}, []scan.Finding{{Kind: "x", Severity: scan.SeverityHigh}}, p)
	if d.Verdict != VerdictDeny {
		t.Fatalf("high severity → %q, want DENY", d.Verdict)
	}
}

func TestDecide_DomainMatchClause(t *testing.T) {
	policyYAML := `
version: 1
default: ALLOW
rules:
  - name: collections needs sign-off
    when:
      impact.domain: Collections
      impact.kind: [CONSUMER_TOUCH, PRODUCER_TOUCH, SCHEMA_BREAKING, NEW_EVENT]
    then:
      verdict: REQUIRE_APPROVAL
      required_approvers:
        - role: compliance
`
	p := mustParse(t, policyYAML)

	// Collections + CONSUMER_TOUCH → match.
	d := Decide(aichange.AIChange{}, classify.Impact{Kind: classify.KindConsumerTouch, Domain: "Collections"}, nil, p)
	if d.Verdict != VerdictRequireApproval {
		t.Fatalf("collections consumer → %q, want REQUIRE_APPROVAL", d.Verdict)
	}

	// Notifications + CONSUMER_TOUCH → no match → default.
	d = Decide(aichange.AIChange{}, classify.Impact{Kind: classify.KindConsumerTouch, Domain: "Notifications"}, nil, p)
	if d.Verdict != VerdictAllow {
		t.Fatalf("notifications consumer → %q, want ALLOW (default)", d.Verdict)
	}
}
