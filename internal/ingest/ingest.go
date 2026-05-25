// Package ingest defines the Adapter interface every ingestion source
// implements, plus concrete adapters for the inputs Plan 5 supports:
// git_heuristic, claude_code_transcript, manual_attestation.
//
// An adapter's job is to produce a normalised aichange.AIChange from
// whatever source-specific format it knows how to read. Downstream
// pipeline stages (classify, scan, decide, bom) never see the source
// format, only the AIChange — which is what keeps the core resilient
// to AI tool churn (design spec §5.1).
package ingest

import (
	"errors"

	"github.com/tzone85/themis/internal/aichange"
)

// ErrAdapterFailed is wrapped by every Ingest error. The decide-pipeline
// CLI uses errors.Is to route adapter failures to a single
// INGEST_ADAPTER_FAILED ledger event.
var ErrAdapterFailed = errors.New("ingest: adapter failed")

// Inputs is the union of fields an adapter may need. Each adapter takes
// only the subset it cares about; passing the same struct to all of them
// keeps the CLI surface uniform.
type Inputs struct {
	// PRID is the upstream pull-request identifier. Required.
	PRID string

	// ActorOverride lets the operator specify an actor when the adapter
	// can't infer one. The manual adapter requires it; git_heuristic uses
	// it as a fallback when the commit author can't be resolved.
	ActorOverride string

	// Workdir is the local checkout from which adapters resolve file
	// content for hashing.
	Workdir string

	// BaseRef (git_heuristic only) is the ref to diff against — "main" or
	// a commit SHA. Empty defaults to "HEAD~1" inside the adapter.
	BaseRef string

	// TranscriptPath (claude_code_transcript only) is the path to the
	// Claude Code session JSON export.
	TranscriptPath string

	// Files (manual_attestation only) maps PR path → [beforeHash, afterHash]
	// declared explicitly by the operator.
	Files map[string][2]string

	// Extra carries adapter-specific key/value pairs. Reserved for later
	// adapters (vxd, cursor, copilot) to avoid expanding this struct
	// every time one lands.
	Extra map[string]string
}

// Adapter is the contract every ingestion source implements.
type Adapter interface {
	Name() string
	Ingest(in Inputs) (aichange.AIChange, error)
}

// Resolve returns the adapter with the given name, or false if no such
// adapter is registered.
func Resolve(name string) (Adapter, bool) {
	switch name {
	case (&GitHeuristic{}).Name():
		return &GitHeuristic{}, true
	case (&ClaudeCodeTranscript{}).Name():
		return &ClaudeCodeTranscript{}, true
	case (&Manual{}).Name():
		return &Manual{}, true
	}
	return nil, false
}

// All returns the registered adapter names — used by the CLI to print
// `--adapter` help text and by tests to verify every adapter is listed.
func All() []string {
	return []string{
		(&GitHeuristic{}).Name(),
		(&ClaudeCodeTranscript{}).Name(),
		(&Manual{}).Name(),
	}
}
