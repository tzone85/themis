package ingest

import "github.com/tzone85/themis/internal/aichange"

// GitHeuristic adapter is filled in by Plan 5 / T2.
type GitHeuristic struct{}

// Name returns the adapter's stable name.
func (g *GitHeuristic) Name() string { return "git_heuristic" }

// Ingest is implemented in Plan 5 / T2.
func (g *GitHeuristic) Ingest(_ Inputs) (aichange.AIChange, error) {
	return aichange.AIChange{}, ErrAdapterFailed
}
