# Themis

**A compliance gateway for AI-generated code.**

Themis records and governs every change that AI coding tools (Claude Code, Cursor, GitHub Copilot, autonomous agents like [VXD](https://github.com/tzone85/vortex-dispatch)) make to your software. It captures *who* (which AI, which prompt, which human) changed *what* (which event contract, which service, which downstream consumer), proves it cryptographically (signed AI Bill of Materials per pull request, tamper-evident ledger), and stops changes that violate your policies before they merge.

## Status

> **Plan 12 (Multi-tenant auth + role model) implemented.** Bearer tokens now resolve to a `(tenant, role)` Identity. Five roles totally ordered: `read < dev < reviewer < compliance < admin`. Endpoint gates enforce minimum role (e.g. POST `/approvals` requires `reviewer`; POST `/anchor` requires `admin`). Legacy `tenants/<id>/api-tokens` files keep working — entries there are treated as `admin` for the tenant. New `themis tokens grant/list/revoke` CLI for managing the structured `tenants/tokens.yaml` store. See the Changelog below.

## End-to-end demo

```bash
go build -o /tmp/themis ./cmd/themis
DIR=/tmp/themis-demo
/tmp/themis tenant init --id acme --base "$DIR"
/tmp/themis catalogue sync --id acme --base "$DIR" \
  --source ./internal/catalogue/testdata/sample
echo '{"pr_id":"demo#1","actor":"claude_code","touched_files":[
  {"path":"README.md","change_kind":"MODIFIED","before_hash":"a","after_hash":"b"}
]}' > "$DIR/ai.json"
echo 'version: 1
default: REQUIRE_APPROVAL
rules:
  - name: doc-only allowed
    when:
      impact.kind: [DOC_ONLY]
    then:
      verdict: ALLOW' > "$DIR/themis.yaml"
/tmp/themis decide --id acme --base "$DIR" --aichange "$DIR/ai.json" --policy "$DIR/themis.yaml"
/tmp/themis bom build --id acme --base "$DIR" --pr-id demo#1
/tmp/themis bom sign  --id acme --base "$DIR" --pr-id demo#1
/tmp/themis ledger doctor --id acme --base "$DIR"
```

## Changelog

### Unreleased — Plan 12 (Multi-tenant auth + role model)

**Added**

- `internal/auth`:
  - `Role` enum + total order: `read (0) < dev (1) < reviewer (2) < compliance (3) < admin (4)`.
  - `Identity { Tenant, Role, Token4, Description }` — the resolved subject of an authenticated request.
  - `TokenStore` interface + `FileTokenStore` that reads `tenants/tokens.yaml` first, falls back to the legacy per-tenant `api-tokens` file (every legacy token = `admin` for that tenant).
  - Constant-time token compare; per-token `description` carried through to audit.
  - 14 unit tests cover role ordering, satisfaction, all four lookup paths (YAML, legacy, precedence, missing).
- `internal/api`:
  - `RequireIdentity(base, tenant, minRole, r)` middleware returns the resolved Identity or `ErrUnauthorized` / `ErrInsufficientRole`.
  - Per-action role gates: GET endpoints require `read`; POST `/decide` requires `dev`; POST `/approvals` requires `reviewer`; POST `/overrides` requires `compliance`; POST `/anchor` + `/heartbeat` require `admin`.
  - 6 role-aware integration tests covering every tier + cross-tenant token rejection (401, not 403).
- `themis tokens` CLI:
  - `grant --tenant <t> --role <r> [--description <d>]` — generates a 32-byte `thm_<hex>` token, prints it once, persists to `tokens.yaml`.
  - `list` — shows last-4 suffix + tenant + role + description (never reprints the full token).
  - `revoke --token-prefix <p>` — removes matching entries.

**Notes**

- `RequireToken(base, id, r)` is preserved as a thin wrapper around `RequireIdentity` so every Plan 6-11 handler keeps working without changes.
- The legacy admin-fallback is intentional: an operator with a Plan 6 deployment can upgrade Themis without rewriting their token store; tokens become `admin`-by-default until the operator migrates them into `tokens.yaml` with explicit roles.

### Unreleased — Plan 11 (Heartbeat + integrity tracking)

**Added**

- `internal/incidents` — sidecar `tenants/<id>/incidents.jsonl` ledger for events that *cannot* live in the main events.jsonl chain (the canonical example: the very record of "the main chain is broken"). One row per line, `{kind, ts, payload}`, 0o600 permissions. 6 unit tests cover happy path, missing-file, empty-kind rejection, nil-payload defaulting, corrupt-line detection, per-tenant path scoping.
- `themis ledger verify` now auto-records `LEDGER_INTEGRITY_BROKEN` to the sidecar when a chain break is detected — the main ledger can't reliably record its own failure, so this is the load-bearing piece of design spec §9.1.3.
- `themis ledger anchor [--sink <id>]` — refuses if the chain is broken (no point anchoring a tampered tip), otherwise appends a `LEDGER_ANCHOR` event with `{tip_hash, event_count, anchored_at, sink}`. Operators schedule the actual upload to S3 / transparency log / public git repo via cron.
- `themis heartbeat report --repo X --expected-check Y --reported-by Z` — appends an `ENFORCEMENT_MISSING` event so external monitoring (a GitHub Action heartbeat, Argo CD policy probe, synthetic monitor) can record "this repo no longer has its required Themis check" — the deadman-switch dataplane (design spec §9.1.2).
- REST API parity:
  - `POST /v1/tenants/{id}/heartbeat` — record an enforcement-missing signal.
  - `POST /v1/tenants/{id}/anchor` — emit `LEDGER_ANCHOR` (409 on broken chain).
  - `GET  /v1/tenants/{id}/incidents` — read the sidecar JSONL as JSON.
- Three new registered ledger event kinds: `ENFORCEMENT_MISSING`, `LEDGER_ANCHOR`, `LEDGER_INTEGRITY_BROKEN` (the last is documented in `DefaultRegistry` for consumer-decoder symmetry even though it never appears in the main chain). Wiring test extended.

**Notes**

- The "deadman switch" daemon that *polls* repos for missing checks is intentionally deferred — Plan 11 ships the dataplane (where the signal lands) so any operator-side observer can wire in immediately without waiting for the centralised poller.
- `LEDGER_ANCHOR` deliberately does not perform the upload itself: that requires per-tenant credentials and external infrastructure choices we want to keep out of the core binary.

### Unreleased — Plan 10 (Emergency override)

**Added**

- `internal/override` — pure value-type + status package:
  - `InvokePayload`, `PostmortemDuePayload`, `PostmortemClosedPayload` mirroring the three new ledger event payloads.
  - `ValidateInvoke(payload, now)` enforces: ≥ 50-char reason, present actor + co-signer, distinct identities, future expiry.
  - `BuildInvoke(payload, now)` fills timestamps + default 24h expiry + 7-day post-mortem due window.
  - `Compute(events, pr_id, now)` returns `{active, expired, postmortem_due, postmortem_closed, postmortem_overdue, actor, co_signer}` — the full state for a PR.
  - 12 unit tests covering: happy path, short-reason rejection, missing fields, actor==co-signer rejection, past-expiry rejection, default 24h expiry, active-before-expiry, expired-after-TTL, overdue post-mortem, closed clears overdue, BuildClosed shape, isolation across PRs.
- `themis override invoke / close-postmortem / status` CLI subcommands. `invoke` appends both `EMERGENCY_OVERRIDE_INVOKED` and `OVERRIDE_POSTMORTEM_DUE` so the timeline carries the obligation explicitly.
- API endpoints — full parity with the CLI:
  - `POST /v1/tenants/{id}/overrides` — invoke
  - `GET /v1/tenants/{id}/overrides?pr_id=…` — status
  - `POST /v1/tenants/{id}/overrides/postmortem` — close post-mortem
  - Status codes: 400 bad body, 401 missing auth, 404 unknown PR / unknown sub-route, 405 wrong method, 409 already closed.
- Three new registered ledger event kinds: `EMERGENCY_OVERRIDE_INVOKED`, `OVERRIDE_POSTMORTEM_DUE`, `OVERRIDE_POSTMORTEM_CLOSED`. Wiring test extended.

**Notes**

- Constants are derived from the design spec, not configurable: `MinReasonLength = 50`, `DefaultDuration = 24h`, `PostmortemWindow = 7 * 24h`. Plan 11 may add a per-tenant policy override layer for these once the regulator-mapping work begins.
- The `actor != co_signer` rule prevents one human from satisfying the trust requirement alone — the design spec calls this out as the load-bearing piece of the override flow.

### Unreleased — Plan 9 (Approval flows)

**Added**

- `internal/approvals` — pure functions over a ledger slice:
  - `Compute(events, pr_id)` returns the per-PR status: matched decision, granted-role set, sticky denial state, finalisation state, projected verdict.
  - `CanFinalise(status)` reports whether the current state is ripe for `DECISION_FINALISED` (denial → DENY, all required roles granted → ALLOW, otherwise stays REQUIRE_APPROVAL).
  - `BuildFinalised(status, pr_id, now)` constructs the `FinalisedPayload` envelope embedded in the `DECISION_FINALISED` event.
  - 12 unit tests cover: no decision yet, ALLOW passthrough, single-role grant, partial multi-role grant (not finalised), full multi-role grant (ALLOW), denial (DENY), denial sticky across later grants, re-decide resets approvals, already-finalised handling, no-required-roles (any grant ALLOWs), BuildFinalised captures grants, and PR isolation.
- `themis approval grant / deny / status` CLI subcommands. `grant` and `deny` append the corresponding ledger event and emit `DECISION_FINALISED` when the state is ripe; `status` prints the current `Status` as JSON.
- `POST /v1/tenants/{id}/approvals` + `GET /v1/tenants/{id}/approvals?pr_id=…` — same semantics as the CLI, with status codes that surface real failure modes: 400 on bad body, 401 missing auth, 404 unknown PR, 409 already finalised, 405 wrong method.
- Three new registered ledger event kinds: `APPROVAL_GRANTED`, `APPROVAL_DENIED`, `DECISION_FINALISED`. Wiring test extended; all three are reachable through both surfaces.

**Notes**

- Denials are deliberately sticky for the current decision: a later grant doesn't unblock a denial. The only way to clear a denial is to issue a fresh `DECISION_ISSUED` (i.e. re-run `themis decide` after the diff changes), which models real-world re-review semantics correctly.
- The approval logic lives in a pure package with no I/O; the CLI/API layers just append events and re-run `Compute`. This makes the audit story trivial — the ledger alone reconstructs the entire approval history.

### Unreleased — Plan 8 (MCP server + embedded dashboard)

**Added**

- `GET /v1/tenants/{id}/events` — paginated, newest-first ledger feed. Query params: `limit` (1-500, default 50), `offset`, `kind`. Returns `{events, total, returned}`.
- Embedded SPA dashboard served at `GET /` from the binary via `embed.FS`. No build step; vanilla JS + CSS. Reads `?token=` + `?tenant=` from the URL, calls the existing REST endpoints, renders the audit timeline + a JSON detail pane. Kind-filter dropdown maps to the events endpoint.
- `internal/mcp` — stdio JSON-RPC bridge implementing the [Model Context Protocol](https://modelcontextprotocol.io/) v2024-11-05 handshake plus four read-only tools:
  - `themis_health` — ledger health for the configured tenant.
  - `themis_decisions(pr_id)` — most recent DECISION_ISSUED for a PR.
  - `themis_bom(hash)` — canonical signed BOM body (64-hex hash validated).
  - `themis_events(kind?, limit?)` — newest-first timeline.
  All tools route through the existing REST surface so tenant isolation + auth + audit logging stay centralised. 12 unit tests cover handshake, list, every tool, every error path (unknown method/tool, bad version, parse error, notification semantics).
- `themis mcp --base-url <url> --tenant-id <id> --token <t>` — runs the MCP bridge over stdio, handles SIGINT/SIGTERM for clean shutdown.

**Notes**

- The dashboard is intentionally minimal: it surfaces the audit story (timeline + verdict + JSON) so compliance officers can see at a glance what landed. A richer policy editor / BOM viewer lands later.
- The MCP server is the agentic-first surface called out in the design spec (§5.1). It's read-only at Plan 8 — agents can ask "would this PR pass?" via the existing decision history; a `themis_decide` MCP tool is intentionally not exposed because policy must remain deterministic, not agent-driven.

### Unreleased — Plan 7 (Write API + GitHub Action)

**Added**

- `internal/pipeline` — `pipeline.Run(store, tenantID, AIChange, Graph, Policy, bodies, scanners)` extracts the classify→scan→decide orchestration so the CLI (`themis decide`) and the HTTP surface call the same code path and emit the same ledger events in the same order. CLI refactored to delegate to `pipeline.Run`.
- `internal/api/decide.go` — `POST /v1/tenants/{id}/decide`:
  - JSON body `{ai_change, policy_yaml, workdir_files (base64 map, optional)}`.
  - Validates AIChange + parses policy (emits `POLICY_INVALID` on parse failure, then 400).
  - Returns `{pr_id, actor, impact, findings, decision}` with status 200.
  - Auth: Bearer (per-tenant `api-tokens`).
- 9 integration tests covering: happy path, secret-deny via base64 workdir files, missing auth, method-not-allowed, malformed JSON, missing policy, malformed policy (+ POLICY_INVALID ledger emit), invalid base64, invalid AIChange.
- `actions/themis-check/action.yml` — composite GitHub Action with inputs `themis-base-url`, `themis-token`, `tenant-id`, `pr-id`, `policy-path`, `actor`, `fail-on-require-approval`; outputs `verdict`, `decision-json`. Exits non-zero on `DENY` (and optionally `REQUIRE_APPROVAL`).
- `scripts/themis-check.sh` — shim used by the GitHub Action; uses only `curl + jq + git` so no extra dependencies on the runner. Computes per-file SHA-256 hashes against `${{ github.event.pull_request.base.sha }}`.
- `scripts/hooks/pre-push.sh` — local enforcement hook. Lazy-initialises a `local` tenant if missing; runs `themis ingest --adapter git_heuristic` + `themis decide`; fails the push on `DENY`.

**Notes**

- The pre-push hook intentionally falls closed on unknown verdicts so a partial server upgrade can't open a window where developers think they're protected but aren't.
- The Action's outputs use the GitHub-Actions multi-line `<<__THEMIS_EOF__` syntax so the full JSON envelope survives intact for downstream `if:` expressions.

### Unreleased — Plan 6 (REST read API)

**Added**

- `internal/api`:
  - `Tokens(base, id)` — reads `tenants/<id>/api-tokens` (one per line; `#` comments and blanks ignored). Missing file ⇒ deny-all.
  - `RequireToken(base, id, r)` — Bearer-only, constant-time compare to thwart timing-based token guessing.
  - `NewMux(base)` returns a `*http.ServeMux` exposing:
    - `GET /v1/health` — unauthenticated; returns version + `tenants_count`.
    - `GET /v1/tenants/{id}/health` — `ledger.Doctor()` report as JSON.
    - `GET /v1/tenants/{id}/decisions?pr_id=…` — most recent `DECISION_ISSUED` payload for the PR (404 if none).
    - `GET /v1/tenants/{id}/boms/{hash}` and `…/{hash}.sig` — serve the canonical BOM + ed25519 signature sidecar. The hash is validated to be exactly 64 lowercase hex characters so path traversal is structurally impossible.
- `themis serve --base <state> [--addr :8787]` — listens on `addr`, installs SIGINT/SIGTERM handlers for clean shutdown.
- Integration tests cover every endpoint × every status code (200/400/401/404/405) including the path-traversal guard.

**Notes**

- The API is intentionally read-only at Plan 6. The write surface (POST /v1/decide) lands in Plan 7 along with the GitHub Action wrapper.
- The Authorization scheme is `Bearer <token>`; tokens are full opaque strings (no JWT) so the auth path stays trivial to audit.

### Unreleased — Plan 5 (Ingest Adapters)

**Added**

- `internal/ingest`:
  - `Adapter` interface + `Inputs` value type; `Resolve(name)` / `All()` registry.
  - `git_heuristic` — shells `git diff --name-status` against an operator-supplied base ref, hashes each file's content at the base and HEAD blobs, infers the actor from the latest commit's author email (`human:<email>`).
  - `claude_code_transcript` — parses a Claude Code session JSON export, attaches `session_id`/`model`/`user` to metadata, records the SHA-256 of the raw transcript so the audit trail can later prove which transcript was consumed.
  - `manual_attestation` — operator-declared change shape via repeatable `--file path=before,after` flags; only accepts `human:*` actors so it can't be used to retroactively label changes as AI-authored.
  - Every adapter wraps `ErrAdapterFailed` on failure so the CLI routes errors to a single `INGEST_ADAPTER_FAILED` ledger event.
- `internal/ledger` — two new registered kinds: `INGEST_COMPLETED`, `INGEST_ADAPTER_FAILED`. Wiring test extended.
- `themis ingest --id <t> --base <state> --adapter <name> --pr-id <id> [...adapter flags]` — runs an adapter, validates output, writes the AIChange JSON to `tenants/<id>/aichange/<sanitised-pr>.json`, and emits `INGEST_COMPLETED` (or `INGEST_ADAPTER_FAILED`).
- `TestE2E_RealGitRepo` — proves the full pipeline against a real git workspace: `tenant init → catalogue sync → ingest (git_heuristic) → decide → bom build → bom sign → verify`.

**Notes**

- The git_heuristic adapter hashes file contents with raw SHA-256 (not git's own blob SHA) so AIChange hashes are portable across git versions and consistent with the rest of the Themis hashing surface.
- File-flag parsing is deliberately strict — a malformed `--file` value is rejected before any ledger writes happen, so partial state never lands.

### Unreleased — Plan 4 (AI-BOM + Signing)

**Added**

- `internal/bom`:
  - `BOM` value type with `SchemaVersion="themis.bom.v1"`, references the AIChange, Impact, Findings, Decision, and the current LedgerTip.
  - `Canonical(BOM)` produces deterministic, timezone-agnostic JSON bytes — same logical inputs always reproduce byte-identical output (proven by tests).
  - `Hash(BOM)` returns hex SHA-256 of the canonical form.
- `internal/sign`:
  - ed25519 keypair management — `GenerateKey`, `LoadOrGenerate(dir)` (creates on first call, 0o600 priv perms, half-present detection).
  - `Sign(payload, priv)` + `Verify(payload, sig, pub)` with `ErrSign` and `ErrVerify` sentinels for callers to route errors.
- `internal/ledger` — two new registered kinds: `BOM_BUILT`, `BOM_SIGNED`. Wiring test extended.
- `themis bom build --id <t> --base <state> --pr-id <id>` — reconstructs the BOM from the ledger's most recent `DECISION_ISSUED` matching `--pr-id`, prints canonical JSON, emits `BOM_BUILT`.
- `themis bom sign --id <t> --base <state> --pr-id <id>` — rebuilds + signs the BOM with the per-tenant ed25519 keypair (auto-generated on first use), writes `tenants/<id>/bom/<hash>.bom.json` plus `.sig`, emits `BOM_SIGNED` carrying the bom hash, hex signature, and hex public key.
- End-to-end test (`internal/cli/e2e_test.go`) drives the full lifecycle: tenant init → catalogue sync → decide → bom build → bom sign → verify, asserting every expected ledger kind landed and the Merkle chain remains intact.

**Notes**

- Local ed25519 is the air-gapped fallback per design spec §6.1. Sigstore keyless is intentionally deferred to a later plan; the canonical-bytes + Sign/Verify primitives here are the substrate both paths share.

### Unreleased — Plan 3 (Scanners + Policy Engine)

**Added**

- `internal/scan`:
  - `Scanner` interface, `Finding` value type, `Severity` enum (info < low < med < high < critical) with rank ordering.
  - Secrets scanner — AWS access keys, PEM private-key blocks, secret-flavoured key=value pairs (≥ 16 char values).
  - PII scanner — Luhn-validated credit-card-shaped digit runs, SA ID-shaped 13-digit runs (with YYMMDD validity check), email addresses.
  - Both scanners emit redacted descriptions only — raw secret/PII material never leaves the scanner.
  - Property tests: secrets scanner has zero false positives on low-entropy ASCII prose; always fires on AWS prefix + 16 upper-alphanumerics.
  - `RunAll` orchestrator: runs all scanners, captures scanner crashes as `scan_failure` findings, sorts output for deterministic ledger projection.
- `internal/policy`:
  - YAML schema: `Policy { version, default, required_approvers_for_default, rules: [{name, when, then}] }`.
  - `Parse` rejects: missing version, unknown version, missing/invalid default verdict, nameless rules, invalid verdicts in rules, malformed severity clauses. Every parse failure wraps `ErrPolicyInvalid` so the CLI can route to a `POLICY_INVALID` ledger event.
  - Pure `Decide(AIChange, Impact, []Finding, Policy) Decision` — first-rule-wins, fail-closed on missing default.
  - Match clauses: `impact.kind` (list), `impact.domain` (string), `findings.kind` (string), `findings.severity` (`>=info|low|med|high|critical`).
  - Property tests: determinism (same inputs → identical Decision bytes), no-default-always-denies, secret-rule-always-denies on non-DOC_ONLY input.
- `internal/ledger` — three new registered kinds: `SCAN_FINDING`, `DECISION_ISSUED`, `POLICY_INVALID`. Wiring test extended.
- `themis policy lint --file <path>` — parses and validates a policy YAML; non-zero exit on failure.
- `themis decide --id <t> --base <state> --aichange <file> --policy <yaml> [--workdir <dir>] [--catalogue <json>]` — orchestrates classify → scan → policy → ledger:
  1. loads catalogue snapshot,
  2. loads + validates AIChange,
  3. parses policy (emits `POLICY_INVALID` on failure),
  4. classifies into an Impact,
  5. reads workdir file bodies + runs every default scanner,
  6. emits one `SCAN_FINDING` per finding,
  7. runs the policy engine,
  8. emits `DECISION_ISSUED`,
  9. prints the Decision JSON.

**Notes**

- Scanner crashes never abort the decide pipeline — they become `scan_failure`-kind findings at `high` severity so policy can decide whether to require approval (design spec §8.1).
- Findings are stored individually (one ledger event per finding), so adding or removing a scanner does not invalidate historical decisions.

### Unreleased — Plan 2 (Catalogue + Classifier)

**Added**

- `internal/aichange` — `AIChange` value type (the normalised "what this PR did" record), `FileTouch` with `ADDED|MODIFIED|DELETED`, JSON round-trip + `Validate()`.
- `internal/catalogue`:
  - `CatalogueGraph` value type with `Domain`, `Service`, `EventDef` plus `ConsumersOf` / `ProducerOf` / `DomainOfService` lookups.
  - `Parse(root) (CatalogueGraph, error)` — reads EventCatalog v2 markdown front-matter under `domains/*/index.md`, `services/*/index.md`, `events/*/index.md`.
  - `ContentHash` is deterministic over the graph's semantic content (proven by property test — invariant to filesystem ordering, sensitive to field edits).
  - Mini EventCatalog fixture: 2 domains, 4 services, 6 events.
- `internal/classify`:
  - `Impact` + `Kind` with seven classifications: `SCHEMA_BREAKING`, `NEW_EVENT`, `PRODUCER_TOUCH`, `CONSUMER_TOUCH`, `NON_CONTRACT`, `OFF_CATALOGUE`, `DOC_ONLY`.
  - Pure `Classify(AIChange, CatalogueGraph) → Impact`.
  - Property tests: determinism (same inputs → same Impact bytes) and monotonicity-in-evidence (appending touched files never downgrades severity).
- `internal/ledger` — registered two new event kinds in `DefaultRegistry`: `CATALOGUE_SYNCED`, `IMPACT_CLASSIFIED`. Wiring test extended.
- `themis catalogue sync --id <t> --base <state> --source <path>` — parses the catalogue tree, writes a per-tenant `catalogue.json` snapshot, emits `CATALOGUE_SYNCED`.
- `themis classify --id <t> --base <state> --aichange <file>` — classifies an AIChange JSON against the cached catalogue snapshot, emits `IMPACT_CLASSIFIED`, prints the Impact JSON.
- Wiring-guard test: `themis classify` refuses to emit if `IMPACT_CLASSIFIED` is not in the registry (runtime complement to the static wiring test).

**Notes**

- Severity ordering (lowest → highest): `DOC_ONLY` < `OFF_CATALOGUE` < `NON_CONTRACT` < `CONSUMER_TOUCH` < `PRODUCER_TOUCH` < `NEW_EVENT` < `SCHEMA_BREAKING`. `OFF_CATALOGUE` ranks below `NON_CONTRACT` so that adding catalogue-adjacent files never downgrades severity (proven by the monotonicity property test). Out-of-tree changes get bespoke handling via policy YAML, not via inflated severity.

### Unreleased — Plan 1 (Foundation)

**Added**

- Go module scaffold + Makefile + golangci-lint config + GitHub Actions CI workflow + coverage gate.
- `internal/tenant` package — `Tenant` value type, validated IDs (DNS-label-safe), per-tenant filesystem paths, cross-tenant isolation property test.
- `internal/ledger` package:
  - `Event` struct with deterministic SHA-256 content hash + Merkle-style hash chain.
  - Append-only JSONL `Store` with fsync durability and chain-check on every append.
  - SQLite WAL `Projection` with kind-checked, idempotent `Project()`.
  - Event-kind `Registry` + `DefaultRegistry` + wiring test ensuring every used kind is registered before it can be projected.
  - `Replay`, `Verify`, and `Doctor` for ledger reconstruction and integrity checks.
  - Property tests covering hash determinism, hash sensitivity to every field, and `Replay ≡ live Project`.
- `themis` CLI (`cmd/themis`):
  - `themis tenant init` — initialise a tenant directory tree + emit `TENANT_INITIALISED`.
  - `themis ledger doctor / verify / replay` — health (JSON), integrity check, projection rebuild.
- `make vulncheck` target running `govulncheck` against the module.

**Fixed**

- `scripts/cover_check.sh`: `grep -v 'total:' || true | awk …` was binding `||` to the whole pipeline, so the per-package awk reducer never ran and per-package thresholds silently never enforced. Now grouped with braces.
- `scripts/cover_check.sh`: forced `LC_ALL=C` so awk emits `.` (not `,`) as decimal separator on locales where `bc -l` would otherwise fail to parse the per-pkg pct.

**Notes**

- Multi-tenant filesystem isolation enforced at the storage layer (`tenants/<id>/`).
- Pure-Go SQLite driver (`modernc.org/sqlite`) — no CGO, cross-compile friendly, air-gapped-friendly.
- Apache 2.0 licence (per design spec §16).
- Tests use `pgregory.net/rapid` for property testing.
- Coverage gate thresholds calibrated to the highest level achievable without dependency-injected I/O mocks: global 90 %, `internal/tenant` 95 %, `internal/ledger` 90 %, `internal/cli` 90 %, `cmd/themis` exempt (covered indirectly via integration smoke). Wrapped-error branches in `store.go` / `projection.go` for post-marshal `ContentHash`, `bw.Flush` and `fsync` failures after successful writes are structurally unreachable in production paths.



## Documentation

- 📄 **[Executive summary](docs/superpowers/specs/2026-05-22-themis-exec-summary.md)** — one page.
- 📄 **[Full design specification](docs/superpowers/specs/2026-05-22-themis-design.md)** — the canonical artefact.
- 📁 **[Stakeholder briefs index](docs/stakeholders/README.md)** — compliance, engineering, security, exec, anchor pilot proposal.

## About the name

**Themis** is the Greek personification of *divine law, order, and custom* — the principle that decisions follow rules, that the rules are knowable, and that outcomes can be measured against them. The mapping to this product is direct: AI-influenced code changes follow a stated policy, the policy is recorded, every decision is replayable, and the audit trail is independently verifiable. The iconography (Lady Justice with scales) lands instantly with compliance buyers without explanation.

Three practical reasons the name earned its place:

1. **Enterprise-credible without being cute.** Compliance, risk, and regulator audiences read the reference immediately; there's no "what does that mean?" gap to bridge.
2. **One unambiguous pronunciation worldwide.** *THEE-miss.* Short to say, easy to spell, unlikely to mis-hear in a meeting.
3. **Generalises across future modules.** If Themis grows beyond AI-coding governance into general AI-system runtime governance or broader change management, the name still fits. Submodules can take complementary mythological names (`Themis Gateway` = the policy engine, `Themis Ledger` = the AI-BOM store, `Themis Eye` = the dashboard) without forcing a rebrand.

Rejected during naming (search-namespace collisions in our buyer space):
- *Catalyst* — Cisco, Salesforce.
- *Argus* — Argus Cyber Security (incumbent in adjacent infosec).
- *Aegis* — Aegis Group is itself an insurer.
- *Lineage* — Lineage Logistics, Lineage OS.

Fallback if `themis.dev` / `themis.ai` are unobtainable: **Provene** (a provenance-rooted neologism with a clean trademark surface).

## What problem it solves

In regulated environments (insurance, banking, healthcare, public sector), AI-assisted engineering creates an audit gap: when AI writes code, the trail of *which AI, which prompt, which model, which human reviewed it* disappears the moment a pull request merges. Compliance teams reasonably ask: *"prove the AI didn't break a contract, didn't leak data, and that a human signed off on anything that matters."* Today, most organisations cannot.

The unofficial industry answer has been "don't use AI for anything that matters." That costs productivity, breeds workarounds, and isn't sustainable as AI-assisted engineering becomes the norm.

Themis closes the gap: routine AI work flows automatically, contract-affecting work pauses for explicit sign-off, every change leaves a signed audit packet, and silent bypass of the system is detected and alerted.

## Architectural pillars (one-line each)

1. **Adapter-driven ingestion** — one normalised `AIChange` event format; every AI tool plugs in via its own adapter.
2. **[EventCatalog](https://eventcatalog.dev) as ground truth** — we know which events, services, and domains a change affects.
3. **Policy as code** — YAML rules; deterministic verdicts; pure-function engine.
4. **Append-only ledger** — per-tenant `events.jsonl` + SQLite WAL; Merkle-chained for tamper-evidence.
5. **Sigstore-style signing** — cosign keyless by default; local-key fallback for air-gapped.
6. **Multi-tenant by filesystem isolation** — cross-tenant data leakage is physically impossible at the storage layer.
7. **Multiple surfaces, one core** — EventCatalog plugin, CLI, GitHub Action / git hook, web dashboard, REST API.

## First pilot target

An anchor regulated organisation (insurance / financial services, South Africa) — see [the anchor pilot proposal](docs/superpowers/specs/2026-05-22-themis-anchor-pilot-proposal.md) for scope, plan, exit criteria, and risk profile.

## Licence (planned)

- **EventCatalog plugin** (`@themis/eventcatalog-plugin`): Apache 2.0.
- **AI-BOM schema** (`themis-bom-schema`): Apache 2.0.
- **Themis Core**: open-core vs. closed-source initially is a stakeholder decision. Decision needed before code is published.

---

© 2026 Thando Mini. All rights reserved until licence decision finalised.
