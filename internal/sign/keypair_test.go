package sign

import (
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateKey_LengthsCorrect(t *testing.T) {
	priv, pub, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(priv) != ed25519.PrivateKeySize {
		t.Errorf("priv len = %d", len(priv))
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Errorf("pub len = %d", len(pub))
	}
}

func TestLoadOrGenerate_CreatesOnFirstCall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	priv, pub, err := LoadOrGenerate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(priv) != ed25519.PrivateKeySize || len(pub) != ed25519.PublicKeySize {
		t.Fatal("key lengths wrong")
	}
	// Permissions on private key.
	info, err := os.Stat(filepath.Join(dir, "ed25519.priv"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("priv permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestLoadOrGenerate_ReadsSecondCall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	priv1, pub1, err := LoadOrGenerate(dir)
	if err != nil {
		t.Fatal(err)
	}
	priv2, pub2, err := LoadOrGenerate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(priv1) != string(priv2) || string(pub1) != string(pub2) {
		t.Fatal("second LoadOrGenerate must return the same keypair")
	}
}

func TestLoadOrGenerate_HalfPresentErrors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Write only the public key — simulate a partial restore.
	if err := os.WriteFile(filepath.Join(dir, "ed25519.pub"), make([]byte, ed25519.PublicKeySize), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrGenerate(dir); !errors.Is(err, ErrSign) {
		t.Fatalf("half-present keys should error with ErrSign wrap, got: %v", err)
	}
}

func TestLoadOrGenerate_WrongLengthErrors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ed25519.priv"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ed25519.pub"), []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrGenerate(dir); err == nil {
		t.Fatal("wrong-length keys should error")
	}
}

func TestPublicKeyHex(t *testing.T) {
	_, pub, _ := GenerateKey()
	h := PublicKeyHex(pub)
	if len(h) != ed25519.PublicKeySize*2 {
		t.Fatalf("hex length = %d, want %d", len(h), ed25519.PublicKeySize*2)
	}
}
