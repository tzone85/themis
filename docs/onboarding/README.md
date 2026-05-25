# Onboarding — Themis in 30 minutes

This walkthrough takes a fresh operator from a clean machine to a fully
running Themis deployment serving one tenant, with every surface
(CLI, REST API, dashboard, MCP) exercised end-to-end.

If you only have five minutes, skim §1–§3 to install the binary and
verify the smoke test. The remaining sections deepen each concept.

## Audience

- **Compliance officers** — see §5 (decisions), §6 (approvals + overrides),
  §10 (audit timeline + dashboard).
- **Platform engineers** — see §3 (install), §4 (tenant init), §11 (REST + MCP),
  §12 (running in CI).
- **Developers** — see §7 (writing policy), §8 (using the CLI from a PR),
  §9 (GitHub Action).

## Themis at a glance

Themis records, classifies, and gates every change AI coding tools make
to your software:

1. An adapter normalises whatever the AI produced (Claude Code transcript,
   git diff, manual attestation) into a single `AIChange` record.
2. A classifier examines the change against your EventCatalog graph and
   labels its impact (`DOC_ONLY` … `SCHEMA_BREAKING`).
3. Scanners (secrets, PII, slopsquat, hallucinated imports) inspect the
   touched files for risk findings.
4. A pure YAML policy engine takes `(AIChange, Impact, Findings)` and
   produces an `ALLOW` / `REQUIRE_APPROVAL` / `DENY` decision.
5. Themis builds a signed AI Bill-of-Materials (BOM) for the PR; every
   step lands as an append-only Merkle-chained ledger event.
6. Reviewers grant or deny approvals; emergency overrides require a
   co-signer, a 50-character reason, and a 7-day post-mortem.

The full design spec is in [`docs/superpowers/specs/2026-05-22-themis-design.md`](../superpowers/specs/2026-05-22-themis-design.md).

---

## §1 — Prerequisites

- Go 1.26+ (`brew install go`).
- `git` 2.30+.
- A POSIX shell. Steps assume bash or zsh.
- (optional) `jq` for prettier API output.

## §2 — Get the source

```bash
git clone https://github.com/tzone85/themis.git
cd themis
```

## §3 — Smoke test

```bash
make ci
```

You should see a green `[cover-check] PASS` at the end. If anything fails,
stop here and file an issue — every subsequent section assumes a healthy
local build.

Build the binary:

```bash
go build -o /tmp/themis ./cmd/themis
/tmp/themis --version
```

## §4 — Bootstrap your first tenant

A *tenant* is the isolation boundary: every customer/business-unit gets
its own directory under `<base>/tenants/<id>/`. Cross-tenant access is
physically impossible.

```bash
export DIR=/tmp/themis-onboarding
/tmp/themis tenant init --id acme --base "$DIR"
ls "$DIR/tenants/acme/"
```

You should see:

```
bom            # signed BOMs land here
events.jsonl   # the append-only Merkle ledger
mempalace-wing # advisory + audit drawer storage
```

## §5 — Catalogue + decide your first PR

Themis classifies changes against an EventCatalog (events, services,
domains). For the tutorial we use the bundled sample tree.

```bash
/tmp/themis catalogue sync --id acme --base "$DIR" \
  --source ./internal/catalogue/testdata/sample
```

Now write a minimal AIChange and a permissive policy:

```bash
cat > "$DIR/ai.json" <<EOF
{
  "pr_id": "demo#1",
  "actor": "claude_code",
  "touched_files": [
    {"path": "README.md", "change_kind": "MODIFIED",
     "before_hash": "a", "after_hash": "b"}
  ]
}
EOF

cat > "$DIR/themis.yaml" <<EOF
version: 1
default: REQUIRE_APPROVAL
required_approvers_for_default:
  - role: senior
rules:
  - name: doc-only allowed
    when:
      impact.kind: [DOC_ONLY]
    then:
      verdict: ALLOW
  - name: secrets block
    when:
      findings.kind: secret
    then:
      verdict: DENY
EOF

/tmp/themis decide --id acme --base "$DIR" \
  --aichange "$DIR/ai.json" --policy "$DIR/themis.yaml"
```

