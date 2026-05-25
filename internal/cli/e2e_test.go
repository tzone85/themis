package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/sign"
)

// TestE2E_FullPipeline drives the entire user-visible flow end-to-end:
//
//	tenant init → catalogue sync → decide → bom build → bom sign → verify
//
// It is the single load-bearing test that proves every internal package
// wires together correctly, every event kind is registered, and the
// ledger chain remains intact across the full lifecycle.
func TestE2E_FullPipeline(t *testing.T) {
	base := t.TempDir()
	id := "acme"

	run := func(args ...string) string {
		cmd := NewRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("`themis %v` failed: %v\n%s", args, err, out.String())
		}
		return out.String()
	}

	// 1. tenant init.
	run("tenant", "init", "--id", id, "--base", base)

	// 2. catalogue sync.
	run("catalogue", "sync", "--id", id, "--base", base, "--source", fixtureRoot(t))

	// 3. decide on a doc-only PR — expect ALLOW.
	change := aichange.AIChange{
		PRID:  "gh:e2e#1",
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	cp := writeAIChange(t, t.TempDir(), change)
	pol := writePolicy(t, t.TempDir(), validPolicyYAML)
	out := run("decide", "--id", id, "--base", base, "--aichange", cp, "--policy", pol)
	var dec map[string]any
	_ = json.Unmarshal([]byte(out), &dec)
	if dec["verdict"] != "ALLOW" {
		t.Fatalf("decide verdict = %v, want ALLOW", dec["verdict"])
	}

	// 4. bom build.
	run("bom", "build", "--id", id, "--base", base, "--pr-id", "gh:e2e#1")

	// 5. bom sign — capture artefact paths.
	signOut := run("bom", "sign", "--id", id, "--base", base, "--pr-id", "gh:e2e#1")
	var signed map[string]string
	if err := json.Unmarshal([]byte(signOut), &signed); err != nil {
		t.Fatal(err)
	}

	// 6. Independent signature verification using the sign package.
	canon, err := os.ReadFile(signed["bom_path"])
	if err != nil {
		t.Fatal(err)
	}
	sigHex, err := os.ReadFile(signed["signature_path"])
	if err != nil {
		t.Fatal(err)
	}
	signature, _ := hex.DecodeString(string(sigHex))
	pubKey, _ := hex.DecodeString(signed["public_key_hex"])
	if err := sign.Verify(canon, signature, pubKey); err != nil {
		t.Fatalf("e2e signature verify failed: %v", err)
	}

	// 7. Ledger doctor — chain must still be intact at the end.
	verifyOut := run("ledger", "doctor", "--id", id, "--base", base)
	var rep map[string]any
	_ = json.Unmarshal([]byte(verifyOut), &rep)
	if rep["chain_intact"] != true {
		t.Fatalf("ledger chain broken after e2e: %v", rep)
	}

	// 8. Confirm every expected event kind appears in the ledger.
	events, err := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Kind] = true
	}
	for _, want := range []string{
		"TENANT_INITIALISED",
		"CATALOGUE_SYNCED",
		"DECISION_ISSUED",
		"BOM_BUILT",
		"BOM_SIGNED",
	} {
		if !seen[want] {
			t.Errorf("ledger missing expected kind %q after e2e (saw: %v)", want, seen)
		}
	}
}

// TestE2E_RealGitRepo runs the lifecycle starting from a real git repository:
//
//	tenant init → catalogue sync → ingest (git_heuristic) → decide → bom build → bom sign → verify
//
// This is the proof that the pipeline can consume real-world PR data, not
// just hand-crafted AIChange JSON.
func TestE2E_RealGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	base := t.TempDir()
	id := "acme"

	run := func(args ...string) string {
		t.Helper()
		cmd := NewRootCmd()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("`themis %v` failed: %v\n%s", args, err, out.String())
		}
		return out.String()
	}

	// 1. tenant + catalogue.
	run("tenant", "init", "--id", id, "--base", base)
	run("catalogue", "sync", "--id", id, "--base", base, "--source", fixtureRoot(t))

	// 2. Build a real git workspace touching docs only.
	workdir := t.TempDir()
	gitOK := func(args ...string) {
		// #nosec G204
		c := exec.Command("git", args...)
		c.Dir = workdir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitOK("init", "-q", "--initial-branch=main")
	gitOK("config", "user.email", "ci@test")
	gitOK("config", "user.name", "ci")
	gitOK("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(workdir, "README.md"), []byte("# v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitOK("add", ".")
	gitOK("commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(workdir, "README.md"), []byte("# v2 — edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitOK("commit", "-q", "-am", "edit")

	// 3. Ingest from the real repo.
	ingOut := run(
		"ingest",
		"--id", id, "--base", base,
		"--adapter", "git_heuristic",
		"--pr-id", "gh:e2e#git-real-1",
		"--workdir", workdir,
		"--base-ref", "HEAD~1",
	)
	var ing map[string]any
	_ = json.Unmarshal([]byte(ingOut), &ing)
	aiPath := ing["aichange_path"].(string)

	// 4. Decide on the ingested AIChange.
	pol := writePolicy(t, t.TempDir(), validPolicyYAML)
	decOut := run(
		"decide",
		"--id", id, "--base", base,
		"--aichange", aiPath,
		"--policy", pol,
	)
	var dec map[string]any
	_ = json.Unmarshal([]byte(decOut), &dec)
	if dec["verdict"] != "ALLOW" {
		t.Fatalf("decide verdict = %v, want ALLOW (README-only change)", dec["verdict"])
	}

	// 5. Build + sign + verify the BOM.
	run("bom", "build", "--id", id, "--base", base, "--pr-id", "gh:e2e#git-real-1")
	signOut := run("bom", "sign", "--id", id, "--base", base, "--pr-id", "gh:e2e#git-real-1")
	var signed map[string]string
	_ = json.Unmarshal([]byte(signOut), &signed)

	canon, _ := os.ReadFile(signed["bom_path"])
	sigHex, _ := os.ReadFile(signed["signature_path"])
	signature, _ := hex.DecodeString(string(sigHex))
	pubKey, _ := hex.DecodeString(signed["public_key_hex"])
	if err := sign.Verify(canon, signature, pubKey); err != nil {
		t.Fatalf("real-git e2e signature verify failed: %v", err)
	}

	// 6. Confirm every expected ledger kind landed.
	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	_ = aichange.AIChange{} // touch import; keeps formatter happy in some IDEs
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Kind] = true
	}
	for _, want := range []string{"INGEST_COMPLETED", "DECISION_ISSUED", "BOM_BUILT", "BOM_SIGNED"} {
		if !seen[want] {
			t.Errorf("real-git e2e missing %q (saw: %v)", want, seen)
		}
	}
}
