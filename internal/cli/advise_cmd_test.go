package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
)

func TestAdvise_WritesDrawerAndReturnsNote(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	pol := writePolicy(t, t.TempDir(), validPolicyYAML)
	change := aichange.AIChange{
		PRID:  "gh:test#advise-1",
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	cp := writeAIChange(t, t.TempDir(), change)

	// Issue a decision first.
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"decide", "--id", id, "--base", base, "--aichange", cp, "--policy", pol})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now run advise.
	out := &bytes.Buffer{}
	cmd = NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"advise", "--id", id, "--base", base, "--pr-id", "gh:test#advise-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("advise: %v\n%s", err, out.String())
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("advise output not JSON: %v\n%s", err, out.String())
	}
	if got["backed_by"] != "null" {
		t.Fatalf("backed_by = %v", got["backed_by"])
	}
	if got["verdict"] != "ALLOW" {
		t.Fatalf("verdict = %v", got["verdict"])
	}
	if got["drawer_path"] == nil {
		t.Fatalf("drawer_path missing: %+v", got)
	}
	// Drawer file exists.
	if _, err := os.Stat(got["drawer_path"].(string)); err != nil {
		t.Fatalf("drawer file missing: %v", err)
	}
}

func TestAdvise_RejectsUnknownPR(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"advise", "--id", id, "--base", base, "--pr-id", "gh:phantom#999"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("advise should fail for missing decision")
	}
}

func TestAdvise_RequiresFlags(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"advise"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when required flags missing")
	}
	_ = filepath.Join // keep imports honest
}
