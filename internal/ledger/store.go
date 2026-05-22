package ledger

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "sync"
)

// Store is an append-only JSONL event store. The file is the authoritative
// source of truth; the SQLite projection is rebuildable from it.
//
// Store is safe for use by ONE writer goroutine. Multiple readers may call
// ReadAll concurrently provided no concurrent Append is in flight on the
// same path.
type Store struct {
    path string
    f    *os.File
    bw   *bufio.Writer
    last string // hex of last event's ContentHash
    mu   sync.Mutex
}

// OpenStore opens (creates if missing) a JSONL store at path. It scans
// the existing file (if any) to recover LastHash.
func OpenStore(path string) (*Store, error) {
    f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
    if err != nil {
        return nil, fmt.Errorf("open %s: %w", path, err)
    }
    s := &Store{path: path, f: f, bw: bufio.NewWriter(f), last: ZeroHash}

    // Recover last hash by scanning existing content.
    events, err := ReadAll(path)
    if err != nil {
        _ = f.Close()
        return nil, fmt.Errorf("recover %s: %w", path, err)
    }
    if len(events) > 0 {
        h, err := events[len(events)-1].ContentHash()
        if err != nil {
            _ = f.Close()
            return nil, fmt.Errorf("compute last hash for %s: %w", path, err)
        }
        s.last = h
    }
    return s, nil
}

// LastHash returns the content hash of the most recently appended event,
// or ZeroHash if the store is empty.
func (s *Store) LastHash() string {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.last
}

// Append serialises e and writes it as one JSONL line. It returns the
// content hash of the appended event.
//
// Append is fsync-on-every-write. Throughput is intentionally bounded by
// disk fsync latency; the audit story requires durability before reporting
// success.
func (s *Store) Append(e Event) (string, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    if e.PrevHash != s.last {
        return "", fmt.Errorf("chain mismatch: event.PrevHash=%q, store.LastHash=%q", e.PrevHash, s.last)
    }
    line, err := json.Marshal(e)
    if err != nil {
        return "", fmt.Errorf("marshal event: %w", err)
    }
    if _, err := s.bw.Write(line); err != nil {
        return "", fmt.Errorf("write event: %w", err)
    }
    if err := s.bw.WriteByte('\n'); err != nil {
        return "", fmt.Errorf("write newline: %w", err)
    }
    if err := s.bw.Flush(); err != nil {
        return "", fmt.Errorf("flush: %w", err)
    }
    if err := s.f.Sync(); err != nil {
        return "", fmt.Errorf("fsync: %w", err)
    }
    h, err := e.ContentHash()
    if err != nil {
        return "", fmt.Errorf("content hash: %w", err)
    }
    s.last = h
    return h, nil
}

// Close flushes and closes the underlying file.
func (s *Store) Close() error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.bw != nil {
        if err := s.bw.Flush(); err != nil { return err }
    }
    if s.f != nil {
        return s.f.Close()
    }
    return nil
}

// ReadAll reads the entire JSONL file from path and returns the events in
// file order. Empty / missing file = empty slice, nil error.
func ReadAll(path string) ([]Event, error) {
    f, err := os.Open(path)
    if err != nil {
        if os.IsNotExist(err) { return nil, nil }
        return nil, fmt.Errorf("open %s: %w", path, err)
    }
    defer f.Close()

    var out []Event
    dec := json.NewDecoder(bufio.NewReader(f))
    for {
        var e Event
        if err := dec.Decode(&e); err != nil {
            if err == io.EOF { return out, nil }
            return nil, fmt.Errorf("decode: %w", err)
        }
        out = append(out, e)
    }
}
