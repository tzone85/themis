# Plan 11 — Heartbeat & Integrity Tracking

**Date:** 2026-05-25
**Depends on:** Plans 1-10.
**Scope:** Close the last two trust-layer items from design spec §9:

- §9.1.2 Silent-disable detection — operator-triggered `ENFORCEMENT_MISSING`
  events let external monitoring pipe "the GitHub check isn't installed"
  signals into the ledger. (A polling daemon ships later; Plan 11 wires the
  data-plane.)
- §9.1.3 Ledger tampering — `themis ledger verify` writes a
  `LEDGER_INTEGRITY_BROKEN` record to a separate, sidecar incidents file
  when the Merkle chain is broken. We use a sidecar (not events.jsonl
  itself) because the main ledger can no longer be trusted to record its
  own failure.
- §9.1.3 also describes weekly tip-anchoring — Plan 11 ships
  `LEDGER_ANCHOR` events so an operator can publish the current tip hash
  to an external transparency log on a cron.

## New ledger kinds

- `ENFORCEMENT_MISSING`     — payload `{repo, expected_check, last_seen, reported_by, reported_at}`
- `LEDGER_INTEGRITY_BROKEN` — payload `{detected_at, chain_error, tip_hash_before_tamper?}` — written to `tenants/<id>/incidents.jsonl`, not events.jsonl
- `LEDGER_ANCHOR`           — payload `{tip_hash, event_count, anchored_at, sink?}` — written to events.jsonl

## Tasks

### T1: Register kinds + wiring test

Three new kinds in `DefaultRegistry`, two new entries in `wiring_test.go`
(`LEDGER_INTEGRITY_BROKEN` lives outside the main ledger but is still a
documented kind so consumers know how to decode it).

### T2: `internal/incidents` package

- `Path(base, tenantID) string` returns `tenants/<id>/incidents.jsonl`.
- `Append(base, tenantID string, kind string, payload []byte) error` — appends a JSONL row with timestamp.
- `ReadAll(base, tenantID)` returns the rows for the API/CLI.

The sidecar is intentionally simple (timestamp + kind + payload) — no
Merkle chain — because integrity failures of the main chain are the very
thing it records, so a parallel chain offers no extra guarantee.

### T3: `themis ledger verify` auto-emits `LEDGER_INTEGRITY_BROKEN`

On chain break, the CLI appends to `incidents.jsonl` before returning the
error. Behaviour stays compatible with Plan 1 (still non-zero exit, same
error message).

### T4: `themis ledger anchor` CLI

Computes the current tip hash via `ledger.Doctor`, appends a `LEDGER_ANCHOR`
event with that hash. Optional `--sink` flag for the external destination
(stored only in the payload at Plan 11; actual upload to S3/git/transparency
log is a later plan).

### T5: `themis heartbeat report` CLI

`themis heartbeat report --id <t> --base <b> --repo <r> --expected-check <c> --reported-by <who>`

Appends an `ENFORCEMENT_MISSING` event. The signal source (a GitHub Action
heartbeat, an Argo CD policy check, …) is operator-provided at Plan 11;
the polling daemon comes later.

### T6: API endpoints

- `POST /v1/tenants/{id}/heartbeat` — body `{repo, expected_check, last_seen?, reported_by}` → emits `ENFORCEMENT_MISSING`.
- `POST /v1/tenants/{id}/anchor` — body `{sink?}` → emits `LEDGER_ANCHOR`.
- `GET  /v1/tenants/{id}/incidents` → returns the sidecar `incidents.jsonl` content.

### T7: Tests (unit + CLI + API)

### T8: README Plan 11 changelog

### T9: `make ci` pass
