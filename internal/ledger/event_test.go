package ledger

import (
    "encoding/json"
    "testing"
    "time"
)

func TestEvent_ContentHashIsDeterministic(t *testing.T) {
    e := Event{
        Kind:      "TEST_EVENT",
        Tenant:    "acme",
        Timestamp: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
        Payload:   json.RawMessage(`{"hello":"world"}`),
        PrevHash:  "0000000000000000000000000000000000000000000000000000000000000000",
    }
    h1, err := e.ContentHash()
    if err != nil { t.Fatal(err) }
    h2, err := e.ContentHash()
    if err != nil { t.Fatal(err) }
    if h1 != h2 {
        t.Fatalf("hash not deterministic: %s vs %s", h1, h2)
    }
    if len(h1) != 64 { // hex sha256
        t.Fatalf("hash wrong length: %d", len(h1))
    }
}

func TestEvent_HashChangesWithAnyField(t *testing.T) {
    base := Event{
        Kind: "TEST", Tenant: "acme",
        Timestamp: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
        Payload:   json.RawMessage(`{}`),
        PrevHash:  "00",
    }
    baseHash, _ := base.ContentHash()

    cases := map[string]Event{
        "kind":      {Kind: "OTHER", Tenant: "acme", Timestamp: base.Timestamp, Payload: base.Payload, PrevHash: "00"},
        "tenant":    {Kind: "TEST", Tenant: "beta", Timestamp: base.Timestamp, Payload: base.Payload, PrevHash: "00"},
        "timestamp": {Kind: "TEST", Tenant: "acme", Timestamp: base.Timestamp.Add(time.Second), Payload: base.Payload, PrevHash: "00"},
        "payload":   {Kind: "TEST", Tenant: "acme", Timestamp: base.Timestamp, Payload: json.RawMessage(`{"x":1}`), PrevHash: "00"},
        "prev":      {Kind: "TEST", Tenant: "acme", Timestamp: base.Timestamp, Payload: base.Payload, PrevHash: "01"},
    }
    for field, e := range cases {
        h, err := e.ContentHash()
        if err != nil { t.Fatalf("%s: %v", field, err) }
        if h == baseHash {
            t.Errorf("hash unchanged when %s differs", field)
        }
    }
}

func TestChain_LinksConsecutiveEvents(t *testing.T) {
    a := Event{Kind: "A", Tenant: "x", Timestamp: time.Unix(1, 0).UTC(), Payload: json.RawMessage(`{}`), PrevHash: ZeroHash}
    aHash, _ := a.ContentHash()

    b := Event{Kind: "B", Tenant: "x", Timestamp: time.Unix(2, 0).UTC(), Payload: json.RawMessage(`{}`)}
    b = Chain(b, aHash)

    if b.PrevHash != aHash {
        t.Fatalf("Chain did not set PrevHash; got %q want %q", b.PrevHash, aHash)
    }
}

func TestChain_DifferentPriorsProduceDifferentChildHashes(t *testing.T) {
    base := Event{Kind: "X", Tenant: "t", Timestamp: time.Unix(10, 0).UTC(), Payload: json.RawMessage(`{}`)}
    h1, _ := Chain(base, "AAAA").ContentHash()
    h2, _ := Chain(base, "BBBB").ContentHash()
    if h1 == h2 {
        t.Fatal("child events with different priors should hash differently")
    }
}
