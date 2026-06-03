// Package cli implements the themis CLI surface.
package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build metadata, injected at link time via ldflags. Defaults make a
// `go build ./...` (without ldflags) still emit something honest.
//
//	go build -ldflags=" \
//	  -X github.com/tzone85/themis/internal/cli.Version=v0.1.0 \
//	  -X github.com/tzone85/themis/internal/cli.Commit=abc1234 \
//	  -X github.com/tzone85/themis/internal/cli.Date=2026-06-03T12:34:56Z" \
//	  ./cmd/themis
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// versionString renders the four-field build identity. Kept as a function
// (not a const) so tests can swap Version/Commit/Date and observe.
func versionString() string {
	return fmt.Sprintf("%s (commit %s, built %s, %s)", Version, Commit, Date, runtime.Version())
}

// NewRootCmd constructs the root `themis` command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "themis",
		Short:         "Themis — a compliance gateway for AI-generated code",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       versionString(),
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
	root.AddCommand(newMCPCmd())
	root.AddCommand(newApprovalCmd())
	root.AddCommand(newOverrideCmd())
	root.AddCommand(newHeartbeatCmd())
	root.AddCommand(newTokensCmd())
	root.AddCommand(newAdviseCmd())
	return root
}
