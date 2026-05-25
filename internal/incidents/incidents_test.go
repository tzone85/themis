package incidents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppend_CreatesFileAndAppends(t *testing.T) {
	base := t.TempDir()
	if err := Append(base, "acme", "LEDGER_INTEGRITY_BROKEN", json.RawMessage(`{"err":"chain break at 7"}`)); err != nil {
		t.Fatal(err)
	}
	if err := Append(base, "acme", "ENFORCEMENT_MISSING", json.RawMessage(`{"repo":"x/y"}`)); err != nil {
		t.Fatal(err)
	}

	recs, err := ReadAll(base, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("len = %d, want 2", len(recs))
	}
	if recs[0].Kind != "LEDGER_INTEGRITY_BROKEN" || recs[1].Kind != "ENFORCEMENT_MISSING" {
		t.Fatalf("order wrong: %+v", recs)
	}

	// File mode is 0o600.
	info, err := os.Stat(Path(base, "acme"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("incidents.jsonl perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestReadAll_MissingFileReturnsEmpty(t *testing.T) {
	recs, err := ReadAll(t.TempDir(), "no-such-tenant")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected empty, got %+v", recs)
	}
}

func TestAppend_RejectsEmptyKind(t *testing.T) {
	if err := Append(t.TempDir(), "acme", "", nil); err == nil {
		t.Fatal("empty kind should error")
	}
}

func TestAppend_NilPayloadBecomesEmptyObject(t *testing.T) {
	base := t.TempDir()
	if err := Append(base, "acme", "LEDGER_ANCHOR", nil); err != nil {
		t.Fatal(err)
	}
	recs, _ := ReadAll(base, "acme")
	if string(recs[0].Payload) != "{}" {
		t.Fatalf("payload = %q, want {}", recs[0].Payload)
	}
}

func TestReadAll_RejectsCorruptLine(t *testing.T) {
	base := t.TempDir()
	if err := Append(base, "acme", "X", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	// Tamper: append non-JSON.
	f, _ := os.OpenFile(Path(base, "acme"), os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString("garbage not json\n")
	_ = f.Close()
	if _, err := ReadAll(base, "acme"); err == nil {
		t.Fatal("corrupt line should error")
	}
}

func TestPath_ScopedPerTenant(t *testing.T) {
	a := Path("/base", "alpha")
	b := Path("/base", "beta")
	if a == b {
		t.Fatalf("paths must differ per tenant")
	}
	if filepath.Base(a) != FileName {
		t.Fatalf("Path basename = %q, want %q", filepath.Base(a), FileName)
	}
}
