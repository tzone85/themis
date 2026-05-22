package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/tenant"
)

func newTenantCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tenant", Short: "Manage Themis tenants"}
	cmd.AddCommand(newTenantInitCmd())
	return cmd
}

func newTenantInitCmd() *cobra.Command {
	var id, base string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialise a tenant directory tree + write TENANT_INITIALISED event",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := tenant.Init(base, id)
			if err != nil {
				return fmt.Errorf("init tenant: %w", err)
			}
			s, err := ledger.OpenStore(t.Events())
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			payload, _ := json.Marshal(map[string]string{"id": id, "base": base})
			e := ledger.Event{
				Kind:      "TENANT_INITIALISED",
				Tenant:    id,
				Timestamp: time.Now().UTC(),
				Payload:   payload,
				PrevHash:  s.LastHash(),
			}
			if _, err := s.Append(e); err != nil {
				return fmt.Errorf("append init event: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "tenant %q initialised at %s\n", id, t.Root())
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id (lowercase letters, digits, dash)")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	return cmd
}
