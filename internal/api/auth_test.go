package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func writeTenantTokens(t *testing.T, base, id, body string) {
	t.Helper()
	dir := filepath.Join(base, "tenants", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api-tokens"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTokens_LoadsLinesIgnoresBlanksAndComments(t *testing.T) {
	base := t.TempDir()
	writeTenantTokens(t, base, "acme", "# a comment\nfirst-token\n\n  second-token  \n# another comment\n")
	got, err := Tokens(base, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "first-token" || got[1] != "second-token" {
		t.Fatalf("got %v", got)
	}
}

func TestTokens_MissingFileReturnsEmpty(t *testing.T) {
	got, err := Tokens(t.TempDir(), "no-such")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("missing file should return empty, got %v", got)
	}
}

func TestRequireToken_AcceptsValidBearer(t *testing.T) {
	base := t.TempDir()
	writeTenantTokens(t, base, "acme", "secret-1\nsecret-2\n")

	r, _ := http.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer secret-2")
	if err := RequireToken(base, "acme", r); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestRequireToken_RejectsMissingHeader(t *testing.T) {
	base := t.TempDir()
	writeTenantTokens(t, base, "acme", "secret\n")
	r, _ := http.NewRequest(http.MethodGet, "/x", nil)
	if err := RequireToken(base, "acme", r); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("missing header should ErrUnauthorized, got %v", err)
	}
}

func TestRequireToken_RejectsWrongScheme(t *testing.T) {
	base := t.TempDir()
	writeTenantTokens(t, base, "acme", "secret\n")
	r, _ := http.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Basic c2VjcmV0")
	if err := RequireToken(base, "acme", r); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Basic scheme should ErrUnauthorized, got %v", err)
	}
}

func TestRequireToken_RejectsEmptyBearer(t *testing.T) {
	base := t.TempDir()
	writeTenantTokens(t, base, "acme", "secret\n")
	r, _ := http.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer ")
	if err := RequireToken(base, "acme", r); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("empty bearer should ErrUnauthorized, got %v", err)
	}
}

func TestRequireToken_RejectsUnknownToken(t *testing.T) {
	base := t.TempDir()
	writeTenantTokens(t, base, "acme", "secret\n")
	r, _ := http.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	if err := RequireToken(base, "acme", r); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unknown token should ErrUnauthorized, got %v", err)
	}
}

func TestRequireToken_RejectsWhenNoTokensFile(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer x")
	if err := RequireToken(t.TempDir(), "acme", r); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("no tokens file should ErrUnauthorized, got %v", err)
	}
}
