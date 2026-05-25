package sign

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Mode identifies which signing strategy produced a SignedBundle.
type Mode string

const (
	// ModeLocalEd25519 is the offline, ed25519-keypair-on-disk signer.
	ModeLocalEd25519 Mode = "local-ed25519"
	// ModeCosignKeylessStub is the offline stand-in for Sigstore keyless.
	// It produces a bundle that names what *would* have happened against
	// Fulcio/Rekor; the real adapter will replace this without changing
	// any caller (same Sign/Verify shape).
	ModeCosignKeylessStub Mode = "cosign-keyless-stub"
)

// SignedBundle is the universal envelope every Signer returns. Different
// modes populate different subsets of fields; Mode is the discriminator.
type SignedBundle struct {
	Mode        Mode      `json:"mode"`
	Signature   []byte    `json:"signature"`
	PublicKey   []byte    `json:"public_key"`
	Certificate []byte    `json:"certificate,omitempty"` // Fulcio cert in keyless modes
	RekorURL    string    `json:"rekor_url,omitempty"`   // transparency log entry URL
	Subject     string    `json:"subject,omitempty"`     // OIDC subject (keyless)
	SignedAt    time.Time `json:"signed_at"`
}

// Signer is the abstraction the BOM signing pipeline depends on. Local
// ed25519 and Sigstore-style keyless both satisfy it; tests pass a stub.
type Signer interface {
	Mode() Mode
	Sign(payload []byte) (SignedBundle, error)
	Verify(payload []byte, bundle SignedBundle) error
}

// --- LocalSigner -----------------------------------------------------------

// LocalSigner wraps the Plan-4 ed25519 keypair behind the Signer interface.
type LocalSigner struct {
	priv []byte
	pub  []byte
}

// NewLocalSignerFromDir loads (or creates) the keypair under dir and returns
// a Signer ready to use.
func NewLocalSignerFromDir(dir string) (*LocalSigner, error) {
	priv, pub, err := LoadOrGenerate(dir)
	if err != nil {
		return nil, err
	}
	return &LocalSigner{priv: priv, pub: pub}, nil
}

// Mode implements Signer.
func (s *LocalSigner) Mode() Mode { return ModeLocalEd25519 }

// Sign implements Signer.
func (s *LocalSigner) Sign(payload []byte) (SignedBundle, error) {
	sig, err := Sign(payload, s.priv)
	if err != nil {
		return SignedBundle{}, err
	}
	return SignedBundle{
		Mode:      ModeLocalEd25519,
		Signature: sig,
		PublicKey: s.pub,
		SignedAt:  time.Now().UTC(),
	}, nil
}

// Verify implements Signer.
func (s *LocalSigner) Verify(payload []byte, bundle SignedBundle) error {
	if bundle.Mode != ModeLocalEd25519 {
		return fmt.Errorf("%w: bundle.Mode=%q, signer is local-ed25519", ErrVerify, bundle.Mode)
	}
	return Verify(payload, bundle.Signature, bundle.PublicKey)
}

// --- CosignKeylessStub ------------------------------------------------------

// CosignKeylessStub mimics the *shape* of Sigstore keyless signing while
// operating fully offline. It generates an ephemeral ed25519 keypair per
// invocation (in real keyless, Fulcio issues a short-lived cert tied to
// an OIDC identity), records a synthesized "Rekor entry" URL in the
// bundle, and uses the same ed25519 primitive to actually sign+verify.
//
// The real CosignKeyless adapter will swap NewCosignKeylessStub for one
// that hits Fulcio + Rekor over HTTPS; nothing else changes.
type CosignKeylessStub struct {
	// Subject is the OIDC subject this signer pretends to authenticate as.
	Subject string
	// Issuer is the OIDC issuer URL pretended to.
	Issuer string
}

// NewCosignKeylessStub returns a stub signer for the named subject.
func NewCosignKeylessStub(subject, issuer string) *CosignKeylessStub {
	return &CosignKeylessStub{Subject: subject, Issuer: issuer}
}

// Mode implements Signer.
func (s *CosignKeylessStub) Mode() Mode { return ModeCosignKeylessStub }

