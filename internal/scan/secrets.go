package scan

import (
	"bufio"
	"bytes"
	"regexp"

	"github.com/tzone85/themis/internal/aichange"
)

// SecretsScanner finds credentials embedded in source. It is intentionally
// conservative: false-negatives are preferable to noisy false-positives,
// because every Finding becomes a ledger event. Higher-recall scanning is
// expected to land in Plan 4 via a pluggable detector marketplace.
type SecretsScanner struct{}

// NewSecretsScanner returns a stateless secrets detector.
func NewSecretsScanner() *SecretsScanner { return &SecretsScanner{} }

// Name implements Scanner.
func (s *SecretsScanner) Name() string { return "secrets" }

// Compiled patterns. AWS access keys, generic key=value with secret-flavoured
// keys, and PEM private-key blocks.
var (
	reAWSAccessKey  = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	rePEMPrivateKey = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	// secretKV matches `<key>=<value>` or `<key>: <value>` where key looks
	// secret-flavoured and the value is a non-empty quoted-or-bareword token
	// of ≥ 16 chars. The 16-char floor is what suppresses prose false-positives.
	reSecretKV = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?key|secret|token|password|passwd|pwd)\s*[:=]\s*["']?([A-Za-z0-9+/=_\-]{16,})["']?`)
)

// Scan implements Scanner.
func (s *SecretsScanner) Scan(c aichange.AIChange, fileBodies map[string][]byte) ([]Finding, error) {
	out := make([]Finding, 0)
	for _, ft := range c.TouchedFiles {
		if ft.ChangeKind == aichange.FileDeleted {
			continue
		}
		body, ok := fileBodies[ft.Path]
		if !ok {
			continue
		}
		out = append(out, s.scanBody(ft.Path, body)...)
	}
	return out, nil
}

// scanBody walks body line-by-line so findings carry a 1-indexed line number.
func (s *SecretsScanner) scanBody(path string, body []byte) []Finding {
	out := make([]Finding, 0)
	sc := bufio.NewScanner(bytes.NewReader(body))
	// Allow long lines (vendored .js minified bundles can blow stock 64k limit).
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if reAWSAccessKey.Find(line) != nil {
			out = append(out, Finding{
				Kind:        "secret",
				Severity:    SeverityCritical,
				File:        path,
				Line:        lineNo,
				Description: "AWS access key pattern detected on line " + itoa(lineNo),
				Detector:    "secrets",
			})
		}
		if rePEMPrivateKey.Find(line) != nil {
			out = append(out, Finding{
				Kind:        "secret",
				Severity:    SeverityCritical,
				File:        path,
				Line:        lineNo,
				Description: "PEM PRIVATE KEY block detected on line " + itoa(lineNo),
				Detector:    "secrets",
			})
		}
		if reSecretKV.Find(line) != nil {
			out = append(out, Finding{
				Kind:        "secret",
				Severity:    SeverityHigh,
				File:        path,
				Line:        lineNo,
				Description: "secret-flavoured key=value pair detected on line " + itoa(lineNo),
				Detector:    "secrets",
			})
		}
	}
	return out
}

// itoa is a small helper to avoid pulling strconv just for the description.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
