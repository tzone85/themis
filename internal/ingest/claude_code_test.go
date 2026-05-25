package ingest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
)

func writeTranscript(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "transcript.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClaudeCode_HappyPath(t *testing.T) {
	dir := t.TempDir()
	p := writeTranscript(t, dir, `{
  "session_id": "sess-123",
  "model": "claude-sonnet-4-6",
  "user": "thandi",
  "edits": [
    {"path": "src/b.go", "before_hash": "old1", "after_hash": "new1"},
    {"path": "src/a.go", "before_hash": "", "after_hash": "new2"}
  ]
}`)
	got, err := (&ClaudeCodeTranscript{}).Ingest(Inputs{PRID: "gh:test#cc-1", TranscriptPath: p})
	if err != nil {
		t.Fatal(err)
	}
	if got.Actor != "claude_code" {
		t.Errorf("actor = %q", got.Actor)
	}
	if got.RawTranscriptHash == "" || len(got.RawTranscriptHash) != 64 {
		t.Errorf("RawTranscriptHash wrong: %q", got.RawTranscriptHash)
	}
	if len(got.TouchedFiles) != 2 || got.TouchedFiles[0].Path != "src/a.go" {
		t.Fatalf("files not sorted: %+v", got.TouchedFiles)
	}
	if got.TouchedFiles[0].ChangeKind != aichange.FileAdded {
		t.Errorf("src/a.go kind = %q, want ADDED", got.TouchedFiles[0].ChangeKind)
	}
	if got.Metadata["claude_code:model"] != "claude-sonnet-4-6" {
		t.Errorf("metadata model = %q", got.Metadata["claude_code:model"])
	}
}

func TestClaudeCode_RequiresPRID(t *testing.T) {
	p := writeTranscript(t, t.TempDir(), `{"edits":[{"path":"x","after_hash":"y"}]}`)
	_, err := (&ClaudeCodeTranscript{}).Ingest(Inputs{TranscriptPath: p})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("missing prid should ErrAdapterFailed, got %v", err)
	}
}

func TestClaudeCode_RequiresTranscriptPath(t *testing.T) {
	_, err := (&ClaudeCodeTranscript{}).Ingest(Inputs{PRID: "x"})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("missing transcript should ErrAdapterFailed, got %v", err)
	}
}

func TestClaudeCode_RejectsMissingTranscriptFile(t *testing.T) {
	_, err := (&ClaudeCodeTranscript{}).Ingest(Inputs{PRID: "x", TranscriptPath: filepath.Join(t.TempDir(), "no-such.json")})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("missing file should ErrAdapterFailed, got %v", err)
	}
}

func TestClaudeCode_RejectsMalformedJSON(t *testing.T) {
	p := writeTranscript(t, t.TempDir(), "not json at all")
	_, err := (&ClaudeCodeTranscript{}).Ingest(Inputs{PRID: "x", TranscriptPath: p})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("garbled json should ErrAdapterFailed, got %v", err)
	}
}

func TestClaudeCode_RejectsEmptyEdits(t *testing.T) {
	p := writeTranscript(t, t.TempDir(), `{"session_id":"s","edits":[]}`)
	_, err := (&ClaudeCodeTranscript{}).Ingest(Inputs{PRID: "x", TranscriptPath: p})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("empty edits should ErrAdapterFailed, got %v", err)
	}
}

func TestClaudeCode_DeterministicHash(t *testing.T) {
	p := writeTranscript(t, t.TempDir(), `{"edits":[{"path":"x","after_hash":"y"}]}`)
	a, err := (&ClaudeCodeTranscript{}).Ingest(Inputs{PRID: "x", TranscriptPath: p})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := (&ClaudeCodeTranscript{}).Ingest(Inputs{PRID: "x", TranscriptPath: p})
	if a.RawTranscriptHash != b.RawTranscriptHash {
		t.Fatal("RawTranscriptHash should be stable for same input")
	}
}
