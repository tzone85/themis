# Themis — Engineering Brief

**For:** Tech leads, engineering managers, platform engineers, dev productivity.
**Reading time:** 5 minutes.

---

## What problem does this solve for your team?

Your developers use AI tools. They want to keep using them. But every AI-influenced change keeps getting flagged in code review for context that nobody captures consistently:
- *"Did Cursor write this?"*
- *"What was the prompt?"*
- *"Did someone actually check the secrets scanner?"*
- *"Does this touch a published event?"*

This is friction with no upside. The same information re-litigated on every PR.

Themis captures the answer once, at the source, automatically. Then it tells you (and your reviewers, and your compliance officer) whether the PR is routine or genuinely needs human attention.

## What it looks like for a developer

A new check appears on PRs: `themis/verify`. For most PRs (docs, internal refactors, tests), it goes green in under two seconds and the PR proceeds. The developer sees a one-line summary:

> ✓ Themis: ALLOW · DOC_ONLY · no scanner findings · BOM signed

For PRs that touch contract surfaces (event schemas, public APIs, breaking changes), the check goes yellow:

> ⚠ Themis: REQUIRE_APPROVAL · SCHEMA_BREAKING (NotificationDispatchedEventV2)
> approvers needed: senior, compliance · 0/2 approved
> [review in EventCatalog plugin →]

A named approver clicks "approve" in EventCatalog or in the Themis dashboard. The check turns green. The PR proceeds.

For PRs with hard-blocking issues (secret committed, slopsquatted import), the check goes red and explains exactly what:

> ✗ Themis: DENY · secret detected at `src/config.go:42` (AWS-style access key)
> [scan finding →] · [policy rule →]

The developer fixes, pushes, re-runs.

## What changes for the tech lead

- **Less re-litigating context.** Themis captures AI authorship + model + scanner results in the PR itself.
- **Routine AI changes flow.** You set the policy once; you don't review the same kind of change ten times.
- **Real reviews get real attention.** When Themis asks for sign-off, it's because the policy you wrote thinks it's worth a human's time.
- **Latency budget.** p95 PR-check latency is enforced at < 2 seconds in CI; if Themis regresses on this, the regression itself breaks the build.

## What *doesn't* change

- Your AI tools keep working as they are. Themis listens to them through adapters; you do not change how engineers prompt, code, or commit.
- Your existing code review process keeps working. Themis is a CI check, not a replacement for human review.
- Your branch strategy, your release process, your linters, your SAST — all keep working. Themis composes with them.

## Integration footprint

- **GitHub Action** (recommended): one workflow file added per repo; one required-status-check added in branch protection.
- **Pre-receive hook** (optional, for self-hosted Git): drop a Go binary in `hooks/`.
- **EventCatalog plugin** (recommended): one `npm install` + one config change in the catalogue's `eventcatalog.config.js`.
- **No code changes** required in your repos — Themis observes diffs and adapter feeds; it does not inject itself into application code.

## What we'd ask of the engineering team in the pilot

1. **Name an engineering sponsor** (tech lead of pilot squad).
2. **Install the GitHub Action** on the pilot squad's primary repo(s) in week 3.
3. **Co-author the first policy** (week 2–3) with the compliance sponsor, starting from one of our three templates.
4. **Provide AI-tool inventory** in week 1 so we know which adapters to verify (Claude Code, Cursor, Copilot, VXD — most are pre-built).
5. **Honest weekly feedback** on developer experience and check latency.

## What you walk away with at the end of 90 days

- Routine AI-assisted engineering no longer slowed by ad-hoc compliance review.
- A consistent AI-authorship signal on every PR (useful even outside compliance — code archaeology, post-mortems, retros).
- An honest measure of how much AI-assisted code your squad ships and what kind.
- A reusable Themis policy you can take to the next squad.

## Architectural notes (for the curious)

- **Go static binary**, no JVM, no Node runtime to manage.
- **SQLite + JSONL ledger**, no external DB to operate.
- **Sigstore signing** by default; no key management on your side.
- **Multi-tenant from day one**, single-tenant for the pilot — same binary.
- **Self-hostable, air-gappable, open-source plugin** for the EventCatalog tab (Apache 2.0).
- **Event-sourced internals** (same pattern VXD has run in production for months) → fully replayable, fully auditable, recoverable from process crashes.
- **Performance targets** enforced in CI: ingest-to-decision p95 < 2s; 10k-event catalogue parse < 30s; 1M-event ledger replay < 60s.

---

For the full spec, see [`2026-05-22-themis-design.md`](2026-05-22-themis-design.md). Sections 5 (Architecture), 6 (Components), and 10 (Testing strategy) are most relevant.
