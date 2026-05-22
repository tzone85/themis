# Themis

**A compliance gateway for AI-generated code.**

Themis records and governs every change that AI coding tools (Claude Code, Cursor, GitHub Copilot, autonomous agents like [VXD](https://github.com/tzone85/vortex-dispatch)) make to your software. It captures *who* (which AI, which prompt, which human) changed *what* (which event contract, which service, which downstream consumer), proves it cryptographically (signed AI Bill of Materials per pull request, tamper-evident ledger), and stops changes that violate your policies before they merge.

## Status

> **Pre-implementation.** This repository currently contains the design specification and stakeholder briefs. Implementation begins after stakeholder review and pilot approval.

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
