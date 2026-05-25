# Plan 8 — MCP Server + Embedded Web Dashboard

**Date:** 2026-05-25
**Depends on:** Plans 1-7.
**Scope:** Two new surfaces, both layered over the existing REST API:

1. **MCP server** — exposes read-only tools (catalogue lookup, classify, decisions, BOMs) so Claude Code / Cursor / VXD agents can query Themis pre-write. This is the "agentic-first" pillar from design spec §5.1.
2. **Embedded web dashboard** — a single-page HTML+JS bundle served from the same binary at `/` that shows the audit timeline + decision detail + BOM viewer. No build step; vanilla JS so the binary stays a single Go static binary.

## MCP server

Bridges the open MCP standard (stdio JSON-RPC) to the existing REST API:

- `themis_health(tenant_id)` → tenant health
- `themis_decisions(tenant_id, pr_id)` → most recent decision
- `themis_bom(tenant_id, hash)` → BOM body
- `themis_catalogue(tenant_id)` → graph summary (counts + content_hash)

All tools are read-only at Plan 8 (no `themis_decide` MCP tool yet — the
agentic pillar is "advise pre-write", not "decide autonomously").

## Web dashboard

`GET /` → embedded HTML. Two views, no routing:

- **Timeline** — paginated list of ledger events for the tenant (newest first), filterable by kind + PR id.
- **PR detail** — clicking a PR id pulls the latest DECISION_ISSUED + linked SCAN_FINDINGs + BOM.

Frontend talks to the existing JSON endpoints with a token taken from a `?token=` query param (good enough for local + air-gapped; cookie sessions land later).

## Tasks

### T1: Embedded HTML+JS dashboard

`internal/api/web/index.html` + `web.go` (uses `embed.FS`). Serve at `GET /`.

### T2: New JSON endpoint `GET /v1/tenants/{id}/events` for the timeline

Returns paginated events (newest first); query params `limit` + `kind` filter.

### T3: MCP server

`internal/mcp/server.go` — stdio JSON-RPC server that translates each MCP tool call into an HTTP call against the running API.

### T4: `themis mcp` CLI

`themis mcp --base-url <url> --token <t> --tenant-id <id>` — runs the MCP server bridge over stdio.

### T5: Tests

- Dashboard endpoint serves HTML.
- `/v1/tenants/{id}/events` paginates correctly + filters by kind.
- MCP server handles `initialize` + a few `tools/call` requests.

### T6: README Plan 8 changelog

### T7: `make ci` pass
