package ingest

import "github.com/tzone85/themis/internal/aichange"

// Manual adapter is filled in by Plan 5 / T4.
type Manual struct{}

// Name returns the adapter's stable name.
func (m *Manual) Name() string { return "manual_attestation" }

// Ingest is implemented in Plan 5 / T4.
func (m *Manual) Ingest(_ Inputs) (aichange.AIChange, error) {
	return aichange.AIChange{}, ErrAdapterFailed
}
