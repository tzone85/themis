# Plan 9 — Approval Flows

**Date:** 2026-05-25
**Depends on:** Plans 1-8.
**Scope:** Close the loop on REQUIRE_APPROVAL decisions. Named approvers grant
or deny; once the required role-set is satisfied, the decision becomes
finalised (DECISION_FINALISED). Approvals are ledger events themselves, so
the trust story holds: every approval is signed by the chain.

## New ledger kinds

- `APPROVAL_GRANTED` — payload `{pr_id, approver, role, comment?, granted_at}`
- `APPROVAL_DENIED` — payload `{pr_id, approver, role, reason, denied_at}`
- `DECISION_FINALISED` — emitted automatically when all required roles have granted

## Tasks

### T1: Approval logic

`internal/approvals/approvals.go`:
- `Status(events, prID) { Decision, GrantedRoles, DeniedRoles, Finalised, FinalVerdict }` — pure function over a ledger slice.
- `Grant(...)`, `Deny(...)` builders for ledger payloads.
- `Finalise(decision, granted) (DECISION_FINALISED payload | nil)` — returns the finalisation envelope when every required role has signed off.

### T2: `themis approval grant / deny` CLI

`internal/cli/approval_cmd.go`:
- `themis approval grant --id <t> --base <state> --pr-id <p> --approver <name> --role <role> [--comment …]`
- `themis approval deny  --id <t> --base <state> --pr-id <p> --approver <name> --role <role> --reason …`
- After append, recompute status; if finalised, emit DECISION_FINALISED.

### T3: API endpoints

`internal/api/approvals.go`:
- `POST /v1/tenants/{id}/approvals` — body `{pr_id, approver, role, action ("grant"|"deny"), comment?, reason?}`. Returns the updated approval status.
- `GET /v1/tenants/{id}/approvals?pr_id=…` — returns current status.

### T4: Register `APPROVAL_GRANTED` + `APPROVAL_DENIED` + `DECISION_FINALISED`

Ledger + wiring test.

### T5: Tests

- Grant a single role required → finalised ALLOW.
- Two roles required, only one granted → not yet finalised.
- Deny → not finalised; status reflects denial.
- Re-grant after deny stays denied (deny is sticky for the PR).
- API endpoint integration tests.

### T6: README Plan 9 changelog

### T7: `make ci` pass
