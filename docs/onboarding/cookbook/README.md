# Cookbook — common Themis recipes

Each recipe is self-contained: copy-paste the block, swap the tenant id and
base directory, and you've got a working setup for the named pattern.
Every recipe is also covered by an integration test, listed in the
"Tested by" footer.

---

## 1. "Locked-down" policy — secrets, PII, and schema breaks block

Use when you want the safest possible default before tuning rules.

```yaml
version: 1
default: REQUIRE_APPROVAL
required_approvers_for_default:
  - role: senior

rules:
  - name: secrets always block
    when:
      findings.kind: secret
    then:
      verdict: DENY
      reason: secret detected by scanner

  - name: high-severity PII blocks
    when:
      findings.kind: pii
      findings.severity: ">=high"
    then:
      verdict: DENY
      reason: high-severity PII finding in AI-touched diff

  - name: schema breaking needs compliance
    when:
      impact.kind: [SCHEMA_BREAKING]
    then:
      verdict: REQUIRE_APPROVAL
      required_approvers:
        - role: senior
        - role: compliance

  - name: doc-only allowed
    when:
      impact.kind: [DOC_ONLY]
    then:
      verdict: ALLOW
```

**Tested by:** `internal/policy/engine_test.go`,
`internal/policy/engine_property_test.go`.

---

## 2. "Domain-scoped sign-off" — only flag changes to a specific business unit

```yaml
- name: Collections domain requires compliance
  when:
    impact.domain: Collections
    impact.kind: [CONSUMER_TOUCH, PRODUCER_TOUCH, NEW_EVENT, SCHEMA_BREAKING]
  then:
    verdict: REQUIRE_APPROVAL
    required_approvers:
      - role: compliance
```

Pair with a permissive default (`default: ALLOW`) so other domains stay
fast.

**Tested by:** `internal/policy/engine_test.go::TestDecide_DomainMatchClause`.

---

## 3. "Catch slopsquats and hallucinated imports"

Already on by default — both ship in `scan.DefaultScanners()`. Tune
severity thresholds in your policy:

```yaml
- name: hallucinated imports block
  when:
    findings.kind: hallucinated_import
  then:
    verdict: DENY
    reason: package unknown to package oracle (possible AI hallucination)

- name: slopsquat needs review
  when:
    findings.kind: slopsquat
  then:
    verdict: REQUIRE_APPROVAL
    required_approvers:
      - role: senior
```

**Tested by:** `internal/scan/supply_chain_test.go`.

---

## 4. "Emergency override with a co-signer"

When the system itself is on fire:

```bash
themis override invoke \
  --id "$TENANT" --base "$BASE" \
  --pr-id "incident-2026-05-25-001" \
  --actor "human:alice" \
  --co-signer "human:bob" \
  --scope "one-pr" \
  --ttl-minutes 60 \
  --reason "Catalogue cache stuck on stale revision; need to roll forward fix before EOD."
```

Themis schedules a post-mortem 7 days out. Close it as soon as the root
cause is documented:

```bash
themis override close-postmortem \
  --id "$TENANT" --base "$BASE" \
  --pr-id "incident-2026-05-25-001" \
  --closer "human:compliance" \
  --notes "root cause: stale cache TTL; bumped to 30s + alerting"
```

**Tested by:** `internal/override/override_test.go`,
`internal/cli/override_cmd_test.go`.

---

## 5. "Sigstore-keyless signing (stub for now, real later)"

```bash
themis bom sign \
  --id "$TENANT" --base "$BASE" \
  --pr-id "demo#1" \
  --signer cosign-keyless-stub \
  --oidc-subject alice@example.com \
  --oidc-issuer https://your-idp.example.com
```

The stub today produces a Sigstore-shaped bundle (cert + Rekor URL)
without contacting Fulcio/Rekor. When you're ready for the real path,
replace the `sign.Resolve` registration with the production keyless adapter —
nothing else changes.

**Tested by:** `internal/sign/signer_test.go`,
`internal/cli/bom_cmd_test.go::TestBOMSign_CosignKeylessStub_RoundTrip`.

---

## 6. "Multi-role approvals"

```yaml
- name: production payment schema needs both
  when:
    impact.domain: Collections
    impact.kind: [SCHEMA_BREAKING]
  then:
    verdict: REQUIRE_APPROVAL
    required_approvers:
      - role: senior
      - role: compliance
```

Both grants are needed before Themis emits `DECISION_FINALISED`. A denial
from either role is sticky for the current decision.

**Tested by:** `internal/approvals/approvals_test.go`.

---

## 7. "OIDC-backed tokens with a file-based fallback"

```go
chain := auth.NewChainStore(
    auth.NewFileTokenStore(base),
    &auth.OIDCTokenStore{
        IssuerUserinfoURL: "https://auth.example.com/oauth2/v1/userinfo",
        CacheTTL:          30 * time.Second,
    },
)
```

The file store satisfies legacy and CI workflows; OIDC handles end-users.
Lookups try the file store first; only `ErrUnauthorized` falls through to
OIDC. Any hard error short-circuits — no silent degradation.

**Tested by:** `internal/auth/oidc_test.go::TestChainStore_*`.

---

## 8. "Custom claim mapping" — derive tenant from a group claim

```go
oidc := &auth.OIDCTokenStore{
    IssuerUserinfoURL: "https://auth.example.com/userinfo",
    ClaimMapper: func(raw map[string]any) (auth.Identity, error) {
        groups, _ := raw["groups"].([]any)
        for _, g := range groups {
            // Example: "themis-acme-admin" → (acme, admin).
            if name, ok := g.(string); ok && strings.HasPrefix(name, "themis-") {
                parts := strings.Split(name, "-")
                if len(parts) == 3 {
                    return auth.Identity{
                        Tenant: parts[1],
                        Role:   auth.Role(parts[2]),
                    }, nil
                }
            }
        }
        return auth.Identity{}, errors.New("no themis-* group on identity")
    },
}
```

**Tested by:** `internal/auth/oidc_test.go::TestOIDC_CustomClaimMapper`.
