package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/catalogue"
	"github.com/tzone85/themis/internal/ledger"
)

func newCatalogueCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "catalogue", Short: "Manage tenant catalogue snapshots"}
	cmd.AddCommand(newCatalogueSyncCmd())
	return cmd
}

func newCatalogueSyncCmd() *cobra.Command {
	var id, base, source string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Parse an EventCatalog tree + emit a CATALOGUE_SYNCED ledger event",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := catalogue.Parse(source)
			if err != nil {
				return fmt.Errorf("parse catalogue at %s: %w", source, err)
			}
			events, _ := tenantPaths(base, id)
			s, err := ledger.OpenStore(events)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			snapshot := snapshotPath(base, id)
			if err := writeSnapshot(snapshot, g); err != nil {
				return fmt.Errorf("write snapshot: %w", err)
			}

			payload := map[string]any{
				"source":         source,
				"content_hash":   g.ContentHash,
				"synced_at":      time.Now().UTC().Format(time.RFC3339Nano),
				"domains":        len(g.Domains),
				"services":       len(g.Services),
				"events":         len(g.Events),
				"snapshot_path":  snapshot,
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("marshal payload: %w", err)
			}

			e := ledger.Event{
				Kind:      "CATALOGUE_SYNCED",
				Tenant:    id,
				Timestamp: time.Now().UTC(),
				Payload:   raw,
				PrevHash:  s.LastHash(),
			}
			if _, err := s.Append(e); err != nil {
				return fmt.Errorf("append CATALOGUE_SYNCED: %w", err)
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(payload)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&source, "source", "", "path to the EventCatalog repository tree")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	_ = cmd.MarkFlagRequired("source")
	return cmd
}

// snapshotPath returns where the parsed CatalogueGraph JSON is cached per
// tenant — the classify command reads from this file.
func snapshotPath(base, id string) string {
	return filepath.Join(base, "tenants", id, "catalogue.json")
}

func writeSnapshot(path string, g catalogue.CatalogueGraph) error {
	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
