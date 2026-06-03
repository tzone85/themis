---
title: Themis production-readiness pass → v0.1.0
date: 2026-06-03
status: design
owner: thando.mini@sanlam.co.za
supersedes: —
related:
  - "[[2026-05-22-themis-design]]"
  - "[[CHANGELOG]]"
tags: [project/themis, type/spec, status/draft]
---

# Themis production-readiness pass → v0.1.0

## Goal

Ship Themis as a tagged, signed, container-runnable, ops-documented `v0.1.0` release that satisfies the trust posture the product itself sells.

## Non-goals

- No new feature work. Plans 1-18 set the functional scope; this pass closes packaging, governance, ops, and docs gaps.
- No CI overhaul. Existing `ci.yml` is retained; release adds a *separate* job.
- No multi-arch image push to private registry. v0.1.0 ships images to `ghcr.io/tzone85/themis` only.
- No managed-hosting bits (those belong to the commercial closed-source side per design §16).

## Success criteria

A tagged release is "production-ready" when *all* hold:

1. `git tag v0.1.0` triggers a GitHub Actions workflow that produces signed binaries (linux/darwin/windows × amd64/arm64), a signed container image on `ghcr.io`, and an SPDX SBOM attached to the release.
2. `cosign verify-blob` against any published binary succeeds using GitHub OIDC issuer.
3. `docker run ghcr.io/tzone85/themis:v0.1.0 --version` prints `themis v0.1.0 (<commit> <date>, go1.26.1)`.
4. `SECURITY.md` documents a working disclosure path; `CONTRIBUTING.md` reproduces the local dev loop; `CODE_OF_CONDUCT.md` is Contributor Covenant 2.1; `SUPPORT.md` scopes what is in/out.
5. An operator following `docs/ops/deployment.md` end-to-end on a clean Linux host reaches a running `themis serve` with the bundled sample tenant, *without* reading source.
6. Obsidian `Themis/Themis.md` status reflects "Plans 1-18 shipped; v0.1.0 cut 2026-06-03" with wikilinks to a new `Production Readiness 2026-06-03` vault note.
7. `make ci` is green on the tag commit; `govulncheck` is *gating* on the release job (advisory remains on PR/main).

## Architecture (existing — unchanged)

This pass adds *packaging and process artefacts* around the existing architecture. The trust-critical core (ledger, catalogue, policy, BOM, signer, OIDC) is not modified. Phase 2 touches `internal/cli/root.go` to widen the existing `Version` var into a `BuildInfo` struct — see §Phase 2 — but the behaviour change is additive.

## Phase plan

Eight phases, sequenced by dependency. One commit per phase, conventional-commits prefix in parens.

### Phase 1 — Governance scaffolding `(docs)`

**Deliverables**

| File | Contents |
|---|---|
| `SECURITY.md` | Supported versions table (only latest minor); disclosure path: GitHub Security Advisories (`/security/advisories/new`) — preferred and self-routing — with `thando.mini@sanlam.co.za` as fallback; 90-day coordinated disclosure window; explicit "do not file public issues for vulns"; reference to `govulncheck` posture (advisory on main, gating on release). No PGP key required at v0.1.0 — GH Advisories is end-to-end encrypted. |
| `CONTRIBUTING.md` | Dev setup (`make build && make test`); coverage gate (`scripts/cover_check.sh` + `coverage.thresholds.yaml`); plan-flow conventions (`docs/superpowers/plans/`); commit message format (conventional commits); branch protection note. |
| `SUPPORT.md` | In-scope: bug reports, security reports, design questions. Out-of-scope until v1.x: feature requests, third-party policy authoring help. Pointer to discussions vs. issues. |
| `CODE_OF_CONDUCT.md` | Contributor Covenant 2.1 verbatim; enforcement contact `thando.mini@sanlam.co.za` (single-maintainer project). |
| `CHANGELOG.md` | New `## v0.1.0 — 2026-06-03` heading at top; entries grouped Governance / Packaging / Ops / Docs / CI. |

**Verification**

- `gh repo view` shows the four governance files in the repo "Community Standards" check.
- No file references nonexistent paths.

**YAGNI exclusions**

- No `GOVERNANCE.md` (single-maintainer project; revisit at v1.0).
- No issue/PR templates (defer until external contributors arrive).
- No `FUNDING.yml` (no sponsorship channel yet).

### Phase 2 — Binary versioning `(feat(cli))`

**Problem.** `internal/cli/root.go` exposes a single ldflag-injectable `Version` string. Operators auditing a running deployment need *commit* and *build date* too — and they cannot derive them from the binary today.

**Change.** Widen the version surface to a `BuildInfo` struct with `Version`, `Commit`, `Date`, `GoVersion`. Cobra's `--version` template prints all four.

