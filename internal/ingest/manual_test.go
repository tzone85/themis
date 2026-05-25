package ingest

import (
	"errors"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
)

func TestManual_HappyPath(t *testing.T) {
	m := &Manual{}
	got, err := m.Ingest(Inputs{
		PRID:          "gh:test#manual-1",
		ActorOverride: "human:thandi",
		Files: map[string][2]string{
			"src/a.go":  {"deadbeef", "feedface"},
			"src/b.go":  {"", "cafef00d"},
			"docs/x.md": {"abc", ""},
		},
		Extra: map[string]string{"reason": "retroactive PR"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.PRID != "gh:test#manual-1" || got.Actor != "human:thandi" {
		t.Fatalf("identity wrong: %+v", got)
	}
	if len(got.TouchedFiles) != 3 {
		t.Fatalf("files = %d, want 3", len(got.TouchedFiles))
	}
	// Sorted order: docs/x.md < src/a.go < src/b.go.
	if got.TouchedFiles[0].Path != "docs/x.md" || got.TouchedFiles[1].Path != "src/a.go" || got.TouchedFiles[2].Path != "src/b.go" {
		t.Fatalf("paths not sorted: %+v", got.TouchedFiles)
	}
	// Kind inference.
	if got.TouchedFiles[0].ChangeKind != aichange.FileDeleted {
		t.Errorf("docs/x.md kind = %q, want DELETED", got.TouchedFiles[0].ChangeKind)
	}
	if got.TouchedFiles[1].ChangeKind != aichange.FileModified {
		t.Errorf("src/a.go kind = %q, want MODIFIED", got.TouchedFiles[1].ChangeKind)
	}
	if got.TouchedFiles[2].ChangeKind != aichange.FileAdded {
		t.Errorf("src/b.go kind = %q, want ADDED", got.TouchedFiles[2].ChangeKind)
	}
}

func TestManual_DeterministicAcrossCalls(t *testing.T) {
	in := Inputs{
		PRID:          "gh:test#det",
		ActorOverride: "human:x",
		Files: map[string][2]string{
			"z.go": {"", "1"},
			"a.go": {"", "2"},
			"m.go": {"", "3"},
		},
	}
	m := &Manual{}
	a, _ := m.Ingest(in)
	b, _ := m.Ingest(in)
	for i := range a.TouchedFiles {
		if a.TouchedFiles[i] != b.TouchedFiles[i] {
			t.Fatalf("file[%d] differs across calls", i)
		}
	}
}

func TestManual_RequiresPRID(t *testing.T) {
	_, err := (&Manual{}).Ingest(Inputs{ActorOverride: "human:x", Files: map[string][2]string{"a": {"", "1"}}})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("missing prid should ErrAdapterFailed, got %v", err)
	}
}

func TestManual_RequiresHumanActor(t *testing.T) {
	_, err := (&Manual{}).Ingest(Inputs{PRID: "x", ActorOverride: "claude_code", Files: map[string][2]string{"a": {"", "1"}}})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("non-human actor should ErrAdapterFailed, got %v", err)
	}
}

func TestManual_RequiresAtLeastOneFile(t *testing.T) {
	_, err := (&Manual{}).Ingest(Inputs{PRID: "x", ActorOverride: "human:x"})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("empty files should ErrAdapterFailed, got %v", err)
	}
}
