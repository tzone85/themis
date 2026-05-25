# Plans 16 + 17 — Mempalace bridge + Advisory agent

**Date:** 2026-05-25
**Depends on:** Plans 1-15.
**Status:** ✅ Shipped — see commit `f80fb28` ("feat(mempalace,advisor,cli):
content-addressed drawer bridge + LLM-agnostic advisory agent + themis advise").
**Scope:**

- **Plan 16 — Mempalace bridge:** persist decisions, BOMs, and advisor
  notes into the per-tenant Mempalace wing as content-addressed JSON
  drawers. Themis writes; the upstream Mempalace daemon (or any consumer)
  reads out-of-band.
- **Plan 17 — Advisory agent:** an LLM-agnostic advisor that drafts a
  plain-language review note from a `DECISION_ISSUED`. Per design spec
  §5.1 the advisor is **never** on the trust-critical path — it produces
  suggestion text only; the deterministic policy engine still issues the
  verdict.

## Plan 16 tasks

### 16-T1 — `internal/mempalace/bridge.go`

`Bridge.Write(Drawer)` writes content-addressed JSON drawers to
`tenants/<id>/mempalace-wing/<kind>/<sha256>.json`. `Read`, `List`,
`WingDir` helpers + per-tenant scoping.

### 16-T2 — Tests

6 unit tests: content-addressed write, missing-field rejection, round-trip,
list enumeration, missing-dir empty list, per-tenant scoping.

## Plan 17 tasks

### 17-T1 — `internal/advisor/advisor.go`

`LLM` interface (`Name`, `Generate(ctx, prompt)`), `NullLLM` deterministic
fallback, `Draft(ctx, llm, Input) Output` composing a deterministic
prompt + structured `Summary` (verdict / impact kind / findings count /
high-severity kinds).

### 17-T2 — `themis advise` CLI

Walks the ledger to the matching `DECISION_ISSUED`, drafts a note, writes
it to the Mempalace wing as an `advisor-note` drawer, prints the note +
drawer path.

### 17-T3 — Tests

8 advisor unit tests cover ALLOW / REQUIRE_APPROVAL / DENY text shapes,
summary aggregation, nil-LLM defaulting, LLM error propagation. 3 CLI
tests cover happy path, unknown-PR rejection, missing-flag rejection.

## Definition of done

- [x] Drawer files persist with content-addressed names; same body = same path.
- [x] `themis advise --pr-id <p>` produces a deterministic, NullLLM-backed note.
- [x] Real LLM providers (OpenAI / Anthropic / local) drop in by implementing `LLM`.
- [x] The advisor never issues a verdict — only drafts text for human reviewers.
