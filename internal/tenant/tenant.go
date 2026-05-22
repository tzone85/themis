// Package tenant models the per-customer isolation boundary in Themis.
// Every package that touches per-customer state takes a Tenant explicitly.
// There are no globals; cross-tenant access requires constructing a
// different Tenant value.
package tenant

import (
	"errors"
	"fmt"
	"regexp"
)

// Tenant is the per-customer isolation boundary. All Themis state for one
// customer lives under Tenant.Root().
type Tenant struct {
	ID   string // stable, opaque, filesystem-safe identifier
	Base string // absolute path to the Themis state root (e.g. /var/lib/themis)
}

// validID restricts tenant IDs to a portable, filesystem-safe subset.
// Lowercase letters, digits, dash; 1-63 chars (DNS-label safe).
var validID = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62})$`)

// ErrInvalidID indicates a tenant ID failed validation.
var ErrInvalidID = errors.New("invalid tenant id")

// New constructs a Tenant after validating the id. It does NOT create
// directories on disk — see Init for that.
func New(base, id string) (Tenant, error) {
	if !validID.MatchString(id) {
		return Tenant{}, fmt.Errorf("%w: %q (must match %s)", ErrInvalidID, id, validID.String())
	}
	if base == "" {
		return Tenant{}, fmt.Errorf("base path required")
	}
	return Tenant{ID: id, Base: base}, nil
}
