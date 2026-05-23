// Package bom builds the canonical AI Bill of Materials document Themis
// signs and stores per PR. The schema version (SchemaVersion field) is
// stable and independently published so other tools can consume it.
package bom

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
)

// CurrentSchemaVersion is the BOM schema identifier embedded in every document.
// Bumped only with a documented migration; consumers MUST refuse versions they
// don't understand.
const CurrentSchemaVersion = "themis.bom.v1"

// BOM is the canonical Bill-of-Materials value type. Field order in the
// struct is the field order used by Canonical(), so reordering here changes
// the canonical bytes — that's a deliberate schema break.
type BOM struct {
	SchemaVersion string             `json:"schema_version"`
	PRID          string             `json:"pr_id"`
	Tenant        string             `json:"tenant"`
	Actor         string             `json:"actor"`
	BuiltAt       time.Time          `json:"built_at"`
	AIChange      aichange.AIChange  `json:"ai_change"`
	Impact        classify.Impact    `json:"impact"`
	Findings      []scan.Finding     `json:"findings"`
	Decision      policy.Decision    `json:"decision"`
	LedgerTip     string             `json:"ledger_tip"`
}

// Canonical returns the byte representation hashed for signing. Output is
// deterministic — same logical inputs always produce the same bytes — so
// signatures are reproducible and the audit chain can verify offline.
//
// Determinism rules:
//   - JSON encoded with stable key order (json.Encoder + struct field order).
//   - Timestamps formatted as RFC3339Nano in UTC.
//   - HTML escaping disabled (json.Encoder defaults to escaping <, >, &).
//   - Trailing newline stripped (json.Encoder appends one).
func Canonical(b BOM) ([]byte, error) {
	// Force UTC + RFC3339Nano on BuiltAt so reformatting at a different
	// timezone doesn't change the canonical bytes.
	b.BuiltAt = b.BuiltAt.UTC()
	// Normalise nil slices to empty slices so consumers see [] not null.
	if b.Findings == nil {
		b.Findings = []scan.Finding{}
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(b); err != nil {
		return nil, fmt.Errorf("bom canonical encode: %w", err)
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}

// Hash returns the hex SHA-256 of the canonical form. This is the value
// signers operate on and verifiers check; storing the hash separately in
// ledger payloads makes integrity proofs cheap to evaluate without parsing
// the full BOM.
func Hash(b BOM) (string, error) {
	raw, err := Canonical(b)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
