package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/ledger"
)

func setupTenantWithCatalogue(t *testing.T) (base, id string) {
	t.Helper()
	base = t.TempDir()
	id = "acme"
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"tenant", "init", "--id", id, "--base", base})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	return
}

// fixtureRoot resolves the catalogue fixture path from internal/cli.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "catalogue", "testdata", "sample")
}

func TestCatalogueSync_EmitsEventAndSnapshot(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)
	out := &bytes.Buffer{}

	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"catalogue", "sync", "--id", id, "--base", base, "--source", fixtureRoot(t)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("catalogue sync: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if payload["domains"].(float64) != 2 || payload["services"].(float64) != 4 || payload["events"].(float64) != 6 {
		t.Fatalf("counts wrong: %+v", payload)
	}
	if h, ok := payload["content_hash"].(string); !ok || h == "" {
		t.Fatalf("missing content_hash in payload: %+v", payload)
	}

	// Snapshot file exists and is non-empty JSON.
	snap := filepath.Join(base, "tenants", id, "catalogue.json")
	fi, err := os.Stat(snap)
	if err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}
	if fi.Size() < 100 {
		t.Fatalf("snapshot suspiciously small: %d bytes", fi.Size())
	}

	// Ledger now contains TENANT_INITIALISED + CATALOGUE_SYNCED.
	events, err := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2 (init + catalogue_synced)", len(events))
	}
	if events[1].Kind != "CATALOGUE_SYNCED" {
		t.Fatalf("event[1].Kind = %q, want CATALOGUE_SYNCED", events[1].Kind)
	}
}

func TestCatalogueSync_DedupableContentHash(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)
	src := fixtureRoot(t)

	run := func() string {
		out := &bytes.Buffer{}
		cmd := NewRootCmd()
		cmd.SetOut(out)
		cmd.SetErr(out)
		cmd.SetArgs([]string{"catalogue", "sync", "--id", id, "--base", base, "--source", src})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var p map[string]any
		_ = json.Unmarshal(out.Bytes(), &p)
		return p["content_hash"].(string)
	}
	h1 := run()
	h2 := run()
	if h1 != h2 || h1 == "" {
		t.Fatalf("content_hash should be stable across re-syncs: %q vs %q", h1, h2)
	}
}

func TestCatalogueSync_RejectsBrokenCatalogue(t *testing.T) {
	base, id := setupTenantWithCatalogue(t)
	broken := t.TempDir()
	// Create an events/ subdir with malformed front-matter so parsing fails.
	if err := os.MkdirAll(filepath.Join(broken, "events", "X"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "events", "X", "index.md"), []byte("not yaml at all\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"catalogue", "sync", "--id", id, "--base", base, "--source", broken})
	if err := cmd.Execute(); err == nil {
		t.Fatal("catalogue sync should reject broken catalogue")
	}
}
