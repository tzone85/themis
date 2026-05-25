// Package cli implements the themis CLI surface.
package cli

import (
	"github.com/spf13/cobra"
)

// Version is the embedded build version. ldflags-injectable at build time:
//
//	go build -ldflags="-X github.com/tzone85/themis/internal/cli.Version=v0.1.0" ./cmd/themis
var Version = "dev"

// NewRootCmd constructs the root `themis` command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "themis",
		Short:         "Themis — a compliance gateway for AI-generated code",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
	}
	root.SetVersionTemplate("themis {{.Version}}\n")
	root.AddCommand(newTenantCmd())
	root.AddCommand(newLedgerCmd())
	root.AddCommand(newCatalogueCmd())
	root.AddCommand(newClassifyCmd())
	root.AddCommand(newPolicyCmd())
	root.AddCommand(newDecideCmd())
	root.AddCommand(newBOMCmd())
	root.AddCommand(newIngestCmd())
	root.AddCommand(newServeCmd())
	return root
}
