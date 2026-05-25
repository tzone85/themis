package ledger

import "testing"

// TestWiring_DefaultRegistryHasMinimumKinds documents the kinds that must
// exist in the default registry for this Plan-1 layer. Later plans expand
// this list when they add new event types.
//
// The test exists to ensure that adding new ledger event kinds is a deliberate,
// reviewable change rather than something that can creep in via Store.Append
// without a corresponding projector.
func TestWiring_DefaultRegistryHasMinimumKinds(t *testing.T) {
	r := DefaultRegistry()

	// Plan 1 only needs the housekeeping kinds — real product event types
	// (INGEST_COMPLETED, DECISION_ISSUED, etc.) arrive in later plans and
	// MUST add themselves to DefaultRegistry + extend this test.
	want := []string{
		"TENANT_INITIALISED",
		"LEDGER_REPLAYED",
		"LEDGER_VERIFIED",
		"CATALOGUE_SYNCED",  // Plan 2
		"IMPACT_CLASSIFIED", // Plan 2
		"SCAN_FINDING",      // Plan 3
		"DECISION_ISSUED",   // Plan 3
		"POLICY_INVALID",    // Plan 3
		"BOM_BUILT",            // Plan 4
		"BOM_SIGNED",           // Plan 4
		"INGEST_COMPLETED",      // Plan 5
		"INGEST_ADAPTER_FAILED", // Plan 5
		"APPROVAL_GRANTED",      // Plan 9
		"APPROVAL_DENIED",       // Plan 9
		"DECISION_FINALISED",    // Plan 9
	}
	for _, kind := range want {
		if _, ok := r.Projector(kind); !ok {
			t.Errorf("default registry missing required kind %q", kind)
		}
	}
}
