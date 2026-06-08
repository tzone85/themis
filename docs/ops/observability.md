# Observability

What Themis emits today, what it doesn't, and how to reason about it.

Verified against `v0.1.0` on `2026-06-03`.

## Honest current state

| Signal | Status | Notes |
|---|---|---|
| **Structured logs** | Partial | `themis serve` emits a one-line startup notice via `fmt.Fprintf` to stdout. Subcommands emit human-readable text only. No JSON log format yet. |
| **Metrics (Prometheus)** | Not shipped | No `/metrics` endpoint. Tracked as a v0.2.0 candidate. |
| **Tracing (OTEL)** | Not shipped | No `OTEL_*` env wiring. v0.2.0 candidate. |
| **Health check** | Indirect | Use `themis ledger doctor --id <tenant> --base <dir>` as a liveness probe; it returns non-zero on ledger corruption or missing tenant. |
| **Audit trail** | Strong | Append-only Merkle-chained ledger per tenant (`tenants/<id>/ledger.jsonl` + SQLite WAL projection). This is the *primary* observability surface — every decision, override, BOM build, BOM sign, OIDC issuance, anchor commit appears as a ledger event. |

Treat the ledger as your observability backbone for now. The flat-file
logs are best-effort and not load-bearing.

## What the ledger records

Each ledger event is a single JSON line under
`tenants/<tenant>/ledger.jsonl` with a hash linking it to the previous
event. Event kinds you can grep for:

| Kind | Emitter | Why you care |
|---|---|---|
| `TENANT_INIT` | `themis tenant init` | Tenant lifecycle. |
| `CATALOGUE_SYNC` | `themis catalogue sync` | Catalogue snapshots. |
| `AICHANGE_INGESTED` | `themis ingest` | AI change adapter fired. |
| `DECISION_ISSUED` | `themis decide` | Policy verdict. Embeds the AIChange. |
| `BOM_BUILT` | `themis bom build` | Bill-of-materials per PR. |
| `BOM_SIGNED` | `themis bom sign` | Signer fired (cosign keyless stub or ed25519). |
| `ANCHOR_COMMITTED` | `themis ledger anchor` | External anchor sink ack. |
| `HEARTBEAT_OK` / `HEARTBEAT_FAIL` | `themis heartbeat` | Polling daemon ticks. |
| `OVERRIDE_RECORDED` | `themis override` | Human override on a verdict. |
| `APPROVAL_RECORDED` | `themis approval` | Approval flow event. |
| `TOKEN_ISSUED` | `themis tokens grant` | Auth-token issuance. |

To stream events into a log aggregator, tail
`tenants/*/ledger.jsonl` with vector / fluent-bit / promtail.

## What `themis serve` logs

Today, only this on startup:

```
themis: serving on 127.0.0.1:8787
```

The HTTP server uses the Go `net/http` defaults and does not log
per-request access lines. Front it with a reverse proxy that does
(Caddy + Nginx examples in [`deployment.md`](deployment.md)).

## Health checking

There is no dedicated `/healthz` endpoint at v0.1.0. Two options:

1. **TCP connect** to the listener address. Detects daemon-up, not
   tenant-up.
2. **`themis ledger doctor`** for each tenant. Detects ledger
   corruption and missing files. Non-zero exit on failure — suitable
   as a Docker `HEALTHCHECK` or systemd `WatchdogSec`.

The Compose snippet in `deployment.md` uses option 2.

## Recommended scrape strategy

Until v0.2.0 ships a `/metrics` endpoint:

- **Ledger events** → tail to your log pipeline; derive counters per
  `kind` field with the aggregator's filter language.
- **Heartbeat output** → `themis heartbeat watch --interval 60s
  --base /var/lib/themis` writes `HEARTBEAT_{OK,FAIL}` per tick;
  alert on the failure ratio.
- **govulncheck on disk** → run nightly on the `dist/` artefacts
  themselves (out of band from runtime).

## Log field reference

The CLI prints freeform strings. The fields below appear in **ledger
events** (the JSON in `ledger.jsonl`):

| Field | Type | Notes |
|---|---|---|
| `seq` | int64 | Monotonic per tenant. Gaps mean tampering. |
| `kind` | string | Event kind from the table above. |
| `at` | RFC 3339 | UTC. |
| `tenant` | string | Stable tenant ID. |
| `pr_id` | string | Pull request identifier, when applicable. |
| `actor` | string | Token description or OIDC subject. |
| `payload` | object | Kind-specific body. See `internal/ledger/event.go`. |
| `prev_hash` | hex(sha256) | Hash of previous event. |
| `hash` | hex(sha256) | Hash of this event. |

## Roadmap

- v0.2.0: structured stdlib `slog` JSON logs gated on `THEMIS_LOG_FORMAT=json`.
- v0.2.0: `/metrics` Prometheus endpoint with per-tenant counters.
- v0.3.0: OpenTelemetry tracing on the decide → BOM → sign path.

## Related

- [`runbook.md`](runbook.md) — common incidents and diagnosis steps.
- [`backup-restore.md`](backup-restore.md) — what to snapshot.
- Design spec §observability (`docs/design.md`).
