package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRole_RankOrdering(t *testing.T) {
	order := []Role{RoleRead, RoleDev, RoleReviewer, RoleCompliance, RoleAdmin}
	for i := 1; i < len(order); i++ {
		if order[i-1].Rank() >= order[i].Rank() {
			t.Errorf("rank(%q)=%d should be < rank(%q)=%d",
				order[i-1], order[i-1].Rank(), order[i], order[i].Rank())
		}
	}
}

func TestRole_UnknownRankNegativeOne(t *testing.T) {
	if Role("phantom").Rank() != -1 {
		t.Fatal("unknown role rank should be -1")
	}
}

func TestRole_SatisfiesEmptyMinAlwaysTrue(t *testing.T) {
	if !RoleRead.Satisfies("") {
		t.Fatal("empty min should be satisfied by any role")
	}
}

func TestRole_SatisfiesAdminCoversAll(t *testing.T) {
	for _, r := range []Role{RoleRead, RoleDev, RoleReviewer, RoleCompliance, RoleAdmin} {
		if !RoleAdmin.Satisfies(r) {
			t.Errorf("admin should satisfy %q", r)
		}
	}
}

func TestRole_SatisfiesReadFailsForElevated(t *testing.T) {
	if RoleRead.Satisfies(RoleDev) {
		t.Fatal("read must NOT satisfy dev")
	}
}

func TestRole_SatisfiesUnknownReturnsFalse(t *testing.T) {
	if Role("???").Satisfies(RoleRead) {
		t.Fatal("unknown role must not satisfy")
	}
	if RoleAdmin.Satisfies(Role("???")) {
		t.Fatal("unknown min must not be satisfiable")
	}
}

func writeTokensYAML(t *testing.T, base, body string) {
	t.Helper()
	dir := filepath.Join(base, "tenants")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tokens.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeLegacyTokens(t *testing.T, base, tenantID, body string) {
	t.Helper()
	dir := filepath.Join(base, "tenants", tenantID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api-tokens"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFileTokenStore_LookupYAML(t *testing.T) {
	base := t.TempDir()
	writeTokensYAML(t, base, `
tokens:
  - token: "abc123"
    tenant: "acme"
    role: "dev"
    description: "alice"
  - token: "xyz789"
    tenant: "acme"
    role: "admin"
`)
	store := NewFileTokenStore(base)
	id, err := store.Lookup("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if id.Tenant != "acme" || id.Role != RoleDev || id.Description != "alice" || id.Token4 != "c123" {
		t.Fatalf("identity = %+v", id)
	}
}

func TestFileTokenStore_LookupUnknownReturnsErr(t *testing.T) {
	base := t.TempDir()
	writeTokensYAML(t, base, `tokens: []`)
	_, err := NewFileTokenStore(base).Lookup("not-a-real-token")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestFileTokenStore_EmptyTokenRejected(t *testing.T) {
	_, err := NewFileTokenStore(t.TempDir()).Lookup("")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for empty token: %v", err)
	}
}

func TestFileTokenStore_LegacyApiTokensFile(t *testing.T) {
	base := t.TempDir()
	writeLegacyTokens(t, base, "acme", "secret-1\nsecret-2\n")
	store := NewFileTokenStore(base)

	id, err := store.Lookup("secret-2")
	if err != nil {
		t.Fatal(err)
	}
	if id.Tenant != "acme" || id.Role != RoleAdmin {
		t.Fatalf("legacy token should map to RoleAdmin: %+v", id)
	}
	if id.Description != "legacy api-tokens" {
		t.Errorf("description = %q", id.Description)
	}
}

func TestFileTokenStore_YAMLTakesPrecedenceOverLegacy(t *testing.T) {
	base := t.TempDir()
	writeTokensYAML(t, base, `
tokens:
  - token: "shared"
    tenant: "acme"
    role: "dev"
`)
	writeLegacyTokens(t, base, "acme", "shared\n")
	id, err := NewFileTokenStore(base).Lookup("shared")
	if err != nil {
		t.Fatal(err)
	}
	if id.Role != RoleDev {
		t.Fatalf("YAML role should win over legacy admin: %+v", id)
	}
}

func TestFileTokenStore_MissingFilesReturnUnauthorized(t *testing.T) {
	_, err := NewFileTokenStore(t.TempDir()).Lookup("any")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized: %v", err)
	}
}

func TestFileTokenStore_BrokenYAMLReturnsError(t *testing.T) {
	base := t.TempDir()
	// Use yaml that *parses* into wrong shape so the unmarshal fails.
	writeTokensYAML(t, base, "tokens: \"this should be a list, not a string\"\n")
	_, err := NewFileTokenStore(base).Lookup("x")
	if err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected hard error on broken yaml, got %v", err)
	}
}

func TestLast4_ShortString(t *testing.T) {
	if last4("ab") != "ab" {
		t.Fatal("short string should pass through")
	}
	if last4("abcdef") != "cdef" {
		t.Fatal("long string should be tail-trimmed")
	}
}
