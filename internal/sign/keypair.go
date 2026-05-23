// Package sign holds the cryptographic signing primitives Themis uses to
// attest AI Bills of Materials. Plan 4 ships local ed25519 only — Plan 5
// will add Sigstore keyless as the default path with local keys remaining
// as the air-gapped fallback (design spec §6.1).
package sign

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrSign is wrapped by every non-verification error returned by this
// package so callers can route any crypto failure to a BOM_UNSIGNED
// (Plan 5) ledger event.
var ErrSign = errors.New("sign: failure")

// GenerateKey produces a new ed25519 keypair using crypto/rand.
func GenerateKey() (privateKey, publicKey []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: ed25519.GenerateKey: %v", ErrSign, err)
	}
	return []byte(priv), []byte(pub), nil
}

// LoadOrGenerate reads the keypair under dir/, creating it on first call.
// Files: dir/ed25519.priv (0600), dir/ed25519.pub (0644).
//
// dir is created with 0o700 if missing — the keypair MUST be tenant-private.
func LoadOrGenerate(dir string) (privateKey, publicKey []byte, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("%w: mkdir %s: %v", ErrSign, dir, err)
	}
	privPath := filepath.Join(dir, "ed25519.priv")
	pubPath := filepath.Join(dir, "ed25519.pub")

	priv, perr := os.ReadFile(privPath) // #nosec G304 -- key dir tenant-scoped.
	pub, uerr := os.ReadFile(pubPath)   // #nosec G304
	if perr == nil && uerr == nil {
		if len(priv) == ed25519.PrivateKeySize && len(pub) == ed25519.PublicKeySize {
			return priv, pub, nil
		}
		return nil, nil, fmt.Errorf("%w: existing keys at %s have wrong length", ErrSign, dir)
	}
	if (perr == nil) != (uerr == nil) {
		return nil, nil, fmt.Errorf("%w: keypair half-present at %s (one of priv/pub missing)", ErrSign, dir)
	}

	// Both missing → generate.
	priv, pub, err = GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(privPath, priv, 0o600); err != nil {
		return nil, nil, fmt.Errorf("%w: write %s: %v", ErrSign, privPath, err)
	}
	if err := os.WriteFile(pubPath, pub, 0o644); err != nil { // #nosec G306 -- public key intentionally world-readable.
		return nil, nil, fmt.Errorf("%w: write %s: %v", ErrSign, pubPath, err)
	}
	return priv, pub, nil
}

// PublicKeyHex returns the hex encoding of pub — convenient for printing
// in `BOM_SIGNED` payloads.
func PublicKeyHex(pub []byte) string {
	return hex.EncodeToString(pub)
}
