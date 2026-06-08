# Themis

[![CI](https://github.com/tzone85/themis/actions/workflows/ci.yml/badge.svg)](https://github.com/tzone85/themis/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tzone85/themis?sort=semver)](https://github.com/tzone85/themis/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/tzone85/themis)](https://go.dev)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![SLSA 3](https://img.shields.io/badge/SLSA-3-success)](https://slsa.dev)

**A compliance gateway for AI-generated code.**

Themis records and governs every change that AI coding tools (Claude Code, Cursor, GitHub Copilot, autonomous agents like [VXD](https://github.com/tzone85/vortex-dispatch)) make to your software. It captures *who* (which AI, which prompt, which human) changed *what* (which event contract, which service, which downstream consumer), proves it cryptographically (signed AI Bill of Materials per pull request, tamper-evident ledger), and stops changes that violate your policies before they merge.

## Status

> **`v0.1.0` cut 2026-06-03.** Plans 1-18 shipped; full production-readiness pass landed (governance scaffolding, four-field `--version`, distroless container, goreleaser + cosign keyless + syft SBOM on tag, ops docs, CI hardening with SHA-pinned actions). Foundations through OIDC; every trust-layer item from design spec §9 implemented; every deferred item from Plan 11 closed. Full plan-by-plan detail in [`CHANGELOG.md`](CHANGELOG.md); 30-minute walkthrough in [`docs/onboarding/README.md`](docs/onboarding/README.md); operator install paths in [`docs/ops/deployment.md`](docs/ops/deployment.md).

## Install

Three install paths. Pick one. Full operator guide:
[`docs/ops/deployment.md`](docs/ops/deployment.md).

```bash
# A. Container (recommended).
docker pull ghcr.io/tzone85/themis:0.1.0

# B. Signed binary.
VERSION=v0.1.0
curl -sLo themis.tar.gz \
  "https://github.com/tzone85/themis/releases/download/${VERSION}/themis_${VERSION#v}_linux_x86_64.tar.gz"
tar -xzf themis.tar.gz && sudo install -m 0755 themis /usr/local/bin/themis

# C. From source.
git clone https://github.com/tzone85/themis && cd themis && make build
```

## Verify a release

Every release is signed with [cosign keyless](https://docs.sigstore.dev/)
via GitHub Actions OIDC and ships SPDX SBOMs (`syft`) per archive and
per image. Before deploying:

```bash
# Container image.
cosign verify ghcr.io/tzone85/themis:0.1.0 \
  --certificate-identity-regexp '^https://github.com/tzone85/themis/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

# Binary: verify the checksums file, then check the archive matches its line.
cosign verify-blob \
  --certificate checksums.txt.pem --signature checksums.txt.sig \
  --certificate-identity-regexp '^https://github.com/tzone85/themis/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
grep themis_0.1.0_linux_x86_64.tar.gz checksums.txt | sha256sum -c
```

## Quickstart

For a guided, top-to-bottom walkthrough see [`docs/onboarding/README.md`](docs/onboarding/README.md).
This block is the smallest possible end-to-end demo.

```bash
go build -o /tmp/themis ./cmd/themis
DIR=/tmp/themis-demo

# 1. Tenant bootstrap.
/tmp/themis tenant init --id acme --base "$DIR"

# 2. Mint an admin token for the tenant.
/tmp/themis tokens grant --base "$DIR" --tenant acme --role admin --description quickstart

# 3. Catalogue snapshot (uses the bundled sample tree).
/tmp/themis catalogue sync --id acme --base "$DIR" \
  --source ./internal/catalogue/testdata/sample

# 4. Ingest a real PR via git_heuristic (or write your own AIChange JSON).
/tmp/themis ingest --id acme --base "$DIR" \
  --adapter git_heuristic --pr-id "demo#1" \
  --workdir "$PWD" --base-ref HEAD~1

# 5. Policy + decide.
echo 'version: 1
default: REQUIRE_APPROVAL
rules:
  - name: doc-only allowed
    when:
      impact.kind: [DOC_ONLY]
    then:
      verdict: ALLOW' > "$DIR/themis.yaml"
/tmp/themis decide --id acme --base "$DIR" \
  --aichange "$DIR/tenants/acme/aichange/demo_1.json" \
  --policy "$DIR/themis.yaml"

# 6. Build + sign the BOM (local ed25519; cosign-keyless-stub also available).
/tmp/themis bom build --id acme --base "$DIR" --pr-id "demo#1"
/tmp/themis bom sign  --id acme --base "$DIR" --pr-id "demo#1"

# 7. Get an LLM-style advisory note (NullLLM by default).
/tmp/themis advise --id acme --base "$DIR" --pr-id "demo#1"

# 8. Ledger integrity + external anchor.
/tmp/themis ledger doctor --id acme --base "$DIR"
/tmp/themis ledger anchor --id acme --base "$DIR" --sink "s3://demo/"

# 9. Start the REST API + dashboard.
/tmp/themis serve --base "$DIR" --addr 127.0.0.1:8787 &
open http://127.0.0.1:8787/?tenant=acme    # the dashboard
```

## Changelog

See [`CHANGELOG.md`](CHANGELOG.md) for the full plan-by-plan history
(plans 1 through 18). The README used to embed it; that section grew
to ~360 lines and pushed the operator-facing content below the fold.

## Documentation

- 🚀 **[Onboarding tutorial](docs/onboarding/README.md)** — 30-minute walkthrough from clean install to a running deployment, every command verified live. Start here if you want to *use* Themis.
- 📒 **[Cookbook](docs/onboarding/cookbook/README.md)** — 8 named recipes (locked-down policy, supply-chain, override, Sigstore-stub, OIDC chain, custom claim mapping, …), each citing the test that proves it.
- 🧪 **[Exercises](docs/onboarding/exercises/README.md)** — 6 hands-on exercises with check commands.
- 📜 **[Changelog](CHANGELOG.md)** — plan-by-plan history.
- 📄 **[Executive summary](docs/stakeholders/exec-summary.md)** — one page.
- 📄 **[Full design specification](docs/design.md)** — the canonical artefact.
- 📁 **[Stakeholder briefs index](docs/stakeholders/README.md)** — compliance, engineering, security, exec, anchor pilot proposal.
- 🛠 **[Operations](docs/ops/)** — [`deployment`](docs/ops/deployment.md), [`observability`](docs/ops/observability.md), [`backup-restore`](docs/ops/backup-restore.md), [`runbook`](docs/ops/runbook.md).
- 🔐 **Governance** — [`SECURITY`](SECURITY.md), [`CONTRIBUTING`](CONTRIBUTING.md), [`SUPPORT`](SUPPORT.md), [`CODE_OF_CONDUCT`](CODE_OF_CONDUCT.md).

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

An anchor regulated organisation (insurance / financial services, South Africa) — see [the anchor pilot proposal](docs/stakeholders/anchor-pilot-proposal.md) for scope, plan, exit criteria, and risk profile.

## Licence (planned)

- **EventCatalog plugin** (`@themis/eventcatalog-plugin`): Apache 2.0.
- **AI-BOM schema** (`themis-bom-schema`): Apache 2.0.
- **Themis Core**: open-core vs. closed-source initially is a stakeholder decision. Decision needed before code is published.

---

© 2026 Thando Mini. All rights reserved until licence decision finalised.
