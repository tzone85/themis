# Themis

**A compliance gateway for AI-generated code.**

Themis records and governs every change that AI coding tools (Claude Code, Cursor, GitHub Copilot, autonomous agents like [VXD](https://github.com/tzone85/vortex-dispatch)) make to your software. It captures *who* (which AI, which prompt, which human) changed *what* (which event contract, which service, which downstream consumer), proves it cryptographically (signed AI Bill of Materials per pull request, tamper-evident ledger), and stops changes that violate your policies before they merge.

## Status

> **Plan 2 (Catalogue + Classifier) implemented.** Plans 1 and 2 are in: project skeleton, tenant model, append-only Merkle ledger, SQLite WAL projection, replay/verify/doctor, EventCatalog parser, pure `Classify(AIChange, CatalogueGraph) → Impact` with property tests, and the `themis catalogue sync` + `themis classify` CLI commands. See the Changelog below. Plan 3 (scanners + policy engine) is next.

## Changelog

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
