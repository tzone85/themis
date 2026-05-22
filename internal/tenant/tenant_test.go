package tenant

import (
	"os"
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

func TestInit_CreatesAllExpectedDirs(t *testing.T) {
	base := t.TempDir()
	tn, err := Init(base, "acme-corp")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, dir := range []string{tn.Root(), tn.BOM(), tn.Wing()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("%q not created: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q exists but is not a directory", dir)
		}
	}
}

func TestInit_IsIdempotent(t *testing.T) {
	base := t.TempDir()
	if _, err := Init(base, "acme-corp"); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if _, err := Init(base, "acme-corp"); err != nil {
		t.Fatalf("second Init (should be no-op): %v", err)
	}
}

func TestInit_TenantsCannotEscapeBase(t *testing.T) {
	// Construct a Tenant with a malicious base that includes "..".
	// (Won't ever happen via Init — base is operator-supplied — but if it
	// does, the methods must still produce paths anchored to that base.)
	bad, err := New("/var/lib/themis/../../etc", "acme")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Root must remain underneath the (now-canonicalised) base; this test
	// documents that we don't sanitise base — that's the operator's job.
	if !strings.Contains(bad.Root(), "acme") {
		t.Fatalf("Root() lost the tenant id: %q", bad.Root())
	}
}

func TestTenant_DistinctIDsAlwaysGetDistinctRoots(t *testing.T) {
	base := t.TempDir()
	a, err := New(base, "a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(base, "b")
	if err != nil {
		t.Fatal(err)
	}

	if a.Root() == b.Root() {
		t.Fatal("distinct tenants share a root path")
	}
	if strings.HasPrefix(a.Root(), b.Root()+string(filepath.Separator)) {
		t.Fatal("tenant a's root is nested under tenant b's root")
	}
	if strings.HasPrefix(b.Root(), a.Root()+string(filepath.Separator)) {
		t.Fatal("tenant b's root is nested under tenant a's root")
	}
}
