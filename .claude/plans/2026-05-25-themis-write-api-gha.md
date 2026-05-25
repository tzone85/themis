# Plan 7 — Write API + GitHub Action

**Date:** 2026-05-25
**Depends on:** Plans 1-6.
**Scope:** Add a write endpoint to the REST API so CI systems can submit an
AIChange and get back a Decision. Add a GitHub Action wrapper that calls
`themis decide` from a workflow, and a sample git pre-push hook.

## Endpoints (new)

| Method | Path | Behaviour |
|---|---|---|
| POST | `/v1/tenants/{id}/decide` | JSON body `{ai_change, policy_yaml}` → runs Classify+RunAll+Decide, emits SCAN_FINDING/DECISION_ISSUED, returns the Decision payload. Requires Bearer. |

Body shape (request):

```json
{
  "ai_change": { ... AIChange ... },
  "policy_yaml": "version: 1\ndefault: REQUIRE_APPROVAL\n...",
  "workdir_files": { "src/x.go": "<base64-encoded file body>" }  // optional; for scanners
}
```

Body shape (response): the same envelope `themis decide` prints — `{pr_id, actor, impact, findings, decision}`.

## Tasks

### T1: Decide handler

`internal/api/decide.go`:
- POST only, requires Bearer.
- Parses body; loads catalogue snapshot from disk; runs the pipeline as in `cli/decide_cmd.go`.
- Refactor: extract the orchestration body into a reusable `pipeline.Run(...)` so HTTP and CLI share code.

### T2: Pipeline helper

`internal/pipeline/decide.go`:
- `Run(ctx, base, id string, ai aichange.AIChange, pol policy.Policy, bodies map[string][]byte) (Decision, []Finding, Impact, error)` — does the work; emits ledger events.
- CLI's `runDecide` becomes a thin wrapper.

### T3: Tests for POST /v1/tenants/{id}/decide

- Happy path: doc-only AIChange → 200 ALLOW.
- Secret in workdir_files (base64 of `AKIA…`) → 200 DENY + SCAN_FINDING.
- Missing body → 400.
- Missing bearer → 401.
- Bad policy YAML → 400 with POLICY_INVALID emitted.

### T4: GitHub Action wrapper

`actions/themis-check/action.yml`:
- Inputs: `themis-base-url`, `themis-token`, `tenant-id`, `pr-id`, `policy-path`.
- Steps: composite action that:
  1. Builds an AIChange from `git diff` against `${{ github.event.pull_request.base.sha }}`.
  2. POSTs to `${themis-base-url}/v1/tenants/${tenant-id}/decide`.
  3. Exit non-zero if verdict is DENY; comment back if REQUIRE_APPROVAL.

For Plan 7 we ship the YAML + a `scripts/themis-check.sh` that the action invokes. The shell script uses the existing `themis ingest` + `curl` to keep the action's runtime small.

### T5: Git pre-push hook

`scripts/hooks/pre-push.sh`:
- Reads the local repo, runs `themis ingest --adapter git_heuristic`, then `themis decide`.
- Prints decision; non-zero exit on DENY.

### T6: README Plan 7 changelog

### T7: `make ci` pass
