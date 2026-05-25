package pipeline

import (
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/catalogue"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
)

const validPolicy = `version: 1
default: REQUIRE_APPROVAL
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
`

func openStore(t *testing.T) (*ledger.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	s, err := ledger.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}

func mustParsePolicy(t *testing.T, raw string) policy.Policy {
	t.Helper()
	p, err := policy.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRun_DocOnlyAllows(t *testing.T) {
	s, _ := openStore(t)
	ai := aichange.AIChange{
		PRID:  "x", Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"}},
	}
	res, err := Run(s, "tnt", ai, catalogue.CatalogueGraph{}, mustParsePolicy(t, validPolicy), map[string][]byte{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision.Verdict != policy.VerdictAllow {
		t.Fatalf("verdict = %q, want ALLOW", res.Decision.Verdict)
	}
	if res.Impact.Kind != classify.KindDocOnly {
		t.Fatalf("impact = %q, want DOC_ONLY", res.Impact.Kind)
	}
}

func TestRun_SecretFindingDenies(t *testing.T) {
	s, _ := openStore(t)
	ai := aichange.AIChange{
		PRID:  "y", Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{{Path: "src/leak.go", ChangeKind: aichange.FileAdded, AfterHash: "h"}},
	}
	bodies := map[string][]byte{"src/leak.go": []byte("aws_id = AKIAIOSFODNN7EXAMPLE\n")}
	res, err := Run(s, "tnt", ai, catalogue.CatalogueGraph{}, mustParsePolicy(t, validPolicy), bodies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision.Verdict != policy.VerdictDeny {
		t.Fatalf("verdict = %q, want DENY", res.Decision.Verdict)
	}
	if len(res.Findings) == 0 {
		t.Fatal("expected findings, got none")
	}
}

func TestRun_EmitsExpectedLedgerKinds(t *testing.T) {
	s, dir := openStore(t)
	ai := aichange.AIChange{
		PRID:  "z", Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{{Path: "src/leak.go", ChangeKind: aichange.FileAdded, AfterHash: "h"}},
	}
	bodies := map[string][]byte{"src/leak.go": []byte("aws_id = AKIAIOSFODNN7EXAMPLE\n")}
	if _, err := Run(s, "tnt", ai, catalogue.CatalogueGraph{}, mustParsePolicy(t, validPolicy), bodies, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	events, err := ledger.ReadAll(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, e := range events {
		seen[e.Kind]++
	}
	if seen["SCAN_FINDING"] < 1 {
		t.Errorf("expected ≥1 SCAN_FINDING; got %d", seen["SCAN_FINDING"])
	}
	if seen["DECISION_ISSUED"] != 1 {
		t.Errorf("expected exactly 1 DECISION_ISSUED; got %d", seen["DECISION_ISSUED"])
	}
}

// crashing scanner exercises the per-scanner error capture path.
type crashScanner struct{}

func (crashScanner) Name() string { return "boom" }
func (crashScanner) Scan(_ aichange.AIChange, _ map[string][]byte) ([]scan.Finding, error) {
	return nil, errBoom
}

var errBoom = errBoomT{}

type errBoomT struct{}

func (errBoomT) Error() string { return "boom" }

func TestRun_AcceptsCustomScannerSet(t *testing.T) {
	s, _ := openStore(t)
	ai := aichange.AIChange{
		PRID:  "x", Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{{Path: "src/x.go", ChangeKind: aichange.FileAdded, AfterHash: "h"}},
	}
	res, err := Run(s, "tnt", ai, catalogue.CatalogueGraph{}, mustParsePolicy(t, validPolicy), nil, []scan.Scanner{crashScanner{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 || res.Findings[0].Kind != "scan_failure" {
		t.Fatalf("expected one scan_failure finding, got %+v", res.Findings)
	}
}
