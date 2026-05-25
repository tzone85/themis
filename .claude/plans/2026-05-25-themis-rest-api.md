# Plan 6 — REST API (read-only)

**Date:** 2026-05-25
**Depends on:** Plans 1-5.
**Scope:** HTTP server exposing read-only endpoints over the tenant ledger:
health, decisions, BOMs. Plus API-key auth (simple per-tenant allowlist; OIDC
deferred per design spec §6.1). `themis serve` CLI command.

This is the minimum surface a web dashboard, an MCP server, or a CI integration
needs to query Themis without parsing the JSONL ledger directly.

## Out of scope

- Write endpoints (`POST /v1/decide`) — Plan 7.
- WebSocket subscriptions — Plan 7.
- OIDC/SAML — Plan 8.

## Endpoints

| Method | Path | Behaviour |
|---|---|---|
| GET | `/v1/health` | Always 200; reports `{version, tenants_count}`. No auth. |
| GET | `/v1/tenants/{id}/health` | Per-tenant: ledger event count, chain status. Requires token. |
| GET | `/v1/tenants/{id}/decisions?pr_id=X` | Returns the most recent DECISION_ISSUED for prID. 404 if none. |
| GET | `/v1/tenants/{id}/boms/{hash}` | Returns the signed BOM file by content hash. 404 if missing. |
| GET | `/v1/tenants/{id}/boms/{hash}.sig` | Returns the hex-encoded signature sidecar. |

All authenticated endpoints expect `Authorization: Bearer <token>` matching
the per-tenant token file at `tenants/<id>/api-tokens` (one token per line).

## Tasks

### T1: Token store + auth middleware

`internal/api/auth.go`:
- `Tokens(base, id) []string` — reads `tenants/<id>/api-tokens` (one per line, ignoring blanks/comments).
- `RequireToken(base, id, r *http.Request) error` — verifies Bearer.

Tests cover: missing file = no auth allowed; multiple tokens accepted; bad header rejected.

### T2: HTTP handlers

`internal/api/server.go`:
- `func NewMux(base string) *http.ServeMux` builds the route table.
- `/v1/health` — global, no auth.
- `/v1/tenants/{id}/health` — calls `ledger.Doctor`.
- `/v1/tenants/{id}/decisions` — finds latest `DECISION_ISSUED` matching `?pr_id=`.
- `/v1/tenants/{id}/boms/{hash}` — serves bytes from `tenants/<id>/bom/<hash>.bom.json`.

### T3: `themis serve` CLI

`internal/cli/serve_cmd.go`:
- `themis serve --base <state> --addr :8787`
- Binds to addr, runs `http.Server`, ListenAndServe.

### T4: Integration test

`internal/api/server_test.go`:
- Boot in-process server pointing at a fully-seeded tenant (init+sync+ingest+decide+sign).
- Hit every endpoint, assert status codes + payload shape.
- Auth-required endpoints reject missing/wrong tokens with 401.

### T5: README Plan 6 changelog

### T6: `make ci` pass
