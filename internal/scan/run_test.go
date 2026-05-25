package scan

import (
	"errors"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
)

// crashScanner deliberately returns an error so RunAll's failure-capture
// branch is exercised.
type crashScanner struct{}

func (crashScanner) Name() string { return "crashbox" }
func (crashScanner) Scan(_ aichange.AIChange, _ map[string][]byte) ([]Finding, error) {
	return nil, errors.New("simulated crash")
}

func TestRunAll_AggregatesAndSorts(t *testing.T) {
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "a.go", ChangeKind: aichange.FileAdded, AfterHash: "h1"},
			{Path: "b.go", ChangeKind: aichange.FileAdded, AfterHash: "h2"},
		},
	}
	bodies := map[string][]byte{
		"a.go": []byte("AKIAIOSFODNN7EXAMPLE\nemail=alice@example.com\n"),
		"b.go": []byte("card = 4242424242424242\n"),
	}
	findings := RunAll(DefaultScanners(), c, bodies)
	if len(findings) < 3 {
		t.Fatalf("expected ≥ 3 findings, got %d: %+v", len(findings), findings)
	}
	// Verify sort: detector ascending.
	for i := 1; i < len(findings); i++ {
		if findings[i-1].Detector > findings[i].Detector {
			t.Fatalf("not sorted by detector: %v then %v", findings[i-1], findings[i])
		}
	}
}

func TestRunAll_CapturesScannerCrash(t *testing.T) {
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "x.go", ChangeKind: aichange.FileAdded, AfterHash: "h"},
		},
	}
	findings := RunAll([]Scanner{crashScanner{}}, c, map[string][]byte{})
	if len(findings) != 1 {
		t.Fatalf("expected 1 scan_failure finding, got %d", len(findings))
	}
	if findings[0].Kind != "scan_failure" || findings[0].Detector != "crashbox" {
		t.Fatalf("unexpected finding: %+v", findings[0])
	}
	if findings[0].Severity != SeverityHigh {
		t.Errorf("scan_failure severity = %q, want high", findings[0].Severity)
	}
}

func TestDefaultScanners_HasSecretsPIIAndSupplyChain(t *testing.T) {
	names := map[string]bool{}
	for _, s := range DefaultScanners() {
		names[s.Name()] = true
	}
	for _, want := range []string{"secrets", "pii", "supply_chain"} {
		if !names[want] {
			t.Errorf("DefaultScanners missing %q", want)
		}
	}
}
