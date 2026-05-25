// Package api implements the read-only HTTP surface Themis exposes.
// Authenticated endpoints use a simple per-tenant Bearer token model;
// OIDC/SAML lands in a later plan (design spec §6.1).
package api

import (
	"bufio"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnauthorized is returned by RequireToken when no valid token is presented.
var ErrUnauthorized = errors.New("api: unauthorized")

// Tokens reads the per-tenant token file at tenants/<id>/api-tokens.
// One token per non-blank, non-comment line. Lines beginning with '#' are
// ignored. Returns an empty slice (and no error) when the file is missing —
// which the auth middleware treats as "no tokens registered, deny all".
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
func RequireToken(base, id string, r *http.Request) error {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ErrUnauthorized
	}
	presented := strings.TrimSpace(auth[len(prefix):])
	if presented == "" {
		return ErrUnauthorized
	}

	allowed, err := Tokens(base, id)
	if err != nil {
		return err
	}
	for _, tok := range allowed {
		// Constant-time comparison protects against timing-based token guessing.
		if constantTimeEqual(tok, presented) {
			return nil
		}
	}
	return ErrUnauthorized
}

// constantTimeEqual returns true iff a and b are equal, in time that does
// not depend on the position of the first differing byte.
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range len(a) {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
