// Package incidents writes a per-tenant sidecar ledger of trust-layer
// incidents that cannot live in the main events.jsonl chain.
//
// The classic example is LEDGER_INTEGRITY_BROKEN: by the time we detect
// chain tampering, the main ledger is no longer trustworthy as a record
// of that detection. The incidents file is intentionally simple
// (timestamp + kind + raw JSON payload, one per line) — a parallel
// Merkle chain would only push the trust problem one layer further.
//
// Consumers (CLI, REST API, audit exports) call ReadAll to surface
// incidents to compliance dashboards.
package incidents

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileName is the per-tenant filename inside tenants/<id>/.
const FileName = "incidents.jsonl"

// Record is one row in the sidecar ledger.
type Record struct {
	Kind      string          `json:"kind"`
	Timestamp time.Time       `json:"ts"`
	Payload   json.RawMessage `json:"payload"`
}

// Path returns the absolute path to a tenant's incidents file.
func Path(base, tenantID string) string {
	return filepath.Join(base, "tenants", tenantID, FileName)
}

// Append writes one record to the tenant's incidents file. The file is
// created if missing; existing rows are preserved (append-only).
// Permissions: 0o600 — incidents may carry redacted forensic info.
func Append(base, tenantID, kind string, payload json.RawMessage) error {
	if kind == "" {
		return fmt.Errorf("incidents: kind required")
	}
	if payload == nil {
		payload = json.RawMessage("{}")
	}
	rec := Record{Kind: kind, Timestamp: time.Now().UTC(), Payload: payload}
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	dir := filepath.Dir(Path(base, tenantID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	f, err := os.OpenFile(Path(base, tenantID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- tenant-scoped path.
	if err != nil {
		return fmt.Errorf("open %s: %w", Path(base, tenantID), err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write incident: %w", err)
	}
	return nil
}

// ReadAll returns every record in the tenant's incidents file, in append
// order. Missing file returns an empty slice with no error.
func ReadAll(base, tenantID string) ([]Record, error) {
	path := Path(base, tenantID)
	f, err := os.Open(path) // #nosec G304 -- tenant-scoped path.
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return out, nil
}
