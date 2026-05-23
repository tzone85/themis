// Package classify implements the pure function that maps
// (AIChange, CatalogueGraph) → Impact. The classifier never touches I/O,
// never hits the network, and always returns the same Impact for the
// same inputs — this is what makes audit-replay work.
package classify

// Kind is the categorical impact of an AIChange on the catalogue.
//
// The string values are stable wire identifiers — they appear verbatim
// in ledger payloads, policy rules, and the BOM document — and must never
// change without a schema-version bump.
type Kind string

const (
	// KindSchemaBreaking: an event already in the catalogue had its schema
	// or contract definition changed.
	KindSchemaBreaking Kind = "SCHEMA_BREAKING"

	// KindNewEvent: a new event document was added under events/.
	KindNewEvent Kind = "NEW_EVENT"

	// KindConsumerTouch: a service that consumes one or more events had
	// its source modified.
	KindConsumerTouch Kind = "CONSUMER_TOUCH"

	// KindProducerTouch: a service that produces one or more events had
	// its source modified.
	KindProducerTouch Kind = "PRODUCER_TOUCH"

	// KindDocOnly: every touched file is documentation (markdown, README).
	KindDocOnly Kind = "DOC_ONLY"

	// KindOffCatalogue: touched files live outside any catalogue-mapped path.
	KindOffCatalogue Kind = "OFF_CATALOGUE"

	// KindNonContract: touched files are inside the catalogue tree but
	// don't match any contract-bearing pattern (e.g. internal helpers).
	KindNonContract Kind = "NON_CONTRACT"
)

// severityRank orders Kinds by audit-severity, lowest (DOC_ONLY) to highest
// (SCHEMA_BREAKING). The monotonicity property test relies on this ordering:
// adding evidence to an AIChange may only push the Kind *up* this scale.
//
// OFF_CATALOGUE ranks BELOW NON_CONTRACT for a subtle reason: appending a
// catalogue-adjacent file to an off-catalogue base must not *downgrade* the
// classification. The semantic intuition ("OFF_CAT is more mysterious so
// more severe") is wrong for monotonicity: severity is the floor a policy
// rule may rely on, not a free-form risk score. Out-of-tree changes get
// their bespoke handling via the OFF_CATALOGUE rule in policy YAML rather
// than via severity inflation.
var severityRank = map[Kind]int{
	KindDocOnly:        0,
	KindOffCatalogue:   1,
	KindNonContract:    2,
	KindConsumerTouch:  3,
	KindProducerTouch:  4,
	KindNewEvent:       5,
	KindSchemaBreaking: 6,
}

// Severity returns the rank of k in [0..6]. Unknown kinds report -1.
func (k Kind) Severity() int {
	r, ok := severityRank[k]
	if !ok {
		return -1
	}
	return r
}

// Impact is the structured output of Classify.
type Impact struct {
	Kind   Kind   `json:"kind"`
	Domain string `json:"domain,omitempty"`
	// Service, when set, is the catalogue ID of the principal service
	// implicated by the change. For SCHEMA_BREAKING/NEW_EVENT the field
	// reflects the producing service when known.
	Service           string   `json:"service,omitempty"`
	EventID           string   `json:"event_id,omitempty"`
	Reason            string   `json:"reason"`
	AffectedEvents    []string `json:"affected_events,omitempty"`
	AffectedConsumers []string `json:"affected_consumers,omitempty"`
}

// IsContract reports whether the impact is on a contract-bearing path —
// i.e. event schemas or service-to-event wiring. Policy templates use this
// to decide whether a change can ever be safely auto-allowed.
func (i Impact) IsContract() bool {
	switch i.Kind {
	case KindSchemaBreaking, KindNewEvent, KindConsumerTouch, KindProducerTouch:
		return true
	}
	return false
}
