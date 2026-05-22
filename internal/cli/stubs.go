package cli

import "github.com/spf13/cobra"

func newLedgerCmd() *cobra.Command { return &cobra.Command{Use: "ledger"} }
