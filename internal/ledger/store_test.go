package ledger

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"
)

func newTestEvent(kind string, prev string) Event {
    return Event{
        Kind:      kind,
        Tenant:    "test-tenant",
        Timestamp: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
        Payload:   json.RawMessage(`{}`),
        PrevHash:  prev,
    }
}

func TestStore_AppendAndRead(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "events.jsonl")

    s, err := OpenStore(path)
    if err != nil { t.Fatal(err) }
    defer s.Close()

    if got := s.LastHash(); got != ZeroHash {
        t.Fatalf("empty store LastHash() = %q, want %q", got, ZeroHash)
    }

    e1 := newTestEvent("A", s.LastHash())
    if _, err := s.Append(e1); err != nil { t.Fatal(err) }

    e2 := newTestEvent("B", s.LastHash())
    if _, err := s.Append(e2); err != nil { t.Fatal(err) }

    // Read back.
    events, err := ReadAll(path)
    if err != nil { t.Fatal(err) }
    if len(events) != 2 {
        t.Fatalf("ReadAll: got %d events, want 2", len(events))
    }
    if events[0].Kind != "A" || events[1].Kind != "B" {
        t.Fatalf("unexpected order: %v", events)
    }

    // The on-disk file is also valid JSONL (one JSON object per line).
    raw, err := os.ReadFile(path)
    if err != nil { t.Fatal(err) }
    if want := 2; bytesLines(raw) != want {
        t.Fatalf("file has %d lines, want %d", bytesLines(raw), want)
    }
}

func bytesLines(b []byte) int {
    n := 0
    for _, c := range b {
        if c == '\n' { n++ }
    }
    return n
}

func TestStore_RejectsAppendWithStaleChain(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "events.jsonl")
    s, err := OpenStore(path)
    if err != nil { t.Fatal(err) }
    defer s.Close()

    e1 := newTestEvent("A", s.LastHash())
    if _, err := s.Append(e1); err != nil { t.Fatal(err) }

    // Now try to append an event whose PrevHash is wrong.
    bad := newTestEvent("B", "WRONG_PREVIOUS_HASH")
    if _, err := s.Append(bad); err == nil {
        t.Fatal("Append accepted stale-chain event; should have rejected")
    }
}
