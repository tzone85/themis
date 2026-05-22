# Themis — Design Specification

**Status:** Draft v1 · Awaiting stakeholder review
**Date:** 2026-05-22
**Author:** Thando Mini (with AI pair)
**Audience:** Engineering leads · Compliance · Risk · Executive sponsor · Security
**Confidentiality:** Internal · Pre-pilot

---

## TL;DR (one paragraph)

Themis is a compliance gateway that records and governs every change AI coding tools make to your software. It captures who (which AI, which prompt, which human reviewer) changed what (which event contract, which service, which downstream consumer), proves it cryptographically (signed AI Bill of Materials per pull request, tamper-evident ledger), and stops changes that violate your policies before they merge. It plugs into your existing EventCatalog as ground truth for "what's safe to touch," and into your existing AI tools (Claude Code, Cursor, Copilot, VXD) without forcing a workflow change. The first pilot target is a Sanlam Digisure team; the design generalises to any regulated organisation adopting AI-assisted engineering.

---

## Table of contents

1. [The problem (in plain language)](#1-the-problem)
2. [What Themis is](#2-what-themis-is)
3. [Stakeholder views](#3-stakeholder-views)
4. [How it works — high-level walkthrough](#4-how-it-works)
5. [Architecture](#5-architecture)
6. [Components](#6-components)
7. [Data flow](#7-data-flow)
8. [Error handling & resilience](#8-error-handling)
9. [The trust story (audit, tampering, override)](#9-trust-story)
10. [Testing strategy](#10-testing-strategy)
11. [Pilot plan](#11-pilot-plan)
12. [Roadmap](#12-roadmap)
13. [Open questions, risks, decisions to make](#13-open-questions)
14. [Glossary](#14-glossary)
15. [FAQ](#15-faq)
16. [Appendix A — Architecture diagrams](#appendix-a)
17. [Appendix B — Event taxonomy](#appendix-b)
18. [Appendix C — Policy YAML examples](#appendix-c)
19. [Companion documents](#companion-documents)

---

## 1. The problem

AI coding tools (Claude Code, Cursor, GitHub Copilot, internal agents like VXD) are now writing meaningful percentages of production code in many organisations. Three things make that adoption fragile in regulated environments:

1. **No chain of custody.** When AI writes a line of code, the audit trail typically vanishes after the PR is merged. Compliance officers, auditors, and regulators want to answer: *who or what produced this code, from which prompt, against which version of which model, reviewed by whom, signed off by whom, and against which version of the system's documented contracts?* Today that answer is usually "we don't know."
2. **No risk-aware gating.** Generic AI scanners flag the same way for a typo in a README as for a breaking change to a customer-facing event schema. There is no way to say "AI may freely change documentation, but cannot modify the `NotificationDispatchedEventV2` contract without a senior engineer and a compliance officer signing off."
3. **No safety net for the new failure modes.** AI tools introduce specific risks generic tooling doesn't catch: prompt-injection in commit messages, hallucinated package imports (slopsquatting), PII leaked into prompts, model-version drift between agent runs.

The result: many regulated teams have *unofficial* policies of "don't use AI for anything that matters." That's expensive — they lose the productivity gains — and it's brittle — engineers route around it anyway, and now there's a hidden problem.

> **Themis exists to make AI-assisted engineering safe to approve, easy to audit, and impossible to silently bypass.**

---

## 2. What Themis is

**Themis is a self-hostable service + EventCatalog plugin + CLI** that sits alongside your existing AI tools and:

- **Captures** every AI-touched change (commits, prompts, models, reviewers) by listening to your AI tools through pluggable adapters.
- **Classifies** each change against your EventCatalog — what events, services, domains, and downstream consumers are affected?
- **Scans** for AI-specific risks: secrets in diffs, PII in prompts (hashed by default), slopsquatted or hallucinated package imports, OWASP-class issues in AI output.
- **Decides** using your policies expressed as YAML — allow, require approval (and from whom), or deny.
- **Records** every decision in a tamper-evident append-only ledger; produces a signed **AI Bill of Materials (AI-BOM)** per pull request.
- **Surfaces** the audit trail inside your EventCatalog (so reviewers see AI activity where they already work), in a web dashboard, and via CLI/API.
- **Prevents** silent bypass with deadman-switch detection of missing CI enforcement and Merkle-chained ledger integrity checks.

**It does not** replace your AI tools, your code review, or your security scanners. It composes with them.

---

## 3. Stakeholder views

### 3.1 For the Compliance Officer
**What you get:**
- One place to see every AI-authored change across the organisation, filterable by domain, service, event, person, model, or outcome.
- A signed PDF audit packet per pull request — replayable evidence for internal audit, external auditor, or regulator.
- Policies expressed in version-controlled YAML you can read and review without writing code; every policy version is itself recorded in the ledger.
- Tamper-evident logs: any silent edit to the audit history is detected and surfaced.
- Confidence that emergency overrides require an explicit, time-boxed reason from a named authority and leave a permanent red banner on the audit packet.

**What changes for you day-to-day:** Instead of saying "no AI in regulated code" or "AI requires manual audit" (both expensive), you write the policy once. Themis enforces it consistently and produces the evidence by default.

### 3.2 For the Engineering Lead
**What you get:**
- AI-assisted engineering remains available to your team — without each PR turning into a meeting.
- Routine AI changes (docs, tests, internal refactors) flow through automatically; only changes that genuinely warrant senior review trigger one.
- Your team's tools (Claude Code, Cursor, VXD) keep working as-is — Themis is a CI gate plus an out-of-band recorder.
- Faster onboarding for compliance-sensitive features because the rules are explicit.

**What changes for your team:** A `themis verify` check appears on PRs. Most go green automatically. Some ask for an explicit approval click. None are silently merged.

### 3.3 For the Executive Sponsor
**What you get:**
- AI productivity adoption ceases to be a compliance liability.
- One artefact (the audit packet) answers regulator questions about AI use that would otherwise take weeks to assemble.
- Position to publish a defensible AI-engineering posture externally (board, regulators, customers).
- A reusable asset: Themis runs on every codebase, not one project at a time.

**Cost:** Engineering time (one team, one quarter for pilot); modest infra (self-hosted; one binary, one SQLite file per tenant). No per-seat licensing of third-party tools.

### 3.4 For the Security Team
**What you get:**
- A consistent place to plug AI-specific scanners (secrets, slopsquatting, prompt-injection, PII heuristics) into the existing PR flow.
- Deadman's-switch detection: if a developer or attacker removes the required CI check, Themis raises an alarm.
- Cryptographic provenance for every AI-generated artefact — SBOM-style answers to "where did this code come from?" — using Sigstore (industry-standard, no key management for you).
- A clear extension point to add organisation-specific scanners (Sanlam-internal rules, PCI/POPIA-specific PII patterns).

---

## 4. How it works

### Walkthrough — the lifecycle of one AI-touched PR

1. A developer (or autonomous agent like VXD) opens a pull request.
2. CI runs `themis verify`. Themis pulls evidence: the git diff, commit trailers, any attached AI tool transcript (Claude Code session, Cursor MCP log, Copilot audit export), VXD event log slice — whatever is available. Adapters normalise all of these into a single `AIChange` record.
3. Themis loads the latest snapshot of your EventCatalog and computes **what the PR actually changes** in domain terms: which events, which services, breaking-or-not, owners, downstream consumers.
4. Themis runs its scanners in parallel: secrets, PII heuristic, slopsquat, hallucinated imports.
5. The **Policy Engine** — a pure function — takes `(AIChange, Impact, Findings, Policy)` and returns a `Decision`: ALLOW, REQUIRE_APPROVAL (with which approvers), or DENY.
6. Themis canonicalises everything into a signed **AI Bill of Materials**, attests it via Sigstore, and writes the decision + the BOM digest to the ledger.
7. The PR's CI check turns green (ALLOW), yellow (REQUIRE_APPROVAL), or red (DENY).
8. If approval is required, named approvers click "approve" in the EventCatalog plugin or the Themis dashboard; the approval itself becomes a ledger event.
9. The PR merges. The audit packet is now permanent, signed, queryable.

### Walkthrough — the compliance officer's morning

1. Open the EventCatalog. There's a new "AI Activity" tab next to existing event pages.
2. Filter: "all AI-authored changes touching the Collections domain in the last 30 days."
3. See a timeline. Click any item → the signed AI-BOM, the policy version that decided it, the reasons, the scanner findings, the approval chain.
4. Export a signed PDF audit packet for the auditor visit next week.

### Walkthrough — when something goes wrong

- An AI agent tries to commit a hardcoded API key. Themis denies the PR; the finding is in the ledger; the developer sees a clear error.
- The build fails because Themis's CI check is now mandatory. A developer removes the check from the repo's required-status-checks settings to bypass it. Themis's heartbeat detects the missing enforcement within minutes and pages the on-call security engineer.
- A service outage prevents Sigstore from signing. The PR check turns yellow ("BOM unsigned — held"); policy treats it as REQUIRE_APPROVAL until Sigstore returns or a tenant-configured local-key fallback signs it. No PRs are silently merged unsigned.

---

## 5. Architecture

### 5.1 Architectural pillars
1. **Adapter-driven ingestion.** Single normalised `AIChange` event format. Each AI tool gets its own adapter; the core never depends on tool specifics.
2. **EventCatalog as ground truth.** Themis reads the catalogue at MVP (parses domains, services, events, AsyncAPI/OpenAPI schemas) and builds a dependency graph.
3. **Policy as code.** YAML rules live in the consuming repo. The engine is a pure function — deterministic, replayable, testable.
4. **Append-only ledger.** Per-tenant `events.jsonl` + SQLite WAL projection. Merkle-chained for tamper-evidence.
5. **Sigstore-style signing.** Cosign keyless attestation by default; local keypair fallback for air-gapped tenants.
6. **Multi-tenant from day one.** Per-tenant filesystem isolation (`tenants/<id>/` paths) so cross-tenant leakage is physically prevented, not merely WHERE-clause-prevented.
7. **Multiple surfaces, one core.** EventCatalog plugin, CLI, git/CI hook, web dashboard, REST API — all read from the same ledger; none can write outside the documented event taxonomy.

### 5.2 Top-level diagram
See [Appendix A](#appendix-a) for the full diagram. In words: AI tool sources fan in through adapters → ingest → classify → scan → policy → AI-BOM build → sign → ledger. Surfaces read from the ledger projection; they never write to it.

### 5.3 Tech stack
- **Themis Core**: Go (matches existing VXD/NXD muscle memory; static binary; air-gappable).
- **EventCatalog plugin**: TypeScript / Astro (native to the EventCatalog v2 stack).
- **Storage**: SQLite WAL + `events.jsonl` per tenant. No external database.
- **Signing**: Sigstore (cosign keyless) by default; ed25519 local keypair fallback.
- **Memory bridge**: Mempalace (per-tenant wing) for cross-repo search of past decisions.
- **CLI / TUI**: Cobra + Bubbletea (consistent with VXD).

### 5.4 Hosting modes
All produced from the same binary:
- **Self-hosted single-tenant** — one org, one binary. Pilot mode.
- **Self-hosted multi-tenant** — one binary, multiple tenant directories. For org groups (e.g. Sanlam Group with multiple regulated entities).
- **Managed SaaS (future)** — Themis-operated; same code, tenant dirs on managed storage.
- **Air-gapped** — same binary, no outbound network. Local-key signing replaces Sigstore.

---

## 6. Components

### 6.1 Core packages (Go, under `themis/internal/`)

| Package | Purpose | Notes |
|---|---|---|
| `tenant` | Resolves API-key → `tenant_id`. Owns `tenants/<id>/` paths. Every downstream call takes a `Tenant` explicitly. | No globals; wiring test enforces. |
| `ingest` | Adapter interface + concrete adapters. Normalises to `AIChange` events. | Adapters: `vxd`, `claude_code_transcript`, `cursor_mcp`, `copilot_audit`, `git_heuristic`, `manual_attestation`, plus `null` for tests. |
| `catalogue` | Parses EventCatalog repo (Markdown + AsyncAPI + OpenAPI). Builds dependency graph. | Read-only at MVP. v2 may propose updates. |
| `classify` | Pure: `(AIChange, CatalogueGraph) → Impact`. | `kind ∈ {SCHEMA_BREAKING, NEW_EVENT, CONSUMER_TOUCH, PRODUCER_TOUCH, DOC_ONLY, OFF_CATALOGUE, NON_CONTRACT}`. |
| `policy` | YAML rule engine. Pure: `(AIChange, Impact, Findings, Policy) → Decision`. | Fail-closed on missing default. |
| `scan` | Active scanners: secrets, PII heuristic, slopsquatting, hallucinated imports. | Each = interface; pluggable. |
| `bom` | Builds canonical `themis.bom.v1` JSON-LD per PR. | Schema versioned + published openly. |
| `sign` | Sigstore (cosign keyless) by default; ed25519 local keypair fallback. | Verification helpers included. |
| `ledger` | Append-only `events.jsonl` + SQLite WAL projection. Merkle-chained. | Per tenant. Wiring tests enforce no silent event drops. |
| `mempalace` | Bridge: writes selected drawers to per-tenant Mempalace wing; read-only retrieval. | Decouples Themis from Mempalace schema evolution. |
| `surface/cli` | `themis verify / sign / policy lint / ledger query / catalogue sync / tenant init / ledger doctor / ledger verify`. | Statically linked binary. |
| `surface/api` | REST + WebSocket. API-key auth at MVP; OIDC/SAML in v2. | OpenAPI spec emitted; versioned `/v1/`. |
| `surface/web` | Embedded static SPA: tenant home, audit timeline, BOM viewer, policy editor, scan findings. | Air-gappable; single binary. |
| `surface/githook` | Pre-receive / pre-push hook + GitHub Action wrapper. | Fails the build on policy denial. |
| `auth` | API keys + per-tenant role model: `dev`, `reviewer`, `compliance`, `admin`. | OIDC/SAML slot in at v2. |
| `runtime/secrets` | Pluggable secrets sources (env, file, AWS SM, Vault, Doppler). | Per tenant. |

### 6.2 Separate packages

| Package | Lang | Purpose |
|---|---|---|
| `@themis/eventcatalog-plugin` | TypeScript | EventCatalog v2 plugin. Open-sourced (Apache 2.0). |
| `@themis/sdk-node` | TypeScript | Thin Node SDK for the REST API. |
| `themis-sdk-go` | Go | Same idea for Go consumers (VXD dashboard integration). |
| `themis-bom-schema` | JSON Schema | Published, versioned AI-BOM schema. Independent so other tools can produce/consume. |

### 6.3 Multi-tenancy model
Per-tenant filesystem isolation under `tenants/<tenant_id>/`. Cross-tenant queries are physically impossible at the storage layer. Every request handler resolves `Tenant` early; every downstream call takes a `Tenant` explicitly. Wiring test ensures no package can construct `ledger`, `scan`, or `policy` without a `Tenant`.

### 6.4 What we deliberately defer
- Billing / metering (post-pilot).
- Full identity provider — start with API keys; SSO is v2.
- Custom scanner marketplace — bundled scanners only at MVP; plugin loader designed, not advertised.
- Notification fan-out (Slack/email/Teams) — webhooks at MVP.

---

## 7. Data flow

### 7.1 The hot path (one PR end-to-end)
Detailed in [§4 Walkthrough](#4-how-it-works) and [Appendix A](#appendix-a). Key invariants:
- Every step emits a ledger event **before** producing externally visible side effects.
- Scanners run in parallel; their findings are stored individually so adding/removing scanners doesn't invalidate historical decisions.
- The policy engine is the only place that issues a verdict, and it is pure — `(inputs, policy) → decision` is deterministic forever.

### 7.2 Catalogue sync (cold path)
Tenant configures source in `tenant.yaml` (git URL + ref + refresh cadence). `themis catalogue sync` clones, parses, builds CatalogueGraph, computes diff against previous graph, emits `CATALOGUE_SYNCED`, re-classifies any in-flight PRs.

### 7.3 Audit query (compliance officer path)
Web UI or `themis ledger query`. Filter by entity / person / outcome / scanner / free-text (Mempalace-powered). Returns signed PDF audit packet or JSON. **Every export is itself logged** as `AUDIT_EXPORTED`.

### 7.4 Replay & recovery
On startup or `themis ledger replay`: truncate SQLite projection, stream `events.jsonl` from offset 0, apply each via `Project()`. Wiring tests guarantee every event type has a `Project()` branch. Final state is byte-identical regardless of crash count.

### 7.5 Data residency / privacy
- **Prompts stored as content-addressed hashes by default** (SHA-256). Verbatim text is opt-in per tenant.
- **PII detected by scanners is redacted in stored findings** ("credit-card-shaped string at file.go:142", not the digits).
- **All disk paths include tenant_id**; ledger queries take tenant context; no global mutable state.

---

## 8. Error handling & resilience

### 8.1 Principle
**Every failure becomes a ledger event.** A missing decision is itself a finding. Silent fall-through is not allowed.

### 8.2 Failure response table
| Layer | Failure | Response |
|---|---|---|
| Ingest | Adapter unreachable | EMIT `INGEST_PARTIAL`; continue with available evidence; BOM marked `evidence_completeness=partial`; policy may demote to `REQUIRE_APPROVAL`. |
| Ingest | Malformed payload | EMIT `INGEST_ADAPTER_FAILED`; circuit-breaker after 3; alert; never fallback to "no AI detected." |
| Catalogue | Repo unreachable | EMIT `CATALOGUE_STALE`; continue with cached graph; refuse decisions if cache > `max_staleness` (default 24h). |
| Catalogue | Parse error | EMIT `CATALOGUE_PARSE_FAILED`; preserve previous good cache; alert. |
| Classify | Unmappable file | Classify as `NON_CONTRACT/unmappable`; policy decides. |
| Scan | Timeout / crash | EMIT `SCAN_TIMEOUT` or `SCAN_CRASHED`; other scanners continue; circuit-breaker after 3. |
| Policy | Invalid YAML | `themis policy lint` at commit time; at runtime, EMIT `POLICY_INVALID` and refuse all decisions for that tenant until fixed. **Fail closed.** |
| Policy | No matching rule | Tenant MUST declare a `default` verdict. None → misconfigured → fail closed. |
| BOM | Signing unavailable | EMIT `BOM_UNSIGNED`; BOM still stored; decision held in `REQUIRE_APPROVAL` until signed. |
| Ledger | Write failure | fsync + checksum on every write; failed write → 5xx + non-zero CLI exit + red PR check. |
| Ledger | Projection drift | Startup consistency check (`themis ledger doctor`); mismatch → read-only mode + alert + `themis ledger replay`. |
| Surface | Web dashboard down | Surfaces are read-only consumers; decisions unaffected. |
| Auth | Invalid API key | 401 + EMIT `AUTH_FAILED`; rate-limit by source IP. |
| Tenant | Key misroute | Reject; EMIT `TENANT_ISOLATION_BREACH_ATTEMPT`; page immediately. |

### 8.3 Crash recovery
- No global mutable state; every long-running operation has a checkpoint event.
- Long scans emit `SCAN_HEARTBEAT`; watchdog detects stuck.
- In-flight PR with daemon death: on restart, scan ledger for `DECISION_ISSUED`-less ingested PRs; resume from last step.
- Lock files per tenant prevent two daemons writing the same ledger.

### 8.4 Graceful degradation matrix
| Failure | Decisions still made? | BOM signed? | Audit intact? |
|---|---|---|---|
| Catalogue cached + unreachable | Yes (with stale warning) | Yes | Yes |
| All scanners down | Yes (often demoted to REQUIRE_APPROVAL) | Yes | Yes |
| Sigstore down | No (fail closed) unless local-key fallback configured | No | Yes |
| Mempalace unavailable | Yes (drawer write retried) | Yes | Yes (ledger is authoritative) |
| Web dashboard down | Yes | Yes | Yes |
| Disk 100% full | No (writes refused; PR red) | No | Yes (existing trail unaffected) |

---

## 9. Trust story

### 9.1 Three trust-layer failure modes (the ones regulators probe)

**1. Emergency override.** Sometimes legitimate. Requires:
- Named actor identity.
- Reason ≥ 50 characters.
- Tenant-configured override authority (default: `compliance` role + at least one `senior` co-sign).
- Time-boxed (default 24h) and scope-boxed (one PR or one tenant).
- Auto-generated 7-day post-mortem ledger entry that compliance team must close out.
- Permanent red banner on the audit packet: "merged via emergency override; reason; co-signers."

**2. Silent disable.** Someone removes the GitHub Action / pre-receive hook.
- Themis heartbeat calls tenant repos via GitHub API to verify the required Action is installed.
- Missing → EMIT `ENFORCEMENT_MISSING { repo, expected_check, last_seen }` → immediate alert.
- "Absence of signal is itself a signal" — the deadman's switch.

**3. Ledger tampering.** Someone edits `events.jsonl` directly.
- Every event has a content hash; each references the prior event's hash (Merkle-style chain).
- `themis ledger verify` walks the chain. Break → EMIT `LEDGER_INTEGRITY_BROKEN`, mark tenant read-only, page admin.
- Optional weekly `LEDGER_ANCHOR` event publishes tip-hash to external sink (S3 with object-lock, public git repo, transparency log). Per-tenant opt-in.

### 9.2 What we explicitly do not do
- **No retries on policy decisions.** Same inputs → same verdict, always.
- **No best-effort scanning.** Either we have evidence + verdict, or we say "I can't decide — escalate."
- **No external-network calls in the hot path** unless configured (Sigstore is the one allowed exception).

---

## 10. Testing strategy

### 10.1 Coverage targets
| Type | Target | Where |
|---|---|---|
| Pure-function unit tests | **100%** | `classify`, `policy`, `bom`, `sign`, normaliser in `ingest` |
| Stateful unit tests | **95%** | `ledger`, `catalogue`, `tenant`, `scan/*`, `auth` |
| Adapter tests with fixtures | **90%** | Each `ingest/*` adapter |
| Property tests | All pure functions in `policy`, `classify`, `bom` | `*_property_test.go` (using `rapid`) |
| Fuzz tests | Parsers (catalogue, policy YAML, BOM), API bodies | `*_fuzz_test.go` |
| Integration tests | All hot-path flows | `internal/engine` suite |
| E2E tests | All 4 surfaces × all 3 hosting modes | `e2e/` |

Global gate: **≥95% overall coverage**, enforced in CI; per-package thresholds in `coverage.thresholds.yaml`.

### 10.2 Test categories that earn their keep
- **Wiring tests** — every new event type must have a `Project()` branch *and* a test asserting it gets projected. Same pattern as VXD's `wiring_test.go`, which has already caught two silent-event-drop bugs.
- **Property tests** — `classify` produces internally consistent Impacts; `policy` is deterministic and monotonic-in-the-safe-direction; `bom` round-trips; `ledger` replay equals project.
- **Multi-tenant isolation tests** — instrument filesystem writes; assert no cross-tenant writes ever happen.
- **Integrity & tampering tests** — Merkle chain break detected; deadman's switch triggers; override missing pieces rejected.
- **Replay tests** — kill daemon mid-write; restart; byte-identical projection.
- **Adapter golden tests** — each adapter has `testdata/<adapter>/` with fixtures and golden JSON; behaviour drift impossible to land silently.
- **E2E binary tests** — four canonical scenarios across all three hosting modes (single-tenant, multi-tenant, air-gapped).
- **Performance/load** — 100 PRs/min sustained 10 min, p95 < 2s; 10k-event catalogue parses < 30s; 1M-event ledger replays < 60s.
- **Security tests** — `gosec`, `staticcheck`, `govulncheck` gates; fuzz parsers; lint testdata for accidental real secrets; **Themis runs on Themis** in CI (we eat our own dogfood).

### 10.3 TDD workflow rules
- Tests first, every cycle. RED → GREEN → REFACTOR.
- Wiring test required for any new event type; CI rejects without it.
- Property test required for any new pure function in `classify`/`policy`/`bom`.
- Coverage gate fails PR on regression.
- No real `time.Sleep` in tests; no network in unit tests; deterministic IDs only.

---

## 11. Pilot plan

> **Timeline note.** Pilot weeks (below) run **after** MVP delivery (see §12.1). The MVP build (Themis Core single-tenant + EventCatalog plugin + four bundled scanners + four canonical adapters) takes ~6 weeks from project kickoff. The 90-day pilot (12 weeks below) starts at MVP+0. Pilot Phase 1 (weeks 1–2) can begin while the last two weeks of MVP are in flight (stakeholder identification + data-handling addendum + infra ticket can all be progressed in parallel) — Phase 2 deployment, however, requires the MVP binary to exist.

### 11.1 First pilot — Sanlam Digisure
**Why:** Themis's author works at Sanlam. Digisure already runs EventCatalog. The compliance + risk posture of Sanlam Group is conservative enough that any defensible AI-engineering tool is welcome. The catalogue has 3 domains (CapstonePAS, Collections, Trustflow), nested subdomains, services, and event schemas — enough surface to prove the classifier without overwhelming the pilot.

### 11.2 Pilot scope (90 days)
**Week 1–2:** Stakeholder alignment.
- Identify pilot team (one Digisure squad with active AI-tool usage).
- Identify compliance sponsor (Sanlam Digisure compliance / risk lead).
- Identify engineering sponsor (squad tech lead).
- Confirm catalogue access (read-only fork or pull from origin).
- Sign internal data-handling addendum (hashed-prompts default, full text opt-out).

**Week 3–6:** Themis MVP deployment (single-tenant, self-hosted in Sanlam infra).
- Pilot team's repo(s) wired to Themis (GitHub Action installed; pre-receive optional).
- Catalogue ingestion live; policies drafted with compliance sponsor in YAML.
- Mempalace per-tenant wing initialised; existing repo history mined to seed.
- Web dashboard available to pilot team + compliance sponsor.

**Week 7–10:** Soak.
- All AI-touched PRs flow through Themis.
- Weekly review with compliance sponsor of any DENY or REQUIRE_APPROVAL outcomes; policy refined.
- Audit packets generated for a synthetic compliance-officer "audit visit" exercise.

**Week 11–12:** Evaluation + decision.
- Metrics: % of AI PRs gated, false-positive rate, average decision latency, audit-packet generation time, deadman's-switch detections, override invocations.
- Compliance sponsor produces a written assessment: is the artefact (audit packet + ledger) sufficient for the next external audit?
- Joint decision: expand to second squad / second Sanlam entity, or iterate.

### 11.3 Risks to the pilot
- **Catalogue drift** — if the catalogue is itself out of date vs. reality, Themis classifies wrong. Mitigation: include "catalogue freshness" as a tenant-level metric.
- **Policy authoring overhead** — compliance officer + tech lead need to co-write the first policy. Mitigation: ship 3 starter policy templates (conservative, balanced, permissive).
- **Tool adapter blind spots** — if pilot team uses an AI tool we haven't built an adapter for yet, we fall back to git heuristic. Mitigation: confirm tool list in week 1; build adapter in week 2 if needed.
- **Performance** — if Themis adds noticeable PR latency, devs route around it. Mitigation: p95 < 2s target enforced in CI; latency dashboard visible to pilot team.

---

## 12. Roadmap

### 12.1 MVP (weeks 1–6 from project kickoff)
Catalyst (the "C now, A roadmap" decision): EventCatalog-native plugin + Themis Core single-tenant + VXD + Claude Code adapters + the four bundled scanners + REST API + CLI + web dashboard. SQLite ledger. Sigstore keyless signing. Goal: binary + plugin ready for Sanlam pilot deployment.

> The MVP **build** (weeks 1–6 from kickoff) precedes the **pilot** (12 weeks; see §11). The pilot's Phase 1 (alignment) can run in parallel with weeks 5–6 of the MVP build to compress total elapsed time.

### 12.2 GA (months 3–6)
Multi-tenant hardening. Cursor + Copilot adapters. OIDC/SAML SSO. Notification fan-out (Slack/Teams/email). Air-gapped mode validated with a second buyer. Hosted SaaS (managed) optional offering. AI-BOM schema v2 (incorporating pilot feedback).

### 12.3 AEGIS expansion (months 6–12)
Generalise beyond EventCatalog. Generic dependency-graph ingest (Backstage, OpenAPI/Protobuf-only orgs, monorepo-without-catalogue). Custom scanner marketplace. Policy templates library (per-regulator: POPIA, PCI, HIPAA, SOC2, ISO 27001). Multi-buyer reference architectures.

### 12.4 Possible adjacencies (deferred decisions)
- "Themis Compliance Pack" — bundled policies + scanners pre-mapped to specific regulator language.
- Integrations with GRC platforms (ServiceNow GRC, Workiva, Drata).
- "Themis for IT change management" — extend beyond AI code changes to general change management with the same audit substrate.

---

## 13. Open questions

These are flagged for stakeholder resolution before implementation begins.

1. **Pilot team selection.** Which Sanlam Digisure squad? Need a sponsor name by week 1.
2. **Hosting decision.** Self-hosted in Sanlam infra vs. Themis-managed VM in Sanlam tenancy. The former is more aligned with conservative compliance posture; the latter is faster to stand up.
3. **Policy ownership.** Who writes the first policy? (Proposed: compliance sponsor + tech lead, with Themis providing 3 starter templates.)
4. **Catalogue write-back.** v1 is read-only. Should we plan v2 catalogue-update PRs explicitly into the roadmap, or wait for pilot feedback?
5. **Sanlam IP / open-source split.** The EventCatalog plugin is intended Apache 2.0. The Themis Core can be open-core (community edition + paid features) or closed-source initially. Need exec view.
6. **External anchoring default.** Per-tenant opt-in is the proposed default. Sanlam may want opt-out everywhere (no outbound calls); confirm.

---

## 14. Glossary

- **AI-BOM (AI Bill of Materials)** — A signed JSON-LD document attached to a pull request, describing every AI-authored aspect: prompts (hashed), models, versions, reviewers, scanners run, findings, policy decision, signing chain.
- **CatalogueGraph** — Themis's in-memory representation of the EventCatalog: nodes (domains, services, events) and edges (produces, consumes, owns, depends-on).
- **Classifier** — The pure function that takes an `AIChange` and `CatalogueGraph` and returns an `Impact` ("breaking change to NotificationDispatchedEventV2, owned by Team X, with 3 downstream consumers").
- **Decision** — The policy engine's output: `ALLOW` / `REQUIRE_APPROVAL` (with required approvers) / `DENY`.
- **Deadman's switch** — A heartbeat that detects the *absence* of an expected signal — used here to detect when a required CI enforcement has been silently removed.
- **EventCatalog** — Open-source documentation tool for event-driven architectures (eventcatalog.dev). Sanlam Digisure runs an instance.
- **Hallucinated import / slopsquatting** — AI tools sometimes suggest package names that do not exist (hallucinated) or are typo-squats of real packages (slopsquatting), both of which are critical supply-chain risks.
- **Hosting modes** — Single-tenant, multi-tenant, managed SaaS, air-gapped. All produced from the same binary.
- **Ledger** — Themis's append-only `events.jsonl` plus a SQLite WAL projection for queries. Per tenant. Merkle-chained.
- **Mempalace** — Local-first memory store used across Sanlam projects; Themis writes per-tenant drawers for cross-repo search of past AI decisions.
- **Merkle chain** — Each event references the prior event's content hash, making silent edits detectable.
- **Policy** — YAML rules expressed in the consuming repo's `themis.yaml`. Reviewed and versioned like code. Pure function input alongside `AIChange` and `Impact`.
- **Sigstore (cosign)** — Keyless code-signing infrastructure (sigstore.dev) used to attest AI-BOMs without managing private keys.
- **Tenant** — One isolated customer / org / business unit. Multi-tenant means many tenants share the binary; per-tenant means each gets its own directory tree.
- **VXD / NXD** — Vortex Dispatch / Nexus Dispatch — existing Go AI-agent orchestrators authored by Thando; Themis ingests their event logs natively.
- **Wiring test** — A test that asserts a feature is *activated*, not just *implemented* — e.g. that a new event type has a `Project()` branch in the ledger projector.

---

## 15. FAQ

**Q: Does this slow down our engineering team?**
A: For typical AI changes (docs, internal refactors), Themis adds < 2 seconds to PR CI. For changes that genuinely touch contract surfaces (event schemas, public APIs), it adds a review checkbox — which is what the policy intends.

**Q: Does this require us to change our AI tools?**
A: No. Themis adapts to the tools you use. If your team uses Claude Code, Cursor, Copilot, or an internal agent, Themis listens to whichever ones expose evidence. For tools with no machine-readable AI signal, Themis falls back to heuristic detection (commit trailers, entropy, comment style).

**Q: What if Themis is wrong?**
A: Decisions are deterministic and explainable. Every decision shows you the rule that fired, the inputs it saw, and the reasoning. If a policy rule is wrong, you change the policy YAML (which itself versions in git). If Themis itself is wrong, the issue is reproducible (replay the same inputs against the same policy version).

**Q: Can engineers bypass it?**
A: There are three controlled bypass paths and one detected illicit one:
- *Emergency override* (controlled): named authority, reason ≥ 50 chars, time-boxed, scope-boxed, permanent red banner on audit.
- *Policy update* (controlled): change the YAML, get it reviewed like any other code, ship.
- *Tool not yet adapted* (controlled): Themis classifies as `NON_CONTRACT/unattributed`; policy decides default behaviour.
- *Removing the CI check* (detected illicit): deadman's switch fires and alerts.

**Q: What about prompts containing customer data?**
A: By default, prompts are stored as SHA-256 hashes only. Verbatim prompt text is opt-in per tenant. PII surfaced by scanners is redacted in findings ("credit-card-shaped string at file.go:142", not the digits).

**Q: What happens if Sigstore is down?**
A: PR check turns yellow ("BOM unsigned — held"); policy treats it as REQUIRE_APPROVAL. Tenants with a local-key fallback configured continue signing. No PRs are silently merged unsigned.

**Q: What if we want to use this on a non-event-driven codebase?**
A: At MVP, Themis classifies non-catalogue files as `NON_CONTRACT` — policy still decides what to do. The full AEGIS-mode (generic dependency-graph ingest) is on the roadmap (months 6–12).

**Q: Is this just SLSA / in-toto / GitHub Advanced Security in disguise?**
A: No. SLSA, in-toto, and similar attestations describe *build* provenance (source → artefact). Themis describes *authorship* provenance (prompt → human → model → diff → policy decision → signed audit), and pairs it with *contract-aware* policy gates that none of those tools provide. We complement them; we can in fact emit SLSA-compatible attestations alongside our own.

**Q: Open source?**
A: The EventCatalog plugin is intended Apache 2.0 (community distribution channel). The Core's licensing is a stakeholder decision (open-core vs. closed source initially). The AI-BOM schema is openly published either way.

**Q: How do we know the ledger hasn't been tampered with?**
A: Merkle chain + integrity-check command + optional external anchoring (per-tenant opt-in). Combined, these match the trust model of Certificate Transparency / Sigstore Rekor — well-understood by security buyers.

**Q: Who maintains the policies as the catalogue evolves?**
A: Policies live in git and are reviewed like code. We provide three starter templates (conservative / balanced / permissive). The compliance officer + tech lead co-own the policy file for their domain. Policy versions are themselves recorded in the ledger.

---

## Appendix A — Architecture diagrams

### A.1 Component diagram
```
                       ┌───────────────────────────────────────────────┐
                       │           AI tool sources (adapters)          │
                       │  VXD events.jsonl │ Claude Code transcripts │ │
                       │  Cursor MCP feed  │ Copilot audit log API   │ │
                       │  Git commit msgs (heuristic + trailer)       │
                       └────────────────────┬──────────────────────────┘
                                            │ normalized: AIChange
                                            ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         Themis Core (Go service)                     │
│  ┌────────────────┐  ┌──────────────────┐  ┌──────────────────────┐ │
│  │ Ingestor       │→ │ Classifier       │→ │ Policy Engine        │ │
│  │ (adapters)     │  │ (vs EventCatalog)│  │ (rules + sign-off)   │ │
│  └────────────────┘  └──────────────────┘  └──────────────────────┘ │
│         │                    │                       │              │
│         ▼                    ▼                       ▼              │
│  ┌────────────────┐  ┌──────────────────┐  ┌──────────────────────┐ │
│  │ Scanner        │  │ AI-BOM Builder   │  │ Attestation Signer   │ │
│  │ (secrets/PII/  │  │ (canonical doc)  │  │ (Sigstore / cosign)  │ │
│  │  slopsquatting)│  │                  │  │                      │ │
│  └────────────────┘  └──────────────────┘  └──────────────────────┘ │
│         │                    │                       │              │
│         └──────────┬─────────┴───────────────────────┘              │
│                    ▼                                                 │
│  ┌────────────────────────────────────────────────────────────┐    │
│  │  Append-only Ledger (event-sourced, SQLite WAL + JSONL)    │    │
│  │  + Mempalace integration (wing per tenant)                 │    │
│  └────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
            ┌────────────────────────────────────────────┐
            │  Surfaces (all read-only consumers)        │
            │  • EventCatalog plugin tab (audit/AI-BOM)  │
            │  • CLI (`themis verify / sign / query`)    │
            │  • GitHub Action / pre-receive hook        │
            │  • Web dashboard (Compliance Officer view) │
            │  • REST API + Mempalace search             │
            └────────────────────────────────────────────┘
```

### A.2 PR-lifecycle sequence (events emitted)
```
INGEST_COMPLETED  →  IMPACT_CLASSIFIED  →  SCAN_FINDING* →  DECISION_ISSUED
        ↓                                                          ↓
  BOM built ───────────────────────────────────────────► BOM_SIGNED
                                                                 ↓
                              [if REQUIRE_APPROVAL]               │
                                  APPROVAL_GRANTED ──→ DECISION_FINALISED
                                                                 ↓
                                                            PR_MERGED
```
On any failure: corresponding `*_FAILED` / `*_STALE` / `*_PARTIAL` event is emitted instead of (or in addition to) the success event. No path silently drops.

### A.3 Multi-tenant filesystem layout
```
/var/lib/themis/                          (or ~/.themis/ for self-hosted dev)
├── config.yaml                            (binary-level config)
└── tenants/
    ├── sanlam-digisure/                  (one directory per tenant)
    │   ├── tenant.yaml                   (catalogue source, policies path, secrets refs)
    │   ├── events.jsonl                  (append-only ledger)
    │   ├── projection.sqlite             (WAL-mode projection)
    │   ├── catalogue-cache/              (last good EventCatalog snapshot)
    │   ├── bom/                          (signed AI-BOM artefacts per PR)
    │   └── mempalace-wing/               (Mempalace drawers for this tenant)
    └── <next-tenant>/                    (same shape; physically isolated)
```

---

## Appendix B — Event taxonomy (v1)

Reserved event types in the Themis ledger. Every type must have a `Project()` branch in the projector; the wiring test enforces this. The full registry is in `internal/ledger/events.go`.

| Event | When emitted |
|---|---|
| `INGEST_COMPLETED` | Adapter fan-in produced a normalised `AIChange`. |
| `INGEST_PARTIAL` | One or more adapters failed; continuing with partial evidence. |
| `INGEST_ADAPTER_FAILED` | Adapter produced malformed payload or unreachable. |
| `CATALOGUE_SYNCED` | Catalogue refresh completed; includes diff summary + content-hash. |
| `CATALOGUE_STALE` | Catalogue cache is older than configured max-staleness. |
| `CATALOGUE_PARSE_FAILED` | Catalogue could not be parsed; previous cache retained. |
| `IMPACT_CLASSIFIED` | Classifier produced an `Impact` for the change. |
| `SCAN_FINDING` | A scanner produced a finding (one event per finding). |
| `SCAN_TIMEOUT` / `SCAN_CRASHED` | Scanner failed; circuit-breaker may engage. |
| `DECISION_ISSUED` | Policy engine produced a verdict. |
| `DECISION_FINALISED` | If approval was required, after all approvals received. |
| `BOM_BUILT` | Canonical AI-BOM document constructed. |
| `BOM_SIGNED` | Signing succeeded (Sigstore or local-key). |
| `BOM_UNSIGNED` | Signing unavailable; decision held. |
| `APPROVAL_GRANTED` / `APPROVAL_DENIED` | Human approver responded. |
| `EMERGENCY_OVERRIDE_INVOKED` | Override applied; includes actor, reason, scope, expiry. |
| `OVERRIDE_POSTMORTEM_DUE` / `OVERRIDE_POSTMORTEM_CLOSED` | Required follow-up after override. |
| `ENFORCEMENT_MISSING` | Heartbeat detected missing CI check on a tenant repo. |
| `LEDGER_INTEGRITY_BROKEN` | Merkle chain verification failed. |
| `LEDGER_ANCHOR` | Tip-hash published to external sink (optional, per-tenant). |
| `AUDIT_EXPORTED` | A compliance officer (or API caller) exported a report. |
| `AUTH_FAILED` / `TENANT_ISOLATION_BREACH_ATTEMPT` | Security events; alerted immediately. |
| `POLICY_INVALID` | Runtime policy parse failure; tenant in misconfigured state. |
| `PR_MERGED` | Final merge confirmation; links back to BOM digest. |

---

## Appendix C — Policy YAML examples

### C.1 Conservative starter template (`themis.yaml`)
```yaml
version: 1
default: REQUIRE_APPROVAL          # fail-closed default
required_approvers_for_default:
  - role: senior

rules:
  - name: doc-only changes allowed without approval
    when:
      impact.kind: DOC_ONLY
    then:
      verdict: ALLOW

  - name: breaking schema changes require senior + compliance
    when:
      impact.kind: SCHEMA_BREAKING
    then:
      verdict: REQUIRE_APPROVAL
      required_approvers:
        - role: senior
        - role: compliance

  - name: any PII finding blocks
    when:
      findings.kind: pii
      findings.severity: ">=high"
    then:
      verdict: DENY
      reason: PII finding in AI-touched diff

  - name: secrets in diff always block
    when:
      findings.kind: secret
    then:
      verdict: DENY
      reason: Secret detected by Themis scanner

  - name: Collections domain requires compliance sign-off
    when:
      impact.domain: Collections
      impact.kind: ["SCHEMA_BREAKING", "NEW_EVENT", "CONSUMER_TOUCH"]
    then:
      verdict: REQUIRE_APPROVAL
      required_approvers:
        - role: compliance

  - name: off-catalogue additions require catalogue owner
    when:
      impact.kind: OFF_CATALOGUE
    then:
      verdict: REQUIRE_APPROVAL
      required_approvers:
        - role: catalogue_owner
```

### C.2 Balanced starter (allows more autonomous work)
```yaml
version: 1
default: ALLOW                     # permissive default; named exceptions below

rules:
  - name: breaking changes still need senior
    when:
      impact.kind: SCHEMA_BREAKING
    then:
      verdict: REQUIRE_APPROVAL
      required_approvers:
        - role: senior

  - name: any secret blocks
    when:
      findings.kind: secret
    then: { verdict: DENY }

  - name: any slopsquat blocks
    when:
      findings.kind: slopsquat
    then: { verdict: DENY }
```

---

## Companion documents

Audience-specific one-pagers and the Sanlam pilot proposal live alongside this spec:

- [Executive summary](2026-05-22-themis-exec-summary.md) — one page, exec-sponsor framing.
- [Compliance brief](2026-05-22-themis-compliance-brief.md) — for compliance, risk, audit.
- [Engineering brief](2026-05-22-themis-engineering-brief.md) — for tech leads.
- [Security brief](2026-05-22-themis-security-brief.md) — for AppSec / security engineering.
- [Sanlam pilot proposal](2026-05-22-themis-sanlam-pilot-proposal.md) — the formal pilot ask.

---

**End of spec.**
