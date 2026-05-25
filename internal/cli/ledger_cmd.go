package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/incidents"
	"github.com/tzone85/themis/internal/ledger"
)

func newLedgerCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ledger", Short: "Inspect, replay, and verify tenant ledgers"}
	cmd.AddCommand(
		newLedgerDoctorCmd(),
		newLedgerVerifyCmd(),
		newLedgerReplayCmd(),
		newLedgerAnchorCmd(),
	)
	return cmd
}

// newLedgerAnchorCmd writes a LEDGER_ANCHOR event recording the current
// tip hash + event count. Optional --sink names the external transparency
// log the operator plans to publish to; upload itself is intentionally
// out-of-scope at Plan 11 — operators schedule it via cron + their own
// uploader and pass the same --sink string each run for audit traceability.
func newLedgerAnchorCmd() *cobra.Command {
	var id, base, sink string
	cmd := &cobra.Command{
		Use:   "anchor",
		Short: "Append a LEDGER_ANCHOR event with the current tip hash for external publication",
		RunE: func(cmd *cobra.Command, args []string) error {
			eventsPath, _ := tenantPaths(base, id)
			rep, err := ledger.Doctor(eventsPath)
			if err != nil {
				return fmt.Errorf("doctor: %w", err)
			}
			if !rep.ChainIntact {
				return fmt.Errorf("refusing to anchor: chain not intact (%s)", rep.ChainError)
			}

			s, err := ledger.OpenStore(eventsPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			now := time.Now().UTC()
			payload, _ := json.Marshal(map[string]any{
				"tip_hash":    rep.LastHash,
				"event_count": rep.EventCount,
				"anchored_at": now.Format(time.RFC3339Nano),
				"sink":        sink,
			})
			if _, err := s.Append(ledger.Event{
				Kind:      "LEDGER_ANCHOR",
				Tenant:    id,
				Timestamp: now,
				Payload:   payload,
				PrevHash:  s.LastHash(),
			}); err != nil {
				return fmt.Errorf("append LEDGER_ANCHOR: %w", err)
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{
				"tip_hash":    rep.LastHash,
				"event_count": rep.EventCount,
				"sink":        sink,
				"anchored_at": now.Format(time.RFC3339Nano),
			})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&sink, "sink", "", "external sink identifier (free-text; e.g. 's3://audit-bucket/themis/anchors/' or 'git@github.com:org/themis-anchors.git')")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
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
		Short: "Walk the Merkle chain; non-zero exit + LEDGER_INTEGRITY_BROKEN incident if tampering detected",
		RunE: func(cmd *cobra.Command, args []string) error {
			events, _ := tenantPaths(base, id)
			verifyErr := ledger.Verify(events)
			if verifyErr != nil {
				// Record the integrity failure to the sidecar incidents file
				// before surfacing the error — the main ledger can no longer
				// be trusted to record its own failure (design spec §9.1.3).
				payload, _ := json.Marshal(map[string]string{
					"detected_at":  time.Now().UTC().Format(time.RFC3339Nano),
					"chain_error":  verifyErr.Error(),
					"tenant":       id,
					"source":       "themis ledger verify",
				})
				if logErr := incidents.Append(base, id, "LEDGER_INTEGRITY_BROKEN", payload); logErr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to record LEDGER_INTEGRITY_BROKEN: %v\n", logErr)
				}
				return verifyErr
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
