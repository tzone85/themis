package aichange

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestFileChangeKind_Valid(t *testing.T) {
	for _, k := range []FileChangeKind{FileAdded, FileModified, FileDeleted} {
		if !k.Valid() {
			t.Errorf("%q should be Valid()", k)
		}
	}
	if FileChangeKind("PHANTOM").Valid() {
		t.Error("unknown kind should not be Valid()")
	}
}

func TestAIChange_RoundTrip(t *testing.T) {
	in := AIChange{
		PRID:  "gh:tzone85/themis#42",
		Actor: "claude_code",
		TouchedFiles: []FileTouch{
			{Path: "internal/x.go", ChangeKind: FileModified, BeforeHash: "deadbeef", AfterHash: "feedface"},
			{Path: "docs/y.md", ChangeKind: FileAdded, AfterHash: "cafef00d"},
		},
		RawTranscriptHash: "abc123",
		Metadata:          map[string]string{"claude_code:model": "claude-sonnet-4-6"},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out AIChange
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.PRID != in.PRID || out.Actor != in.Actor || len(out.TouchedFiles) != 2 {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	if out.TouchedFiles[0].Path != "internal/x.go" {
		t.Errorf("file[0].Path = %q", out.TouchedFiles[0].Path)
	}
	if out.Metadata["claude_code:model"] != "claude-sonnet-4-6" {
		t.Errorf("metadata lost: %v", out.Metadata)
	}
}

func TestAIChange_EmptyTouchedFilesMarshalsAsArrayNotNull(t *testing.T) {
	in := AIChange{PRID: "x", Actor: "y"}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatal(err)
	}
	tf, ok := probe["touched_files"]
	if !ok {
		t.Fatal("touched_files missing from JSON output")
	}
	if tf == nil {
		t.Fatal("touched_files emitted as null; should be [] for nil-safe consumers")
	}
}

func TestAIChange_Validate_PassesOnEmpty(t *testing.T) {
	if err := (AIChange{}).Validate(); err != nil {
		t.Fatalf("empty AIChange should be valid: %v", err)
	}
}

func TestAIChange_Validate_RejectsBadChangeKind(t *testing.T) {
	c := AIChange{
		TouchedFiles: []FileTouch{
			{Path: "a.go", ChangeKind: FileChangeKind("WUT")},
		},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("Validate should reject unknown ChangeKind")
	}
	if !errors.Is(err, ErrInvalidChangeKind) {
		t.Fatalf("error %v should wrap ErrInvalidChangeKind", err)
	}
}
