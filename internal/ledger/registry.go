package ledger

import (
	"fmt"
	"sort"
	"sync"
)

// Projector is a per-event-kind projection function. It is called with
// the raw payload bytes; the function returns an error if the event
// cannot be projected. Projectors MUST be deterministic and side-effect
// free except for writing to their tenant's SQLite projection.
type Projector func(payload []byte) error

// Registry holds the set of known event kinds and their projectors.
// New event kinds MUST be registered here — the wiring test will fail
// the build if a kind is appended via Store.Append() but absent from
// the registry, or if the registry has an entry with no projector.
type Registry struct {
	mu sync.RWMutex
	p  map[string]Projector
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{p: map[string]Projector{}} }

// Register adds a projector for kind. Re-registering is allowed (last write wins)
// to support test setup; production code should call Register exactly once
// per kind during package init.
func (r *Registry) Register(kind string, p Projector) {
	if kind == "" {
		panic("ledger: cannot register empty kind")
	}
	if p == nil {
		panic(fmt.Sprintf("ledger: cannot register nil projector for %q", kind))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.p[kind] = p
}

// Projector returns the projector registered for kind and whether it exists.
func (r *Registry) Projector(kind string) (Projector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.p[kind]
	return p, ok
}

// Kinds returns a sorted slice of all registered kinds.
func (r *Registry) Kinds() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.p))
	for k := range r.p {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DefaultRegistry returns the registry pre-populated with all event kinds
// the Themis core ledger uses.
//
// IMPORTANT: when adding a new event kind anywhere in the codebase, add
// its projector here AND extend internal/ledger/wiring_test.go to assert
// the kind is registered. Wiring tests prevent the default-case-eats-events
// bug class observed in adjacent systems (see VXD shared-learnings).
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register("TENANT_INITIALISED", noopProject)
	r.Register("LEDGER_REPLAYED", noopProject)
	r.Register("LEDGER_VERIFIED", noopProject)
	r.Register("CATALOGUE_SYNCED", noopProject)
	r.Register("IMPACT_CLASSIFIED", noopProject)
	r.Register("SCAN_FINDING", noopProject)
	r.Register("DECISION_ISSUED", noopProject)
	r.Register("POLICY_INVALID", noopProject)
	r.Register("BOM_BUILT", noopProject)
	r.Register("BOM_SIGNED", noopProject)
	r.Register("INGEST_COMPLETED", noopProject)
	r.Register("INGEST_ADAPTER_FAILED", noopProject)
	r.Register("APPROVAL_GRANTED", noopProject)
	r.Register("APPROVAL_DENIED", noopProject)
	r.Register("DECISION_FINALISED", noopProject)
	r.Register("EMERGENCY_OVERRIDE_INVOKED", noopProject)
	r.Register("OVERRIDE_POSTMORTEM_DUE", noopProject)
	r.Register("OVERRIDE_POSTMORTEM_CLOSED", noopProject)
	return r
}

// noopProject is a placeholder projector used for kinds whose projection
// is "just record the event" — the row already lands in events table via
// the generic projector path; no kind-specific work is needed.
func noopProject(_ []byte) error { return nil }
