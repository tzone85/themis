package advisor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
)

func TestNullLLM_AllowSuggestsNoAction(t *testing.T) {
	out, err := Draft(context.Background(), NullLLM{}, Input{
		PRID:     "gh:test#1",
		Actor:    "claude_code",
		Impact:   classify.Impact{Kind: classify.KindDocOnly},
		Decision: policy.Decision{Verdict: policy.VerdictAllow},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "automatically") {
		t.Fatalf("expected ALLOW phrasing, got %q", out.Text)
	}
	if out.Summary.Verdict != "ALLOW" {
		t.Fatalf("summary verdict = %q", out.Summary.Verdict)
	}
	if out.BackedBy != "null" {
		t.Fatalf("backed_by = %q", out.BackedBy)
	}
}

func TestNullLLM_DenyExplainsEscalation(t *testing.T) {
	out, _ := Draft(context.Background(), NullLLM{}, Input{
		PRID:     "x",
		Actor:    "y",
		Impact:   classify.Impact{Kind: classify.KindSchemaBreaking},
		Decision: policy.Decision{Verdict: policy.VerdictDeny},
		Findings: []scan.Finding{
			{Kind: "secret", Severity: scan.SeverityCritical},
		},
	})
	if !strings.Contains(out.Text, "emergency override") {
		t.Fatalf("expected escalation hint, got %q", out.Text)
	}
	if len(out.Summary.HighSeverityKinds) != 1 || out.Summary.HighSeverityKinds[0] != "secret" {
		t.Fatalf("summary high-sev kinds = %+v", out.Summary.HighSeverityKinds)
	}
}

func TestNullLLM_RequireApprovalAsksForApprovers(t *testing.T) {
	out, _ := Draft(context.Background(), NullLLM{}, Input{
		PRID:     "x",
		Impact:   classify.Impact{Kind: classify.KindConsumerTouch},
		Decision: policy.Decision{Verdict: policy.VerdictRequireApproval},
	})
	if !strings.Contains(out.Text, "Approvers") {
		t.Fatalf("expected approver mention, got %q", out.Text)
	}
}

func TestDraft_RequiresPRID(t *testing.T) {
	_, err := Draft(context.Background(), NullLLM{}, Input{})
	if !errors.Is(err, ErrEmptyInput) {
		t.Fatalf("expected ErrEmptyInput, got %v", err)
	}
}

func TestDraft_NilLLMUsesNullLLM(t *testing.T) {
	out, err := Draft(context.Background(), nil, Input{
		PRID:     "x",
		Decision: policy.Decision{Verdict: policy.VerdictAllow},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.BackedBy != "null" {
		t.Fatalf("backed_by = %q", out.BackedBy)
	}
}

func TestSummarise_AggregatesHighSeverityFindings(t *testing.T) {
	out := summarise(Input{
		Findings: []scan.Finding{
			{Kind: "secret", Severity: scan.SeverityCritical},
			{Kind: "pii", Severity: scan.SeverityHigh},
			{Kind: "noise", Severity: scan.SeverityInfo},
		},
	})
	if out.FindingsCount != 3 {
		t.Fatalf("count = %d, want 3", out.FindingsCount)
	}
	want := []string{"pii", "secret"}
	if len(out.HighSeverityKinds) != 2 {
		t.Fatalf("hi-sev kinds = %+v", out.HighSeverityKinds)
	}
	for i := range want {
		if out.HighSeverityKinds[i] != want[i] {
			t.Fatalf("hi-sev[%d] = %q, want %q", i, out.HighSeverityKinds[i], want[i])
		}
	}
}

func TestBuildPrompt_Deterministic(t *testing.T) {
	in := Input{
		PRID: "p", Actor: "a",
		Impact:   classify.Impact{Kind: classify.KindDocOnly, Reason: "docs"},
		Decision: policy.Decision{Verdict: policy.VerdictAllow, RuleName: "doc-only"},
	}
	if buildPrompt(in) != buildPrompt(in) {
		t.Fatal("buildPrompt should be deterministic")
	}
}

// errorLLM lets us probe the error-propagation path.
type errorLLM struct{}

func (errorLLM) Name() string                                  { return "broken" }
func (errorLLM) Generate(_ context.Context, _ string) (string, error) {
	return "", errors.New("llm exploded")
}

func TestDraft_PropagatesLLMError(t *testing.T) {
	_, err := Draft(context.Background(), errorLLM{}, Input{
		PRID:     "x",
		Decision: policy.Decision{Verdict: policy.VerdictAllow},
	})
	if err == nil {
		t.Fatal("expected error from broken LLM")
	}
}
