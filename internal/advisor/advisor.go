// Package advisor implements Themis's advisory agent — the LLM-backed
// reviewer that drafts human-readable notes about a Decision/BOM. Per
// design spec §5.1, the advisor is *never* on the trust-critical path:
// it produces suggestion text only; the deterministic policy engine still
// issues the verdict.
//
// Plan 17 ships:
//
//   - An `LLM` interface so a real provider plugs in.
//   - A `NullLLM` that produces deterministic, template-based notes — useful
//     for tests, air-gapped deployments, and CI runs where reaching an LLM
//     provider isn't acceptable.
//   - A `Draft(...)` helper that composes the prompt context (Impact +
//     Findings + Decision) and calls the LLM.
//
// The output is always plain text + a structured summary; callers store
// it in the Mempalace wing alongside the decision so reviewers can read
// the advisor's take from the dashboard.
package advisor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
)

// LLM is the pluggable interface every advisor backend implements.
//
// Plan 17 ships only NullLLM; a real provider (OpenAI, Anthropic, local
// llama.cpp) drops in by implementing this interface — Draft() doesn't
// change.
type LLM interface {
	Name() string
	// Generate returns the advisory text for the supplied prompt + context.
	// Implementations MUST be deterministic when context.Canceled is set
	// — long-running LLM calls must be interruptible.
	Generate(ctx context.Context, prompt string) (string, error)
}

// Input is the structured context the advisor consumes.
type Input struct {
	PRID     string
	Actor    string
	Impact   classify.Impact
	Findings []scan.Finding
	Decision policy.Decision
}

// Output carries the advisor's response. Text is the human-facing draft;
// Summary is a structured roll-up the dashboard renders as chips.
type Output struct {
	Text       string   `json:"text"`
	Summary    Summary  `json:"summary"`
	Suggestion string   `json:"suggestion,omitempty"`
	BackedBy   string   `json:"backed_by"` // LLM.Name() of the provider used
	Warnings   []string `json:"warnings,omitempty"`
}

// Summary is the structured tl;dr.
type Summary struct {
	Verdict        string   `json:"verdict"`
	ImpactKind     string   `json:"impact_kind"`
	FindingsCount  int      `json:"findings_count"`
	HighSeverityKinds []string `json:"high_severity_kinds,omitempty"`
}

// ErrEmptyInput surfaces caller-side problems.
var ErrEmptyInput = errors.New("advisor: empty input")

// Draft builds the prompt, calls the LLM, and returns the structured
// Output. The deterministic NullLLM is used as a fallback when llm==nil.
func Draft(ctx context.Context, llm LLM, in Input) (Output, error) {
	if in.PRID == "" {
		return Output{}, fmt.Errorf("%w: pr_id required", ErrEmptyInput)
	}
	if llm == nil {
		llm = NullLLM{}
	}
	prompt := buildPrompt(in)
	text, err := llm.Generate(ctx, prompt)
	if err != nil {
		return Output{}, fmt.Errorf("advisor llm: %w", err)
	}
	return Output{
		Text:     text,
		Summary:  summarise(in),
		BackedBy: llm.Name(),
	}, nil
}

// buildPrompt composes the deterministic prompt — same inputs produce the
// same prompt bytes, which is what makes audit replay work even for the
// non-deterministic LLM call (the prompt is recorded; the response is
// what varies).
func buildPrompt(in Input) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Draft a brief review note for the following AI-touched pull request.\n\n")
	fmt.Fprintf(&b, "PR: %s\nActor: %s\n", in.PRID, in.Actor)
	fmt.Fprintf(&b, "Impact: %s", in.Impact.Kind)
	if in.Impact.Domain != "" {
		fmt.Fprintf(&b, " (domain=%s)", in.Impact.Domain)
	}
	if in.Impact.Reason != "" {
		fmt.Fprintf(&b, " — %s", in.Impact.Reason)
	}
	fmt.Fprintf(&b, "\nVerdict: %s", in.Decision.Verdict)
	if in.Decision.RuleName != "" {
		fmt.Fprintf(&b, " (rule=%s)", in.Decision.RuleName)
	}
	fmt.Fprintf(&b, "\n\nFindings (%d):\n", len(in.Findings))
	for _, f := range in.Findings {
		fmt.Fprintf(&b, "  - %s [%s] %s\n", f.Kind, f.Severity, f.Description)
	}
	return b.String()
}

func summarise(in Input) Summary {
	hi := map[string]struct{}{}
	for _, f := range in.Findings {
		if f.Severity == scan.SeverityHigh || f.Severity == scan.SeverityCritical {
			hi[f.Kind] = struct{}{}
		}
	}
	out := Summary{
		Verdict:       string(in.Decision.Verdict),
		ImpactKind:    string(in.Impact.Kind),
		FindingsCount: len(in.Findings),
	}
	for k := range hi {
		out.HighSeverityKinds = append(out.HighSeverityKinds, k)
	}
	sort.Strings(out.HighSeverityKinds)
	return out
}

// --- NullLLM ----------------------------------------------------------------

// NullLLM is the deterministic, template-based backend. It produces a
// plain-language summary built from the structured inputs only — never
// hallucinates and never depends on a network. Use it in:
//
//   - tests (so the prompt → output mapping is stable);
//   - air-gapped deployments (no upstream allowed);
//   - the first stage of a fallback chain (real LLM first, NullLLM if down).
type NullLLM struct{}

// Name implements LLM.
func (NullLLM) Name() string { return "null" }

// Generate implements LLM. The prompt parameter is ignored — NullLLM
// re-derives its output from a fixed template applied to the prompt's
// header lines. Determinism is the entire point.
func (NullLLM) Generate(_ context.Context, prompt string) (string, error) {
	var verdict, impact string
	for _, line := range strings.Split(prompt, "\n") {
		switch {
		case strings.HasPrefix(line, "Verdict:"):
			verdict = strings.TrimSpace(strings.TrimPrefix(line, "Verdict:"))
			if idx := strings.Index(verdict, " "); idx >= 0 {
				verdict = verdict[:idx]
			}
		case strings.HasPrefix(line, "Impact:"):
			impact = strings.TrimSpace(strings.TrimPrefix(line, "Impact:"))
			if idx := strings.Index(impact, " "); idx >= 0 {
				impact = impact[:idx]
			}
		}
	}
	if verdict == "" {
		verdict = "unknown"
	}
	if impact == "" {
		impact = "unknown"
	}
	switch verdict {
	case "ALLOW":
		return "Themis allowed this change automatically: classified as " + impact + " with no blocking findings. No additional reviewer action is needed.", nil
	case "REQUIRE_APPROVAL":
		return "Themis flagged this change for review: impact is " + impact + ". Approvers should confirm the listed findings and grant the required role(s) before merge.", nil
	case "DENY":
		return "Themis denied this change: impact is " + impact + " and at least one blocking finding was raised. Resolve the findings or escalate via emergency override (compliance + co-signer) before merging.", nil
	}
	return "Themis decision is " + verdict + " (impact " + impact + "). Review the structured payload for details.", nil
}
