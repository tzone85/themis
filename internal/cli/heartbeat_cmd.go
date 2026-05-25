package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/ledger"
)

func newHeartbeatCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "heartbeat", Short: "Record ENFORCEMENT_MISSING signals from external monitoring"}
	cmd.AddCommand(newHeartbeatReportCmd())
	return cmd
}

// newHeartbeatReportCmd is the dataplane endpoint design spec §9.1.2 expects:
// an external observer (a GitHub Action heartbeat job, an Argo CD policy
// check, a synthetic monitoring probe) records that a required enforcement
// check is missing on a repo. The Themis core itself doesn't poll — it
// just records the signal so the audit trail captures the gap.
func newHeartbeatReportCmd() *cobra.Command {
	var id, base, repo, expectedCheck, lastSeen, reportedBy string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Record an ENFORCEMENT_MISSING event when external monitoring detects a missing required check",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repo == "" || expectedCheck == "" || reportedBy == "" {
				return fmt.Errorf("--repo, --expected-check, --reported-by are required")
			}
			now := time.Now().UTC()
			payload := map[string]string{
				"repo":            repo,
				"expected_check":  expectedCheck,
				"reported_by":     reportedBy,
				"reported_at":     now.Format(time.RFC3339Nano),
			}
			if lastSeen != "" {
				payload["last_seen"] = lastSeen
			}
			raw, _ := json.Marshal(payload)

			eventsPath, _ := tenantPaths(base, id)
			s, err := ledger.OpenStore(eventsPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			if _, err := s.Append(ledger.Event{
				Kind:      "ENFORCEMENT_MISSING",
				Tenant:    id,
				Timestamp: now,
				Payload:   raw,
				PrevHash:  s.LastHash(),
			}); err != nil {
				return fmt.Errorf("append ENFORCEMENT_MISSING: %w", err)
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(payload)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&repo, "repo", "", "repository the missing check applies to (e.g. gh:org/svc)")
	cmd.Flags().StringVar(&expectedCheck, "expected-check", "", "name of the required check (e.g. 'themis-check')")
	cmd.Flags().StringVar(&lastSeen, "last-seen", "", "last RFC3339 timestamp the check was observed (optional)")
	cmd.Flags().StringVar(&reportedBy, "reported-by", "", "who/what is reporting (e.g. 'gh-action-watchdog')")
	for _, n := range []string{"id", "base", "repo", "expected-check", "reported-by"} {
		_ = cmd.MarkFlagRequired(n)
	}
	return cmd
}
