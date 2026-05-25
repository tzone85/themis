# Plan 10 — Emergency Override

**Date:** 2026-05-25
**Depends on:** Plans 1-9.
**Scope:** Implement the emergency-override flow from design spec §9.1.1:
a named actor (with a tenant-configured co-signer role) can override a DENY
decision, but only with a >= 50-character reason, a time-boxed expiry, and
a mandatory 7-day post-mortem that the compliance team must close out.

## New ledger kinds

- `EMERGENCY_OVERRIDE_INVOKED` — payload `{pr_id, actor, reason, co_signer, scope, invoked_at, expires_at, postmortem_due_at}`
- `OVERRIDE_POSTMORTEM_DUE`   — synthetic event for the timeline; emitted alongside invoke
- `OVERRIDE_POSTMORTEM_CLOSED` — payload `{pr_id, closer, notes, closed_at}`

## Tasks

### T1: `internal/override` pure package

- `Invoke(...)` validation (≥50 char reason, future expires_at, both actor+co_signer set).
- `Status(events, pr_id)` returning `{active, expired, postmortem_due, postmortem_closed}`.
- `BuildInvokedPayload`, `BuildPostmortemDuePayload`, `BuildPostmortemClosedPayload` helpers.

### T2: `themis override invoke` + `themis override postmortem close` + `themis override status` CLI

### T3: API: `POST /v1/tenants/{id}/overrides` (invoke), `POST /v1/tenants/{id}/overrides/postmortem` (close), `GET /v1/tenants/{id}/overrides?pr_id=X`

### T4: Register 3 new ledger kinds + wiring test

### T5: Tests (unit + CLI + API)

### T6: README Plan 10 changelog

### T7: `make ci` pass
