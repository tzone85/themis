// Package api implements the read-only HTTP surface Themis exposes.
// Authentication uses Bearer tokens resolved via the auth package.
// Plan 12 wires role-aware identity; Plan 17/18 will swap in OIDC behind
// the same auth.TokenStore interface.
package api

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tzone85/themis/internal/auth"
)

// ErrUnauthorized mirrors auth.ErrUnauthorized at the api layer so existing
// tests keep compiling against the api package.
var ErrUnauthorized = auth.ErrUnauthorized

// Tokens reads the per-tenant token file at tenants/<id>/api-tokens.
// Retained for back-compat tests + the legacy code path. New endpoints
// should use RequireIdentity.
func Tokens(base, id string) ([]string, error) {
	path := filepath.Join(base, "tenants", id, "api-tokens")
	f, err := os.Open(path) // #nosec G304 -- path constructed from tenant id, not direct user input.
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// RequireToken verifies the Authorization header on r against the tenant's
// allowlist. Returns ErrUnauthorized for any missing/malformed/unknown token.
//
// Plan 12: this is now a thin wrapper around RequireIdentity that ignores
// the role component, so existing handlers keep working while new handlers
// opt-in to role checks via RequireIdentity.
func RequireToken(base, id string, r *http.Request) error {
	_, err := RequireIdentity(base, id, "", r)
	return err
}

// RequireIdentity resolves the Bearer token from r against the auth
// TokenStore. It returns ErrUnauthorized if the token is unknown or
// belongs to a different tenant; ErrInsufficientRole if the resolved
// identity's role does not satisfy minRole.
//
// minRole=="" disables the role gate (anyone with a valid token for the
// tenant passes — equivalent to RequireToken).
func RequireIdentity(base, tenantID string, minRole auth.Role, r *http.Request) (auth.Identity, error) {
	presented := bearerToken(r)
	if presented == "" {
		return auth.Identity{}, auth.ErrUnauthorized
	}

	store := auth.NewFileTokenStore(base)
	id, err := store.Lookup(presented)
	if err != nil {
		return auth.Identity{}, err
	}
	if id.Tenant != tenantID {
		return auth.Identity{}, auth.ErrUnauthorized
	}
	if !id.Role.Satisfies(minRole) {
		return auth.Identity{}, auth.ErrInsufficientRole
	}
	return id, nil
}

// bearerToken extracts the token from a `Bearer <tok>` Authorization
// header. Empty when no header or unsupported scheme.
func bearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(v, prefix) {
		return ""
	}
	return strings.TrimSpace(v[len(prefix):])
}

