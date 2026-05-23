package classify

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/catalogue"
)

func loadPropertyGraph(t *testing.T) catalogue.CatalogueGraph {
	t.Helper()
	g, err := catalogue.Parse(filepath.Join("..", "catalogue", "testdata", "sample"))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// drawAIChange generates a random AIChange that mixes paths the classifier
// understands (events/, services/) with paths it doesn't (random scripts/docs).
func drawAIChange(rt *rapid.T, g catalogue.CatalogueGraph) aichange.AIChange {
	knownEvents := keysOf(g.Events)
	knownServices := keysOf(g.Services)

	pathChoices := []string{
		"events/" + pick(rt, knownEvents) + "/index.md",
		"events/" + pick(rt, knownEvents) + "/schema.json",
		"events/SomeNewThing" + rapid.StringMatching(`[A-Z][a-z]{1,5}`).Draw(rt, "newev") + "/index.md",
		"services/" + pick(rt, knownServices) + "/handler.go",
		"services/" + pick(rt, knownServices) + "/README.md",
		"domains/Collections/CONTRIBUTORS",
		"scripts/build.sh",
		"docs/guides/whatever.md",
		"README.md",
		"Makefile",
	}
	kindChoices := []aichange.FileChangeKind{aichange.FileAdded, aichange.FileModified, aichange.FileDeleted}

	n := rapid.IntRange(0, 6).Draw(rt, "n")
	files := make([]aichange.FileTouch, 0, n)
	for range n {
		files = append(files, aichange.FileTouch{
			Path:       rapid.SampledFrom(pathChoices).Draw(rt, "path"),
			ChangeKind: rapid.SampledFrom(kindChoices).Draw(rt, "kind"),
			BeforeHash: "before",
			AfterHash:  "after",
		})
	}
	return aichange.AIChange{
		PRID:         "prop-" + rapid.StringMatching(`[a-z]{1,8}`).Draw(rt, "prid"),
		Actor:        "claude_code",
		TouchedFiles: files,
	}
}

func TestPropClassify_Deterministic(t *testing.T) {
	g := loadPropertyGraph(t)
	rapid.Check(t, func(rt *rapid.T) {
		c := drawAIChange(rt, g)
		a := Classify(c, g)
		b := Classify(c, g)
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if string(ja) != string(jb) {
			rt.Fatalf("Classify is non-deterministic:\n  a=%s\n  b=%s", ja, jb)
		}
	})
}

// pick draws an element from a non-empty slice via rapid's SampledFrom.
// Returns "" for empty slices so the generated path becomes nonsense — which
// is exactly what we want for negative-path coverage.
func pick(rt *rapid.T, ss []string) string {
	if len(ss) == 0 {
		return "phantom"
	}
	return rapid.SampledFrom(ss).Draw(rt, "pick")
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
