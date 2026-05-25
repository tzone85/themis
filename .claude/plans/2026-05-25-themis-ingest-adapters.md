# Plan 5 — Ingest Adapters

**Date:** 2026-05-25
**Depends on:** Plans 1-4.
**Scope:** Turn real-world inputs (git diffs, Claude Code session exports, manual
operator attestations) into normalised `AIChange` JSON files that the existing
`themis decide / bom build / bom sign` pipeline consumes. Without adapters the
pipeline is a stub; with them, it processes real PRs.

## Architectural alignment (design spec §6.1)

> `ingest` — Adapter interface + concrete adapters. Normalises to `AIChange`
> events. Adapters: `vxd`, `claude_code_transcript`, `cursor_mcp`,
> `copilot_audit`, `git_heuristic`, `manual_attestation`, plus `null` for tests.

Plan 5 ships three adapters — the minimum that makes the pipeline real:

- **`git_heuristic`** — read a git diff (file paths + before/after hashes) into AIChange. Universal.
- **`claude_code_transcript`** — parses a Claude Code session JSON export into AIChange (PRID, actor, files modified, transcript hash).
- **`manual_attestation`** — operator declares the change shape via flags (for emergency overrides + retrofitting historic PRs).

Defers `vxd`, `cursor_mcp`, `copilot_audit` to a later plan.

## New ledger kinds

`INGEST_COMPLETED` and `INGEST_ADAPTER_FAILED` (design spec Appendix B).

## Tasks

### T1: `Adapter` interface

`internal/ingest/ingest.go`:
- `Adapter { Name() string; Ingest(inputs Inputs) (aichange.AIChange, error) }`
- `Inputs { PRID, ActorOverride string; Files map[string][2]string /*path → [beforeHash,afterHash]*/; Extra map[string]string }`
- `ErrAdapterFailed` sentinel for INGEST_ADAPTER_FAILED routing.

### T2: `git_heuristic` adapter

`internal/ingest/git_heuristic.go`:
- Takes a `workdir` + base ref. Shells `git diff --name-status --no-renames <base>..HEAD`.
- For each `A|M|D path` parses the change kind, computes before/after SHA-256 of file content from `git show`.
- Maps actor as `human:<git-author>` of the latest commit.

Tests use a real git workspace created in a t.TempDir + `git init`.

### T3: `claude_code_transcript` adapter

`internal/ingest/claude_code.go`:
- Input: path to a transcript JSON containing `{session_id, model, user, edits: [{path, before_hash, after_hash}]}`.
- Maps actor=`claude_code`, RawTranscriptHash=sha256 of transcript file.

Tests use synthetic transcript fixtures.

### T4: `manual_attestation` adapter

`internal/ingest/manual.go`:
- Input: `Inputs.Files` map directly. Actor=`Inputs.ActorOverride` (required, must start with `human:`).
- Used for retroactive PRs + emergency overrides where no machine record exists.

### T5: Register `INGEST_COMPLETED` + `INGEST_ADAPTER_FAILED`

Ledger registry + wiring test.

### T6: `themis ingest` CLI

`internal/cli/ingest_cmd.go`:
- `themis ingest --id <t> --base <state> --adapter <name> --pr-id <id> [--workdir <p>] [--base-ref <ref>] [--transcript <p>] [--actor <a>] [--out <p>]`
- Picks adapter by name, runs it, writes AIChange JSON, emits `INGEST_COMPLETED` (or `INGEST_ADAPTER_FAILED`).

### T7: End-to-end with git_heuristic

Extended e2e: ingest from a real git workdir → decide → bom build → sign. Proves the full real-world pipeline.

### T8: README Plan 5 changelog

### T9: `make ci` pass