**Files**

| File | Action |
|---|---|
| `internal/cli/root.go` | Replace `var Version = "dev"` with `var (Version = "dev"; Commit = "none"; Date = "unknown")`. Set `root.Version` to a formatted string built at command construction: `fmt.Sprintf("%s (commit %s, built %s, go %s)", Version, Commit, Date, runtime.Version())`. Keep cobra's default `--version` flag (no custom `SetVersionTemplate` needed). |
| `internal/cli/root_test.go` | Add `TestVersionFlag` asserting all four fields appear. |
| `Makefile` | Add `VERSION`/`COMMIT`/`DATE` vars derived from `git describe --tags --dirty`, `git rev-parse --short HEAD`, `date -u +%FT%TZ`; pass to `go build -ldflags`. |

**Test:** `themis --version` after `make build` shows `themis v0.1.0-<n>-g<sha>-dirty (commit <sha>, built <iso>, go go1.26.1)`.

### Phase 3 — Containerization `(build)`

**Files**

| File | Contents |
|---|---|
| `Dockerfile` | Multi-stage. Builder: `golang:1.26.1-alpine` (matches `go.mod`); `RUN apk add --no-cache git`; `go mod download` cached; `go build -trimpath -ldflags="..."`. Runtime: `gcr.io/distroless/static-debian12:nonroot`; copy binary; `ENTRYPOINT ["/themis"]`; `USER nonroot:nonroot`; `EXPOSE 8787`. No CMD — operators pass subcommand. |
| `.dockerignore` | Exclude `.git`, `coverage.*`, `*.md` (except LICENSE), `docs/`, IDE files. Keep `go.mod`, `go.sum`, `cmd/`, `internal/`, `actions/`, `scripts/` (smoke script needed for CI step). |
| `scripts/docker_smoke.sh` | Build image, run `--version`, run `tenant init` against a tmp dir, assert image size < 30 MB. Called from CI + locally. |

**Verification**

- `docker build -t themis:local .` succeeds.
- `docker run --rm themis:local --version` prints version.
- `docker run --rm -v $PWD/demo:/data themis:local tenant init --id smoke --base /data` succeeds and writes to host-mounted volume (proves nonroot UID works against operator-supplied dirs).
- Image size < 30 MB (distroless static + statically linked Go binary).

**YAGNI exclusions**

- No `HEALTHCHECK` directive (kubelet/Compose owners can add it; baking it makes the image opinionated).
- No multi-arch buildx in this phase (Phase 4 goreleaser does it).

### Phase 4 — Release pipeline `(ci)`

**Files**

| File | Contents |
|---|---|
| `.goreleaser.yaml` | `version: 2`. Builds: linux/darwin/windows × amd64/arm64; `-trimpath`, ldflags inject `Version`/`Commit`/`Date`. Archives: `tar.gz` (unix), `zip` (windows). Checksums: `sha256`. Dockers: `linux/amd64` + `linux/arm64` push to `ghcr.io/tzone85/themis:{{.Version}}` and `:latest`. Sboms: `syft` SPDX per archive + per image. Signs: `cosign` keyless on archives, checksums, and image — `GITHUB_TOKEN` + `id-token: write`. Release notes: from `CHANGELOG.md` v0.1.0 section. |
| `.github/workflows/release.yml` | Triggers on `push: tags: ['v*']`. Permissions: `contents: write`, `id-token: write`, `packages: write`. Steps: checkout (fetch-depth 0), setup-go from `go.mod`, install `cosign` + `syft`, login to `ghcr.io`, `goreleaser release --clean`. |

**Verification**

- `goreleaser release --snapshot --clean --skip=sign,publish` dry-runs locally without errors.
- After tag push, release page shows: 6 archives, `checksums.txt`, `checksums.txt.sig`, `checksums.txt.pem`, 6 `.spdx.json` SBOMs.
- `cosign verify-blob --certificate-identity-regexp '^https://github.com/tzone85/themis/' --certificate-oidc-issuer https://token.actions.githubusercontent.com --certificate <pem> --signature <sig> <archive>` passes.
- `cosign verify ghcr.io/tzone85/themis:v0.1.0 --certificate-identity-regexp '...' --certificate-oidc-issuer '...'` passes.

**YAGNI exclusions**

- No Homebrew tap, no Scoop bucket, no `nfpms` `.deb`/`.rpm` (deferred until distribution demand surfaces).
- No `winget` manifest.

### Phase 5 — Observability + ops docs `(docs)`

**Files**

