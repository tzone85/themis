package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/catalogue"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/ledger"
)

func newClassifyCmd() *cobra.Command {
	var id, base, changePath, cataloguePath string
	cmd := &cobra.Command{
		Use:   "classify",
		Short: "Classify an AIChange against a tenant's catalogue snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClassify(cmd, id, base, changePath, cataloguePath, ledger.DefaultRegistry())
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&changePath, "aichange", "", "path to AIChange JSON file")
	cmd.Flags().StringVar(&cataloguePath, "catalogue", "", "path to catalogue snapshot JSON (defaults to tenant's catalogue.json)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	_ = cmd.MarkFlagRequired("aichange")
	return cmd
}

// runClassify is exposed as a function (rather than inlined in RunE) so tests
// can substitute a registry without IMPACT_CLASSIFIED, exercising the
// wiring-guard error path that prevents unregistered kinds from landing in
// the ledger.
func runClassify(cmd *cobra.Command, id, base, changePath, cataloguePath string, registry *ledger.Registry) error {
	if _, ok := registry.Projector("IMPACT_CLASSIFIED"); !ok {
		return fmt.Errorf("ledger: IMPACT_CLASSIFIED is not registered; refuse to emit (run wiring tests)")
	}

	if cataloguePath == "" {
		cataloguePath = snapshotPath(base, id)
	}
	rawCat, err := os.ReadFile(cataloguePath) // #nosec G304 -- path comes from tenant-scoped snapshotPath or explicit --catalogue flag.
	if err != nil {
		return fmt.Errorf("read catalogue snapshot %s: %w", cataloguePath, err)
	}
	var g catalogue.CatalogueGraph
	if err := json.Unmarshal(rawCat, &g); err != nil {
		return fmt.Errorf("parse catalogue snapshot: %w", err)
	}

	rawCh, err := os.ReadFile(changePath) // #nosec G304 -- AIChange path supplied by the operator.
	if err != nil {
		return fmt.Errorf("read aichange %s: %w", changePath, err)
	}
	var c aichange.AIChange
	if err := json.Unmarshal(rawCh, &c); err != nil {
		return fmt.Errorf("parse aichange: %w", err)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("invalid aichange: %w", err)
	}

	impact := classify.Classify(c, g)

	events, _ := tenantPaths(base, id)
	s, err := ledger.OpenStore(events)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	payload, err := json.Marshal(map[string]any{
		"pr_id":   c.PRID,
		"actor":   c.Actor,
		"impact":  impact,
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	e := ledger.Event{
		Kind:      "IMPACT_CLASSIFIED",
		Tenant:    id,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		PrevHash:  s.LastHash(),
	}
	if _, err := s.Append(e); err != nil {
		return fmt.Errorf("append IMPACT_CLASSIFIED: %w", err)
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(impact)
}
