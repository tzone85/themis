package catalogue

import (
	"testing"
	"time"
)

func sampleGraph() CatalogueGraph {
	return CatalogueGraph{
		Domains: map[string]Domain{
			"Collections": {ID: "Collections", Name: "Collections", Services: []ServiceRef{{ID: "collector"}, {ID: "dispatcher"}}},
		},
		Services: map[string]Service{
			"collector": {
				ID: "collector", Name: "Collector", Domain: "Collections",
				Produces:   []EventRef{{ID: "PaymentReceived"}},
				SourcePath: "services/collector/index.md",
			},
			"dispatcher": {
				ID: "dispatcher", Name: "Dispatcher", Domain: "Collections",
				Consumes:   []EventRef{{ID: "PaymentReceived"}},
				SourcePath: "services/dispatcher/index.md",
			},
		},
		Events: map[string]EventDef{
			"PaymentReceived": {ID: "PaymentReceived", Name: "PaymentReceived", Domain: "Collections", Owners: []string{"team-collections"}, Version: "1.0.0"},
		},
		SourceRoot:  "/tmp/sample",
		SyncedAt:    time.Unix(1700000000, 0).UTC(),
		ContentHash: "stub",
	}
}

func TestCatalogueGraph_ConsumersOf(t *testing.T) {
	g := sampleGraph()
	got := g.ConsumersOf("PaymentReceived")
	if len(got) != 1 {
		t.Fatalf("expected 1 consumer, got %d", len(got))
	}
	if got[0].ID != "dispatcher" {
		t.Errorf("consumer ID = %q, want dispatcher", got[0].ID)
	}
	if got := g.ConsumersOf("Nope"); len(got) != 0 {
		t.Errorf("expected 0 consumers for unknown event, got %d", len(got))
	}
}

func TestCatalogueGraph_ProducerOf(t *testing.T) {
	g := sampleGraph()
	s, ok := g.ProducerOf("PaymentReceived")
	if !ok || s.ID != "collector" {
		t.Fatalf("producer = %+v ok=%v, want collector", s, ok)
	}
	if _, ok := g.ProducerOf("Phantom"); ok {
		t.Error("expected no producer for Phantom")
	}
}

func TestCatalogueGraph_DomainOfService(t *testing.T) {
	g := sampleGraph()
	d, ok := g.DomainOfService("collector")
	if !ok || d.ID != "Collections" {
		t.Fatalf("domain = %+v ok=%v, want Collections", d, ok)
	}
	if _, ok := g.DomainOfService("Phantom"); ok {
		t.Error("expected no domain for Phantom service")
	}
}

func TestSortedKeys_Deterministic(t *testing.T) {
	m := map[string]int{"c": 3, "a": 1, "b": 2}
	got := sortedKeys(m)
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
