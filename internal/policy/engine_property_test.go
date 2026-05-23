package policy

import (
	"encoding/json"
	"testing"

	"pgregory.net/rapid"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/scan"
)

func drawImpact(rt *rapid.T) classify.Impact {
	kinds := []classify.Kind{
		classify.KindDocOnly, classify.KindOffCatalogue, classify.KindNonContract,
		classify.KindConsumerTouch, classify.KindProducerTouch, classify.KindNewEvent, classify.KindSchemaBreaking,
	}
	domains := []string{"", "Collections", "Notifications", "Risk"}
	return classify.Impact{
		Kind:   rapid.SampledFrom(kinds).Draw(rt, "impact_kind"),
		Domain: rapid.SampledFrom(domains).Draw(rt, "impact_domain"),
	}
}

func drawFindings(rt *rapid.T) []scan.Finding {
	severities := []scan.Severity{scan.SeverityInfo, scan.SeverityLow, scan.SeverityMed, scan.SeverityHigh, scan.SeverityCritical}
	kinds := []string{"secret", "pii", "scan_failure", "other"}
	n := rapid.IntRange(0, 5).Draw(rt, "n")
	out := make([]scan.Finding, 0, n)
	for range n {
		out = append(out, scan.Finding{
			Kind:     rapid.SampledFrom(kinds).Draw(rt, "kind"),
			Severity: rapid.SampledFrom(severities).Draw(rt, "sev"),
		})
	}
	return out
}

// TestPropDecide_Deterministic: same inputs → same Decision bytes.
func TestPropDecide_Deterministic(t *testing.T) {
	p := mustParse(t, validPolicy)
	rapid.Check(t, func(rt *rapid.T) {
		imp := drawImpact(rt)
		findings := drawFindings(rt)
		a := Decide(aichange.AIChange{}, imp, findings, p)
		b := Decide(aichange.AIChange{}, imp, findings, p)
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if string(ja) != string(jb) {
			rt.Fatalf("Decide non-deterministic:\n  a=%s\n  b=%s", ja, jb)
		}
	})
}

// TestPropDecide_NoDefaultAlwaysDenies: a policy with no default + no rules
// always returns DENY regardless of inputs.
func TestPropDecide_NoDefaultAlwaysDenies(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		imp := drawImpact(rt)
		findings := drawFindings(rt)
		d := Decide(aichange.AIChange{}, imp, findings, Policy{Version: 1})
		if d.Verdict != VerdictDeny {
			rt.Fatalf("no-default policy returned %q; must DENY (fail-closed)", d.Verdict)
		}
	})
}

// TestPropDecide_SecretRuleAlwaysDeniesWhenSecretPresent: with the canonical
// validPolicy, any inputs that include at least one secret-kind finding
// must result in DENY.
func TestPropDecide_SecretRuleAlwaysDeniesWhenSecretPresent(t *testing.T) {
	p := mustParse(t, validPolicy)
	rapid.Check(t, func(rt *rapid.T) {
		imp := drawImpact(rt)
		base := drawFindings(rt)
		findings := append(base, scan.Finding{Kind: "secret", Severity: scan.SeverityHigh})

		// The "doc-only allowed" rule fires before the secret rule when
		// impact is DOC_ONLY, so we exclude that case from the property —
		// it is a deliberately-permitted ordering of rules.
		if imp.Kind == classify.KindDocOnly {
			return
		}

		d := Decide(aichange.AIChange{}, imp, findings, p)
		if d.Verdict != VerdictDeny {
			rt.Fatalf("secret-present non-doc-only inputs returned %q; expected DENY", d.Verdict)
		}
	})
}
