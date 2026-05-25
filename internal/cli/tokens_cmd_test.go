package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestTokensGrant_GeneratesAndPersists(t *testing.T) {
	base := t.TempDir()
	out, err := runCLI(t, "tokens", "grant",
		"--base", base, "--tenant", "acme", "--role", "dev", "--description", "alice")
	if err != nil {
		t.Fatalf("grant: %v\n%s", err, out)
	}
	if !strings.Contains(out, "thm_") {
		t.Fatalf("expected token in output: %s", out)
	}
	body, err := os.ReadFile(filepath.Join(base, "tenants", "tokens.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "tenant: acme") || !strings.Contains(string(body), "role: dev") {
		t.Fatalf("tokens.yaml missing fields: %s", body)
	}
}

func TestTokensGrant_RejectsBadRole(t *testing.T) {
	_, err := runCLI(t, "tokens", "grant",
		"--base", t.TempDir(), "--tenant", "x", "--role", "wizard")
	if err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestTokensList_ShowsRegistered(t *testing.T) {
	base := t.TempDir()
	if _, err := runCLI(t, "tokens", "grant",
		"--base", base, "--tenant", "acme", "--role", "dev", "--description", "alice"); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "tokens", "list", "--base", base)
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "acme") || !strings.Contains(out, "dev") || !strings.Contains(out, "alice") {
		t.Fatalf("list missing fields: %s", out)
	}
}

func TestTokensList_EmptyOK(t *testing.T) {
	out, err := runCLI(t, "tokens", "list", "--base", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no tokens registered") {
		t.Fatalf("expected empty marker: %s", out)
	}
}

func TestTokensRevoke_RemovesEntry(t *testing.T) {
	base := t.TempDir()
	// Grant + capture the suffix.
	out, _ := runCLI(t, "tokens", "grant",
		"--base", base, "--tenant", "acme", "--role", "dev")
	tokLine := ""
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "thm_") {
			tokLine = ln
			break
		}
	}
	if tokLine == "" {
		t.Fatalf("no token in grant output: %s", out)
	}
	suffix := tokLine[len(tokLine)-6:]

	revOut, err := runCLI(t, "tokens", "revoke",
		"--base", base, "--token-prefix", suffix)
	if err != nil {
		t.Fatalf("revoke: %v\n%s", err, revOut)
	}
	if !strings.Contains(revOut, "revoked 1") {
		t.Fatalf("expected revoked 1: %s", revOut)
	}
	listOut, _ := runCLI(t, "tokens", "list", "--base", base)
	if !strings.Contains(listOut, "no tokens registered") {
		t.Fatalf("expected empty list after revoke: %s", listOut)
	}
}

func TestTokensRevoke_RejectsNoMatch(t *testing.T) {
	base := t.TempDir()
	_, _ = runCLI(t, "tokens", "grant",
		"--base", base, "--tenant", "acme", "--role", "dev")
	_, err := runCLI(t, "tokens", "revoke",
		"--base", base, "--token-prefix", "no-such-token")
	if err == nil {
		t.Fatal("expected error when no token matches")
	}
}
