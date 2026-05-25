package ingest

import "testing"

func TestResolve_KnownAdapters(t *testing.T) {
	for _, name := range []string{"git_heuristic", "claude_code_transcript", "manual_attestation"} {
		a, ok := Resolve(name)
		if !ok {
			t.Errorf("Resolve(%q) not found", name)
		}
		if a != nil && a.Name() != name {
			t.Errorf("Resolve(%q) returned adapter %q", name, a.Name())
		}
	}
}

func TestResolve_UnknownReturnsFalse(t *testing.T) {
	if _, ok := Resolve("phantom_adapter"); ok {
		t.Fatal("Resolve should return false for unknown adapter")
	}
}

func TestAll_ListsThreeAdapters(t *testing.T) {
	names := All()
	if len(names) != 3 {
		t.Fatalf("All() returned %d adapters, want 3: %v", len(names), names)
	}
}
