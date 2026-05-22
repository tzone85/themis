package tenant

import "path/filepath"

// Root returns the tenant's filesystem root directory.
func (t Tenant) Root() string {
	return filepath.Join(t.Base, "tenants", t.ID)
}

// Events returns the path to the tenant's append-only events.jsonl.
func (t Tenant) Events() string {
	return filepath.Join(t.Root(), "events.jsonl")
}

// Projection returns the path to the tenant's SQLite WAL projection database.
func (t Tenant) Projection() string {
	return filepath.Join(t.Root(), "projection.sqlite")
}

// BOM returns the directory where signed AI-BOM artefacts are stored.
func (t Tenant) BOM() string {
	return filepath.Join(t.Root(), "bom")
}

// Wing returns the per-tenant Mempalace wing directory.
func (t Tenant) Wing() string {
	return filepath.Join(t.Root(), "mempalace-wing")
}
