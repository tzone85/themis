# Plan 15 — Heartbeat polling daemon

**Date:** 2026-05-25
**Depends on:** Plans 1-14.
**Status:** ✅ Shipped — see commit `b7da213` ("feat(heartbeat): polling
daemon + Checker interface + StubChecker").
**Scope:** Plan 11 wired the heartbeat *dataplane* (external observers
post `ENFORCEMENT_MISSING` events). Plan 15 adds an in-binary polling
daemon so Themis itself can probe each tenant repo at a configurable
interval — design spec §9.1.2.

## Tasks

### T1 — `Checker` interface + `StubChecker`

`internal/heartbeat/heartbeat.go` exposes a `Checker` abstraction so the
GitHub / GitLab / Bitbucket integrations are pluggable. `StubChecker`
ships in-box for tests and air-gapped deployments; it accepts explicit
allow/reject lists and defaults to "missing" for unknown repos (fail-closed).

### T2 — `Config` + `LoadConfig`

Per-tenant `tenants/<id>/heartbeat.yaml` carries the list of `{repo,
expected_check}` pairs the daemon probes. Missing file is not an error
(empty config = nothing to probe).

### T3 — `RunOnce(ctx, base, tenant, checker)` + `Watch(...)`

`RunOnce` does one polling pass; emits `ENFORCEMENT_MISSING` per
reported-missing target. Checker errors are themselves counted as
misses ("silence equals problem"). `Watch` loops at the configured
interval, terminates cleanly on `ctx.Done()`.

### T4 — CLI surface

`themis heartbeat run-once` + `themis heartbeat watch` with SIGINT/SIGTERM
clean shutdown. The Plan-11 `themis heartbeat report` command remains
for external observers; both paths emit the same payload shape.

### T5 — Tests

10 unit tests cover stub allow/reject defaulting, RunOnce miss emission,
checker errors counted as miss, context cancellation propagation, Watch
clean shutdown.

### T6 — Docs + ci

README Plan-15 changelog entry; `make ci` green.

## Definition of done

- [x] `themis heartbeat run-once` emits `ENFORCEMENT_MISSING` for every missing target.
- [x] `themis heartbeat watch` loops at `--interval` and shuts down on signal.
- [x] `StubChecker` covers tests + air-gapped operators.
- [x] Real GitHub adapter drops in by implementing `Checker` — no other change.
