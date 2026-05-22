package tenant

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTenant_PathsAreScopedToBase(t *testing.T) {
	base := t.TempDir()
	tn := Tenant{ID: "acme-corp", Base: base}

	want := filepath.Join(base, "tenants", "acme-corp")
	if got := tn.Root(); got != want {
		t.Fatalf("Root() = %q, want %q", got, want)
	}
	if !strings.HasPrefix(tn.Events(), tn.Root()) {
		t.Fatalf("Events() %q not under Root() %q", tn.Events(), tn.Root())
	}
	if !strings.HasPrefix(tn.Projection(), tn.Root()) {
		t.Fatalf("Projection() %q not under Root() %q", tn.Projection(), tn.Root())
	}
}

func TestTenant_RejectsEmptyID(t *testing.T) {
	if _, err := New(t.TempDir(), ""); err == nil {
		t.Fatal("New with empty ID should error")
	}
}

func TestTenant_RejectsTraversalID(t *testing.T) {
	base := t.TempDir()
	bad := []string{"../escape", "..", "a/b", "a\\b", "."}
	for _, id := range bad {
		if _, err := New(base, id); err == nil {
			t.Errorf("New(%q) should error; ID contains illegal characters", id)
		}
	}
}
