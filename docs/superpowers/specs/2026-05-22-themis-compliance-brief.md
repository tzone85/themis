# Themis — Compliance Brief

**For:** Compliance, risk, internal audit, regulatory liaison.
**Reading time:** 5 minutes.

---

## What problem does this solve for you?

Today, when AI tools (Claude Code, Cursor, Copilot, internal agents) contribute to production code, the resulting audit story has gaps:
- We do not consistently know **which** AI tool produced **which** lines.
- We do not consistently know **what prompt** was used, against **which model version**.
- We do not know **who reviewed it** with full context of "this was AI-authored."
- If a regulator or external auditor asks, "show me every AI-influenced change to a customer-facing event contract in Q3," we cannot answer without a forensic excavation.

Themis closes those gaps by design.

## What changes for you day-to-day

**One screen, every AI-touched change.**
You see a timeline of every AI-authored or AI-assisted change across the organisation, filterable by domain, service, event, person, AI model, or outcome (allow / approval required / denied). It appears as a tab in EventCatalog — the same tool engineers already use to read the system — so you and engineers reference the same source of truth.

**Policy as the regulator reads it.**
The rules Themis enforces are written in plain YAML — not buried in code. You can read them. You can propose changes. Each version is itself recorded in the ledger, so "what was the policy on the day this PR merged?" has a precise answer. Three starter templates ship with Themis:
- *Conservative* — anything touching contracts requires senior + compliance.
- *Balanced* — most AI work flows; breaking changes pause for review.
- *Permissive* — only secrets and slopsquat findings block (suitable for non-regulated codebases).

You and engineering co-write the policy for each scope in the first two weeks.

**Audit packet per PR.**
Each merged AI-touched pull request gets a signed audit packet you can hand to any auditor:
- The AI Bill of Materials (which model, which prompt hashes, which scanners ran, which findings surfaced).
- The decision (and the *deterministic* reasoning behind it).
- The reviewer / approver chain.
- The catalogue snapshot at the time of decision.
- A cryptographic signature (Sigstore) you can verify offline.

**Tamper-evidence.**
The ledger that holds all of this is Merkle-chained — every event references the prior event's content hash. If anyone (or any process) silently edits the audit history, `themis ledger verify` reports the break and the system marks itself read-only until investigated. Optionally, the chain's tip-hash is published to an external append-only sink (e.g. an S3 object-lock bucket), making silent tampering provably impossible.

**Deadman's switch.**
If someone removes the required Themis CI check from a repo to bypass enforcement, the system's heartbeat detects the absence and alerts immediately. The most common compliance bypass — "we just turned off the check" — does not work.

**Override is controlled, not hidden.**
For genuine emergencies (production incident, urgent merge), an emergency override path exists:
- Requires a named authority (default: compliance role + at least one senior co-sign).
- Requires a reason ≥ 50 characters.
- Is time-boxed (default 24 hours) and scope-boxed (one PR or one tenant).
- Triggers an automatic 7-day post-mortem entry the compliance team must close out.
- Leaves a permanent red banner on the audit packet — visible to every future viewer.

## What does *not* change

- Your existing controls (code review, change management, release management) keep working. Themis is additive.
- Your existing tools (EventCatalog, JIRA, GRC) keep working. Themis does not replace them; it feeds them.

## Data handling

- **Prompts** are stored as SHA-256 hashes by default. Verbatim text is opt-in per tenant — we recommend keeping the default for POPIA / GDPR comfort.
- **PII surfaced by scanners** is redacted in stored findings ("credit-card-shaped string at file.go:142", not the digits).
- **Per-tenant filesystem isolation.** Each tenant gets its own directory tree. Cross-tenant data leakage is physically impossible at the storage layer (not just SQL-WHERE-clause-prevented).
- **No outbound calls in the hot path** unless explicitly configured (Sigstore is the one exception and can be turned off for air-gapped deployments).

## What we'd ask of you in the pilot

1. **Be the named compliance sponsor** for one Digisure squad — one hour per week for 12 weeks.
2. **Co-author the first policy** with the squad's tech lead, using one of our starter templates as a base.
3. **Run a synthetic audit exercise** in week 10 — pretend you are an external auditor, ask the questions you would ask, and tell us whether the audit packet answers them.
4. **Write the final assessment** in week 12: is the artefact sufficient to take into our next external audit cycle?

## What you walk away with at the end of 90 days

- A documented, defensible AI-engineering posture.
- A reusable policy template you can extend to other Digisure squads, other Sanlam entities, or other organisations entirely.
- A signed body of audit evidence covering every AI-touched PR for the squad's 90 days.
- An explicit go/no-go on expansion, backed by evidence.

---

For the full spec, see [`2026-05-22-themis-design.md`](2026-05-22-themis-design.md). Section 9 (Trust story) and section 15 (FAQ) are the most relevant.
