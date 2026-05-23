package scan

import (
	"testing"

	"github.com/tzone85/themis/internal/aichange"
)

func runSecrets(t *testing.T, body string) []Finding {
	t.Helper()
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "src/x.go", ChangeKind: aichange.FileAdded, AfterHash: "h"},
		},
	}
	out, err := NewSecretsScanner().Scan(c, map[string][]byte{"src/x.go": []byte(body)})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSecrets_DetectsAWSAccessKey(t *testing.T) {
	findings := runSecrets(t, "aws_id = AKIAIOSFODNN7EXAMPLE\n")
	if len(findings) == 0 {
		t.Fatal("expected at least one finding")
	}
	if findings[0].Severity != SeverityCritical {
		t.Errorf("severity = %q, want critical", findings[0].Severity)
	}
	if findings[0].File != "src/x.go" || findings[0].Line != 1 {
		t.Errorf("file/line = %q/%d", findings[0].File, findings[0].Line)
	}
}

func TestSecrets_DetectsPEMPrivateKey(t *testing.T) {
	body := "header\n-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----\n"
	f := runSecrets(t, body)
	if len(f) == 0 {
		t.Fatal("expected PEM finding")
	}
}

func TestSecrets_DetectsKVPair(t *testing.T) {
	f := runSecrets(t, `api_key = "0123456789abcdefABCDEF"`+"\n")
	if len(f) == 0 {
		t.Fatal("expected KV finding")
	}
	if f[0].Severity != SeverityHigh {
		t.Errorf("severity = %q, want high", f[0].Severity)
	}
}

func TestSecrets_IgnoresProse(t *testing.T) {
	body := "this is documentation about API keys and secret tokens.\n" +
		"we don't put credentials here because that would be bad.\n"
	if f := runSecrets(t, body); len(f) != 0 {
		t.Fatalf("prose triggered findings: %+v", f)
	}
}

func TestSecrets_IgnoresShortKVValue(t *testing.T) {
	// 8-char value is below the 16-char floor — should not fire.
	if f := runSecrets(t, `secret = "abcdef12"`+"\n"); len(f) != 0 {
		t.Fatalf("short kv triggered findings: %+v", f)
	}
}

func TestSecrets_RedactsContent(t *testing.T) {
	body := "api_key = " + `"super-secret-token-value-1234567"`
	f := runSecrets(t, body)
	if len(f) == 0 {
		t.Fatal("expected finding")
	}
	if containsString(f[0].Description, "super-secret-token") {
		t.Fatalf("description leaked secret material: %q", f[0].Description)
	}
}

func TestSecrets_SkipsDeletedFiles(t *testing.T) {
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "src/x.go", ChangeKind: aichange.FileDeleted, BeforeHash: "h"},
		},
	}
	// Even if we pass a body, the file is DELETED → scanner ignores it.
	out, err := NewSecretsScanner().Scan(c, map[string][]byte{"src/x.go": []byte("AKIAIOSFODNN7EXAMPLE\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("deleted file should not be scanned: %+v", out)
	}
}

func TestSecrets_HandlesMissingBody(t *testing.T) {
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "src/x.go", ChangeKind: aichange.FileAdded, AfterHash: "h"},
		},
	}
	// Body map missing — must not crash; returns no findings.
	out, err := NewSecretsScanner().Scan(c, map[string][]byte{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("missing body should produce no findings: %+v", out)
	}
}

func containsString(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
