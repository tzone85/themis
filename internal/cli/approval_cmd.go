package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/approvals"
	"github.com/tzone85/themis/internal/ledger"
)

func newApprovalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "approval", Short: "Grant or deny approvals on REQUIRE_APPROVAL decisions"}
	cmd.AddCommand(newApprovalGrantCmd(), newApprovalDenyCmd(), newApprovalStatusCmd())
	return cmd
}

func newApprovalGrantCmd() *cobra.Command {
	var id, base, prID, approver, role, comment string
	cmd := &cobra.Command{
		Use:   "grant",
		Short: "Grant approval for a PR awaiting REQUIRE_APPROVAL",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApproval(cmd, id, base, prID, approver, role, comment, "", "grant")
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&prID, "pr-id", "", "PR identifier")
	cmd.Flags().StringVar(&approver, "approver", "", "approver identity (e.g. human:alice)")
	cmd.Flags().StringVar(&role, "role", "", "role being signed off (e.g. senior, compliance)")
	cmd.Flags().StringVar(&comment, "comment", "", "free-text comment recorded in the ledger payload")
	for _, n := range []string{"id", "base", "pr-id", "approver", "role"} {
		_ = cmd.MarkFlagRequired(n)
	}
	return cmd
}

func newApprovalDenyCmd() *cobra.Command {
	var id, base, prID, approver, role, reason string
	cmd := &cobra.Command{
		Use:   "deny",
		Short: "Deny approval for a PR awaiting REQUIRE_APPROVAL (sticky for the current decision)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApproval(cmd, id, base, prID, approver, role, "", reason, "deny")
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&prID, "pr-id", "", "PR identifier")
	cmd.Flags().StringVar(&approver, "approver", "", "approver identity (e.g. human:alice)")
	cmd.Flags().StringVar(&role, "role", "", "role denying the change")
	cmd.Flags().StringVar(&reason, "reason", "", "reason for the denial (required)")
	for _, n := range []string{"id", "base", "pr-id", "approver", "role", "reason"} {
		_ = cmd.MarkFlagRequired(n)
	}
	return cmd
}

func newApprovalStatusCmd() *cobra.Command {
	var id, base, prID string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print the current approval status for a PR",
		RunE: func(cmd *cobra.Command, args []string) error {
			eventsPath, _ := tenantPaths(base, id)
			events, err := ledger.ReadAll(eventsPath)
			if err != nil {
				return fmt.Errorf("read ledger: %w", err)
			}
			st := approvals.Compute(events, prID)
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

// runApproval is the shared implementation for grant + deny. It validates
// the precondition (a matching REQUIRE_APPROVAL decision exists), appends
// the approval event, and emits DECISION_FINALISED when the approval state
// is ripe.
func runApproval(cmd *cobra.Command, id, base, prID, approver, role, comment, reason, action string) error {
	eventsPath, _ := tenantPaths(base, id)
	events, err := ledger.ReadAll(eventsPath)
	if err != nil {
		return fmt.Errorf("read ledger: %w", err)
	}
	preStatus := approvals.Compute(events, prID)
	if preStatus.Decision.Verdict == "" {
		return errors.New("no DECISION_ISSUED found for this pr-id; run `themis decide` first")
	}
	if preStatus.Finalised {
		return errors.New("decision is already finalised; re-run `themis decide` before further approvals")
	}

	s, err := ledger.OpenStore(eventsPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now().UTC()
	var (
		kind    string
		payload []byte
	)
	switch action {
	case "grant":
		kind = "APPROVAL_GRANTED"
		payload, err = json.Marshal(approvals.GrantPayload{
			PRID: prID, Approver: approver, Role: role, Comment: comment, GrantedAt: now,
		})
	case "deny":
		kind = "APPROVAL_DENIED"
		payload, err = json.Marshal(approvals.DenyPayload{
			PRID: prID, Approver: approver, Role: role, Reason: reason, DeniedAt: now,
		})
	default:
		return fmt.Errorf("internal: unknown approval action %q", action)
	}
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	if _, err := s.Append(ledger.Event{
		Kind: kind, Tenant: id, Timestamp: now, Payload: payload, PrevHash: s.LastHash(),
	}); err != nil {
		return fmt.Errorf("append %s: %w", kind, err)
	}

	// Re-read events (cheap; ledger is per-tenant) to compute the freshly
	// updated status, then finalise if applicable.
	events, err = ledger.ReadAll(eventsPath)
	if err != nil {
		return fmt.Errorf("re-read ledger: %w", err)
	}
	st := approvals.Compute(events, prID)
	if _, ready := approvals.CanFinalise(st); ready {
		payload, err := json.Marshal(approvals.BuildFinalised(st, prID, now))
		if err != nil {
			return fmt.Errorf("marshal finalised: %w", err)
		}
		if _, err := s.Append(ledger.Event{
			Kind: "DECISION_FINALISED", Tenant: id, Timestamp: now, Payload: payload, PrevHash: s.LastHash(),
		}); err != nil {
			return fmt.Errorf("append DECISION_FINALISED: %w", err)
		}
		st.Finalised = true
		st.FinalVerdict = approvals.BuildFinalised(st, prID, now).FinalVerdict
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(st)
}
