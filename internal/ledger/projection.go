package ledger

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no CGO
)

// Projection is the per-tenant SQLite WAL projection of the JSONL ledger.
// It is rebuildable: deleting and re-opening + replaying events.jsonl
// produces a byte-identical Projection.
type Projection struct {
	db   *sql.DB
	path string
}

// OpenProjection opens (and migrates) the SQLite projection at path.
// WAL mode is enabled for concurrent reads + single writer.
func OpenProjection(path string) (*Projection, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	p := &Projection{db: db, path: path}
	if err := p.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return p, nil
}

// DB returns the underlying *sql.DB. Callers must not close it; call
// Projection.Close instead.
func (p *Projection) DB() *sql.DB { return p.db }

// Close closes the underlying connection.
func (p *Projection) Close() error { return p.db.Close() }

// migrate applies the v1 schema. Idempotent.
func (p *Projection) migrate() error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	}
	for _, q := range pragmas {
		if _, err := p.db.Exec(q); err != nil {
			return fmt.Errorf("pragma %q: %w", q, err)
		}
	}
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		seq         INTEGER PRIMARY KEY AUTOINCREMENT,
		kind        TEXT    NOT NULL,
		tenant      TEXT    NOT NULL,
		ts          TEXT    NOT NULL,           -- RFC3339Nano UTC
		prev_hash   TEXT    NOT NULL,
		content_hash TEXT   NOT NULL UNIQUE,    -- enforces idempotent re-projection
		payload     BLOB    NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_events_kind ON events(kind);
	CREATE INDEX IF NOT EXISTS idx_events_ts   ON events(ts);

	CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	if _, err := p.db.Exec(schema); err != nil {
		return fmt.Errorf("schema migrate: %w", err)
	}
	return nil
}

// Project records e in the projection. It is idempotent (same content_hash
// twice = single row). Project refuses kinds not present in registry —
// this is the wiring guard that prevents the default-case-eats-events bug.
func (p *Projection) Project(e Event, registry *Registry) error {
	projector, ok := registry.Projector(e.Kind)
	if !ok {
		return fmt.Errorf("ledger: unknown event kind %q (every kind must be registered in DefaultRegistry)", e.Kind)
	}
	hash, err := e.ContentHash()
	if err != nil {
		return fmt.Errorf("content hash: %w", err)
	}
	// Idempotent insert keyed by content_hash UNIQUE constraint.
	_, err = p.db.Exec(
		`INSERT OR IGNORE INTO events (kind, tenant, ts, prev_hash, content_hash, payload)
         VALUES (?, ?, ?, ?, ?, ?)`,
		e.Kind, e.Tenant, e.Timestamp.UTC().Format(timeFormat), e.PrevHash, hash, []byte(e.Payload),
	)
	if err != nil {
		return fmt.Errorf("project insert: %w", err)
	}
	// Run the kind-specific projector (currently noop for Plan 1 kinds).
	if err := projector([]byte(e.Payload)); err != nil {
		return fmt.Errorf("projector %q: %w", e.Kind, err)
	}
	return nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"
