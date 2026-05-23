package classify

import (
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/catalogue"
)

// loadGraph parses the shared catalogue fixture from the catalogue package's testdata.
func loadGraph(t *testing.T) catalogue.CatalogueGraph {
	t.Helper()
	root := filepath.Join("..", "catalogue", "testdata", "sample")
	g, err := catalogue.Parse(root)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestClassify_SchemaBreaking_OnEventModification(t *testing.T) {
	g := loadGraph(t)
	c := aichange.AIChange{
		PRID:  "x",
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "events/PaymentReceived/schema.json", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	imp := Classify(c, g)
	if imp.Kind != KindSchemaBreaking {
		t.Fatalf("Kind = %q, want SCHEMA_BREAKING", imp.Kind)
	}
	if imp.EventID != "PaymentReceived" || imp.Domain != "Collections" {
		t.Errorf("event/domain = %q/%q", imp.EventID, imp.Domain)
	}
	if imp.Service != "collector" {
		t.Errorf("producer service = %q, want collector", imp.Service)
	}
	if len(imp.AffectedConsumers) != 1 || imp.AffectedConsumers[0] != "dispatcher" {
		t.Errorf("affected consumers = %v", imp.AffectedConsumers)
	}
}

func TestClassify_NewEvent_OnAddedEventDoc(t *testing.T) {
	g := loadGraph(t)
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "events/SettlementCompleted/index.md", ChangeKind: aichange.FileAdded, AfterHash: "abc"},
		},
	}
	imp := Classify(c, g)
	if imp.Kind != KindNewEvent {
		t.Fatalf("Kind = %q, want NEW_EVENT", imp.Kind)
	}
	if imp.EventID != "SettlementCompleted" {
		t.Errorf("EventID = %q", imp.EventID)
	}
}

func TestClassify_ProducerTouch_OnProducingServiceEdit(t *testing.T) {
	g := loadGraph(t)
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "services/collector/handler.go", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	imp := Classify(c, g)
	if imp.Kind != KindProducerTouch {
		t.Fatalf("Kind = %q, want PRODUCER_TOUCH", imp.Kind)
	}
	if imp.Service != "collector" {
		t.Errorf("Service = %q", imp.Service)
	}
}

func TestClassify_ConsumerTouch_OnPureConsumerEdit(t *testing.T) {
	g := loadGraph(t)
	// notifier consumes PaymentDispatched and produces NotificationSent/Failed.
	// dispatcher consumes PaymentReceived and produces PaymentDispatched.
	// Build a graph clone where notifier has Produces stripped so classifier
	// can resolve it as a pure consumer.
	clone := g
	pure := clone.Services["notifier"]
	pure.Produces = nil
	clone.Services = map[string]catalogue.Service{}
	for k, v := range g.Services {
		if k == "notifier" {
			clone.Services[k] = pure
		} else {
			clone.Services[k] = v
		}
	}
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "services/notifier/handler.go", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	imp := Classify(c, clone)
	if imp.Kind != KindConsumerTouch {
		t.Fatalf("Kind = %q, want CONSUMER_TOUCH", imp.Kind)
	}
	if imp.Service != "notifier" {
		t.Errorf("Service = %q", imp.Service)
	}
}

func TestClassify_DocOnly_OnReadmeAndDocsTree(t *testing.T) {
	g := loadGraph(t)
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
			{Path: "docs/guides/onboarding.md", ChangeKind: aichange.FileAdded, AfterHash: "c"},
		},
	}
	imp := Classify(c, g)
	if imp.Kind != KindDocOnly {
		t.Fatalf("Kind = %q, want DOC_ONLY", imp.Kind)
	}
}

func TestClassify_OffCatalogue_OnUnknownPaths(t *testing.T) {
	g := loadGraph(t)
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "scripts/build.sh", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
			{Path: "Makefile", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	imp := Classify(c, g)
	if imp.Kind != KindOffCatalogue {
		t.Fatalf("Kind = %q, want OFF_CATALOGUE", imp.Kind)
	}
}

func TestClassify_NonContract_OnCatalogueAdjacentNonService(t *testing.T) {
	g := loadGraph(t)
	// touch a domain doc — catalogue-adjacent but not contract surface
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "domains/Collections/CONTRIBUTORS", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	imp := Classify(c, g)
	if imp.Kind != KindNonContract {
		t.Fatalf("Kind = %q, want NON_CONTRACT", imp.Kind)
	}
}

func TestClassify_EmptyAIChange_DocOnly(t *testing.T) {
	g := loadGraph(t)
	imp := Classify(aichange.AIChange{}, g)
	if imp.Kind != KindDocOnly {
		t.Fatalf("Kind = %q, want DOC_ONLY (no-files base case sits at severity floor)", imp.Kind)
	}
}

func TestClassify_PriorityOrder_SchemaBeatsServiceTouch(t *testing.T) {
	g := loadGraph(t)
	c := aichange.AIChange{
		TouchedFiles: []aichange.FileTouch{
			{Path: "services/collector/handler.go", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
			{Path: "events/PaymentReceived/schema.json", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	imp := Classify(c, g)
	if imp.Kind != KindSchemaBreaking {
		t.Fatalf("schema-breaking must beat producer-touch: got %q", imp.Kind)
	}
}
