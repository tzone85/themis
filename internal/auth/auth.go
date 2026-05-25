// Package auth models the per-tenant identity Themis attaches to every
// authenticated request. Tokens map to an Identity (tenant + role); roles
// are totally-ordered so endpoint gates can require a *minimum* role
// rather than enumerate every variant.
//
// The package is the substrate the OIDC adapter (Plan 17 / 18) plugs into:
// it replaces FileTokenStore with an OIDC-backed implementation but keeps
// the Identity + Role types unchanged.
package auth

import (
	"bufio"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Role is one of the canonical role identifiers from the design spec.
type Role string

const (
	// RoleRead can call GET endpoints only.
	RoleRead Role = "read"
	// RoleDev can issue decisions (POST /decide).
	RoleDev Role = "dev"
	// RoleReviewer can additionally grant or deny approvals.
	RoleReviewer Role = "reviewer"
	// RoleCompliance can invoke emergency overrides and close postmortems.
	RoleCompliance Role = "compliance"
	// RoleAdmin can additionally anchor the ledger, file heartbeats, manage tokens.
	RoleAdmin Role = "admin"
)

// rank gives each role a total order. Unknown roles return -1.
var rank = map[Role]int{
	RoleRead:       0,
	RoleDev:        1,
	RoleReviewer:   2,
	RoleCompliance: 3,
	RoleAdmin:      4,
}

// Rank reports the role's position in the precedence chain.
func (r Role) Rank() int {
	v, ok := rank[r]
	if !ok {
		return -1
	}
	return v
}

// Satisfies reports whether r is ≥ min in the precedence chain.
func (r Role) Satisfies(min Role) bool {
	if min == "" {
		return true
	}
	rr, mr := r.Rank(), min.Rank()
	if rr < 0 || mr < 0 {
		return false
	}
	return rr >= mr
}

// Identity is the resolved subject of a request.
type Identity struct {
	Tenant      string
	Role        Role
	Token4      string // last 4 chars of the presented token (audit-friendly, never the full token)
	Description string
}

// Sentinel errors so callers can distinguish missing-token from wrong-role.
var (
	ErrUnauthorized     = errors.New("auth: unauthorized")
	ErrInsufficientRole = errors.New("auth: insufficient role")
)

// TokenStore is the lookup abstraction. The OIDC adapter implements this
// against an issuer; the file-backed default reads `tokens.yaml`.
type TokenStore interface {
	Lookup(token string) (Identity, error)
}

// FileTokenStore reads `tenants/tokens.yaml` (preferred) and falls back to
// the legacy per-tenant `tenants/<id>/api-tokens` file (every token in
// that file is treated as RoleAdmin for the tenant, preserving Plan-6
// behaviour).
type FileTokenStore struct {
	Base string
}

// NewFileTokenStore returns a store rooted at the Themis state directory.
func NewFileTokenStore(base string) *FileTokenStore { return &FileTokenStore{Base: base} }

// Lookup returns the Identity matching presented. ErrUnauthorized when no
// entry matches.
func (s *FileTokenStore) Lookup(presented string) (Identity, error) {
	if strings.TrimSpace(presented) == "" {
		return Identity{}, ErrUnauthorized
	}

	if id, ok, err := s.lookupYAML(presented); err != nil {
		return Identity{}, err
	} else if ok {
		return id, nil
	}
	if id, ok, err := s.lookupLegacy(presented); err != nil {
		return Identity{}, err
	} else if ok {
		return id, nil
	}
	return Identity{}, ErrUnauthorized
}

// tokensYAML is the on-disk shape of `tokens.yaml`.
type tokensYAML struct {
	Tokens []struct {
		Token       string `yaml:"token"`
		Tenant      string `yaml:"tenant"`
		Role        Role   `yaml:"role"`
		Description string `yaml:"description"`
	} `yaml:"tokens"`
}

// YAMLPath returns the canonical location of the structured token file.
func (s *FileTokenStore) YAMLPath() string {
	return filepath.Join(s.Base, "tenants", "tokens.yaml")
}

func (s *FileTokenStore) lookupYAML(presented string) (Identity, bool, error) {
	raw, err := os.ReadFile(s.YAMLPath()) // #nosec G304 -- tenant-scoped path.
	if err != nil {
		if os.IsNotExist(err) {
			return Identity{}, false, nil
		}
		return Identity{}, false, fmt.Errorf("read tokens.yaml: %w", err)
	}
	var doc tokensYAML
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Identity{}, false, fmt.Errorf("parse tokens.yaml: %w", err)
	}
	for _, t := range doc.Tokens {
		if t.Tenant == "" || t.Role == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(t.Token), []byte(presented)) == 1 {
			return Identity{
				Tenant:      t.Tenant,
				Role:        t.Role,
				Token4:      last4(presented),
				Description: t.Description,
			}, true, nil
		}
	}
	return Identity{}, false, nil
}

// lookupLegacy scans every tenants/<id>/api-tokens file. Tokens listed
// there are treated as RoleAdmin for back-compat with Plan 6.
func (s *FileTokenStore) lookupLegacy(presented string) (Identity, bool, error) {
	tenantsDir := filepath.Join(s.Base, "tenants")
	entries, err := os.ReadDir(tenantsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return Identity{}, false, nil
		}
		return Identity{}, false, fmt.Errorf("readdir tenants: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		legacy := filepath.Join(tenantsDir, e.Name(), "api-tokens")
		f, err := os.Open(legacy) // #nosec G304 -- tenant-scoped path.
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		matched := false
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if subtle.ConstantTimeCompare([]byte(line), []byte(presented)) == 1 {
				matched = true
				break
			}
		}
		_ = f.Close()
		if matched {
			return Identity{
				Tenant:      e.Name(),
				Role:        RoleAdmin,
				Token4:      last4(presented),
				Description: "legacy api-tokens",
			}, true, nil
		}
	}
	return Identity{}, false, nil
}

// last4 returns the last four characters of token (or all of token if it's
// short). Used purely for audit log breadcrumbs.
func last4(token string) string {
	if len(token) <= 4 {
		return token
	}
	return token[len(token)-4:]
}
