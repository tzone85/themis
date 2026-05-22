package catalogue

import (
	"errors"
	"os"
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

// writeCatalogue lays down a minimal valid catalogue tree at root, then
// applies the supplied mutator so individual tests can break exactly one
// thing.
func writeCatalogue(t *testing.T, root string, mutate func(root string)) {
	t.Helper()
	for _, p := range []string{"domains/D1", "services/s1", "events/E1"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("domains/D1/index.md", "---\nid: D1\nname: D1\n---\n")
	mustWrite("services/s1/index.md", "---\nid: s1\nname: s1\ndomain: D1\nproduces:\n  - E1\n---\n")
	mustWrite("events/E1/index.md", "---\nid: E1\nname: E1\ndomain: D1\nversion: \"1.0.0\"\n---\n")
	if mutate != nil {
		mutate(root)
	}
}

func TestParse_RejectsMissingLeadingDelimiter(t *testing.T) {
	root := t.TempDir()
	writeCatalogue(t, root, func(root string) {
		_ = os.WriteFile(filepath.Join(root, "services/s1/index.md"),
			[]byte("id: s1\nname: s1\n"), 0o600) // no '---'
	})
	if _, err := Parse(root); err == nil {
		t.Fatal("Parse should reject service missing front-matter delimiter")
	}
}

func TestParse_RejectsMissingClosingDelimiter(t *testing.T) {
	root := t.TempDir()
	writeCatalogue(t, root, func(root string) {
		_ = os.WriteFile(filepath.Join(root, "events/E1/index.md"),
			[]byte("---\nid: E1\nname: E1\nno closing delim\n"), 0o600)
	})
	if _, err := Parse(root); err == nil {
		t.Fatal("Parse should reject event missing closing delimiter")
	}
}

func TestParse_RejectsInvalidYAML(t *testing.T) {
	root := t.TempDir()
	writeCatalogue(t, root, func(root string) {
		_ = os.WriteFile(filepath.Join(root, "events/E1/index.md"),
			[]byte("---\nid: : :\n: nope\n---\n"), 0o600)
	})
	if _, err := Parse(root); err == nil {
		t.Fatal("Parse should reject broken YAML front-matter")
	}
}

func TestParse_RejectsDuplicateEventID(t *testing.T) {
	root := t.TempDir()
	writeCatalogue(t, root, func(root string) {
		_ = os.MkdirAll(filepath.Join(root, "events/E1_dup"), 0o755)
		_ = os.WriteFile(filepath.Join(root, "events/E1_dup/index.md"),
			[]byte("---\nid: E1\nname: E1_dup\ndomain: D1\n---\n"), 0o600)
	})
	_, err := Parse(root)
	if err == nil {
		t.Fatal("Parse should reject duplicate event ID")
	}
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("error %v should wrap ErrDuplicateID", err)
	}
}

func TestParse_RejectsServiceConsumingUnknownEvent(t *testing.T) {
	root := t.TempDir()
	writeCatalogue(t, root, func(root string) {
		_ = os.WriteFile(filepath.Join(root, "services/s1/index.md"),
			[]byte("---\nid: s1\nname: s1\ndomain: D1\nconsumes:\n  - PHANTOM_EVENT\n---\n"), 0o600)
	})
	_, err := Parse(root)
	if err == nil {
		t.Fatal("Parse should reject service consuming unknown event")
	}
	if !errors.Is(err, ErrMissingReference) {
		t.Fatalf("error %v should wrap ErrMissingReference", err)
	}
}

func TestParse_RejectsDomainMissingID(t *testing.T) {
	root := t.TempDir()
	writeCatalogue(t, root, func(root string) {
		_ = os.WriteFile(filepath.Join(root, "domains/D1/index.md"),
			[]byte("---\nname: \"D1 (no id)\"\n---\n"), 0o600)
	})
	if _, err := Parse(root); err == nil {
		t.Fatal("Parse should reject domain without id")
	}
}

func TestParse_RejectsServiceMissingID(t *testing.T) {
	root := t.TempDir()
	writeCatalogue(t, root, func(root string) {
		_ = os.WriteFile(filepath.Join(root, "services/s1/index.md"),
			[]byte("---\nname: nameless\ndomain: D1\n---\n"), 0o600)
	})
	if _, err := Parse(root); err == nil {
		t.Fatal("Parse should reject service without id")
	}
}

func TestParse_RejectsEventMissingID(t *testing.T) {
	root := t.TempDir()
	writeCatalogue(t, root, func(root string) {
		_ = os.WriteFile(filepath.Join(root, "events/E1/index.md"),
			[]byte("---\nname: nameless\ndomain: D1\n---\n"), 0o600)
	})
	if _, err := Parse(root); err == nil {
		t.Fatal("Parse should reject event without id")
	}
}
