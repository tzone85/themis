package sign

import (
	"crypto/ed25519"
	"errors"
	"fmt"
)

// ErrVerify is returned by Verify when a signature does not match.
// Wrapped via fmt.Errorf so callers can errors.Is to distinguish a verify
// failure from an I/O or input-shape failure.
var ErrVerify = errors.New("sign: verify failed")

// Sign returns the 64-byte ed25519 signature over payload.
func Sign(payload, privateKey []byte) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: private key length %d, want %d", ErrSign, len(privateKey), ed25519.PrivateKeySize)
	}
	sig := ed25519.Sign(ed25519.PrivateKey(privateKey), payload)
	return sig, nil
}

// Verify returns nil if signature is valid for payload under publicKey,
// ErrVerify otherwise.
func Verify(payload, signature, publicKey []byte) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: public key length %d, want %d", ErrSign, len(publicKey), ed25519.PublicKeySize)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature length %d, want %d", ErrVerify, len(signature), ed25519.SignatureSize)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return ErrVerify
	}
	return nil
}
