package ledger

import (
	"path/filepath"
	"testing"
)

func TestReplay_ReproducesProjection(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "events.jsonl")
	projPath := filepath.Join(dir, "projection.sqlite")

	s, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := OpenProjection(projPath)
	if err != nil {
		t.Fatal(err)
	}
	reg := DefaultRegistry()

	for _, kind := range []string{"TENANT_INITIALISED", "LEDGER_REPLAYED", "LEDGER_VERIFIED"} {
		e := newTestEvent(kind, s.LastHash())
		if _, err := s.Append(e); err != nil {
			t.Fatal(err)
		}
		if err := p.Project(e, reg); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	p.Close()

	if err := DeleteFile(projPath); err != nil {
		t.Fatal(err)
	}
	if err := Replay(storePath, projPath, reg); err != nil {
		t.Fatal(err)
	}

	p2, err := OpenProjection(projPath)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()

	var n int
	if err := p2.DB().QueryRow("SELECT count(*) FROM events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("after replay: %d rows, want 3", n)
	}
}
