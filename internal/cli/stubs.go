package cli

import "github.com/spf13/cobra"

func newTenantCmd() *cobra.Command { return &cobra.Command{Use: "tenant"} }
func newLedgerCmd() *cobra.Command { return &cobra.Command{Use: "ledger"} }
