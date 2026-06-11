// Package mempalace bridges decisions + BOMs into the per-tenant
// Mempalace wing (a content-addressed local-first memory store the author
// uses across projects). The bridge is intentionally minimal — Themis
// writes "drawer" JSON files under tenants/<id>/mempalace-wing/decisions/
// and tenants/<id>/mempalace-wing/boms/; the actual Mempalace daemon
// consumes them out-of-band.
//
// Decoupling Themis from the Mempalace schema means the schema can evolve
// independently, and Themis still works in deployments that don't run
// Mempalace at all (the files just sit there as plain JSON).
package mempalace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// validKind constrains drawer kinds to a portable, filesystem-safe
// subset: lowercase letters, digits, dash, 1-32 chars. Mirrors the
// shape of the on-disk tenant id so the two filesystem-component
// validators share a mental model.
var validKind = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// validTenant matches the tenant.validID grammar locally. We don't
// import the tenant package here to keep mempalace a leaf — it has no
// business knowing the per-customer isolation type, only that the
// string is filesystem-safe.
var validTenant = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62})$`)

// validKey accepts only hex-sha256 strings (the content-addressed
// shape Write produces internally). Explicit keys must match — that's
// the only way to safely thread caller-supplied keys into the path.
var validKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Drawer is one persisted memory record. The Themis bridge writes drawers
// of two kinds — decision and bom — but the type is generic so future
// drawers (scan-finding, override, …) ship the same wire format.
type Drawer struct {
	Kind        string          `json:"kind"`
	Tenant      string          `json:"tenant"`
	Key         string          `json:"key"`    // content-addressed (hex sha256 of Body)
	WrittenAt   time.Time       `json:"written_at"`
	Body        json.RawMessage `json:"body"`
	Tags        []string        `json:"tags,omitempty"`
	Description string          `json:"description,omitempty"`
}

// Bridge writes drawers into a per-tenant Mempalace wing directory.
type Bridge struct {
	Base string
}

// New constructs a Bridge rooted at the Themis state directory.
func New(base string) *Bridge { return &Bridge{Base: base} }

// ErrInvalidInput surfaces caller-side problems before we touch disk.
var ErrInvalidInput = errors.New("mempalace: invalid input")

// WingDir returns the per-tenant directory the bridge writes into.
func (b *Bridge) WingDir(tenantID string) string {
	return filepath.Join(b.Base, "tenants", tenantID, "mempalace-wing")
}

// Write persists d under WingDir/<kind>/<key>.json. The key is computed
// from the body content if d.Key is empty so writes are idempotent —
// re-writing the same body yields the same filename.
func (b *Bridge) Write(d Drawer) (string, error) {
	if d.Kind == "" {
		return "", fmt.Errorf("%w: kind required", ErrInvalidInput)
	}
	if d.Tenant == "" {
		return "", fmt.Errorf("%w: tenant required", ErrInvalidInput)
	}
	if len(d.Body) == 0 {
		return "", fmt.Errorf("%w: body required", ErrInvalidInput)
	}
	if !validKind.MatchString(d.Kind) {
		return "", fmt.Errorf("%w: kind %q must match %s", ErrInvalidInput, d.Kind, validKind.String())
	}
	if !validTenant.MatchString(d.Tenant) {
		return "", fmt.Errorf("%w: tenant %q must match %s", ErrInvalidInput, d.Tenant, validTenant.String())
	}
	if d.Key == "" {
		sum := sha256.Sum256(d.Body)
		d.Key = hex.EncodeToString(sum[:])
	} else if !validKey.MatchString(d.Key) {
		return "", fmt.Errorf("%w: key %q must be 64 lowercase hex chars", ErrInvalidInput, d.Key)
	}
	if d.WrittenAt.IsZero() {
		d.WrittenAt = time.Now().UTC()
	}

	dir := filepath.Join(b.WingDir(d.Tenant), d.Kind)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	out := filepath.Join(dir, d.Key+".json")
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal drawer: %w", err)
	}
	if err := os.WriteFile(out, raw, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", out, err)
	}
	return out, nil
}

// Read returns the drawer at WingDir/<kind>/<key>.json.
func (b *Bridge) Read(tenantID, kind, key string) (Drawer, error) {
	if !validTenant.MatchString(tenantID) {
		return Drawer{}, fmt.Errorf("%w: tenant %q", ErrInvalidInput, tenantID)
	}
	if !validKind.MatchString(kind) {
		return Drawer{}, fmt.Errorf("%w: kind %q", ErrInvalidInput, kind)
	}
	if !validKey.MatchString(key) {
		return Drawer{}, fmt.Errorf("%w: key %q must be 64 lowercase hex chars", ErrInvalidInput, key)
	}
	path := filepath.Join(b.WingDir(tenantID), kind, key+".json")
	raw, err := os.ReadFile(path) // #nosec G304 -- all three components validated above.
	if err != nil {
		return Drawer{}, err
	}
	var d Drawer
	if err := json.Unmarshal(raw, &d); err != nil {
		return Drawer{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return d, nil
}

// List enumerates the keys for a kind. Empty slice when the directory is
// missing.
func (b *Bridge) List(tenantID, kind string) ([]string, error) {
	if !validTenant.MatchString(tenantID) {
		return nil, fmt.Errorf("%w: tenant %q", ErrInvalidInput, tenantID)
	}
	if !validKind.MatchString(kind) {
		return nil, fmt.Errorf("%w: kind %q", ErrInvalidInput, kind)
	}
	dir := filepath.Join(b.WingDir(tenantID), kind)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !filepath.IsAbs(name) && len(name) > 5 && name[len(name)-5:] == ".json" {
			out = append(out, name[:len(name)-5])
		}
	}
	return out, nil
}
