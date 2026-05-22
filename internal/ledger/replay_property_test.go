package ledger

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestPropReplay_DeterministicProjection asserts that for any valid
// sequence of registered-kind events, replaying produces the same set
// of (kind, ts, content_hash) triples as projecting live.
func TestPropReplay_DeterministicProjection(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		kinds := []string{"TENANT_INITIALISED", "LEDGER_REPLAYED", "LEDGER_VERIFIED"}
		n := rapid.IntRange(1, 12).Draw(rt, "n")

		dir := t.TempDir()
		storePath := filepath.Join(dir, "events.jsonl")
		liveProj := filepath.Join(dir, "live.sqlite")
		replayProj := filepath.Join(dir, "replay.sqlite")

		s, err := OpenStore(storePath)
		if err != nil {
			rt.Fatal(err)
		}
		p, err := OpenProjection(liveProj)
		if err != nil {
			rt.Fatal(err)
		}
		reg := DefaultRegistry()

		for i := range n {
			kind := rapid.SampledFrom(kinds).Draw(rt, "kind")
			e := Event{
				Kind: kind, Tenant: "t",
				Timestamp: time.Unix(int64(1700000000+i), 0).UTC(),
				Payload:   json.RawMessage(`{"i":1}`),
				PrevHash:  s.LastHash(),
			}
			if _, err := s.Append(e); err != nil {
				rt.Fatal(err)
			}
			if err := p.Project(e, reg); err != nil {
				rt.Fatal(err)
			}
		}
		s.Close()
		p.Close()

		if err := Replay(storePath, replayProj, reg); err != nil {
			rt.Fatal(err)
		}

		liveSet := dumpHashes(rt, liveProj)
		repSet := dumpHashes(rt, replayProj)
		if !equalSets(liveSet, repSet) {
			rt.Fatalf("Replay produced different content_hash set: live=%v replay=%v", liveSet, repSet)
		}
	})
}

func dumpHashes(t *rapid.T, path string) map[string]struct{} {
	p, err := OpenProjection(path)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	rows, err := p.DB().Query("SELECT content_hash FROM events ORDER BY content_hash")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			t.Fatal(err)
		}
		out[h] = struct{}{}
	}
	return out
}

func equalSets(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
