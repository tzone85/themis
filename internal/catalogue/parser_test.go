package catalogue

import (
	"path/filepath"
	"testing"
)

func TestParse_LoadsFixture(t *testing.T) {
	root := filepath.Join("testdata", "sample")
	g, err := Parse(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Domains) != 2 {
		t.Errorf("domains = %d, want 2", len(g.Domains))
	}
	if len(g.Services) != 4 {
		t.Errorf("services = %d, want 4", len(g.Services))
	}
	if len(g.Events) != 6 {
		t.Errorf("events = %d, want 6", len(g.Events))
	}
	if _, ok := g.Domains["Collections"]; !ok {
		t.Error("missing Collections domain")
	}
	collector, ok := g.Services["collector"]
	if !ok {
		t.Fatal("missing collector service")
	}
	if collector.Domain != "Collections" {
		t.Errorf("collector.Domain = %q, want Collections", collector.Domain)
	}
	if len(collector.Produces) != 1 || collector.Produces[0].ID != "PaymentReceived" {
		t.Errorf("collector.Produces = %+v", collector.Produces)
	}
}

func TestParse_ContentHashStableAcrossCalls(t *testing.T) {
	root := filepath.Join("testdata", "sample")
	g1, err := Parse(root)
	if err != nil {
		t.Fatal(err)
	}
	g2, err := Parse(root)
	if err != nil {
		t.Fatal(err)
	}
	if g1.ContentHash != g2.ContentHash {
		t.Fatalf("ContentHash differs across calls: %q vs %q", g1.ContentHash, g2.ContentHash)
	}
	if g1.ContentHash == "" {
		t.Fatal("ContentHash is empty")
	}
}

func TestParse_ConsumersOfMatchesFixture(t *testing.T) {
	g, err := Parse(filepath.Join("testdata", "sample"))
	if err != nil {
		t.Fatal(err)
	}
	consumers := g.ConsumersOf("PaymentReceived")
	if len(consumers) != 1 || consumers[0].ID != "dispatcher" {
		t.Fatalf("PaymentReceived consumers = %+v, want [dispatcher]", consumers)
	}
	producers, ok := g.ProducerOf("NotificationSent")
	if !ok || producers.ID != "notifier" {
		t.Fatalf("NotificationSent producer = %+v ok=%v", producers, ok)
	}
}

func TestParse_MissingRootReturnsEmpty(t *testing.T) {
	g, err := Parse(filepath.Join(t.TempDir(), "no-such"))
	if err != nil {
		t.Fatalf("Parse on missing root should succeed empty: %v", err)
	}
	if len(g.Domains) != 0 || len(g.Services) != 0 || len(g.Events) != 0 {
		t.Errorf("expected empty graph, got %+v", g)
	}
}
