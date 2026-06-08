# Themis — Stakeholder Documentation Index

Audience-specific briefs and the canonical design spec. Each brief is
< 10 minutes' reading time; the design spec is the deep reference.

## Read this first
- [**Executive summary**](exec-summary.md) — one page; decide in one meeting.

## Read this if you are…
- A **compliance / risk** stakeholder → [Compliance brief](compliance-brief.md)
- An **engineering lead** → [Engineering brief](engineering-brief.md)
- A **security / AppSec** stakeholder → [Security brief](security-brief.md)
- An **anchor-pilot decision-maker** → [Anchor pilot proposal](anchor-pilot-proposal.md)

## Reference
- Full design specification → [`docs/design.md`](../design.md)
- Glossary + FAQ → in the full design specification (§14 and §15)

## Document versions
| Doc | Version | Date | Status |
|---|---|---|---|
| Design specification | Draft v1 | 2026-05-22 | Awaiting stakeholder review |
| Exec summary | Draft v1 | 2026-05-22 | Awaiting stakeholder review |
| Compliance brief | Draft v1 | 2026-05-22 | Awaiting stakeholder review |
| Engineering brief | Draft v1 | 2026-05-22 | Awaiting stakeholder review |
| Security brief | Draft v1 | 2026-05-22 | Awaiting stakeholder review |
| Anchor pilot proposal | Draft v1 | 2026-05-22 | Awaiting stakeholder review |

All drafts are intended to be updated as feedback comes in. Each brief is < 10 minutes' reading time on its own.

## What changed since 2026-05-22

The briefs above are deliberately frozen — they are pitch artefacts;
re-writing them invalidates the conversations they seeded. Status
updates land separately:

- **`v0.1.0` cut 2026-06-03.** All 18 implementation plans complete.
  See [`CHANGELOG.md`](../../CHANGELOG.md) for the plan-by-plan history.
- **Production-readiness pass** — added governance scaffolding
  (SECURITY/SUPPORT/CoC/CONTRIBUTING), signed multi-arch container
  image on `ghcr.io`, signed binary releases with SPDX SBOMs, four ops
  docs under [`../ops/`](../ops/).
- **Verification path now operator-self-serve** — every release artefact
  is `cosign verify`-able by anyone with cosign installed; see
  [`../ops/deployment.md`](../ops/deployment.md).
