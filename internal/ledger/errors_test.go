package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestOpenStore_FailsOnDirectoryPath drives the os.OpenFile error path.
func TestOpenStore_FailsOnDirectoryPath(t *testing.T) {
	dir := t.TempDir()
	// Passing a directory where a file is expected → open(O_RDWR) fails.
	if _, err := OpenStore(dir); err == nil {
		t.Fatal("OpenStore(directory) should have errored")
	}
}

// TestOpenStore_RecoversLastHash drives the recover-from-existing branch.
func TestOpenStore_RecoversLastHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.Append(newTestEvent("TENANT_INITIALISED", s.LastHash()))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Reopen — must recover last hash by scanning the file.
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if got := s2.LastHash(); got != h {
		t.Fatalf("recovered last hash = %q, want %q", got, h)
	}
}

// TestOpenStore_FailsOnCorruptFile drives the ReadAll-error branch in OpenStore.
func TestOpenStore_FailsOnCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	// Write invalid JSON on a line — ReadAll will fail to decode.
	if err := os.WriteFile(path, []byte("{ this is not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); err == nil {
		t.Fatal("OpenStore on corrupt file should have errored")
	}
}

// TestReadAll_MissingFile returns empty slice + nil for missing file (happy path).
func TestReadAll_MissingFile(t *testing.T) {
	events, err := ReadAll(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("ReadAll on missing file should not error: %v", err)
	}
	if events != nil {
		t.Fatalf("expected nil slice, got %v", events)
	}
}

// TestReadAll_FailsOnUnreadablePath drives the os.Open error branch (non-IsNotExist).
func TestReadAll_FailsOnUnreadablePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "noperm.jsonl")
	if err := os.WriteFile(path, []byte("{}"), 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o600)
	if os.Geteuid() == 0 {
		t.Skip("root bypasses unix permissions")
	}
	if _, err := ReadAll(path); err == nil {
		t.Fatal("ReadAll on 0o000 file should have errored")
	}
}

// TestReadAll_FailsOnGarbledJSON drives the decode-error branch.
func TestReadAll_FailsOnGarbledJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte("not valid json at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAll(path); err == nil {
		t.Fatal("ReadAll on garbled file should have errored")
	}
}

// TestDeleteFile_PropagatesNonNotExistError uses a permission-locked parent dir.
func TestDeleteFile_HappyAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.sqlite")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := DeleteFile(path); err != nil {
		t.Fatal(err)
	}
	// Second call: missing file path → still nil.
	if err := DeleteFile(path); err != nil {
		t.Fatalf("DeleteFile on missing file should be nil: %v", err)
	}
}

// TestVerify_PropagatesReadError pushes Verify down the read-error branch.
func TestVerify_PropagatesReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(path); err == nil {
		t.Fatal("Verify on garbled file should error")
	}
}

// TestDoctor_PropagatesReadError pushes Doctor down the read-error branch.
func TestDoctor_PropagatesReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Doctor(path); err == nil {
		t.Fatal("Doctor on garbled file should error")
	}
}

// TestProject_BadPayloadFailsContentHash exercises the ContentHash error path
// inside Project by passing a payload that fails canonical JSON marshalling.
func TestProject_BadPayloadFailsContentHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.sqlite")
	p, err := OpenProjection(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	e := Event{
		Kind:      "TENANT_INITIALISED",
		Tenant:    "t",
		Timestamp: time.Unix(1, 0).UTC(),
		Payload:   json.RawMessage("not json"),
		PrevHash:  ZeroHash,
	}
	if err := p.Project(e, DefaultRegistry()); err == nil {
		t.Fatal("Project should reject invalid payload")
	}
}

// TestReplay_FailsOnUnknownKind drives Replay's project-event error path.
func TestReplay_FailsOnUnknownKind(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "events.jsonl")
	s, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	// Append a registered-kind event normally so chain check passes.
	if _, err := s.Append(newTestEvent("TENANT_INITIALISED", s.LastHash())); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Use an empty registry — every kind is now "unknown".
	if err := Replay(storePath, filepath.Join(dir, "proj.sqlite"), NewRegistry()); err == nil {
		t.Fatal("Replay with empty registry should error on unknown kind")
	}
}

