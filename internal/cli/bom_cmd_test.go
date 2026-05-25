package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/sign"
)

// runDecideOnce drives the decide CLI against the synced fixture so the
// subsequent BOM tests have a real DECISION_ISSUED to build from.
func runDecideOnce(t *testing.T, prID string) (base, id string) {
	t.Helper()
	base, id = setupTenantWithSyncedCatalogue(t)
	pol := writePolicy(t, t.TempDir(), validPolicyYAML)
	change := aichange.AIChange{
		PRID:  prID,
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	cp := writeAIChange(t, t.TempDir(), change)

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"decide", "--id", id, "--base", base, "--aichange", cp, "--policy", pol})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("decide setup: %v", err)
	}
	return
}

func TestBOMBuild_ReproducesCanonicalAndEmitsBuilt(t *testing.T) {
	base, id := runDecideOnce(t, "gh:test#bom-build")

	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"bom", "build", "--id", id, "--base", base, "--pr-id", "gh:test#bom-build"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bom build: %v", err)
	}

	var probe map[string]any
	if err := json.Unmarshal(out.Bytes(), &probe); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if probe["schema_version"] != "themis.bom.v1" {
		t.Fatalf("schema_version = %v", probe["schema_version"])
	}
	if probe["pr_id"] != "gh:test#bom-build" {
		t.Fatalf("pr_id = %v", probe["pr_id"])
	}

	// Ledger now contains BOM_BUILT.
	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if events[len(events)-1].Kind != "BOM_BUILT" {
		t.Fatalf("last event = %q, want BOM_BUILT", events[len(events)-1].Kind)
	}
}

func TestBOMBuild_RejectsUnknownPRID(t *testing.T) {
	base, id := runDecideOnce(t, "gh:test#a")
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"bom", "build", "--id", id, "--base", base, "--pr-id", "gh:test#nonexistent"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("bom build should fail when --pr-id has no DECISION_ISSUED")
	}
}

func TestBOMSign_SignsAndVerifies(t *testing.T) {
	base, id := runDecideOnce(t, "gh:test#bom-sign")
	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"bom", "sign", "--id", id, "--base", base, "--pr-id", "gh:test#bom-sign"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bom sign: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}

	bomPath := got["bom_path"]
	if !strings.HasSuffix(bomPath, ".bom.json") {
		t.Fatalf("bom_path %q does not end with .bom.json", bomPath)
	}

	canon, err := os.ReadFile(bomPath)
	if err != nil {
		t.Fatal(err)
	}
	sigHex, err := os.ReadFile(got["signature_path"])
	if err != nil {
		t.Fatal(err)
	}
	signature, err := hex.DecodeString(string(sigHex))
	if err != nil {
		t.Fatal(err)
	}
	pubKey, err := hex.DecodeString(got["public_key_hex"])
	if err != nil {
		t.Fatal(err)
	}
	if err := sign.Verify(canon, signature, pubKey); err != nil {
		t.Fatalf("Verify of stored signature failed: %v", err)
	}

	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if events[len(events)-1].Kind != "BOM_SIGNED" {
		t.Fatalf("last event = %q, want BOM_SIGNED", events[len(events)-1].Kind)
	}
}

func TestBOMSign_CosignKeylessStub_RoundTrip(t *testing.T) {
	base, id := runDecideOnce(t, "gh:test#bom-cosign")
	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{
		"bom", "sign",
		"--id", id, "--base", base,
		"--pr-id", "gh:test#bom-cosign",
		"--signer", "cosign-keyless-stub",
		"--oidc-subject", "alice@example.com",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bom sign --signer cosign-keyless-stub: %v\n%s", err, out.String())
	}

	var got map[string]string
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if got["signer_mode"] != "cosign-keyless-stub" {
		t.Fatalf("signer_mode = %q", got["signer_mode"])
	}
	if got["rekor_url"] == "" {
		t.Fatal("rekor_url should be populated for keyless mode")
	}

	// Load the bundle file and verify with a freshly-constructed stub signer.
	bundleRaw, err := os.ReadFile(got["bundle_path"])
	if err != nil {
		t.Fatal(err)
	}
	var bundle sign.SignedBundle
	if err := json.Unmarshal(bundleRaw, &bundle); err != nil {
		t.Fatal(err)
	}

	canon, _ := os.ReadFile(got["bom_path"])
	verifier := sign.NewCosignKeylessStub("alice@example.com", "https://oidc.example.com")
	if err := verifier.Verify(canon, bundle); err != nil {
		t.Fatalf("verify cosign-keyless-stub bundle: %v", err)
	}
}

func TestBOMSign_TamperedBOMFailsVerify(t *testing.T) {
	base, id := runDecideOnce(t, "gh:test#bom-tamper")
	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"bom", "sign", "--id", id, "--base", base, "--pr-id", "gh:test#bom-tamper"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	_ = json.Unmarshal(out.Bytes(), &got)

	body, _ := os.ReadFile(got["bom_path"])
	body[10] ^= 0x01
	if err := os.WriteFile(got["bom_path"], body, 0o600); err != nil {
		t.Fatal(err)
	}
	sigHex, _ := os.ReadFile(got["signature_path"])
	signature, _ := hex.DecodeString(string(sigHex))
	pubKey, _ := hex.DecodeString(got["public_key_hex"])
	if err := sign.Verify(body, signature, pubKey); err == nil {
		t.Fatal("tampered BOM should fail signature verify")
	}
}
