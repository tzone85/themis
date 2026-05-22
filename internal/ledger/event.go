package ledger

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "time"
)

// Event is one append to the ledger. The hash chain (PrevHash → ContentHash)
// is what makes the ledger tamper-evident.
type Event struct {
    Kind      string          `json:"kind"`      // e.g. INGEST_COMPLETED
    Tenant    string          `json:"tenant"`    // tenant ID; redundant inside a per-tenant file but kept for portability
    Timestamp time.Time       `json:"ts"`        // UTC
    Payload   json.RawMessage `json:"payload"`   // schema depends on Kind
    PrevHash  string          `json:"prev_hash"` // hex sha256 of the prior event's ContentHash; "GENESIS" for the first
}

// ZeroHash is the prev_hash sentinel for the first event in a tenant's chain.
const ZeroHash = "GENESIS"

// canonical returns the byte representation hashed for ContentHash.
// We marshal a struct (not the receiver) so that field order is fixed and
// the JSON canonical form is deterministic across Go versions.
func (e Event) canonical() ([]byte, error) {
    type canonical struct {
        Kind      string          `json:"kind"`
        Tenant    string          `json:"tenant"`
        Timestamp string          `json:"ts"`
        Payload   json.RawMessage `json:"payload"`
        PrevHash  string          `json:"prev_hash"`
    }
    c := canonical{
        Kind:      e.Kind,
        Tenant:    e.Tenant,
        Timestamp: e.Timestamp.UTC().Format(time.RFC3339Nano),
        Payload:   e.Payload,
        PrevHash:  e.PrevHash,
    }
    var buf bytes.Buffer
    enc := json.NewEncoder(&buf)
    enc.SetEscapeHTML(false)
    if err := enc.Encode(c); err != nil {
        return nil, fmt.Errorf("marshal event for hashing: %w", err)
    }
    // Encoder appends a newline; strip it so the hash is over the JSON value, not "value\n".
    out := buf.Bytes()
    if len(out) > 0 && out[len(out)-1] == '\n' {
        out = out[:len(out)-1]
    }
    return out, nil
}

// ContentHash returns the hex-encoded SHA-256 of the event's canonical form.
func (e Event) ContentHash() (string, error) {
    raw, err := e.canonical()
    if err != nil {
        return "", err
    }
    sum := sha256.Sum256(raw)
    return hex.EncodeToString(sum[:]), nil
}

// Chain returns a copy of e with PrevHash set to prior. Used when appending.
func Chain(e Event, prior string) Event {
    e.PrevHash = prior
    return e
}
