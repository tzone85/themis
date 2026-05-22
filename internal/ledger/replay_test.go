package ledger

import (
	"os"
	"path/filepath"
	"strings"
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

func TestVerify_PassesOnUntamperedLedger(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "events.jsonl")
	s, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"TENANT_INITIALISED", "LEDGER_REPLAYED"} {
		if _, err := s.Append(newTestEvent(kind, s.LastHash())); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	if err := Verify(storePath); err != nil {
		t.Fatalf("Verify on untampered ledger: %v", err)
	}
}

func TestVerify_DetectsByteFlip(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "events.jsonl")
	s, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(newTestEvent("TENANT_INITIALISED", s.LastHash())); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(newTestEvent("LEDGER_REPLAYED", s.LastHash())); err != nil {
		t.Fatal(err)
	}
	s.Close()

	raw, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	raw[10] ^= 0x01
	if err := os.WriteFile(storePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	err = Verify(storePath)
	if err == nil {
		t.Fatal("Verify should have detected tampering")
	}
	if !strings.Contains(err.Error(), "chain") && !strings.Contains(err.Error(), "decode") {
		t.Fatalf("Verify error should mention chain or decode: %v", err)
	}
}
