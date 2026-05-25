package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/ledger"
)

func TestIngest_Manual_WritesAIChangeAndEvent(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)
	out := &bytes.Buffer{}

	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{
		"ingest",
		"--id", id, "--base", base,
		"--adapter", "manual_attestation",
		"--pr-id", "gh:test#ingest-1",
		"--actor", "human:thandi",
		"--file", "src/a.go=before1,after1",
		"--file", "src/b.go=,after2",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("ingest manual: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if got["adapter"] != "manual_attestation" || got["pr_id"] != "gh:test#ingest-1" {
		t.Fatalf("output wrong: %+v", got)
	}

	aiPath := got["aichange_path"].(string)
	raw, err := os.ReadFile(aiPath)
	if err != nil {
		t.Fatalf("aichange file missing: %v", err)
	}
	var ai aichange.AIChange
	if err := json.Unmarshal(raw, &ai); err != nil {
		t.Fatalf("aichange file not JSON: %v", err)
	}
	if len(ai.TouchedFiles) != 2 {
		t.Errorf("aichange files = %d, want 2", len(ai.TouchedFiles))
	}

	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if events[len(events)-1].Kind != "INGEST_COMPLETED" {
		t.Fatalf("last event = %q, want INGEST_COMPLETED", events[len(events)-1].Kind)
	}
}

func TestIngest_AdapterFailure_LogsAdapterFailedEvent(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	// Missing --actor for manual_attestation forces ErrAdapterFailed.
	cmd.SetArgs([]string{
		"ingest",
		"--id", id, "--base", base,
		"--adapter", "manual_attestation",
		"--pr-id", "x",
		"--file", "a.go=,1",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("ingest should fail without --actor for manual_attestation")
	}

	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if events[len(events)-1].Kind != "INGEST_ADAPTER_FAILED" {
		t.Fatalf("last event = %q, want INGEST_ADAPTER_FAILED", events[len(events)-1].Kind)
	}
}

func TestIngest_GitHeuristic_E2E(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	base, id := setupTenantWithCatalogue(t)

	// Build a tiny repo and pass it as --workdir.
	dir := t.TempDir()
	gitOK := func(args ...string) {
		// #nosec G204
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitOK("init", "-q", "--initial-branch=main")
	gitOK("config", "user.email", "ci@test")
	gitOK("config", "user.name", "ci")
	gitOK("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitOK("add", ".")
	gitOK("commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n// edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitOK("commit", "-q", "-am", "edit")

	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{
		"ingest",
		"--id", id, "--base", base,
		"--adapter", "git_heuristic",
		"--pr-id", "gh:e2e#git-1",
		"--workdir", dir,
		"--base-ref", "HEAD~1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("ingest git_heuristic: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if got["file_count"].(float64) != 1 {
		t.Errorf("file_count = %v, want 1", got["file_count"])
	}
}

func TestIngest_UnknownAdapterFails(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"ingest", "--id", id, "--base", base, "--adapter", "phantom", "--pr-id", "x"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("ingest should reject unknown adapter")
	}
}

func TestIngest_BadFileFlag(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"ingest", "--id", id, "--base", base,
		"--adapter", "manual_attestation",
		"--pr-id", "x", "--actor", "human:y",
		"--file", "missing-equals-sign",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("ingest should reject malformed --file flag")
	}
}
