# Themis — Sanlam Digisure Pilot Proposal

**For:** Sanlam Digisure leadership · compliance · risk · platform engineering.
**Status:** Draft — for internal stakeholder review.
**Date:** 2026-05-22.

---

## 1. The ask

Endorse a **90-day, single-squad pilot** of Themis in Sanlam Digisure to evaluate whether it materially improves our ability to safely adopt AI coding tools at scale.

Specifically:
- One pilot squad (to be named in week 1).
- One named compliance sponsor.
- One named engineering sponsor.
- Themis deployed inside Sanlam infrastructure (self-hosted; one Go binary; one SQLite file per tenant; no external SaaS).
- Existing tools (Claude Code, Cursor, GitHub, EventCatalog) unchanged.
- Defined exit criteria (see §5).

## 2. The problem we're trying to solve

Sanlam Digisure engineers use AI coding tools today. Sanlam's compliance and risk posture is — appropriately — conservative about AI involvement in customer-facing or regulated code. The pragmatic outcome is one of:

- **Engineers self-censor** ("don't use AI for anything important") and lose productivity, or
- **Engineers use AI freely** and we lose the audit trail at PR merge time.

Neither is sustainable. The industry is converging on AI-assisted engineering. We need a defensible, auditable, repeatable answer that does not depend on individual discipline.

## 3. The proposal: Themis

A self-hosted compliance gateway that:
- Plugs into Sanlam's existing tools — EventCatalog (already deployed in Digisure), GitHub, Claude Code / Cursor / Copilot, VXD (internal agent orchestrator).
- Records every AI-touched change with full provenance: which AI, which prompt (hashed by default), which model version, which scanner findings, which human reviewers and approvers.
- Evaluates each change against policies we author in plain YAML; routine work flows automatically, contract-affecting work pauses for explicit sign-off.
- Produces a signed audit packet per PR that any external auditor can ingest and verify offline.
- Detects bypass attempts (missing CI check, ledger tampering, missing approval co-signs).
- Runs entirely inside Sanlam infrastructure (no third-party data exposure).

See the full design at [`2026-05-22-themis-design.md`](2026-05-22-themis-design.md).

## 4. The 90-day pilot

