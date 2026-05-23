package bom

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
)

func sampleBOM() BOM {
	return BOM{
		SchemaVersion: CurrentSchemaVersion,
		PRID:          "gh:tzone85/themis#42",
		Tenant:        "acme",
		Actor:         "claude_code",
		BuiltAt:       time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
		AIChange: aichange.AIChange{
			PRID:  "gh:tzone85/themis#42",
			Actor: "claude_code",
			TouchedFiles: []aichange.FileTouch{
				{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
			},
		},
		Impact: classify.Impact{Kind: classify.KindDocOnly, Reason: "every touched file is documentation"},
		Findings: []scan.Finding{
			{Kind: "scan_failure", Severity: scan.SeverityHigh, Detector: "test", Description: "redacted"},
		},
		Decision: policy.Decision{Verdict: policy.VerdictAllow, RuleName: "doc-only allowed"},
		LedgerTip: "abcd1234",
	}
}

func TestCanonical_DeterministicForSameInputs(t *testing.T) {
	a, err := Canonical(sampleBOM())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Canonical(sampleBOM())
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("canonical differs across calls:\n  a=%s\n  b=%s", a, b)
	}
}

func TestCanonical_TimezoneAgnostic(t *testing.T) {
	// Same BuiltAt instant, expressed in two different zones.
	loc, _ := time.LoadLocation("Africa/Johannesburg")
	bUTC := sampleBOM()
	bSAST := sampleBOM()
	bSAST.BuiltAt = bUTC.BuiltAt.In(loc)

	a, err := Canonical(bUTC)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Canonical(bSAST)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("canonical depends on TZ; same instant in different zones produced different bytes")
	}
}

func TestCanonical_SensitiveToFieldEdits(t *testing.T) {
	base := sampleBOM()
	hash1, _ := Hash(base)

	edits := []func(*BOM){
		func(b *BOM) { b.PRID = "gh:other/repo#1" },
		func(b *BOM) { b.Actor = "vxd" },
		func(b *BOM) { b.Tenant = "different" },
		func(b *BOM) { b.Decision.Verdict = policy.VerdictDeny },
		func(b *BOM) { b.Impact.Kind = classify.KindSchemaBreaking },
		func(b *BOM) { b.LedgerTip = "FEED" },
		func(b *BOM) { b.BuiltAt = b.BuiltAt.Add(time.Second) },
		func(b *BOM) {
			b.Findings = append([]scan.Finding{}, b.Findings...)
			b.Findings[0].Severity = scan.SeverityCritical
		},
	}
	for i, mutate := range edits {
		variant := base
		mutate(&variant)
		h2, err := Hash(variant)
		if err != nil {
			t.Fatal(err)
		}
		if h2 == hash1 {
			t.Errorf("edit #%d did not change hash", i)
		}
	}
}

func TestCanonical_RoundTripsThroughJSON(t *testing.T) {
	in := sampleBOM()
	raw, err := Canonical(in)
	if err != nil {
		t.Fatal(err)
	}
	var out BOM
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.SchemaVersion != in.SchemaVersion || out.PRID != in.PRID || out.Decision.Verdict != in.Decision.Verdict {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestCanonical_NilFindingsEmitAsEmpty(t *testing.T) {
	b := sampleBOM()
	b.Findings = nil
	raw, err := Canonical(b)
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	if probe["findings"] == nil {
		t.Fatal("findings should be []; got null")
	}
}

func TestHash_ReturnsHexSHA256(t *testing.T) {
	h, err := Hash(sampleBOM())
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 64 {
		t.Fatalf("Hash length %d, want 64 (hex sha256)", len(h))
	}
}
