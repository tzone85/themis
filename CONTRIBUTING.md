# Contributing to Themis

Welcome. Themis is a compliance gateway for AI-generated code, so the
trust properties of the codebase itself matter as much as the features.
This file covers what you need to know to land a change.

## Before you start

1. Read the canonical design spec: [`docs/design.md`](docs/design.md).
2. Read the changelog for the last few plans to see how recent changes
   landed: [`CHANGELOG.md`](CHANGELOG.md).
3. Read the [`Code of Conduct`](CODE_OF_CONDUCT.md) (Contributor
   Covenant 2.1 by reference) and the [`Security Policy`](SECURITY.md).

If your change is non-trivial (touches a trust-critical path or adds a
public surface), open a GitHub Discussion first to agree on the shape
before you write code.

## Local dev loop

Themis is a single Go module. The only hard prerequisites are:

- Go ≥ the version pinned in [`go.mod`](go.mod) (currently `go 1.26.1`).
- `make`, `bash`, `git`.

Recommended workflow:

```bash
git clone https://github.com/tzone85/themis
cd themis

# Build the CLI binary to ~/.local/bin/themis (override BIN= for elsewhere).
make build

# Run the full unit-test suite with the race detector.
make test

# Run the coverage gate (writes coverage.out + enforces thresholds).
make cover

# Run the full CI suite locally (vet + lint + test + cover + vulncheck).
make ci
```

`make ci` is the single command you should expect to pass before pushing.
It runs `go vet`, `golangci-lint`, `go test -race`, the coverage gate,
and `govulncheck` (in advisory mode locally — same as on PR/main).

### Windows contributors

Themis builds and runs natively on Windows — no WSL required. The
`Makefile` assumes a POSIX shell, so use one of:

- **WSL2 (recommended for hacking)** — clone inside the WSL filesystem and
  run `make ci` exactly as documented above.
- **PowerShell / cmd** — skip the Makefile and drive `go` directly:

  ```powershell
  go build -o $env:USERPROFILE\.local\bin\themis.exe .\cmd\themis
  go test ./... -race -count=1
  ```

Cross-compile from any host to verify the Windows build is green:

```bash
GOOS=windows GOARCH=amd64 go build ./...
```

See the README's Platform Support section for the supported matrix.

## Coverage gate

Coverage is enforced by [`scripts/cover_check.sh`](scripts/cover_check.sh)
against the targets in
[`coverage.thresholds.yaml`](coverage.thresholds.yaml).

- Global floor: 87.0 %.
- Per-package targets override the global floor where they're higher.
- A change that drops coverage on any gated package will fail
  `make cover` and CI.

If you add a new package, add it to `coverage.thresholds.yaml` with a
realistic target. Calibrate slightly above the *current* covered ratio
so future regressions fail loudly.

## Plan-driven workflow

Non-trivial changes follow the plan flow:

1. Open a plan document under [`docs/plans/`](docs/plans/)
   named `YYYY-MM-DD-<topic>.md`. Cover: goal, non-goals, design,
   testing approach, risks.
2. Get the plan reviewed.
3. Implement the plan in a feature branch.
4. Land via a single squash commit (or a short, focused commit
   sequence) referencing the plan.
5. Append a `## Unreleased — <topic>` entry to [`CHANGELOG.md`](CHANGELOG.md).

Trivial changes (typo, dependency bump, single-file fix) skip the plan
flow and go straight to PR.

## Commit messages

Conventional Commits, one type per commit:

```
<type>(<scope>): <subject>

<optional body explaining the why>
```

Types we use:

- `feat` — a new user-visible capability.
- `fix` — a bug fix.
- `refactor` — internal restructuring with no behaviour change.
- `docs` — documentation only.
- `test` — tests only (no production code change).
- `chore` — build, deps, repo housekeeping.
- `perf` — performance work measured by a benchmark.
- `ci` — CI / workflow changes.

Scopes are the package or area: `ledger`, `bom`, `auth`, `cli`, `serve`,
`docs`, `ci`. Look at recent commits for examples.

## Pull requests

- Branch off `main`. Branch name: `<type>/<short-slug>`.
- Open a PR against `main`. Default reviewer: the maintainer.
- The PR description should answer: *what changed*, *why*, *what you
  tested*, *what could go wrong*. Reference the plan file if one
  exists.
- CI must be green. Coverage gate must pass. `govulncheck` finding
  resolution is optional on PR / required on tag.
- Squash on merge is the default. Use a clean conventional-commit
  subject as the merge commit message.

## What lives where

| Path | Purpose |
|---|---|
| [`cmd/themis/`](cmd/themis/) | CLI entry point. Thin — real logic in `internal/cli`. |
| [`internal/cli/`](internal/cli/) | Cobra command tree. One file per subcommand. |
| [`internal/`](internal/) | Trust-critical packages: `ledger`, `bom`, `auth`, `policy`, `tenant`, `catalogue`, `classify`, `pipeline`, `sign`, `scan`, `mempalace`, `heartbeat`, `advisor`, `ingest`, `api`, `mcp`, `approvals`, `incidents`, `override`. Each has its own test files. |
| [`actions/themis-check/`](actions/themis-check/) | GitHub Action wrapper for PR-time enforcement. |
| [`docs/onboarding/`](docs/onboarding/) | Operator-facing tutorial + cookbook + exercises. |
| [`docs/design.md`](docs/design.md) | Canonical design specification. |
| [`docs/plans/`](docs/plans/) | Implementation plans (one per executed change set). |
| [`docs/stakeholders/`](docs/stakeholders/) | Stakeholder briefs (compliance, engineering, security, exec, pilot proposal). |
| [`docs/ops/`](docs/ops/) | Operations docs (deployment, observability, backup-restore, runbook). |

## Style

- Idiomatic Go. Run `gofumpt` + `goimports`. `golangci-lint` enforces.
- Errors wrap with `%w` and carry actionable context.
- Public types and exported funcs carry doc comments.
- Tests are table-driven where it reads better.
- No new dependency without a written justification in the plan or PR
  description.

## Sensitive surfaces

Take extra care when touching:

- `internal/ledger/` — append-only Merkle-chained ledger. Any change
  that mutates a previously-written block is wrong by construction.
- `internal/sign/` — signer plumbing. Read [`SECURITY.md`](SECURITY.md)
  before touching.
- `internal/auth/` — token validation and chain semantics. A weakening
  here is a CVE.
- `internal/policy/` — pure-function decision engine. Determinism
  matters; flakiness here breaks replayability.

Add a security-relevant change to your PR description with the heading
`Security impact`, even if the impact is "none".

## Security reports

Do not file security findings in public Issues. See
[`SECURITY.md`](SECURITY.md).

## Releases

Tagging `vX.Y.Z` on `main` triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml), which
runs `goreleaser`, signs artefacts with cosign keyless, attaches SBOMs,
and publishes to `ghcr.io`. Only the maintainer cuts releases. See
[`docs/ops/deployment.md`](docs/ops/deployment.md) for the verification
steps operators run after a release.

## Questions

Use [`SUPPORT.md`](SUPPORT.md) to find the right channel.
