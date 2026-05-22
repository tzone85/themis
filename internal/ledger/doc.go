// Package ledger implements Themis's tamper-evident append-only event store.
//
// The ledger is per-tenant: each Tenant has its own events.jsonl file (the
// source of truth) and a SQLite WAL projection (rebuildable from the JSONL).
// Every event references the prior event's content hash, forming a
// Merkle-style chain. See docs/superpowers/specs/ for the design.
package ledger