Expected verdict: `ALLOW` (doc-only).

## §6 — Approvals + override

When a decision returns `REQUIRE_APPROVAL`, named approvers grant or deny:

```bash
/tmp/themis approval grant \
  --id acme --base "$DIR" \
  --pr-id "demo#1" \
  --approver "human:alice" \
  --role senior \
  --comment "LGTM"

/tmp/themis approval status \
  --id acme --base "$DIR" --pr-id "demo#1"
```

A `DECISION_FINALISED` event is appended automatically once every required
role has granted. Denials are sticky for the current decision — a new
`themis decide` (after the diff changes) clears them.

For an emergency, the override flow forces a verdict:

```bash
/tmp/themis override invoke \
  --id acme --base "$DIR" \
  --pr-id "emergency#1" \
  --actor "human:alice" \
  --co-signer "human:bob" \
  --reason "Catalogue server outage blocking on-call; need to merge logging fix before next deploy."
```

The reason **must** be ≥ 50 characters, the actor and co-signer **must** differ,
and a post-mortem is scheduled 7 days out:

```bash
/tmp/themis override close-postmortem \
  --id acme --base "$DIR" \
  --pr-id "emergency#1" \
  --closer "human:compliance" \
  --notes "root cause identified; logging filter updated"
```

## §7 — Writing policy

Policy YAML lives next to your source code. Lint it before committing:

```bash
/tmp/themis policy lint --file "$DIR/themis.yaml"
```

A few common rule patterns:

```yaml
# Block on any secret-kind finding.
- name: secrets block
  when:
    findings.kind: secret
  then:
    verdict: DENY

# Require compliance sign-off for any PII at or above "high" severity.
- name: pii needs compliance
  when:
    findings.kind: pii
    findings.severity: ">=high"
  then:
    verdict: REQUIRE_APPROVAL
    required_approvers:
      - role: compliance

# Domain-scoped rule.
- name: collections domain needs sign-off
  when:
    impact.domain: Collections
    impact.kind: [CONSUMER_TOUCH, PRODUCER_TOUCH, NEW_EVENT, SCHEMA_BREAKING]
  then:
    verdict: REQUIRE_APPROVAL
    required_approvers:
      - role: compliance
```

Full schema is in `internal/policy/schema.go`.

## §8 — Ingesting real PRs

Three adapters ship by default:

```bash
# git_heuristic — universal, diffs against a base ref.
/tmp/themis ingest --id acme --base "$DIR" \
  --adapter git_heuristic \
  --pr-id "feat-abc#42" \
  --workdir "$PWD" --base-ref origin/main

# claude_code_transcript — parses a Claude Code session export.
/tmp/themis ingest --id acme --base "$DIR" \
  --adapter claude_code_transcript \
  --pr-id "feat-abc#42" \
  --transcript ./claude-session.json

# manual_attestation — operator declares the shape.
/tmp/themis ingest --id acme --base "$DIR" \
  --adapter manual_attestation \
  --pr-id "retroactive#1" \
  --actor "human:thandi" \
  --file "src/x.go=before-hash,after-hash"
```

All three write the AIChange to `tenants/<id>/aichange/<pr-id>.json` and emit
`INGEST_COMPLETED`. Failures emit `INGEST_ADAPTER_FAILED` with the reason.

## §9 — Running in CI

The repo ships a composite GitHub Action at `actions/themis-check`. Sample
workflow:

```yaml
name: themis-check
on: pull_request
jobs:
  themis:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: ./actions/themis-check
        with:
          themis-base-url: ${{ secrets.THEMIS_URL }}
          themis-token:    ${{ secrets.THEMIS_TOKEN }}
          tenant-id:       acme
          policy-path:     themis.yaml
          fail-on-require-approval: 'false'
```

