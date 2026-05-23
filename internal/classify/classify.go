package classify

import (
	"path"
	"sort"
	"strings"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/catalogue"
)

// Classify is the pure function at the heart of Themis. It examines the
// AIChange's touched files in the context of the catalogue graph and
// returns the highest-severity Impact that applies.
//
// Resolution order (first match wins):
//
//  1. SCHEMA_BREAKING — any MODIFIED/DELETED file under events/<id>/ where
//     <id> is already in the graph (existing event's contract changed).
//  2. NEW_EVENT — any ADDED file under events/<newid>/ where <newid> is
//     not yet in the graph.
//  3. PRODUCER_TOUCH — any MODIFIED/ADDED/DELETED file under
//     services/<sid>/ where the service produces ≥ 1 event.
//  4. CONSUMER_TOUCH — same shape, but the service only consumes.
//  5. DOC_ONLY — every touched file is documentation (*.md, README*, docs/**).
//  6. OFF_CATALOGUE — touched files exist but none fall inside domains/,
//     services/, events/.
//  7. NON_CONTRACT — fallback: catalogue-adjacent but doesn't carry contract
//     surface area.
func Classify(c aichange.AIChange, g catalogue.CatalogueGraph) Impact {
	// Pass 1: schema-breaking + new-event detection.
	for _, ft := range c.TouchedFiles {
		eid := eventIDFromPath(ft.Path)
		if eid == "" {
			continue
		}
		_, exists := g.Events[eid]
		switch {
		case exists && (ft.ChangeKind == aichange.FileModified || ft.ChangeKind == aichange.FileDeleted):
			imp := Impact{
				Kind:    KindSchemaBreaking,
				EventID: eid,
				Reason:  "existing event schema/definition was " + strings.ToLower(string(ft.ChangeKind)),
			}
			fillEventContext(&imp, g, eid)
			return imp
		case !exists && ft.ChangeKind == aichange.FileAdded:
			imp := Impact{
				Kind:    KindNewEvent,
				EventID: eid,
				Reason:  "new event document added under events/" + eid,
			}
			imp.Domain = g.Events[eid].Domain
			return imp
		}
	}

	// Pass 2: producer touch (preferred over consumer when service does both).
	if imp, ok := classifyServiceTouches(c, g); ok {
		return imp
	}

	// Pass 3: doc-only (also catches the "no files at all" base case so the
	// classification floor sits at the lowest severity — this is what makes
	// the monotonicity property hold when an empty AIChange grows new touches).
	if allDocs(c.TouchedFiles) {
		reason := "every touched file is documentation"
		if len(c.TouchedFiles) == 0 {
			reason = "no files touched"
		}
		return Impact{Kind: KindDocOnly, Reason: reason}
	}

	// Pass 4: off-catalogue (no touched path falls under domains/services/events/).
	if !anyCatalogueAdjacent(c.TouchedFiles) {
		return Impact{Kind: KindOffCatalogue, Reason: "no touched file maps to the catalogue tree"}
	}

	// Pass 5: catalogue-adjacent but not contract-bearing.
	return Impact{Kind: KindNonContract, Reason: "catalogue-adjacent but no contract surface affected"}
}

