# Plan 4 — AI-BOM Build + Signing

**Date:** 2026-05-23
**Depends on:** Plans 1, 2, 3.
**Scope:** Build the canonical `themis.bom.v1` JSON-LD document from a Decision
+ Impact + Findings. Sign it with an ed25519 keypair (local fallback; Sigstore
keyless is the GA path — the local path proves the model). Add `themis bom build`
and `themis bom sign` commands. Two new ledger kinds: `BOM_BUILT`, `BOM_SIGNED`.

## Out of scope

- Sigstore keyless (defers cryptographic infrastructure to Plan 5).
- Approval flows (`APPROVAL_GRANTED`/`APPROVAL_DENIED`) — Plan 5.
- BOM-UNSIGNED held-decision state — Plan 5.

## Tasks

### T1: `BOM` value type + canonical JSON

`internal/bom/bom.go`:
- `BOM { SchemaVersion string; PRID string; Tenant string; Actor string; BuiltAt time.Time; Impact classify.Impact; Findings []scan.Finding; Decision policy.Decision; AIChange aichange.AIChange; LedgerTip string }`
- `Canonical(b BOM) []byte` returns the deterministic JSON byte form used for hashing/signing.

### T2: BOM tests

`internal/bom/bom_test.go`:
- Canonical is byte-stable for the same logical input.
- Canonical changes when any field changes.
- JSON round-trip preserves all fields.

### T3: ed25519 key pair management

`internal/sign/keypair.go`:
- `GenerateKey() (privateKey, publicKey []byte, err error)`.
- `LoadOrGenerate(path string) (privKey, pubKey []byte, err error)` — writes per-tenant key on first call, reads thereafter.
- Permissions: 0o600 on private key, 0o644 on public.

### T4: Sign + verify

`internal/sign/sign.go`:
- `Sign(payload, privKey []byte) []byte` returns the raw 64-byte ed25519 signature.
- `Verify(payload, signature, pubKey []byte) error`.
- Tests including: sign-then-verify, tampered payload fails, wrong key fails.

### T5: `themis bom build` CLI

`internal/cli/bom_cmd.go`:
- Reads the most recent `DECISION_ISSUED` event from the ledger (or `--decision-event-seq`).
- Walks back to collect the `IMPACT_CLASSIFIED`, all `SCAN_FINDING`s, and the AIChange referenced.
- Builds a `BOM`, prints canonical JSON, emits `BOM_BUILT` event.

### T6: `themis bom sign` CLI

- Loads/generates the tenant's ed25519 keypair under `tenants/<id>/keys/`.
- Re-builds the BOM (deterministic) from ledger state.
- Signs canonical BOM bytes.
- Emits `BOM_SIGNED` with payload `{bom_hash, signature, public_key_hex}`.
- Writes the signed BOM file to `tenants/<id>/bom/<bom_hash>.bom.json` with sidecar `.sig`.

### T7: Register `BOM_BUILT` + `BOM_SIGNED`

Ledger registry + wiring test.

### T8: End-to-end smoke

Test that runs the full flow: tenant init → catalogue sync → decide → bom build → bom sign → verify signature. Ensures all pieces wire together.

### T9: README Plan 4 changelog

### T10: `make ci` pass
