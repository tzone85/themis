# Themis — Executive Summary

**One page. Read in two minutes. Decide in one meeting.**

---

## The problem
Our teams want to use AI coding tools (Claude Code, Cursor, Copilot, internal agents) to ship faster. Compliance, risk, and audit reasonably ask: *"prove the AI didn't break something, didn't leak data, and that a human signed off on anything that matters."* Today, we cannot. The audit trail for AI-touched code disappears the moment a pull request merges. The unofficial answer in regulated contexts has been "don't use AI for anything that matters" — which costs us productivity and breeds workarounds.

## The proposal
**Themis** — a self-hosted compliance gateway for AI-generated code. It plugs into our existing AI tools and our existing EventCatalog, captures every AI-touched change, evaluates each one against policies we control, and produces a signed, tamper-evident audit trail per pull request. Routine AI work passes through automatically. Changes that genuinely affect contracts, customer data, or breaking interfaces stop for explicit sign-off. Silent bypass of the system is detected and alerted.

## Why this is differentiated
Existing tools (SLSA, in-toto, GitHub Advanced Security, generic AI scanners) describe *build* provenance or flag generic vulnerabilities. None of them know our event catalogue, none of them gate by contract impact, none of them generate a signed AI-specific audit packet. Themis does. It also runs entirely inside our infrastructure — no third-party data exposure.

## Why now
- The anchor pilot organisation already runs EventCatalog at the business-unit level.
- We already run an event-sourced AI orchestrator (VXD), which is 70% of the storage substrate Themis needs.
- Regulators (SA + EU) are formalising AI-system audit expectations; we can either lead with a defensible posture or react when asked.

## Cost
- **People:** one engineer (Thando Mini) + AI pair for 12 weeks.
- **Infrastructure:** one self-hosted Go binary + one SQLite file per tenant. No external SaaS dependency. No per-seat licensing.
- **Pilot:** one the anchor pilot organisation squad, 90 days, no external software spend.

## Risk
- **Pilot risk:** policies need tuning; mitigated by three starter templates and a weekly review with compliance sponsor.
- **Tool-blind risk:** if a team uses an AI tool we don't yet support, the system falls back to heuristic detection and flags lower confidence. Mitigated by confirming the AI tool list at week 1.
- **Adoption risk:** if Themis adds noticeable latency, devs route around it. Mitigated by a hard p95 < 2s target enforced in CI; latency dashboard visible to the team.

## Decision asked
1. **Endorse a 90-day pilot** with one the anchor pilot organisation squad.
2. **Name a compliance sponsor** (target: the anchor pilot organisation compliance / risk lead) and an **engineering sponsor** (target: pilot squad tech lead).
3. **Approve internal data-handling addendum** (default: prompt hashes stored, verbatim text opt-in; PII redacted in stored findings).
4. **Decide IP / open-source posture**: EventCatalog plugin is intended Apache 2.0; Themis Core is open-core vs. closed-source — your call.

## What you get at the end of 90 days
- A working system gating AI-touched PRs in one squad's repos.
- An audit packet that the next external audit cycle can ingest.
- A written assessment from the compliance sponsor on whether to expand.
- An honest evaluation of false-positive rate, latency, override frequency, and developer satisfaction.
- A go/no-go decision based on evidence, not on theory.

---

**Companion documents**
- Full design spec: [`2026-05-22-themis-design.md`](2026-05-22-themis-design.md)
- Compliance brief: [`2026-05-22-themis-compliance-brief.md`](2026-05-22-themis-compliance-brief.md)
- Engineering brief: [`2026-05-22-themis-engineering-brief.md`](2026-05-22-themis-engineering-brief.md)
- Security brief: [`2026-05-22-themis-security-brief.md`](2026-05-22-themis-security-brief.md)
- Anchor pilot proposal: [`2026-05-22-themis-anchor-pilot-proposal.md`](2026-05-22-themis-anchor-pilot-proposal.md)
- Glossary + FAQ: in the full design spec (§14, §15).

**Author:** Thando Mini · 2026-05-22 · Pre-pilot draft for internal review.
