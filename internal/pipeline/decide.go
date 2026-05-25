// Package pipeline shares the classify→scan→decide orchestration between
// the CLI (themis decide) and the REST API (POST /v1/tenants/{id}/decide).
// Keeping the body here ensures both surfaces emit the same ledger events
// in the same order — which is what makes audit replay credible regardless
// of which surface ran the decision.
package pipeline

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/catalogue"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
)

// Result is the structured output of Run, suitable for marshalling to JSON
// for HTTP response bodies or CLI stdout.
type Result struct {
	PRID     string           `json:"pr_id"`
	Actor    string           `json:"actor"`
	Impact   classify.Impact  `json:"impact"`
	Findings []scan.Finding   `json:"findings"`
	Decision policy.Decision  `json:"decision"`
}

// Run executes the full pipeline for a single AIChange:
//
//  1. Classify the AIChange against the catalogue graph.
//  2. Run every default scanner over the supplied file bodies.
//  3. Emit one SCAN_FINDING per finding.
//  4. Decide using the supplied policy.
//  5. Emit DECISION_ISSUED with the full envelope.
//
// All ledger events are appended atomically through the supplied Store
// reference (caller is responsible for OpenStore + Close).
func Run(
	store *ledger.Store,
	tenantID string,
	ai aichange.AIChange,
	g catalogue.CatalogueGraph,
	pol policy.Policy,
	bodies map[string][]byte,
	scanners []scan.Scanner,
) (Result, error) {
	if scanners == nil {
		scanners = scan.DefaultScanners()
	}

	imp := classify.Classify(ai, g)

	findings := scan.RunAll(scanners, ai, bodies)
	for _, f := range findings {
		payload, err := json.Marshal(map[string]any{"pr_id": ai.PRID, "finding": f})
		if err != nil {
			return Result{}, fmt.Errorf("marshal finding: %w", err)
		}
		e := ledger.Event{
			Kind:      "SCAN_FINDING",
			Tenant:    tenantID,
			Timestamp: time.Now().UTC(),
			Payload:   payload,
			PrevHash:  store.LastHash(),
		}
		if _, err := store.Append(e); err != nil {
			return Result{}, fmt.Errorf("append SCAN_FINDING: %w", err)
		}
	}

	decision := policy.Decide(ai, imp, findings, pol)

	result := Result{
		PRID:     ai.PRID,
		Actor:    ai.Actor,
		Impact:   imp,
		Findings: findings,
		Decision: decision,
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return Result{}, fmt.Errorf("marshal decision payload: %w", err)
	}
	e := ledger.Event{
		Kind:      "DECISION_ISSUED",
		Tenant:    tenantID,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		PrevHash:  store.LastHash(),
	}
	if _, err := store.Append(e); err != nil {
		return Result{}, fmt.Errorf("append DECISION_ISSUED: %w", err)
	}
	return result, nil
}
