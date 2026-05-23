# Plan 3 — Scanners + Policy Engine

**Date:** 2026-05-23
**Depends on:** Plans 1 & 2.
**Scope:** Pluggable scanner interface + secrets + PII scanners. YAML-driven
policy engine producing `(ALLOW|REQUIRE_APPROVAL|DENY)` decisions. `themis decide`
command that wires it all together. Three new ledger kinds.

## Architectural alignment (design spec §6.1, §8, Appendix B & C)

- `internal/scan` — `Scanner` interface, `Finding` value type, two scanners (secrets, PII).
- `internal/policy` — YAML rule schema, pure `Decide(c AIChange, imp Impact, fs []Finding, p Policy) Decision`.
- New ledger kinds: `SCAN_FINDING`, `DECISION_ISSUED`, `POLICY_INVALID`.
- CLI: `themis policy lint`, `themis decide` (orchestrates classify+scan+decide).

**Out of scope (deferred to Plan 4):** slopsquatting, hallucinated imports,
BOM build, signing, approvals.

## Tasks

### T1: `Finding` + `Scanner` interface

`internal/scan/scan.go`:
- `Severity` enum: `info`, `low`, `med`, `high`, `critical`.
- `Finding { Kind, Severity, File, Line, Description, Detector string }`.
- `Scanner` interface: `Name() string; Scan(c aichange.AIChange, fileBodies map[string][]byte) ([]Finding, error)`.

### T2: Secrets scanner (regex)

`internal/scan/secrets.go` + tests. Detects:
- AWS access keys (`AKIA[0-9A-Z]{16}`),
- generic high-entropy strings tagged `api[_-]?key`,`token`,`secret`,`password`,`pwd`,
- private keys (`-----BEGIN .* PRIVATE KEY-----`).

Severity: critical. Operates only on `FileAdded` / `FileModified` content (DELETED has no `AfterHash` body to scan).

### T3: Secrets scanner property test

Property: secrets scanner returns zero findings for ASCII text generated from
a low-entropy alphabet (letters + spaces) of length ≤ 200 — proves it doesn't
fire on plain prose.

### T4: PII heuristic scanner

`internal/scan/pii.go` + tests. Detects:
- credit-card-shaped (Luhn-validated 13-19 digit clusters), severity high
- South African ID number (13 digits with date prefix + Luhn-checksum), severity high
- email addresses, severity low

Findings carry redacted descriptions only (e.g. `"credit-card-shaped string at X:Y"`, never the digits).

### T5: Scanner orchestration

`internal/scan/run.go`:
- `RunAll(scanners []Scanner, c aichange.AIChange, files map[string][]byte) ([]Finding, error)`.
- Runs all scanners; aggregates findings; per-scanner failure surfaces as a `SCAN_CRASHED`-shaped Finding rather than aborting (per spec §8.1 "every failure becomes a ledger event").

### T6: Register `SCAN_FINDING` ledger kind

Extend `DefaultRegistry` + wiring test.

### T7: Policy YAML schema + parser

`internal/policy/schema.go` defines:
- `Policy { Version int; Default Verdict; RequiredApproversForDefault []Approver; Rules []Rule }`
- `Rule { Name string; When MatchClause; Then ThenClause }`
- `MatchClause { ImpactKind []string; ImpactDomain string; FindingKind string; FindingSeverityMin string }`
- `ThenClause { Verdict Verdict; RequiredApprovers []Approver; Reason string }`
- `Verdict` enum: `ALLOW`, `REQUIRE_APPROVAL`, `DENY`.

`internal/policy/parse.go`: `Parse(raw []byte) (Policy, error)` validates required fields, default verdict present, version supported.

### T8: `Decision` value type

`internal/policy/decision.go`:
- `Decision { Verdict Verdict; Reason string; RuleName string; RequiredApprovers []Approver }`.

### T9: Policy engine pure function

`internal/policy/engine.go`:
- `Decide(c aichange.AIChange, imp classify.Impact, findings []scan.Finding, p Policy) Decision`.
- First-rule-wins semantics matching the YAML order.
- Fail-closed: if no rule matches and no default is set → returns `DENY` with reason "policy missing default".

### T10: Policy property tests

`internal/policy/engine_property_test.go`:
- Determinism: same inputs → same Decision bytes (200 iterations).
- Fail-closed: policy with no default + no matching rule → always DENY.
- DENY-wins: presence of any rule with `findings.kind=secret` → DENY regardless of order (rapid-driven shuffle of other rules).

### T11: Register `DECISION_ISSUED` + `POLICY_INVALID` ledger kinds

Wire registry + wiring test.

### T12: `themis policy lint`

`internal/cli/policy_cmd.go`. Reads a policy YAML, parses, reports errors with file:line, exits non-zero on failure.

### T13: `themis decide` — orchestration command

`internal/cli/decide_cmd.go`:
- Inputs: `--id`, `--base`, `--aichange`, `--policy`, optional `--catalogue`, `--workdir` (where the diff files are checked out so scanners can read content).
- Steps:
  1. Load catalogue snapshot.
  2. Load AIChange.
  3. Read file bodies from --workdir for the AfterHash side.
  4. Classify → Impact.
  5. RunAll scanners → Findings.
  6. Emit one `SCAN_FINDING` per finding.
  7. `policy.Decide` → Decision.
  8. Emit `DECISION_ISSUED`.
  9. Print Decision JSON.

### T14: README Plan 3 changelog

### T15: make ci local pass
