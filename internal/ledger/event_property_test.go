package ledger

import (
	"encoding/json"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestPropContentHash_DependsOnAllFields(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		e1 := genEvent(rt)
		h1, err := e1.ContentHash()
		if err != nil { rt.Fatal(err) }

		// Mutate one field at random and assert the hash changes.
		field := rapid.SampledFrom([]string{"Kind", "Tenant", "Timestamp", "Payload", "PrevHash"}).Draw(rt, "field")
		e2 := e1
		switch field {
		case "Kind":
			e2.Kind = e1.Kind + "_X"
		case "Tenant":
			e2.Tenant = e1.Tenant + "_X"
		case "Timestamp":
			e2.Timestamp = e1.Timestamp.Add(time.Nanosecond)
		case "Payload":
			// Replace with a different valid JSON value so ContentHash doesn't error.
			if string(e1.Payload) == `{"k":"v"}` {
				e2.Payload = json.RawMessage(`{"k":"w"}`)
			} else {
				e2.Payload = json.RawMessage(`{"k":"v"}`)
			}
		case "PrevHash":
			if e1.PrevHash == "" || e1.PrevHash == "X"+e1.PrevHash {
				e2.PrevHash = e1.PrevHash + "Y"
			} else {
				e2.PrevHash = "X" + e1.PrevHash
			}
		}
		h2, err := e2.ContentHash()
		if err != nil { rt.Fatal(err) }
		if h1 == h2 {
			rt.Fatalf("hash unchanged after mutating %s: %s", field, h1)
		}
	})
}

func genEvent(t *rapid.T) Event {
	return Event{
		Kind:      rapid.StringMatching(`[A-Z][A-Z_]{0,20}`).Draw(t, "kind"),
		Tenant:    rapid.StringMatching(`[a-z][a-z0-9-]{0,20}`).Draw(t, "tenant"),
		Timestamp: time.Unix(rapid.Int64Range(0, 2_000_000_000).Draw(t, "ts"), 0).UTC(),
		Payload:   json.RawMessage(`{"k":"v"}`),
		PrevHash:  rapid.StringMatching(`[a-f0-9]{8,16}|GENESIS`).Draw(t, "prev"),
	}
}