### Phase 1 — Alignment (weeks 1–2)
- Identify pilot squad (Digisure team with active AI-tool usage; current candidates: any squad in CapstonePAS, Collections, or Trustflow domains).
- Identify compliance sponsor (target: Digisure compliance / risk lead, ~1 hour/week commitment).
- Identify engineering sponsor (target: pilot squad tech lead, ~2 hours/week commitment for the first 4 weeks, less thereafter).
- Confirm AI-tool inventory (which adapters we need).
- Sign internal data-handling addendum (prompt hashes default; verbatim text opt-in; PII redaction).
- Confirm EventCatalog access (read-only; we mirror, we don't write back at MVP).
- Decide hosting (Sanlam-internal VM preferred over Themis-managed) and infra request raised.

### Phase 2 — Deployment (weeks 3–6)
- Themis Core (single-tenant) deployed to Sanlam infra.
- GitHub Action installed on pilot squad's primary repo(s); required-status-check added.
- EventCatalog plugin installed in Digisure's EventCatalog instance (Apache-2.0 plugin; one `npm install` + one config line).
- Catalogue ingestion live; weekly auto-refresh.
- Policy v1 co-authored by compliance + engineering sponsors using "Conservative" starter as the base; reviewed in week 5.
- Web dashboard accessible to pilot team + compliance sponsor.
- Mempalace per-tenant wing initialised; squad's repo history mined to seed cross-PR memory.

### Phase 3 — Soak (weeks 7–10)
- All AI-touched PRs on pilot squad's repo(s) flow through Themis.
- Weekly review with compliance sponsor of any `DENY` or `REQUIRE_APPROVAL` outcomes; policy refined as patterns emerge.
- Week 8: latency / false-positive checkpoint with engineering sponsor.
- Week 10: **synthetic audit exercise** — compliance sponsor plays external auditor; asks the questions they would ask in a real audit; assesses whether audit packets answer them.
- (Optional) Week 10: security red-team exercise targeting bypass paths.

### Phase 4 — Evaluation (weeks 11–12)
- Metrics package compiled (see §5 below).
- Compliance sponsor writes formal assessment.
- Engineering sponsor writes formal assessment.
- Joint go / no-go meeting with leadership.

## 5. Exit criteria (how we know it worked)

The pilot is **successful** if at week 12 all six are true:

1. **Coverage** — ≥ 95% of the pilot squad's merged PRs touching AI-influenced code have a signed Themis audit packet.
2. **Latency** — p95 PR-check latency < 2 seconds; p99 < 5 seconds.
3. **False-positive rate** — < 10% of `REQUIRE_APPROVAL` decisions, on retrospective review by the engineering sponsor, were "we'd have shipped this anyway." (Target informed by the iterative policy refinement; an honest measure of policy fit.)
4. **Audit sufficiency** — the synthetic audit exercise (week 10) confirms the audit packets answer the questions an external auditor would ask of an AI-assisted change.
5. **Developer experience** — pilot squad's anonymised feedback survey returns net-positive (more "this saves us re-litigating context" than "this adds friction").
6. **Trust integrity** — no `LEDGER_INTEGRITY_BROKEN`, `TENANT_ISOLATION_BREACH_ATTEMPT`, or unexplained `ENFORCEMENT_MISSING` events during the pilot. Any that occur were investigated and resolved with documented root cause.

The pilot **fails** if any of:
- Themis is the proximate cause of a production incident that would not have occurred without it.
- The compliance sponsor's week 12 assessment concludes the audit artefact does not meet Sanlam's audit needs.
- The engineering sponsor's week 12 assessment concludes the latency / FP rate is unsustainable.
- A scanner produces a false-negative on a real secret / slopsquat / PII case discovered through other means.

## 6. Cost (Sanlam side)

| Item | Cost |
|---|---|
| Internal VM (Themis host) | One existing-fleet VM equivalent (2 vCPU, 4GB RAM, 20GB SSD). Negligible. |
| Compliance sponsor | ~12 hours total over 12 weeks. |
| Engineering sponsor | ~24 hours total over 12 weeks. |
| Pilot squad developers | Zero net cost — Themis replaces ad-hoc review with policy. |
| External software licensing | Zero. |
| Cloud third-party data exposure | Zero (self-hosted; no outbound traffic if Sigstore disabled in favour of local key). |

## 7. Cost (Thando + Themis side)

- ~12 weeks of focused engineering (1 engineer + AI pair).
- All open-source dependencies (Go stdlib, Sigstore, Mempalace, EventCatalog v2).
- Themis Core development happens outside Sanlam infra; only the deployed binary lands inside.

## 8. Risk

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Policy v1 over-restricts; team frustrated | Medium | Medium | Three starter templates; weekly refinement; explicit FP-rate metric |
| Adapter blind spot for a tool the squad uses | Low | Low | Tool inventory in week 1; adapter built in week 2 if needed; heuristic fallback always available |
| Catalogue drift causes wrong classification | Medium | Low | Catalogue freshness exposed as tenant-level metric; weekly auto-refresh |
| Sigstore outage causes pile-up | Low | Medium | Fail-closed by design; local-key fallback configurable; documented as expected behaviour in playbook |
| Pilot uncovers a deeper design issue | Medium | High | This is the *purpose* of the pilot. Evaluation phase explicitly captures it. |

## 9. What changes for Sanlam Digisure at the end of the pilot

**If successful:**
- A documented, defensible posture for AI-assisted engineering: "here is the policy we enforce, here is the audit trail we generate, here is the bypass detection we run."
- A reusable Themis deployment that can extend to other Digisure squads, other Sanlam entities (SFT, Glue, etc.), or be offered to the broader Sanlam Group.
- A position from which Sanlam can publish externally (board, regulators, customers) on its AI-engineering posture.
- A go-decision on expansion based on evidence.

**If unsuccessful or partial:**
- A documented set of findings about why this specific shape of solution did or did not work.
- All evidence (audit packets, metrics, assessments) retained for future reference.
- No sunk infrastructure cost; the VM is decommissioned; the GitHub Action is removed.

## 10. What we ask for now

1. **Sponsorship endorsement** from Sanlam Digisure leadership.
2. **Named compliance sponsor** (with their direct line manager's awareness of the time commitment).
3. **Named engineering sponsor** (likewise).
4. **Approval of the data-handling addendum** (default privacy-conservative settings).
5. **Infra ticket raised** for the Themis host VM in week 1.
6. **Decision on external-anchoring** (default opt-in: we recommend opting it off for the pilot; revisit at expansion).

We can be ready to start the day all six are in place.

---

**Author:** Thando Mini · 2026-05-22 · Pre-pilot draft for internal review.