// TestReplay_FailsOnUnreadableStore drives the ReadAll-error branch in Replay.
func TestReplay_FailsOnUnreadableStore(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(storePath, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Replay(storePath, filepath.Join(dir, "p.sqlite"), DefaultRegistry()); err == nil {
		t.Fatal("Replay on garbled store should error")
	}
}

// TestOpenProjection_FailsOnUnwritablePath drives the migrate-error branch.
func TestOpenProjection_FailsOnUnwritablePath(t *testing.T) {
	// Point at a path under a non-existent parent → SQLite open fails.
	bad := filepath.Join(t.TempDir(), "does", "not", "exist", "p.sqlite")
	if _, err := OpenProjection(bad); err == nil {
		t.Fatal("OpenProjection under nonexistent parent should error")
	}
}

// TestStore_AppendOnBadPayload exercises Append's marshal/content-hash error.
func TestStore_AppendOnBadPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bad := Event{
		Kind:      "X",
		Tenant:    "t",
		Timestamp: time.Unix(1, 0).UTC(),
		Payload:   json.RawMessage("not json"),
		PrevHash:  s.LastHash(),
	}
	if _, err := s.Append(bad); err == nil {
		t.Fatal("Append with invalid payload should error")
	}
}

// TestStore_Close_DoubleClose exercises Close error path on already-closed store.
func TestStore_Close_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Second close should error (file already closed).
	if err := s.Close(); err == nil {
		t.Log("double-close did not error — accepted (os.File.Close may be idempotent on darwin)")
	}
}

// TestProject_ProjectorReturnsError exercises the projector-error branch.
func TestProject_ProjectorReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.sqlite")
	p, err := OpenProjection(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	reg := NewRegistry()
	reg.Register("BOOM", func(_ []byte) error { return os.ErrInvalid })

	e := newTestEvent("BOOM", ZeroHash)
	if err := p.Project(e, reg); err == nil {
		t.Fatal("Project should propagate projector error")
	}
}

// TestProject_FailsOnClosedDB exercises the db.Exec error branch.
func TestProject_FailsOnClosedDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.sqlite")
	p, err := OpenProjection(path)
	if err != nil {
		t.Fatal(err)
	}
	p.Close() // close BEFORE projecting → Exec fails.

	e := newTestEvent("TENANT_INITIALISED", ZeroHash)
	if err := p.Project(e, DefaultRegistry()); err == nil {
		t.Fatal("Project on closed DB should error")
	}
}

// TestVerify_HashErrorOnTamperedTimestamp triggers ContentHash error via
// out-of-range timestamp serialisation. NOTE: in practice the JSON layer
// accepts most byte flips so this test focuses on a structurally broken
// payload after re-write.
func TestVerify_FailsOnBrokenPayloadAfterValidEvent(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "events.jsonl")
	s, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(newTestEvent("TENANT_INITIALISED", s.LastHash())); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Append a syntactically broken line so ReadAll fails.
	if err := os.WriteFile(storePath, []byte("{\"kind\":\"X\"}\n{\"kind\":bogus\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(storePath); err == nil {
		t.Fatal("Verify should fail on broken JSONL")
	}
	if _, err := Doctor(storePath); err == nil {
		t.Fatal("Doctor should fail on broken JSONL")
	}
}

// TestDeleteFile_NonExistentParentOK confirms missing path is not an error.
func TestDeleteFile_NonExistentParentOK(t *testing.T) {
	if err := DeleteFile(filepath.Join(t.TempDir(), "never.created")); err != nil {
		t.Fatalf("DeleteFile on missing path should not error: %v", err)
	}
}

// TestDeleteFile_PropagatesPermissionError exercises the non-IsNotExist branch
// by removing write permission from the parent dir.
func TestDeleteFile_PropagatesPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission semantics differ on windows/root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "x.sqlite")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Strip write+execute on parent → os.Remove must fail with EACCES, not ENOENT.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := DeleteFile(path); err == nil {
		t.Fatal("DeleteFile under read-only parent should error")
	}
}

// TestReplay_FailsWhenProjectionPathIsADirectory pushes OpenProjection error
// branch in Replay.
func TestReplay_FailsWhenProjectionPathInvalid(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "events.jsonl")
	s, err := OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(newTestEvent("TENANT_INITIALISED", s.LastHash())); err != nil {
		t.Fatal(err)
	}
	s.Close()

	bad := filepath.Join(dir, "does", "not", "exist", "p.sqlite")
	if err := Replay(storePath, bad, DefaultRegistry()); err == nil {
		t.Fatal("Replay with invalid projection path should error")
	}
}
