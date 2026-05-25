package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/tzone85/themis/internal/aichange"
)

// ClaudeCodeTranscript ingests an exported Claude Code session JSON.
//
// Expected JSON shape (deliberately minimal — we'll extend as the Claude
// Code transcript format stabilises):
//
//	{
//	  "session_id": "...",
//	  "model": "claude-sonnet-4-6",
//	  "user": "thandi",
//	  "edits": [
//	    {"path": "src/x.go", "before_hash": "...", "after_hash": "..."},
//	    ...
//	  ]
//	}
type ClaudeCodeTranscript struct{}

// Name returns the adapter's stable name.
func (c *ClaudeCodeTranscript) Name() string { return "claude_code_transcript" }

type claudeTranscript struct {
	SessionID string         `json:"session_id"`
	Model     string         `json:"model"`
	User      string         `json:"user"`
	Edits     []claudeEdit   `json:"edits"`
}

type claudeEdit struct {
	Path       string `json:"path"`
	BeforeHash string `json:"before_hash"`
	AfterHash  string `json:"after_hash"`
}

// Ingest reads the transcript at in.TranscriptPath and produces an AIChange.
func (c *ClaudeCodeTranscript) Ingest(in Inputs) (aichange.AIChange, error) {
	if in.PRID == "" {
		return aichange.AIChange{}, fmt.Errorf("%w: claude_code_transcript requires --pr-id", ErrAdapterFailed)
	}
	if in.TranscriptPath == "" {
		return aichange.AIChange{}, fmt.Errorf("%w: claude_code_transcript requires --transcript", ErrAdapterFailed)
	}
	raw, err := os.ReadFile(in.TranscriptPath) // #nosec G304 -- operator-supplied transcript path.
	if err != nil {
		return aichange.AIChange{}, fmt.Errorf("%w: read transcript: %v", ErrAdapterFailed, err)
	}
	var t claudeTranscript
	if err := json.Unmarshal(raw, &t); err != nil {
		return aichange.AIChange{}, fmt.Errorf("%w: parse transcript: %v", ErrAdapterFailed, err)
	}
	if len(t.Edits) == 0 {
		return aichange.AIChange{}, fmt.Errorf("%w: transcript has no edits", ErrAdapterFailed)
	}

	// Sort edits for determinism.
	sort.SliceStable(t.Edits, func(i, j int) bool { return t.Edits[i].Path < t.Edits[j].Path })

	touches := make([]aichange.FileTouch, 0, len(t.Edits))
	for _, e := range t.Edits {
		touches = append(touches, aichange.FileTouch{
			Path:       e.Path,
			ChangeKind: inferChangeKind(e.BeforeHash, e.AfterHash),
			BeforeHash: e.BeforeHash,
			AfterHash:  e.AfterHash,
		})
	}

	sum := sha256.Sum256(raw)
	out := aichange.AIChange{
		PRID:              in.PRID,
		Actor:             "claude_code",
		TouchedFiles:      touches,
		RawTranscriptHash: hex.EncodeToString(sum[:]),
		Metadata: map[string]string{
			"claude_code:session_id": t.SessionID,
			"claude_code:model":      t.Model,
			"claude_code:user":       t.User,
		},
	}
	return out, nil
}
