package sign

import (
	"errors"
	"testing"
)

func TestSignVerify_HappyPath(t *testing.T) {
	priv, pub, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello themis")
	sig, err := Sign(payload, priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(payload, sig, pub); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
}

func TestVerify_TamperedPayloadFails(t *testing.T) {
	priv, pub, _ := GenerateKey()
	payload := []byte("the original payload")
	sig, _ := Sign(payload, priv)
	tampered := []byte("the original payload!")
	if err := Verify(tampered, sig, pub); !errors.Is(err, ErrVerify) {
		t.Fatalf("tampered payload should ErrVerify, got %v", err)
	}
}

func TestVerify_WrongKeyFails(t *testing.T) {
	priv1, _, _ := GenerateKey()
	_, pub2, _ := GenerateKey()
	payload := []byte("p")
	sig, _ := Sign(payload, priv1)
	if err := Verify(payload, sig, pub2); !errors.Is(err, ErrVerify) {
		t.Fatalf("wrong key should ErrVerify, got %v", err)
	}
}

func TestSign_RejectsBadPrivateKey(t *testing.T) {
	_, err := Sign([]byte("p"), []byte("too short"))
	if !errors.Is(err, ErrSign) {
		t.Fatalf("short priv should ErrSign, got %v", err)
	}
}

func TestVerify_RejectsBadPublicKey(t *testing.T) {
	priv, _, _ := GenerateKey()
	sig, _ := Sign([]byte("p"), priv)
	err := Verify([]byte("p"), sig, []byte("short pub"))
	if !errors.Is(err, ErrSign) {
		t.Fatalf("short pub should ErrSign, got %v", err)
	}
}

func TestVerify_RejectsBadSignatureLength(t *testing.T) {
	_, pub, _ := GenerateKey()
	err := Verify([]byte("p"), []byte("short sig"), pub)
	if !errors.Is(err, ErrVerify) {
		t.Fatalf("short sig should ErrVerify, got %v", err)
	}
}
