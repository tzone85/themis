package scan

import (
	"testing"

	"github.com/tzone85/themis/internal/aichange"
)

func runPII(t *testing.T, body string) []Finding {
	t.Helper()
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "src/x.go", ChangeKind: aichange.FileAdded, AfterHash: "h"},
		},
	}
	out, err := NewPIIScanner().Scan(c, map[string][]byte{"src/x.go": []byte(body)})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPII_DetectsLuhnValidCreditCard(t *testing.T) {
	// 4242424242424242 is the canonical Stripe test number — Luhn-valid.
	f := runPII(t, "card = 4242424242424242\n")
	if len(f) == 0 {
		t.Fatal("expected card finding")
	}
	if f[0].Severity != SeverityHigh || f[0].Kind != "pii" {
		t.Errorf("unexpected finding: %+v", f[0])
	}
}

func TestPII_IgnoresLuhnInvalidCardShape(t *testing.T) {
	// Same shape but flipped digit so Luhn fails.
	if f := runPII(t, "card = 4242424242424243\n"); len(f) != 0 {
		t.Fatalf("non-Luhn-valid number triggered finding: %+v", f)
	}
}

func TestPII_DetectsEmail(t *testing.T) {
	f := runPII(t, "contact: example@thandi.co.za\n")
	if len(f) == 0 {
		t.Fatal("expected email finding")
	}
	if f[0].Severity != SeverityLow {
		t.Errorf("severity = %q, want low", f[0].Severity)
	}
}

func TestPII_RedactsContent(t *testing.T) {
	f := runPII(t, "user = alice@example.com\n")
	if len(f) == 0 {
		t.Fatal("expected finding")
	}
	if containsString(f[0].Description, "alice@example.com") {
		t.Fatalf("description leaked PII: %q", f[0].Description)
	}
}

func TestPII_DetectsSAIDLike(t *testing.T) {
	// 8001015009087 is a documented test SA ID (1980-01-01, Luhn-valid).
	f := runPII(t, "id_number = 8001015009087\n")
	if len(f) == 0 {
		t.Fatal("expected SA ID finding")
	}
}

func TestPII_RejectsBadDate(t *testing.T) {
	// month 13 -> not a valid SA ID prefix.
	if f := runPII(t, "id = 8013015009087\n"); len(f) != 0 {
		// could still match the CC heuristic if Luhn-valid; ensure neither fires
		// on this specific input which is also Luhn-invalid.
		for _, x := range f {
			if x.Kind == "pii" && x.Description[:4] == "Sout" {
				t.Fatalf("month=13 should not be SAID: %+v", x)
			}
		}
	}
}

func TestPII_IgnoresPlainProse(t *testing.T) {
	if f := runPII(t, "this is just some normal prose without any PII at all.\n"); len(f) != 0 {
		t.Fatalf("prose triggered findings: %+v", f)
	}
}

func TestPII_SkipsDeletedFiles(t *testing.T) {
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "src/x.go", ChangeKind: aichange.FileDeleted, BeforeHash: "h"},
		},
	}
	out, err := NewPIIScanner().Scan(c, map[string][]byte{"src/x.go": []byte("alice@example.com\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("deleted file should be skipped: %+v", out)
	}
}
