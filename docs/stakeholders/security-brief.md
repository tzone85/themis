# Themis — Security Brief

**For:** Security engineering, AppSec, SOC, GRC tooling.
**Reading time:** 5 minutes.

---

## What problem does this solve for you?

AI coding tools introduce a *new* set of risks that your existing pipeline does not consistently catch:

1. **Hallucinated package imports** ("slopsquatting") — AI suggests `requests-validator` (does not exist) or `requets` (typo-squat of a malicious lookalike). Existing dependency-scanning tools check known CVEs; they do not check whether a package was *invented or typo-squatted by an AI*.
2. **Secrets in AI prompts** — developers paste production credentials into a prompt to "ask the AI to debug." Existing pipeline scans commits; it does not scan the prompt that produced the commit.
3. **PII in prompts** — a developer pastes a customer's account details to ask the AI for help. Same problem.
4. **Prompt injection in commit messages** — a malicious actor (or a careless one) crafts a commit message that influences downstream LLM behaviour (release notes generation, AI code review).
5. **No authorship provenance** — when an incident happens, "who wrote this code?" is a binary distinction (human vs. AI) and increasingly an "which AI, which version, which prompt" distinction. Today we cannot answer.

## What Themis provides

**A consistent place to plug AI-specific scanners.** Four ship in the box:
- `secrets` — entropy + pattern-based (composes with your existing secret scanner; results are stored, not duplicated).
- `pii_heuristic` — credit card, SA ID, email, IP — patterns; redacted in stored findings.
- `slopsquat` — checks every newly-introduced import against package-registry existence and Levenshtein-distance from popular real packages.
- `hallucinated_imports` — checks against the package manifest and registry; flags imports that were declared but do not exist anywhere.

You can add scanners. The interface is small:

```go
type Scanner interface {
    Name() string
    Scan(ctx context.Context, change AIChange) ([]Finding, error)
}
```

**Cryptographic provenance per PR.**
- Every AI-touched PR gets a signed AI-BOM (JSON-LD).
- Signing is keyless via Sigstore (industry standard, no PKI burden on your team).
- Verification is offline: any future investigator can verify the BOM came from Themis at the timestamp claimed, untampered.
- For air-gapped deployments: ed25519 local keypair with the public key published.

**Tamper-evident ledger.**
- `events.jsonl` is append-only with per-event content hashes that chain Merkle-style.
- `themis ledger verify` walks the chain. Any break is detected.
- Optional weekly tip-hash anchoring to S3 object-lock / public git repo / transparency log — makes silent tampering provable, not just detectable.

**Deadman's switch for missing enforcement.**
- The most common bypass of any AI-policy gate is "we just removed the check from CI."
- Themis heartbeat calls the tenant's repos via GitHub API to verify the required Action is installed and the required check is enforced.
- Missing → `ENFORCEMENT_MISSING` event → immediate alert.
- Absence of signal becomes a signal.

**Multi-tenant filesystem isolation.**
- Per-tenant directories under `tenants/<id>/`. Cross-tenant data leakage is physically impossible at the storage layer.
- API-key-to-tenant resolution happens at the request boundary; downstream calls take a `Tenant` explicitly (no globals, no thread-local).
- `TENANT_ISOLATION_BREACH_ATTEMPT` event for misroute attempts; alerted immediately.

**Auditable audit access.**
- Every export from the system is itself a ledger event (`AUDIT_EXPORTED`).
- Insider abuse of audit access — a real failure mode regulators care about — is itself audited.

## Threat model (summary)

| Threat | Mitigation |
|---|---|
| AI introduces secret to repo | `secrets` scanner; policy DENY on any finding |
| AI hallucinates malicious-lookalike import | `slopsquat` + `hallucinated_imports` scanners; policy DENY |
| AI prompt contains PII / customer data | Prompts hashed by default; PII findings redacted; verbatim text opt-in only |
| Developer bypasses by removing CI check | Deadman's switch → `ENFORCEMENT_MISSING` |
| Developer/insider edits ledger | Merkle chain + integrity check + (optional) external anchoring |
| Insider abuses audit access | Exports are logged; access roles separated (`compliance` ≠ `admin`) |
| Cross-tenant data leakage | Per-tenant filesystem paths; isolation tests enforce |
| Sigstore outage causes silent unsigned merge | Fail-closed; PR held until signed (or local-key fallback configured) |
| Policy misconfiguration causes silent allow | Fail-closed; no decisions issued if policy invalid |

## Threat model (out of scope at MVP)

| Threat | Why deferred |
|---|---|
| Compromised Themis daemon itself | Treat Themis-host hardening as customer's responsibility (single Go binary; standard OS hardening applies). Future: SELinux/AppArmor profile + minimal container image. |
| Adversarial AI tools forging their own attribution | All adapters parse externally-produced evidence; cannot independently verify "this prompt produced this code." Future: per-AI-tool signing handshake. |
| Compromised Sigstore root of trust | Industry-wide risk; Sigstore community handles. Local-key fallback available for tenants who reject this risk model. |

## Compliance posture by regulator (suggested mapping)

| Regulator / regime | Themis evidence relevant |
|---|---|
| **POPIA (SA)** | PII redaction in findings; prompt-hash default; per-tenant data residency; audit-access logging |
| **GDPR** | Same as above; plus deletion = `rm` of tenant directory |
| **PCI-DSS** | Secrets scanner; policy gating of payment-related event schemas; tamper-evident audit log |
| **SOC 2** | Append-only audit trail; access control; change management coverage; deadman's switch evidence |
| **ISO 27001 (A.14)** | Secure development + change management evidence |
| **EU AI Act (high-risk systems)** | Model/version provenance + decision logs for AI used in development workflow |

## What we'd ask of security in the pilot

1. **Review the threat model + mitigations** before week 3 deployment.
2. **Approve the four bundled scanners** or specify additional ones for the squad's repos.
3. **Decide on external anchoring** (per-tenant opt-in: S3 object-lock vs. internal git vs. none).
4. **Sign off on the data-handling addendum** (prompt hashes default, verbatim text opt-in, PII redaction).
5. **Optional**: run an internal red-team exercise against the pilot deployment in week 10. We'll instrument and feed findings back into the design.

## What you walk away with at the end of 90 days

- Documented coverage of the five AI-specific attack classes above for one squad.
- A reusable threat model + mitigation matrix for AI-assisted development.
- An honest measure of false-positive / false-negative rates for each scanner against real squad output.
- Red-team findings (if applicable) folded back into the design before expansion.

---

For the full spec, see [`2026-05-22-themis-design.md`](2026-05-22-themis-design.md). Sections 8 (Error handling), 9 (Trust story), and 10 (Testing strategy) are most relevant.