| File | Contents |
|---|---|
| `docs/ops/deployment.md` | Three install paths: (a) `docker run ghcr.io/tzone85/themis:v0.1.0`, (b) binary download + `cosign verify-blob`, (c) `go install` for dev. Tenant bootstrap end-to-end. Systemd unit example (`User=themis`, `ExecStart=/usr/local/bin/themis serve --base /var/lib/themis --addr 127.0.0.1:8787`, `Restart=on-failure`). Reverse-proxy snippet (Caddy + Nginx). |
| `docs/ops/observability.md` | Current state honestly stated: structured logs to stderr (format documented from `internal/api`); no Prometheus surface yet (Plan 19 candidate); no OTEL tracing yet. Log field reference. How to follow `ledger doctor` output. What `themis heartbeat watch` emits. |
| `docs/ops/backup-restore.md` | Base-dir layout (`tenants/<id>/` per design §X). What to snapshot: `ledger.jsonl`, `ledger.sqlite`, `aichange/`, `bom/`, `mempalace/drawers/`. What *not* to snapshot: `cache/`, `tmp/`. Replay semantics: `themis ledger doctor` after restore. Drill: restore-to-empty-host walkthrough. |
| `docs/ops/runbook.md` | Common incidents with diagnosis steps + remediation: (1) `ledger doctor` reports hash mismatch, (2) OIDC chain fallback firing (Plan 18), (3) anchor sink unreachable (Plan 11), (4) BOM signer failure (cosign vs. ed25519), (5) heartbeat checker stuck. Each entry: symptom → quick check → fix → escalation. |

**Verification**

- Each ops doc has a "Verified against `v0.1.0` on `2026-06-03`" footer.
- Every command in `deployment.md` is copy-pasted from a real terminal session.
- `runbook.md` incident entries cite the code path (`internal/<pkg>/<file>.go` symbol name — never line numbers).

### Phase 6 — CI hardening `(ci)`

**Changes to `.github/workflows/ci.yml` and `release.yml`**

| Change | Why |
|---|---|
| Pin all third-party actions to commit SHAs (`actions/checkout@<sha> # v4.x.y`). | Supply-chain hygiene; mirrors what Themis itself asserts for downstream consumers. |
| Add `permissions:` block (least privilege) to each workflow. | GitHub default is overly broad; explicit `contents: read` on `ci.yml`. |
| In `release.yml` only, run `govulncheck ./...` *without* `continue-on-error`. | Releases must not ship known-vulnerable stdlib/deps; PR/main keep advisory posture so dev velocity isn't blocked by Go-toolchain-only advisories. |

**Verification**

- `gh workflow view ci` shows pinned SHAs.
- `gh workflow view release` shows gating govulncheck step.
- Intentionally introducing a vulnerable dep on a branch and tagging triggers a failing release (test via `goreleaser` dry-run + simulated tag).

### Phase 7 — Docs sync (repo + Obsidian) `(docs)`

**Repo changes**

