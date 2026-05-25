# Plan 14 — Pluggable Signer + Sigstore-keyless stub

**Date:** 2026-05-25
**Depends on:** Plans 1-13.
**Scope:** Extract a `Signer` interface from the existing `internal/sign`
package so the BOM signing path can switch between local ed25519 (Plan 4)
and Sigstore keyless (which needs OIDC + Fulcio/Rekor) without changing
the caller. Ship the interface, refactor `internal/sign` to implement it,
and add a `CosignKeylessStub` that records what *would* happen against
Sigstore without actually reaching the network. The real Fulcio/Rekor
adapter lands when an operator is ready to ship Sigstore credentials.

## Tasks

### T1: `Signer` interface in `internal/sign/signer.go`

- `Sign(payload []byte) (SignedBundle, error)`
- `Verify(payload []byte, bundle SignedBundle) error`
- `SignedBundle { Mode string; Signature []byte; PublicKey []byte; Certificate []byte; Rekor entry? }`

### T2: `LocalSigner` (existing ed25519 path) implements Signer

### T3: `CosignKeylessStub` implements Signer offline

- Records "would have called Fulcio for cert, Rekor for transparency log entry" inside the SignedBundle.
- Real adapter (Plan 14b) will replace this without changing callers.

### T4: BOM CLI accepts `--signer local|cosign-keyless-stub`

### T5: Tests

### T6: README Plan 14 changelog

### T7: `make ci` pass
