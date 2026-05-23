package scan

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/tzone85/themis/internal/aichange"
)

// TestPropSecrets_NoFalsePositiveOnLowEntropyProse asserts the secrets
// scanner never fires on text composed exclusively of letters and ASCII
// punctuation. This is the bound on false-positive noise that makes the
// scanner safe to enable by default.
func TestPropSecrets_NoFalsePositiveOnLowEntropyProse(t *testing.T) {
	scanner := NewSecretsScanner()
	rapid.Check(t, func(rt *rapid.T) {
		body := rapid.StringMatching(`[a-zA-Z ,.\-]{0,200}`).Draw(rt, "prose")

		// Drop anything that happens to land "AKIA" + 16 upper-alphanumerics —
		// strictly speaking that's still prose by our alphabet but the regex
		// is intentionally specific so it would correctly fire.
		if strings.Contains(body, "AKIA") {
			return
		}

		c := aichange.AIChange{
			TouchedFiles: []aichange.FileTouch{
				{Path: "src/prose.md", ChangeKind: aichange.FileAdded, AfterHash: "h"},
			},
		}
		findings, err := scanner.Scan(c, map[string][]byte{"src/prose.md": []byte(body)})
		if err != nil {
			rt.Fatal(err)
		}
		if len(findings) != 0 {
			rt.Fatalf("prose body %q triggered findings: %+v", body, findings)
		}
	})
}

// TestPropSecrets_AWSKeysAlwaysDetected confirms the inverse property:
// any line whose payload starts with the AWS prefix + 16 upper-alphanumerics
// is always flagged, regardless of surrounding bytes.
func TestPropSecrets_AWSKeysAlwaysDetected(t *testing.T) {
	scanner := NewSecretsScanner()
	rapid.Check(t, func(rt *rapid.T) {
		suffix := rapid.StringMatching(`[A-Z0-9]{16}`).Draw(rt, "suffix")
		prefix := rapid.StringMatching(`[a-z ]{0,20}`).Draw(rt, "prefix")
		body := prefix + "AKIA" + suffix + "\n"

		c := aichange.AIChange{
			TouchedFiles: []aichange.FileTouch{
				{Path: "src/x.go", ChangeKind: aichange.FileAdded, AfterHash: "h"},
			},
		}
		findings, err := scanner.Scan(c, map[string][]byte{"src/x.go": []byte(body)})
		if err != nil {
			rt.Fatal(err)
		}
		if len(findings) == 0 {
			rt.Fatalf("AWS key body %q failed to fire", body)
		}
	})
}
