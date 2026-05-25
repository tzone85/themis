package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	var baseURL, token, tenantID string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the Themis MCP server over stdio (bridges to a running themis serve)",
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := &mcp.Server{
				BaseURL:  baseURL,
				Token:    token,
				TenantID: tenantID,
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return srv.Run(ctx, os.Stdin, os.Stdout)
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "", "URL of a running themis serve (e.g. http://127.0.0.1:8787)")
	cmd.Flags().StringVar(&token, "token", "", "Bearer token for the tenant")
	cmd.Flags().StringVar(&tenantID, "tenant-id", "", "tenant id all MCP tool calls are scoped to")
	_ = cmd.MarkFlagRequired("base-url")
	_ = cmd.MarkFlagRequired("tenant-id")
	return cmd
}
