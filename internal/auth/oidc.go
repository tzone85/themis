package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// OIDCTokenStore implements TokenStore against an OpenID Connect provider's
// userinfo endpoint. Bearer tokens presented to RequireIdentity are
// validated by calling the userinfo URL; the response's `tenant` and `role`
// claims map directly to the existing Identity shape, so OIDC slots in
// behind the same role gates the FileTokenStore feeds.
//
// Plan 18 ships the structural piece — a real production deployment will:
//
//  1. Provide its OIDC issuer's userinfo URL via `IssuerUserinfoURL`.
//  2. Configure its IdP to mint tokens carrying `tenant` + `role` claims
//     (or supply ClaimMapper to translate from whatever scheme the IdP
//     uses).
//
// In-memory results are cached for `CacheTTL` so a busy ramp doesn't
// hammer the IdP. Cache hits never block on the network.
type OIDCTokenStore struct {
	// IssuerUserinfoURL is the OIDC `/userinfo` endpoint (e.g.
	// https://login.example.com/oauth2/v1/userinfo).
	IssuerUserinfoURL string

	// HTTPClient lets tests substitute an in-process httptest.Server.
	// Defaults to http.DefaultClient.
	HTTPClient *http.Client

	// CacheTTL is how long a successful lookup is reused. Zero disables
	// caching (every Lookup hits the IdP).
	CacheTTL time.Duration

	// ClaimMapper converts the raw userinfo response into an Identity.
	// When nil, the default mapper reads `tenant` + `role` + `sub`
	// directly.
	ClaimMapper func(raw map[string]any) (Identity, error)

	mu    sync.Mutex
	cache map[string]cachedIdentity
}

type cachedIdentity struct {
	Identity Identity
	ExpiresAt time.Time
}

// ErrOIDC is wrapped by every error originating from the OIDC layer so
// callers can distinguish IdP-side failures from FileTokenStore failures.
var ErrOIDC = errors.New("auth: oidc")

// Lookup implements TokenStore.
func (s *OIDCTokenStore) Lookup(presented string) (Identity, error) {
	if strings.TrimSpace(presented) == "" {
		return Identity{}, ErrUnauthorized
	}
	if s.IssuerUserinfoURL == "" {
		return Identity{}, fmt.Errorf("%w: IssuerUserinfoURL not configured", ErrOIDC)
	}
	if id, ok := s.cacheGet(presented); ok {
		return id, nil
	}

	client := s.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.IssuerUserinfoURL, nil)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: build request: %v", ErrOIDC, err)
	}
	req.Header.Set("Authorization", "Bearer "+presented)
	resp, err := client.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrOIDC, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Identity{}, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Identity{}, fmt.Errorf("%w: %s: %s", ErrOIDC, resp.Status, strings.TrimSpace(string(body)))
	}

	var claims map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return Identity{}, fmt.Errorf("%w: decode userinfo: %v", ErrOIDC, err)
	}

	mapper := s.ClaimMapper
	if mapper == nil {
		mapper = DefaultClaimMapper
	}
	id, err := mapper(claims)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrOIDC, err)
	}
	id.Token4 = last4(presented)
	s.cachePut(presented, id)
	return id, nil
}

// DefaultClaimMapper reads `tenant`, `role`, and `description` from the
// userinfo response. Real IdPs typically need a custom mapper (e.g. read
// tenant from a group membership claim, derive role from an AD group).
func DefaultClaimMapper(raw map[string]any) (Identity, error) {
	tenant, _ := raw["tenant"].(string)
	roleStr, _ := raw["role"].(string)
	description, _ := raw["description"].(string)
	if tenant == "" || roleStr == "" {
		return Identity{}, fmt.Errorf("claims missing tenant or role")
	}
	role := Role(roleStr)
	if role.Rank() < 0 {
		return Identity{}, fmt.Errorf("unknown role %q from claims", roleStr)
	}
	return Identity{Tenant: tenant, Role: role, Description: description}, nil
}

func (s *OIDCTokenStore) cacheGet(token string) (Identity, bool) {
	if s.CacheTTL <= 0 {
		return Identity{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		return Identity{}, false
	}
	c, ok := s.cache[token]
	if !ok {
		return Identity{}, false
	}
	if time.Now().After(c.ExpiresAt) {
		delete(s.cache, token)
		return Identity{}, false
	}
	return c.Identity, true
}

func (s *OIDCTokenStore) cachePut(token string, id Identity) {
	if s.CacheTTL <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		s.cache = map[string]cachedIdentity{}
	}
	s.cache[token] = cachedIdentity{
		Identity:  id,
		ExpiresAt: time.Now().Add(s.CacheTTL),
	}
}

// ChainStore composes multiple TokenStores in order. Lookup returns the
// first successful match; ErrUnauthorized from one store falls through to
// the next, while any non-ErrUnauthorized error short-circuits (so an
// IdP outage doesn't silently fall back to a local file store with
// different ACLs).
type ChainStore struct {
	Stores []TokenStore
}

// NewChainStore returns a chain over the supplied stores.
func NewChainStore(stores ...TokenStore) *ChainStore { return &ChainStore{Stores: stores} }

// Lookup implements TokenStore.
func (c *ChainStore) Lookup(token string) (Identity, error) {
	if len(c.Stores) == 0 {
		return Identity{}, ErrUnauthorized
	}
	for _, s := range c.Stores {
		id, err := s.Lookup(token)
		switch {
		case err == nil:
			return id, nil
		case errors.Is(err, ErrUnauthorized):
			continue
		default:
			return Identity{}, err
		}
	}
	return Identity{}, ErrUnauthorized
}