| File | Change |
|---|---|
| `README.md` | Update "Status" badge line → "Plans 1-18 shipped; `v0.1.0` released 2026-06-03". Add new `## Install` section above `## Quickstart` with the three install paths (Docker, signed binary + `cosign verify`, source). Add `## Verify a release` section showing the `cosign verify-blob` invocation. Add badge row at top: build, coverage, govulncheck, release. |
| `docs/stakeholders/README.md` | Re-date; add a "What changed since 2026-05-22" subsection pointing at `CHANGELOG.md` and the new ops docs. |
| `docs/superpowers/specs/2026-05-22-themis-exec-summary.md` | Update "Cost" + "What you get at end of 90 days" if any numbers shifted (likely unchanged; verify don't rewrite). |

**Obsidian vault changes** (vault root: `/Users/mncedimini/Documents/Obsidian Vault/Themis`)

| File | Change |
|---|---|
| `Themis.md` | Rewrite "Status" callout: "Plans 1-18 shipped; `v0.1.0` released 2026-06-03". Add "Recent" section with [[2026-06-03-themis-production-readiness]] link. Update "Local paths" if any moved. |
| `00-README.md` | Replace contents with current repo `README.md` mirror (post-Phase-7 README). |
| `2026-06-03-themis-production-readiness.md` *(new)* | Front-matter (`tags: [project/themis, type/release-notes, status/shipped]`). Summary of what this release pass added — governance, versioning, container, release pipeline, ops docs, CI hardening, v0.1.0 tag. Wikilinks back to [[2026-05-22-themis-design]] and [[CHANGELOG]]. |
| `2026-05-22-themis-engineering-brief.md` | Add a "Status update 2026-06-03" appendix linking the new note; do not rewrite the brief (it's a frozen pitch artefact). Same treatment for compliance + security briefs. |

**Verification**

- Obsidian status line no longer says "pre-implementation".
- Every wikilink resolves to an extant note in the vault.
- Repo README install commands are pastable as-is.

### Phase 8 — Tag v0.1.0 `(release)`

**Steps**

1. Confirm `main` is green: `make ci` passes locally and the latest `ci` workflow on `main` is success.
2. `git tag -s v0.1.0 -m "v0.1.0 — production-readiness pass"` (signed tag).
3. `git push origin v0.1.0`.
4. Watch `release.yml`: expect ~8 min runtime.
5. Verify release page artefacts: 6 archives, checksums + sig + pem, 6 SBOMs, source tarball.
6. Verify image: `cosign verify ghcr.io/tzone85/themis:v0.1.0 ...`.
7. Verify binary: download linux/amd64 archive, extract, `./themis --version` shows `v0.1.0`.
8. Final commit on `main`: append "v0.1.0 released 2026-06-03 — see GitHub Releases" to `CHANGELOG.md` top.

**Rollback** *if release pipeline fails*: delete the tag (`git push --delete origin v0.1.0`), fix the failing phase, re-tag. No artefacts have been published to consumers at this stage so no compatibility cost.

## Testing approach

| Layer | Method |
|---|---|
| Phase 2 (versioning) | Unit test in `internal/cli/root_test.go`. |
| Phase 3 (Dockerfile) | `scripts/docker_smoke.sh` (added in Phase 3): builds image, runs `--version`, runs `tenant init` against a tmp dir, asserts image size < 30 MB. |
| Phase 4 (release pipeline) | `goreleaser release --snapshot --skip=sign,publish` in PR; full pipeline only on tag. |
| Phase 5 (ops docs) | Manual end-to-end on clean Ubuntu 24.04 LXC. |
| Phase 6 (CI) | Force a govulncheck finding in a branch + dry-run tag to confirm gating fails the release. |
| Phase 7 (docs) | Spell-check + wikilink-lint (manual); copy-paste verification of all command blocks. |
| Phase 8 (tag) | Production smoke on the published `ghcr.io` image. |

## Risks + mitigations

| Risk | Mitigation |
|---|---|
| `ghcr.io` push fails for permissions reasons | Pre-create the package + set visibility; document in CONTRIBUTING. |
| `cosign` keyless requires `id-token: write` not granted by default | Explicit `permissions:` block in `release.yml`. |
| Distroless `nonroot` UID 65532 mismatches operator's volume permissions | Document `chown -R 65532:65532 /data` step in deployment.md, *with reason*. |
| `govulncheck` reports stdlib finding on tag day | Advisory mode on main keeps dev unblocked; tag-time gating means we must bump Go toolchain before tagging. Acceptable: forces a fresh Go on every release. |
| Obsidian wikilinks break if repo file paths move | Vault notes link to *titles*, not paths; titles are stable. |

## Glossary

- **BuildInfo** — the four-field version struct introduced in Phase 2.
- **goreleaser** — Go-native release tool that handles cross-compilation, archiving, signing, image build/push, SBOM generation, and GitHub Release creation from a single config.
- **cosign keyless** — Sigstore signing flow using ephemeral certs minted by Fulcio against the GitHub Actions OIDC token; no long-lived key material.
- **distroless static-nonroot** — `gcr.io/distroless/static-debian12:nonroot`; ~2 MB; no shell; runs as UID 65532.
- **SPDX SBOM** — Software Bill of Materials in SPDX JSON format; generated by `syft` per archive + per image; attached to release.

## FAQ

**Why goreleaser instead of hand-rolled workflow?** Single source of truth for build matrix, archiving, SBOM, signing, and image build. Hand-rolled equivalent is ~400 lines of YAML and re-implements problems goreleaser solved years ago.

**Why distroless instead of `scratch`?** Distroless ships CA certs and `/etc/passwd` for the `nonroot` user; `scratch` does not. Themis's anchor sinks talk HTTPS — CA certs are mandatory.

**Why not gate govulncheck on PRs too?** Stdlib advisories can only be cleared by bumping the Go toolchain. Forcing a bump per advisory blocks unrelated PRs. Gating on release means each *release* is current; daily dev tolerates a known advisory window.

**Why ship the SBOM per archive *and* per image?** Different consumers: archive SBOM is for operators auditing the binary they downloaded; image SBOM is what container scanners (Trivy, Grype) consume from the registry.

## Out of scope (deferred)

- Helm chart (deferred to v0.2.0; depends on operator demand).
- Multi-arch image manifest (`linux/arm/v7`, `linux/riscv64`) — unblocked but not justified.
- Reproducible-build attestation — interesting but requires deterministic build env; defer.
- Renovate / Dependabot config — deferred; manual go.mod hygiene is fine at current pace.
