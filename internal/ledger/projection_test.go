package ledger

import (
	"path/filepath"
	"testing"
)

func TestProjection_OpenCreatesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.sqlite")
	p, err := OpenProjection(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	rows, err := p.DB().Query("SELECT count(*) FROM events")
	if err != nil {
		t.Fatalf("events table missing: %v", err)
	}
	rows.Close()
}

func TestProjection_WALModeEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.sqlite")
	p, err := OpenProjection(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	var mode string
	if err := p.DB().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want %q", mode, "wal")
	}
}