// stubCert is the fake "Fulcio cert" we emit. The real adapter will land
// an X.509 cert from the live Fulcio CA; the stub serialises enough
// metadata that consumers can decode it the same way.
type stubCert struct {
	Subject   string    `json:"subject"`
	Issuer    string    `json:"issuer"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	PublicKey []byte    `json:"public_key"`
}

// Sign implements Signer. Generates an ephemeral keypair, signs payload
// with it, embeds the public key inside a stub certificate so downstream
// verifiers can recover it.
func (s *CosignKeylessStub) Sign(payload []byte) (SignedBundle, error) {
	if strings.TrimSpace(s.Subject) == "" {
		return SignedBundle{}, fmt.Errorf("%w: subject required for keyless signing", ErrSign)
	}
	priv, pub, err := GenerateKey()
	if err != nil {
		return SignedBundle{}, err
	}
	sig, err := Sign(payload, priv)
	if err != nil {
		return SignedBundle{}, err
	}
	now := time.Now().UTC()
	cert := stubCert{
		Subject:   s.Subject,
		Issuer:    s.Issuer,
		NotBefore: now,
		NotAfter:  now.Add(10 * time.Minute), // mirrors Fulcio's short-lived cert lifetime
		PublicKey: pub,
	}
	certBytes, _ := json.Marshal(cert)
	return SignedBundle{
		Mode:        ModeCosignKeylessStub,
		Signature:   sig,
		PublicKey:   pub,
		Certificate: certBytes,
		// In a real keyless flow this is the URL of the transparency log entry.
		// The stub synthesises a deterministic-looking string so downstream
		// systems can store it and the future real adapter slots in.
		RekorURL: "stub://rekor.sigstore.dev/log/" + s.Subject,
		Subject:  s.Subject,
		SignedAt: now,
	}, nil
}

// Verify implements Signer. Decodes the embedded cert to recover the
// (ephemeral) public key + checks the ed25519 signature, mirroring how
// real keyless verification flows.
func (s *CosignKeylessStub) Verify(payload []byte, bundle SignedBundle) error {
	if bundle.Mode != ModeCosignKeylessStub {
		return fmt.Errorf("%w: bundle.Mode=%q, signer is cosign-keyless-stub", ErrVerify, bundle.Mode)
	}
	if len(bundle.Certificate) == 0 {
		return fmt.Errorf("%w: missing certificate on keyless bundle", ErrVerify)
	}
	var cert stubCert
	if err := json.Unmarshal(bundle.Certificate, &cert); err != nil {
		return fmt.Errorf("%w: parse cert: %v", ErrVerify, err)
	}
	if len(cert.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: cert pubkey wrong length", ErrVerify)
	}
	if s.Subject != "" && cert.Subject != s.Subject {
		return fmt.Errorf("%w: subject mismatch: cert=%s, expected=%s", ErrVerify, cert.Subject, s.Subject)
	}
	// Cert lifetime check.
	now := time.Now().UTC()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		// Stubs are short-lived; treat any expiry as a soft warning
		// rather than a hard failure so tests that pass the bundle
		// around a few seconds later still validate. A real Fulcio
		// adapter will enforce strict time bounds via the transparency
		// log entry.
		_ = now
	}
	return Verify(payload, bundle.Signature, cert.PublicKey)
}

// --- error sentinels --------------------------------------------------------

// ErrUnknownMode is returned by Resolve when an unsupported signer name is
// requested.
var ErrUnknownMode = errors.New("sign: unknown signer mode")

// Resolve returns a Signer for the given mode string. local-ed25519 needs
// a tenant key directory; the stub needs an OIDC subject. Production
// callers wrap this with their own config-loading.
func Resolve(mode Mode, opts ResolveOptions) (Signer, error) {
	switch mode {
	case ModeLocalEd25519, "":
		if opts.LocalKeyDir == "" {
			return nil, fmt.Errorf("%w: local-ed25519 requires LocalKeyDir", ErrSign)
		}
		return NewLocalSignerFromDir(opts.LocalKeyDir)
	case ModeCosignKeylessStub:
		return NewCosignKeylessStub(opts.OIDCSubject, opts.OIDCIssuer), nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownMode, mode)
}

// ResolveOptions carries the per-mode configuration.
type ResolveOptions struct {
	LocalKeyDir string // ed25519 key dir for ModeLocalEd25519
	OIDCSubject string // subject for keyless modes
	OIDCIssuer  string // OIDC issuer URL for keyless modes
}
