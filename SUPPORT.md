# Support

How to get help with Themis, and what's in scope at the current stage of
the project (v0.1.x).

## Where to ask

| Topic | Channel |
|---|---|
| Bug report | [GitHub Issues](https://github.com/tzone85/themis/issues) — use the "Bug" type. |
| Security report | **Not GitHub Issues.** See [`SECURITY.md`](SECURITY.md). |
| Design / architecture question | [GitHub Discussions](https://github.com/tzone85/themis/discussions) — "Q&A" category. |
| Policy-authoring question | GitHub Discussions — "Policy" category. |
| Feature request | GitHub Discussions — "Ideas" category. We triage these monthly; we do not commit to roadmap items pre-v1.0. |
| Compliance / audit conversation | Email `thando.mini@sanlam.co.za` directly — these conversations are off the public tracker. |

## Response expectations

Themis is currently maintained by one person. Best-effort response
windows:

- **Security report:** acknowledgement within 3 working days. See
  [`SECURITY.md`](SECURITY.md).
- **Bug report:** triage within 5 working days. Fix timeline depends on
  severity.
- **Design / policy question:** answered when seen; usually within a
  week.
- **Feature request:** read and tagged; no acknowledgement of intent to
  build.

If you need a paid support SLA, that lives on the closed-source
commercial side (per the design spec §16); contact via email.

## In scope at v0.1.x

- Reproducible bug reports against `v0.1.x` on Linux, macOS, or
  Windows.
- Security reports per [`SECURITY.md`](SECURITY.md).
- Questions about the canonical design (`docs/design.md`).
- Questions about onboarding (`docs/onboarding/README.md`) and the
  cookbook (`docs/onboarding/cookbook/README.md`).
- Questions about the policy YAML schema (`internal/policy/`).
- Questions about the AI-BOM schema (`internal/bom/`).

## Out of scope at v0.1.x

- Custom policy authoring as a service (commercial offering).
- Help integrating Themis with closed-source enterprise systems we
  haven't documented.
- General LLM-tooling support (Themis is not an LLM framework).
- Help running Themis on unsupported architectures (anything outside
  the matrix in `.goreleaser.yaml`).

## Before opening an issue

1. Run `themis --version` and include the full output.
2. Re-run the failing command with `-v` if it accepts a verbosity flag,
   or set `THEMIS_LOG=debug` for the server.
3. Search existing issues + discussions; many surface twice.
4. For policy questions, include the minimal YAML that reproduces.

## Self-serve resources

- **Quickstart:** [`README.md`](README.md) "Quickstart" section.
- **Onboarding tutorial:** [`docs/onboarding/README.md`](docs/onboarding/README.md).
- **Cookbook:** [`docs/onboarding/cookbook/README.md`](docs/onboarding/cookbook/README.md).
- **Exercises:** [`docs/onboarding/exercises/README.md`](docs/onboarding/exercises/README.md).
- **Runbook (operators):** [`docs/ops/runbook.md`](docs/ops/runbook.md).
- **Design spec:** [`docs/design.md`](docs/design.md).
- **Changelog:** [`CHANGELOG.md`](CHANGELOG.md).
