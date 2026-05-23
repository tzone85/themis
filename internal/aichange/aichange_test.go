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
	// Error message must surface both the index and the bad kind so an
	// operator can find the offending row in a long PR diff.
	msg := err.Error()
	if !contains(msg, "TouchedFiles[0]") || !contains(msg, "WUT") {
		t.Fatalf("error %q should mention index and kind", msg)
	}
}

func TestAIChange_Validate_RejectsBadChangeKind_AtIndex(t *testing.T) {
	c := AIChange{
		TouchedFiles: []FileTouch{
			{Path: "ok.go", ChangeKind: FileAdded},
			{Path: "also-ok.go", ChangeKind: FileModified},
			{Path: "broken.go", ChangeKind: FileChangeKind("nope")},
		},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("Validate should reject")
	}
	if !contains(err.Error(), "TouchedFiles[2]") {
		t.Fatalf("error %q should reference index 2", err.Error())
	}
}

// contains is a strings.Contains shim used only by tests in this file so
// the production package doesn't pull in the strings dep just for errors.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
