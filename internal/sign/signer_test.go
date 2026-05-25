package sign

import (
	"bytes"
	"errors"
	"testing"
)

func TestLocalSigner_SignVerifyRoundTrip(t *testing.T) {
	s, err := NewLocalSignerFromDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := s.Sign([]byte("hello themis"))
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Mode != ModeLocalEd25519 {
		t.Fatalf("mode = %q", bundle.Mode)
	}
	if err := s.Verify([]byte("hello themis"), bundle); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestLocalSigner_VerifyRejectsTamperedPayload(t *testing.T) {
	s, _ := NewLocalSignerFromDir(t.TempDir())
	bundle, _ := s.Sign([]byte("payload"))
	if err := s.Verify([]byte("paylaod"), bundle); !errors.Is(err, ErrVerify) {
		t.Fatalf("tampered payload should ErrVerify, got %v", err)
	}
}

func TestLocalSigner_VerifyRejectsWrongMode(t *testing.T) {
	s, _ := NewLocalSignerFromDir(t.TempDir())
	bundle, _ := s.Sign([]byte("p"))
	bundle.Mode = ModeCosignKeylessStub
	if err := s.Verify([]byte("p"), bundle); !errors.Is(err, ErrVerify) {
		t.Fatalf("wrong-mode bundle should ErrVerify, got %v", err)
	}
}

func TestCosignKeylessStub_SignVerifyRoundTrip(t *testing.T) {
	s := NewCosignKeylessStub("alice@example.com", "https://oidc.example.com")
	bundle, err := s.Sign([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Mode != ModeCosignKeylessStub {
		t.Fatalf("mode = %q", bundle.Mode)
	}
	if bundle.RekorURL == "" {
		t.Fatal("expected stub Rekor URL")
	}
	if len(bundle.Certificate) == 0 {
		t.Fatal("expected stub certificate")
	}
	if err := s.Verify([]byte("hello"), bundle); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestCosignKeylessStub_VerifyRejectsTamperedPayload(t *testing.T) {
	s := NewCosignKeylessStub("alice", "issuer")
	bundle, _ := s.Sign([]byte("orig"))
	if err := s.Verify([]byte("evil"), bundle); !errors.Is(err, ErrVerify) {
		t.Fatalf("expected ErrVerify, got %v", err)
	}
}

func TestCosignKeylessStub_VerifyRejectsSubjectMismatch(t *testing.T) {
	signer := NewCosignKeylessStub("alice", "issuer")
	bundle, _ := signer.Sign([]byte("p"))
	verifier := NewCosignKeylessStub("bob", "issuer")
	if err := verifier.Verify([]byte("p"), bundle); !errors.Is(err, ErrVerify) {
		t.Fatalf("subject-mismatch should ErrVerify, got %v", err)
	}
}

func TestCosignKeylessStub_RejectsEmptySubjectOnSign(t *testing.T) {
	s := NewCosignKeylessStub("", "issuer")
	if _, err := s.Sign([]byte("x")); !errors.Is(err, ErrSign) {
		t.Fatalf("empty-subject sign should ErrSign, got %v", err)
	}
}

func TestCosignKeylessStub_VerifyRejectsMissingCert(t *testing.T) {
	s := NewCosignKeylessStub("alice", "issuer")
	bundle, _ := s.Sign([]byte("p"))
	bundle.Certificate = nil
	if err := s.Verify([]byte("p"), bundle); !errors.Is(err, ErrVerify) {
		t.Fatalf("missing cert should ErrVerify, got %v", err)
	}
}

func TestCosignKeylessStub_VerifyRejectsCorruptCert(t *testing.T) {
	s := NewCosignKeylessStub("alice", "issuer")
	bundle, _ := s.Sign([]byte("p"))
	bundle.Certificate = []byte("not json")
	if err := s.Verify([]byte("p"), bundle); !errors.Is(err, ErrVerify) {
		t.Fatalf("corrupt cert should ErrVerify, got %v", err)
	}
}

func TestResolve_LocalEd25519Default(t *testing.T) {
	s, err := Resolve(ModeLocalEd25519, ResolveOptions{LocalKeyDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode() != ModeLocalEd25519 {
		t.Fatalf("mode = %q", s.Mode())
	}
}

func TestResolve_LocalRequiresKeyDir(t *testing.T) {
	if _, err := Resolve(ModeLocalEd25519, ResolveOptions{}); !errors.Is(err, ErrSign) {
		t.Fatalf("expected ErrSign for missing LocalKeyDir, got %v", err)
	}
}

func TestResolve_CosignStub(t *testing.T) {
	s, err := Resolve(ModeCosignKeylessStub, ResolveOptions{OIDCSubject: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode() != ModeCosignKeylessStub {
		t.Fatalf("mode = %q", s.Mode())
	}
}

func TestResolve_UnknownReturnsError(t *testing.T) {
	if _, err := Resolve("phantom-mode", ResolveOptions{}); !errors.Is(err, ErrUnknownMode) {
		t.Fatalf("expected ErrUnknownMode, got %v", err)
	}
}

func TestResolve_EmptyModeDefaultsToLocal(t *testing.T) {
	s, err := Resolve("", ResolveOptions{LocalKeyDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode() != ModeLocalEd25519 {
		t.Fatalf("empty mode should default to local, got %q", s.Mode())
	}
}

func TestSignedBundle_SignatureBytesNonEmpty(t *testing.T) {
	s, _ := NewLocalSignerFromDir(t.TempDir())
	bundle, _ := s.Sign([]byte("p"))
	if bytes.Equal(bundle.Signature, make([]byte, len(bundle.Signature))) {
		t.Fatal("signature is all-zero")
	}
}
