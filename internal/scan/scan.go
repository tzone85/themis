// Package scan defines the Scanner interface and the value types every
// scanner emits. Concrete scanners (secrets, PII, slopsquat, …) live in
// sibling files and are wired into the orchestrator in run.go.
//
// Scanners are pluggable to satisfy design spec §6.1: each is an interface
// implementation, so adding or removing a scanner does not change the
// shape of historical findings stored in the ledger.
package scan

import "github.com/tzone85/themis/internal/aichange"

// Severity ranks how seriously policy should treat a Finding.
type Severity string

const (
	// SeverityInfo is purely informational — never blocks on its own.
	SeverityInfo Severity = "info"
	// SeverityLow is "worth a glance" — typically aggregated into a summary.
	SeverityLow Severity = "low"
	// SeverityMed is "should be reviewed" — escalates to REQUIRE_APPROVAL in
	// the conservative policy template.
	SeverityMed Severity = "med"
	// SeverityHigh is "should not merge without compliance" — DENY in most templates.
	SeverityHigh Severity = "high"
	// SeverityCritical is "must not merge" — DENY in every reasonable template.
	SeverityCritical Severity = "critical"
)

// severityRank maps each Severity to an ordinal for policy threshold checks
// (e.g. "findings.severity >= high").
var severityRank = map[Severity]int{
	SeverityInfo:     0,
	SeverityLow:      1,
	SeverityMed:      2,
	SeverityHigh:     3,
	SeverityCritical: 4,
}

// Rank returns the ordinal of s in [0..4], or -1 if s is unknown.
func (s Severity) Rank() int {
	r, ok := severityRank[s]
	if !ok {
		return -1
	}
	return r
}

// Finding is one scanner-emitted observation. Findings carry no raw secret
// material — descriptions are redacted by the scanner before construction.
type Finding struct {
	// Kind is the scanner-defined category (e.g. "secret", "pii", "scan_failure").
	Kind string `json:"kind"`
	// Severity drives policy thresholds.
	Severity Severity `json:"severity"`
	// File is the path inside the AIChange the finding belongs to. Empty when
	// the finding is at scanner level (e.g. crash).
	File string `json:"file,omitempty"`
	// Line is 1-indexed within File. 0 means "unknown / file-level".
	Line int `json:"line,omitempty"`
	// Description is a redacted, human-readable explanation. MUST NOT contain
	// the matched secret/PII text itself.
	Description string `json:"description"`
	// Detector is the name of the scanner that produced the finding.
	Detector string `json:"detector"`
}

// Scanner is the contract every active scanner implements.
//
// fileBodies maps the AIChange's FileTouch.Path (for ADDED/MODIFIED) to the
// post-change file content. DELETED files are absent from the map (there's
// no current content to scan). Scanners must tolerate missing entries.
type Scanner interface {
	// Name uniquely identifies the scanner. Lowercase, snake_case.
	Name() string
	// Scan analyses the change. It must be deterministic and side-effect-free.
	Scan(c aichange.AIChange, fileBodies map[string][]byte) ([]Finding, error)
}