// classifyServiceTouches finds the strongest service-level impact. If any
// touched service is a producer, the result is PRODUCER_TOUCH (carrying the
// union of all touched producer services). Otherwise, if any touched service
// is a consumer, CONSUMER_TOUCH. Otherwise (false, no impact).
func classifyServiceTouches(c aichange.AIChange, g catalogue.CatalogueGraph) (Impact, bool) {
	producerIDs := map[string]struct{}{}
	consumerIDs := map[string]struct{}{}
	affectedEvents := map[string]struct{}{}

	for _, ft := range c.TouchedFiles {
		sid := serviceIDFromPath(ft.Path)
		if sid == "" {
			continue
		}
		s, ok := g.Services[sid]
		if !ok {
			continue
		}
		if len(s.Produces) > 0 {
			producerIDs[sid] = struct{}{}
			for _, ref := range s.Produces {
				affectedEvents[ref.ID] = struct{}{}
			}
		}
		if len(s.Consumes) > 0 {
			consumerIDs[sid] = struct{}{}
			for _, ref := range s.Consumes {
				affectedEvents[ref.ID] = struct{}{}
			}
		}
	}

	switch {
	case len(producerIDs) > 0:
		sid := firstSortedKey(producerIDs)
		imp := Impact{
			Kind:    KindProducerTouch,
			Service: sid,
			Reason:  "producing service(s) modified: " + joinSorted(producerIDs),
		}
		if s, ok := g.Services[sid]; ok {
			imp.Domain = s.Domain
		}
		imp.AffectedEvents = sortedSet(affectedEvents)
		return imp, true
	case len(consumerIDs) > 0:
		sid := firstSortedKey(consumerIDs)
		imp := Impact{
			Kind:    KindConsumerTouch,
			Service: sid,
			Reason:  "consuming service(s) modified: " + joinSorted(consumerIDs),
		}
		if s, ok := g.Services[sid]; ok {
			imp.Domain = s.Domain
		}
		imp.AffectedEvents = sortedSet(affectedEvents)
		return imp, true
	}
	return Impact{}, false
}

// fillEventContext populates Domain + AffectedConsumers for an event-centric Impact.
func fillEventContext(imp *Impact, g catalogue.CatalogueGraph, eid string) {
	if e, ok := g.Events[eid]; ok {
		imp.Domain = e.Domain
	}
	if prod, ok := g.ProducerOf(eid); ok {
		imp.Service = prod.ID
	}
	for _, cs := range g.ConsumersOf(eid) {
		imp.AffectedConsumers = append(imp.AffectedConsumers, cs.ID)
	}
}

// --- path classifiers -------------------------------------------------------

// eventIDFromPath returns "PaymentReceived" for paths like
// "events/PaymentReceived/index.md" or "events/PaymentReceived/schema.json".
// Returns "" if the path is not an event-doc.
func eventIDFromPath(p string) string {
	parts := splitClean(p)
	if len(parts) < 2 || parts[0] != "events" {
		return ""
	}
	return parts[1]
}

// serviceIDFromPath returns "dispatcher" for paths like "services/dispatcher/index.md".
func serviceIDFromPath(p string) string {
	parts := splitClean(p)
	if len(parts) < 2 || parts[0] != "services" {
		return ""
	}
	return parts[1]
}

// splitClean normalises and splits a unix-style path into segments.
func splitClean(p string) []string {
	cleaned := path.Clean("/" + p)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." || cleaned == "" {
		return nil
	}
	return strings.Split(cleaned, "/")
}

func allDocs(files []aichange.FileTouch) bool {
	for _, ft := range files {
		if !isDocPath(ft.Path) {
			return false
		}
	}
	return true
}

func isDocPath(p string) bool {
	low := strings.ToLower(p)
	if strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".markdown") || strings.HasSuffix(low, ".rst") || strings.HasSuffix(low, ".txt") {
		return true
	}
	base := strings.ToLower(path.Base(p))
	if strings.HasPrefix(base, "readme") || strings.HasPrefix(base, "changelog") {
		return true
	}
	parts := splitClean(p)
	if len(parts) > 0 && (parts[0] == "docs" || parts[0] == "doc") {
		return true
	}
	return false
}

func anyCatalogueAdjacent(files []aichange.FileTouch) bool {
	for _, ft := range files {
		parts := splitClean(ft.Path)
		if len(parts) == 0 {
			continue
		}
		switch parts[0] {
		case "domains", "services", "events":
			return true
		}
	}
	return false
}

// --- set helpers ------------------------------------------------------------

func sortedSet(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func firstSortedKey(m map[string]struct{}) string {
	keys := sortedSet(m)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func joinSorted(m map[string]struct{}) string {
	return strings.Join(sortedSet(m), ",")
}
