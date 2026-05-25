package ingest

import "github.com/tzone85/themis/internal/aichange"

// ClaudeCodeTranscript adapter is filled in by Plan 5 / T3.
type ClaudeCodeTranscript struct{}

// Name returns the adapter's stable name.
func (c *ClaudeCodeTranscript) Name() string { return "claude_code_transcript" }

// Ingest is implemented in Plan 5 / T3.
func (c *ClaudeCodeTranscript) Ingest(_ Inputs) (aichange.AIChange, error) {
	return aichange.AIChange{}, ErrAdapterFailed
}
