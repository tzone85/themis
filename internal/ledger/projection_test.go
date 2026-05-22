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

func TestProjection_ProjectInsertsEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.sqlite")
	p, err := OpenProjection(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	e := newTestEvent("TENANT_INITIALISED", ZeroHash)
	if err := p.Project(e, DefaultRegistry()); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := p.DB().QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got %d rows, want 1", count)
	}
}

func TestProjection_ProjectIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.sqlite")
	p, err := OpenProjection(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	e := newTestEvent("TENANT_INITIALISED", ZeroHash)
	if err := p.Project(e, DefaultRegistry()); err != nil {
		t.Fatal(err)
	}
	if err := p.Project(e, DefaultRegistry()); err != nil {
		t.Fatalf("second Project (should be idempotent): %v", err)
	}

	var count int
	if err := p.DB().QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got %d rows after second Project, want 1", count)
	}
}

func TestProjection_RefusesUnknownKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.sqlite")
	p, err := OpenProjection(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	e := newTestEvent("DEFINITELY_NOT_REGISTERED", ZeroHash)
	if err := p.Project(e, DefaultRegistry()); err == nil {
		t.Fatal("Project accepted unknown kind; should have failed")
	}
}
