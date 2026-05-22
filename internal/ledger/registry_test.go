package ledger

import "testing"

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register("FOO", func(payload []byte) error { return nil })

	if _, ok := r.Projector("FOO"); !ok {
		t.Fatal("Projector(FOO) not found after Register")
	}
	if _, ok := r.Projector("MISSING"); ok {
		t.Fatal("Projector(MISSING) should not be found")
	}
}

func TestRegistry_KindsListsAll(t *testing.T) {
	r := NewRegistry()
	r.Register("A", noopProjector)
	r.Register("B", noopProjector)
	r.Register("C", noopProjector)
	got := r.Kinds()
	if len(got) != 3 {
		t.Fatalf("Kinds returned %d items: %v", len(got), got)
	}
}

func TestRegistry_RegisterEmptyKindPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Register(\"\") should panic")
		}
	}()
	NewRegistry().Register("", noopProjector)
}

func TestRegistry_RegisterNilProjectorPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Register(nil projector) should panic")
		}
	}()
	NewRegistry().Register("X", nil)
}

func noopProjector(_ []byte) error { return nil }
