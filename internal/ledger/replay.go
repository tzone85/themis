package ledger

import (
	"fmt"
	"os"
)

// DeleteFile removes path. Used by replay to drop a stale projection
// before rebuilding from the JSONL source of truth.
func DeleteFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Also remove WAL + shm sidecars if present.
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
	return nil
}

// Replay rebuilds the SQLite projection at projPath from the JSONL
// at storePath. The projection file is left in a fully consistent state
// (same byte-identical content regardless of how many times Replay runs).
func Replay(storePath, projPath string, registry *Registry) error {
	events, err := ReadAll(storePath)
	if err != nil {
		return fmt.Errorf("read events: %w", err)
	}
	p, err := OpenProjection(projPath)
	if err != nil {
		return fmt.Errorf("open projection: %w", err)
	}
	defer p.Close()

	for i, e := range events {
		if err := p.Project(e, registry); err != nil {
			return fmt.Errorf("project event %d (%s): %w", i, e.Kind, err)
		}
	}
	return nil
}

// Verify walks the JSONL ledger and asserts the Merkle chain is intact.
// Returns nil if every event's PrevHash matches the prior event's content hash
// (or ZeroHash for the first), and every event hashes to a stable value.
func Verify(storePath string) error {
	events, err := ReadAll(storePath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	prev := ZeroHash
	for i, e := range events {
		if e.PrevHash != prev {
			return fmt.Errorf("ledger: chain break at event %d (%s): prev_hash=%q, expected=%q",
				i, e.Kind, e.PrevHash, prev)
		}
		h, err := e.ContentHash()
		if err != nil {
			return fmt.Errorf("ledger: hash event %d: %w", i, err)
		}
		prev = h
	}
	return nil
}