For local enforcement, install the pre-push hook:

```bash
cp scripts/hooks/pre-push.sh .git/hooks/pre-push
chmod +x .git/hooks/pre-push
```

## §10 — BOMs, dashboard, audit

Build + sign a BOM for the decision:

```bash
/tmp/themis bom build --id acme --base "$DIR" --pr-id "demo#1"
/tmp/themis bom sign  --id acme --base "$DIR" --pr-id "demo#1"
```

The BOM file lands at `tenants/acme/bom/<hash>.bom.json` with a `.sig`
sidecar and a `.bundle.json` envelope (carries the signer mode, public key,
and Rekor URL for Sigstore-style signing).

Mint an admin token and start the dashboard:

```bash
TOKEN=$(/tmp/themis tokens grant --base "$DIR" \
  --tenant acme --role admin --description onboarding \
  | grep ^thm_)

/tmp/themis serve --base "$DIR" --addr 127.0.0.1:8787 &
open "http://127.0.0.1:8787/?tenant=acme&token=$TOKEN"
```

The dashboard shows the audit timeline (newest first), a kind filter, and
a detail pane that renders each event's payload as JSON.

## §11 — REST API + MCP

Every CLI command has a REST equivalent. Hit them with curl:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://127.0.0.1:8787/v1/tenants/acme/health

curl -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8787/v1/tenants/acme/decisions?pr_id=demo%231"

curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"ai_change":{"pr_id":"x","actor":"y","touched_files":[]},
       "policy_yaml":"version: 1\ndefault: ALLOW\n"}' \
  http://127.0.0.1:8787/v1/tenants/acme/decide
```

For agentic clients, the MCP bridge speaks JSON-RPC over stdio:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | /tmp/themis mcp \
      --base-url http://127.0.0.1:8787 \
      --token "$TOKEN" --tenant-id acme
```

Four tools are exposed: `themis_health`, `themis_decisions`,
`themis_bom`, `themis_events`.

## §12 — Heartbeat + integrity

Themis isn't useful if someone silently removes the CI check. The
heartbeat daemon polls each tenant repo and records
`ENFORCEMENT_MISSING` when the required check disappears.

Configure targets:

```bash
cat > "$DIR/tenants/acme/heartbeat.yaml" <<EOF
targets:
  - repo: gh:org/svc-a
    expected_check: themis-check
  - repo: gh:org/svc-b
    expected_check: themis-check
EOF
```

One-shot:

```bash
/tmp/themis heartbeat run-once --id acme --base "$DIR" \
  --stub-allow gh:org/svc-a
# → "heartbeat: 1 miss(es) recorded"
```

Long-running:

```bash
/tmp/themis heartbeat watch --id acme --base "$DIR" --interval 300
```

For ledger integrity, `themis ledger verify` walks the Merkle chain; on
any break it appends `LEDGER_INTEGRITY_BROKEN` to a sidecar
`incidents.jsonl` (the main ledger can't be trusted at that point).
For external transparency anchoring:

```bash
/tmp/themis ledger anchor --id acme --base "$DIR" \
  --sink "s3://audit-bucket/themis/anchors/"
```

## §13 — Where to go next

- Recipes: [`docs/onboarding/cookbook/README.md`](cookbook/README.md).
- Hands-on exercises: [`docs/onboarding/exercises/README.md`](exercises/README.md).
- Full design spec: [`docs/superpowers/specs/2026-05-22-themis-design.md`](../superpowers/specs/2026-05-22-themis-design.md).
- Stakeholder briefs (compliance, security, exec): [`docs/stakeholders/`](../stakeholders/).
- Plan history: every plan in [`.claude/plans/`](../../.claude/plans/) documents
  one shippable slice end-to-end.

If you found a step that didn't work, open an issue. The walkthrough is
covered by `internal/cli/e2e_test.go::TestE2E_FullPipeline` and
`TestE2E_RealGitRepo`; new defects should fail at least one of those
tests in CI before reaching this doc.
