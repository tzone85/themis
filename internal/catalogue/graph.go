// Package catalogue parses an EventCatalog repository tree into an
// in-memory CatalogueGraph. The graph is read-only at Plan 2; later
// plans may add catalogue-update PR proposals.
package catalogue

import "time"

// Domain groups related services and events.
type Domain struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Services []ServiceRef `json:"services,omitempty"`
}

// ServiceRef is a lightweight pointer used inside Domain.Services to
// avoid duplicating full Service structs in the graph.
type ServiceRef struct {
	ID string `json:"id"`
}

// EventRef is a lightweight pointer used inside Service.Produces/Consumes.
type EventRef struct {
	ID string `json:"id"`
}

// Service is one logical service. SourcePath is the path (relative to the
// catalogue root) of its index.md so the classifier can map touched files
// back to a Service.
type Service struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Domain     string     `json:"domain"`
	Produces   []EventRef `json:"produces,omitempty"`
	Consumes   []EventRef `json:"consumes,omitempty"`
	SourcePath string     `json:"source_path"`
}

// EventDef defines an event in the catalogue. SchemaPath is the relative
// path to the AsyncAPI/JSON-Schema document describing the event payload.
type EventDef struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Domain     string   `json:"domain"`
	SchemaPath string   `json:"schema_path,omitempty"`
	Owners     []string `json:"owners,omitempty"`
	Version    string   `json:"version,omitempty"`
}

// CatalogueGraph is the in-memory representation of an EventCatalog tree.
//
// The maps are keyed by stable IDs (domain id, service id, event id) — these
// IDs are what AIChange.TouchedFiles paths map onto.
type CatalogueGraph struct {
	Domains     map[string]Domain   `json:"domains"`
	Services    map[string]Service  `json:"services"`
	Events      map[string]EventDef `json:"events"`
	SourceRoot  string              `json:"source_root"`
	SyncedAt    time.Time           `json:"synced_at"`
	ContentHash string              `json:"content_hash"`
}

// ConsumersOf returns every Service that lists eventID under Consumes.
// Order is deterministic (services sorted by ID).
func (g CatalogueGraph) ConsumersOf(eventID string) []Service {
	out := make([]Service, 0)
	for _, id := range sortedKeys(g.Services) {
		s := g.Services[id]
		for _, ref := range s.Consumes {
			if ref.ID == eventID {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// ProducerOf returns the (single) producing service for eventID, if any.
// If multiple services claim production, the first by sorted ID wins —
// catalogues with multiple producers per event are themselves a smell that
// will be flagged in a later plan via a structural lint.
func (g CatalogueGraph) ProducerOf(eventID string) (Service, bool) {
	for _, id := range sortedKeys(g.Services) {
		s := g.Services[id]
		for _, ref := range s.Produces {
			if ref.ID == eventID {
				return s, true
			}
		}
	}
	return Service{}, false
}

// DomainOfService returns the Domain that owns serviceID, if any.
func (g CatalogueGraph) DomainOfService(serviceID string) (Domain, bool) {
	s, ok := g.Services[serviceID]
	if !ok {
		return Domain{}, false
	}
	d, ok := g.Domains[s.Domain]
	return d, ok
}
