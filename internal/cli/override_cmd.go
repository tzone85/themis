package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/override"
)

func newOverrideCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "override", Short: "Invoke and manage emergency overrides + their post-mortems"}
	cmd.AddCommand(newOverrideInvokeCmd(), newOverrideClosePMCmd(), newOverrideStatusCmd())
	return cmd
}

func newOverrideInvokeCmd() *cobra.Command {
	var id, base, prID, actor, coSigner, reason, scope string
	var ttlMinutes int
	cmd := &cobra.Command{
		Use:   "invoke",
		Short: "Invoke an emergency override (requires ≥50-char reason, co-signer, time-boxed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			now := time.Now().UTC()
			expires := now.Add(override.DefaultDuration)
			if ttlMinutes > 0 {
				expires = now.Add(time.Duration(ttlMinutes) * time.Minute)
			}
			payload := override.InvokePayload{
				PRID: prID, Actor: actor, CoSigner: coSigner,
				Reason: reason, Scope: scope, ExpiresAt: expires,
			}
			if err := override.ValidateInvoke(payload, now); err != nil {
				return err
			}
			invoke, due := override.BuildInvoke(payload, now)

			eventsPath, _ := tenantPaths(base, id)
			s, err := ledger.OpenStore(eventsPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			ip, _ := json.Marshal(invoke)
			if _, err := s.Append(ledger.Event{
				Kind: "EMERGENCY_OVERRIDE_INVOKED", Tenant: id, Timestamp: now, Payload: ip, PrevHash: s.LastHash(),
			}); err != nil {
				return fmt.Errorf("append invoke: %w", err)
			}
			dp, _ := json.Marshal(due)
			if _, err := s.Append(ledger.Event{
				Kind: "OVERRIDE_POSTMORTEM_DUE", Tenant: id, Timestamp: now, Payload: dp, PrevHash: s.LastHash(),
			}); err != nil {
				return fmt.Errorf("append postmortem-due: %w", err)
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(invoke)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&prID, "pr-id", "", "PR identifier")
	cmd.Flags().StringVar(&actor, "actor", "", "named actor invoking the override (e.g. human:alice)")
	cmd.Flags().StringVar(&coSigner, "co-signer", "", "named co-signer (different from --actor)")
	cmd.Flags().StringVar(&reason, "reason", "", "free-text reason (≥50 chars, required)")
	cmd.Flags().StringVar(&scope, "scope", "", "scope: e.g. 'one-pr', 'one-tenant'")
	cmd.Flags().IntVar(&ttlMinutes, "ttl-minutes", 0, "override time-to-live in minutes (default 24h)")
	for _, n := range []string{"id", "base", "pr-id", "actor", "co-signer", "reason"} {
		_ = cmd.MarkFlagRequired(n)
	}
	return cmd
}

func newOverrideClosePMCmd() *cobra.Command {
	var id, base, prID, closer, notes string
	cmd := &cobra.Command{
		Use:   "close-postmortem",
		Short: "Close the mandatory post-mortem for a previously-invoked override",
		RunE: func(cmd *cobra.Command, args []string) error {
			now := time.Now().UTC()
			eventsPath, _ := tenantPaths(base, id)
			events, err := ledger.ReadAll(eventsPath)
			if err != nil {
				return fmt.Errorf("read ledger: %w", err)
			}
			st := override.Compute(events, prID, now)
			if !st.PostmortemDue {
				return fmt.Errorf("no override post-mortem due for pr-id %q", prID)
			}
			if st.PostmortemClosed {
				return fmt.Errorf("post-mortem already closed for pr-id %q", prID)
			}
			s, err := ledger.OpenStore(eventsPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()
			payload, _ := json.Marshal(override.BuildClosed(prID, closer, notes, now))
			if _, err := s.Append(ledger.Event{
				Kind: "OVERRIDE_POSTMORTEM_CLOSED", Tenant: id, Timestamp: now, Payload: payload, PrevHash: s.LastHash(),
			}); err != nil {
				return fmt.Errorf("append closed: %w", err)
			}
			refreshed, _ := ledger.ReadAll(eventsPath)
			st2 := override.Compute(refreshed, prID, now)
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(st2)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&prID, "pr-id", "", "PR identifier")
	cmd.Flags().StringVar(&closer, "closer", "", "actor closing the post-mortem (e.g. human:compliance)")
	cmd.Flags().StringVar(&notes, "notes", "", "post-mortem notes (required)")
	for _, n := range []string{"id", "base", "pr-id", "closer", "notes"} {
		_ = cmd.MarkFlagRequired(n)
	}
	return cmd
}

func newOverrideStatusCmd() *cobra.Command {
	var id, base, prID string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print the current override status for a PR",
		RunE: func(cmd *cobra.Command, args []string) error {
			eventsPath, _ := tenantPaths(base, id)
			events, err := ledger.ReadAll(eventsPath)
			if err != nil {
				return fmt.Errorf("read ledger: %w", err)
			}
			st := override.Compute(events, prID, time.Now().UTC())
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(st)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&prID, "pr-id", "", "PR identifier")
	for _, n := range []string{"id", "base", "pr-id"} {
		_ = cmd.MarkFlagRequired(n)
	}
	return cmd
}
