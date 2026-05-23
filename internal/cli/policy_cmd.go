package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/policy"
)

func newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "policy", Short: "Validate and inspect tenant policy YAML"}
	cmd.AddCommand(newPolicyLintCmd())
	return cmd
}

func newPolicyLintCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Parse a policy YAML file; non-zero exit on any structural problem",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(path) // #nosec G304 -- operator-supplied policy path.
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
			p, err := policy.Parse(raw)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "policy %s: ok (version %d, default %s, %d rules)\n",
				path, p.Version, p.Default, len(p.Rules))
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "path to policy YAML")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
