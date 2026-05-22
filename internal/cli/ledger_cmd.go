package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/ledger"
)

func newLedgerCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ledger", Short: "Inspect, replay, and verify tenant ledgers"}
	cmd.AddCommand(newLedgerDoctorCmd(), newLedgerVerifyCmd(), newLedgerReplayCmd())
	return cmd
}

func tenantPaths(base, id string) (events, projection string) {
	root := filepath.Join(base, "tenants", id)
	return filepath.Join(root, "events.jsonl"), filepath.Join(root, "projection.sqlite")
}

func newLedgerDoctorCmd() *cobra.Command {
	var id, base string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report ledger health (event count, chain status, last hash) as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			events, _ := tenantPaths(base, id)
			rep, err := ledger.Doctor(events)
			if err != nil {
				return fmt.Errorf("doctor: %w", err)
			}
			out := map[string]any{
				"event_count":  rep.EventCount,
				"chain_intact": rep.ChainIntact,
				"chain_error":  rep.ChainError,
				"last_hash":    rep.LastHash,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	return cmd
}

func newLedgerVerifyCmd() *cobra.Command {
	var id, base string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Walk the Merkle chain; non-zero exit if tampering detected",
		RunE: func(cmd *cobra.Command, args []string) error {
			events, _ := tenantPaths(base, id)
			if err := ledger.Verify(events); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "ledger: chain intact")
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	return cmd
}

func newLedgerReplayCmd() *cobra.Command {
	var id, base string
	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Rebuild the SQLite projection from events.jsonl",
		RunE: func(cmd *cobra.Command, args []string) error {
			events, projection := tenantPaths(base, id)
			if err := ledger.DeleteFile(projection); err != nil {
				return err
			}
			return ledger.Replay(events, projection, ledger.DefaultRegistry())
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	return cmd
}
