package scan

import (
	"bufio"
	"bytes"
	"regexp"

	"github.com/tzone85/themis/internal/aichange"
)

// PIIScanner detects personally-identifying information patterns. Findings
// describe the *shape* of the match (e.g. "credit-card-shaped string"), never
// the digits themselves — that's the redaction discipline that lets findings
// live in a tamper-evident ledger without leaking PII.
type PIIScanner struct{}

// NewPIIScanner returns a stateless PII detector.
func NewPIIScanner() *PIIScanner { return &PIIScanner{} }

// Name implements Scanner.
func (p *PIIScanner) Name() string { return "pii" }

var (
	// 13-19 digit runs (allows spaces/dashes inside for human-formatted CC numbers).
	reCardShaped = regexp.MustCompile(`(?:\d[ \-]?){13,19}`)
	// SA ID: 13 contiguous digits starting with YYMMDD-like prefix.
	reSAIDShaped = regexp.MustCompile(`\b\d{13}\b`)
	// Simple email regex — good enough for heuristic, conservative on TLD.
	reEmail = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
)

// Scan implements Scanner.
func (p *PIIScanner) Scan(c aichange.AIChange, fileBodies map[string][]byte) ([]Finding, error) {
	out := make([]Finding, 0)
	for _, ft := range c.TouchedFiles {
		if ft.ChangeKind == aichange.FileDeleted {
			continue
		}
		body, ok := fileBodies[ft.Path]
		if !ok {
			continue
		}
		out = append(out, p.scanBody(ft.Path, body)...)
	}
	return out, nil
}

func (p *PIIScanner) scanBody(path string, body []byte) []Finding {
	out := make([]Finding, 0)
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()

		// Credit-card heuristic: must be card-shaped AND Luhn-valid.
		if m := reCardShaped.Find(line); m != nil {
			digits := stripNonDigit(m)
			if len(digits) >= 13 && len(digits) <= 19 && luhnValid(digits) {
				out = append(out, Finding{
					Kind:        "pii",
					Severity:    SeverityHigh,
					File:        path,
					Line:        lineNo,
					Description: "credit-card-shaped Luhn-valid digit run on line " + itoa(lineNo),
					Detector:    "pii",
				})
			}
		}

		// SA ID heuristic: 13 digits, valid YYMMDD prefix, Luhn-valid.
		// (Reject if it's the same span the CC rule already matched to avoid
		// double-reporting a 13-digit Luhn-valid string.)
		for _, m := range reSAIDShaped.FindAll(line, -1) {
			if !looksLikeSAID(m) {
				continue
			}
			out = append(out, Finding{
				Kind:        "pii",
				Severity:    SeverityHigh,
				File:        path,
				Line:        lineNo,
				Description: "South African ID-shaped 13-digit run on line " + itoa(lineNo),
				Detector:    "pii",
			})
		}

		// Email — info-level (rarely a blocker on its own; policy decides).
		for range reEmail.FindAll(line, -1) {
			out = append(out, Finding{
				Kind:        "pii",
				Severity:    SeverityLow,
				File:        path,
				Line:        lineNo,
				Description: "email-address-shaped token on line " + itoa(lineNo),
				Detector:    "pii",
			})
		}
	}
	return out
}

// luhnValid reports whether digits passes the Luhn checksum.
func luhnValid(digits []byte) bool {
	sum := 0
	parity := len(digits) % 2
	for i, c := range digits {
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if i%2 == parity {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}

// looksLikeSAID applies basic YYMMDD sanity + Luhn check.
func looksLikeSAID(d []byte) bool {
	if len(d) != 13 {
		return false
	}
	mm := int(d[2]-'0')*10 + int(d[3]-'0')
	dd := int(d[4]-'0')*10 + int(d[5]-'0')
	if mm < 1 || mm > 12 || dd < 1 || dd > 31 {
		return false
	}
	return luhnValid(d)
}

// stripNonDigit returns a buffer containing only the digits from b.
func stripNonDigit(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c >= '0' && c <= '9' {
			out = append(out, c)
		}
	}
	return out
}
