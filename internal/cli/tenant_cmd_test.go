package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestTenantInit_CreatesDirectoryAndEmitsEvent(t *testing.T) {
	base := t.TempDir()
	out := &bytes.Buffer{}

	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"tenant", "init", "--id", "acme", "--base", base})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	tenantDir := filepath.Join(base, "tenants", "acme")
	if _, err := os.Stat(tenantDir); err != nil {
		t.Fatalf("tenant dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tenantDir, "events.jsonl")); err != nil {
		t.Fatalf("events.jsonl not created: %v", err)
	}
}

func TestTenantInit_RejectsInvalidID(t *testing.T) {
	base := t.TempDir()
	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"tenant", "init", "--id", "../escape", "--base", base})
	if err := cmd.Execute(); err == nil {
		t.Fatal("invalid id should have errored")
	}
}
