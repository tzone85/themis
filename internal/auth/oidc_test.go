package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// stubIdP simulates an OIDC /userinfo endpoint with configurable responses.
func stubIdP(t *testing.T, responder func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *OIDCTokenStore) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(responder))
	t.Cleanup(srv.Close)
	return srv, &OIDCTokenStore{
		IssuerUserinfoURL: srv.URL,
		HTTPClient:        srv.Client(),
	}
}

func TestOIDC_LookupHappyPath(t *testing.T) {
	_, store := stubIdP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tenant":"acme","role":"dev","description":"alice"}`))
	})
	id, err := store.Lookup("good-token")
	if err != nil {
		t.Fatal(err)
	}
	if id.Tenant != "acme" || id.Role != RoleDev || id.Description != "alice" || id.Token4 != "oken" {
		t.Fatalf("identity = %+v", id)
	}
}

func TestOIDC_LookupUnauthorizedMapsToErrUnauthorized(t *testing.T) {
	_, store := stubIdP(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	})
	_, err := store.Lookup("nope")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestOIDC_LookupServerErrorWrapsErrOIDC(t *testing.T) {
	_, store := stubIdP(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := store.Lookup("anything")
	if !errors.Is(err, ErrOIDC) {
		t.Fatalf("expected ErrOIDC, got %v", err)
	}
}

func TestOIDC_LookupRejectsEmptyToken(t *testing.T) {
	_, store := stubIdP(t, func(w http.ResponseWriter, _ *http.Request) {})
	if _, err := store.Lookup(""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestOIDC_LookupRejectsMissingURL(t *testing.T) {
	store := &OIDCTokenStore{}
	if _, err := store.Lookup("x"); !errors.Is(err, ErrOIDC) {
		t.Fatalf("expected ErrOIDC for missing URL, got %v", err)
	}
}

func TestOIDC_DefaultClaimMapperRejectsMissingClaims(t *testing.T) {
	if _, err := DefaultClaimMapper(map[string]any{"role": "dev"}); err == nil {
		t.Fatal("missing tenant claim should error")
	}
	if _, err := DefaultClaimMapper(map[string]any{"tenant": "acme"}); err == nil {
		t.Fatal("missing role claim should error")
	}
	if _, err := DefaultClaimMapper(map[string]any{"tenant": "acme", "role": "wizard"}); err == nil {
		t.Fatal("unknown role should error")
	}
}

func TestOIDC_CustomClaimMapper(t *testing.T) {
	_, store := stubIdP(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"alice","groups":["themis-acme-admin"]}`))
	})
	store.ClaimMapper = func(raw map[string]any) (Identity, error) {
		// Demo mapper: derive tenant + role from a `groups` claim.
		for _, g := range raw["groups"].([]any) {
			s := g.(string)
			if s == "themis-acme-admin" {
				return Identity{Tenant: "acme", Role: RoleAdmin}, nil
			}
		}
		return Identity{}, errors.New("no themis groups")
	}
	id, err := store.Lookup("any")
	if err != nil {
		t.Fatal(err)
	}
	if id.Role != RoleAdmin || id.Tenant != "acme" {
		t.Fatalf("custom mapper output = %+v", id)
	}
}

func TestOIDC_CacheReusesIdentity(t *testing.T) {
	calls := 0
	_, store := stubIdP(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tenant":"acme","role":"dev"}`))
	})
	store.CacheTTL = time.Minute

	if _, err := store.Lookup("tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lookup("tok"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 IdP call (cache hit on 2nd lookup), got %d", calls)
	}
}

func TestOIDC_CacheExpiresAfterTTL(t *testing.T) {
	calls := 0
	_, store := stubIdP(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tenant":"acme","role":"dev"}`))
	})
	store.CacheTTL = 10 * time.Millisecond

	_, _ = store.Lookup("tok")
	time.Sleep(20 * time.Millisecond)
	_, _ = store.Lookup("tok")
	if calls != 2 {
		t.Fatalf("expected 2 IdP calls (cache expired), got %d", calls)
	}
}

func TestChainStore_FirstHitWins(t *testing.T) {
	base := t.TempDir()
	writeTokensYAML(t, base, `
tokens:
  - token: "yaml-tok"
    tenant: "acme"
    role: "dev"
`)
	yaml := NewFileTokenStore(base)
	_, oidc := stubIdP(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tenant":"acme","role":"admin"}`))
	})

	chain := NewChainStore(yaml, oidc)
	id, err := chain.Lookup("yaml-tok")
	if err != nil {
		t.Fatal(err)
	}
	if id.Role != RoleDev {
		t.Fatalf("expected yaml dev role to win: %+v", id)
	}
}

func TestChainStore_FallsThroughOnUnauthorized(t *testing.T) {
	base := t.TempDir()
	writeTokensYAML(t, base, "tokens: []") // yaml doesn't know the token
	yaml := NewFileTokenStore(base)
	_, oidc := stubIdP(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tenant":"acme","role":"admin"}`))
	})

	chain := NewChainStore(yaml, oidc)
	id, err := chain.Lookup("oidc-tok")
	if err != nil {
		t.Fatal(err)
	}
	if id.Role != RoleAdmin {
		t.Fatalf("expected oidc admin role from fallback: %+v", id)
	}
}

func TestChainStore_ShortCircuitsOnNonUnauthorizedError(t *testing.T) {
	base := t.TempDir()
	writeTokensYAML(t, base, "tokens: \"bad-shape\"") // forces a parse error
	yaml := NewFileTokenStore(base)
	chain := NewChainStore(yaml, &OIDCTokenStore{IssuerUserinfoURL: "https://nope"})
	if _, err := chain.Lookup("x"); errors.Is(err, ErrUnauthorized) {
		t.Fatalf("hard error should short-circuit, got ErrUnauthorized")
	}
}

func TestChainStore_EmptyReturnsUnauthorized(t *testing.T) {
	if _, err := NewChainStore().Lookup("x"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("empty chain should ErrUnauthorized, got %v", err)
	}
}
