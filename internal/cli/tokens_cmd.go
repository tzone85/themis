package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/tzone85/themis/internal/auth"
)

func newTokensCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tokens", Short: "Manage API tokens (tenant + role)"}
	cmd.AddCommand(newTokensGrantCmd(), newTokensListCmd(), newTokensRevokeCmd())
	return cmd
}

// tokensFile is the on-disk shape we read + write.
type tokensFile struct {
	Tokens []tokenEntry `yaml:"tokens"`
}

type tokenEntry struct {
	Token       string `yaml:"token"`
	Tenant      string `yaml:"tenant"`
	Role        string `yaml:"role"`
	Description string `yaml:"description,omitempty"`
}

func tokensYAMLPath(base string) string {
	return filepath.Join(base, "tenants", "tokens.yaml")
}

func readTokensFile(base string) (tokensFile, error) {
	raw, err := os.ReadFile(tokensYAMLPath(base)) // #nosec G304 -- operator-supplied base.
	if err != nil {
		if os.IsNotExist(err) {
			return tokensFile{}, nil
		}
		return tokensFile{}, err
	}
	var f tokensFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return tokensFile{}, fmt.Errorf("parse tokens.yaml: %w", err)
	}
	return f, nil
}

func writeTokensFile(base string, f tokensFile) error {
	if err := os.MkdirAll(filepath.Dir(tokensYAMLPath(base)), 0o700); err != nil {
		return err
	}
	raw, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal tokens.yaml: %w", err)
	}
	return os.WriteFile(tokensYAMLPath(base), raw, 0o600)
}

func newTokensGrantCmd() *cobra.Command {
	var base, tenant, roleStr, description string
	cmd := &cobra.Command{
		Use:   "grant",
		Short: "Generate a new bearer token bound to <tenant, role>; prints once",
		RunE: func(cmd *cobra.Command, args []string) error {
			role := auth.Role(roleStr)
			if role.Rank() < 0 {
				return fmt.Errorf("unknown role %q (must be one of read|dev|reviewer|compliance|admin)", roleStr)
			}
			tok := make([]byte, 32)
			if _, err := rand.Read(tok); err != nil {
				return fmt.Errorf("random: %w", err)
			}
			plain := "thm_" + hex.EncodeToString(tok)

			doc, err := readTokensFile(base)
			if err != nil {
				return err
			}
			doc.Tokens = append(doc.Tokens, tokenEntry{
				Token:       plain,
				Tenant:      tenant,
				Role:        string(role),
				Description: description,
			})
			if err := writeTokensFile(base, doc); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "TOKEN (record this now — it will not be shown again):\n%s\n", plain)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "tenant=%s role=%s description=%q\n", tenant, role, description)
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&tenant, "tenant", "", "tenant id this token belongs to")
	cmd.Flags().StringVar(&roleStr, "role", "", "role (read|dev|reviewer|compliance|admin)")
	cmd.Flags().StringVar(&description, "description", "", "free-text reminder (e.g. 'alice's laptop')")
	for _, n := range []string{"base", "tenant", "role"} {
		_ = cmd.MarkFlagRequired(n)
	}
	return cmd
}

func newTokensListCmd() *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered tokens (last-4 suffix only; full token never reprinted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := readTokensFile(base)
			if err != nil {
				return err
			}
			if len(doc.Tokens) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(no tokens registered)")
				return nil
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "SUFFIX  TENANT          ROLE        DESCRIPTION")
			for _, t := range doc.Tokens {
				suffix := t.Token
				if len(suffix) > 4 {
					suffix = suffix[len(suffix)-4:]
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-6s  %-14s  %-10s  %s\n", suffix, t.Tenant, t.Role, t.Description)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	_ = cmd.MarkFlagRequired("base")
	return cmd
}

func newTokensRevokeCmd() *cobra.Command {
	var base, prefix string
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Remove a token entry by matching its suffix or substring",
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := readTokensFile(base)
			if err != nil {
				return err
			}
			kept := doc.Tokens[:0]
			removed := 0
			for _, t := range doc.Tokens {
				if strings.Contains(t.Token, prefix) {
					removed++
					continue
				}
				kept = append(kept, t)
			}
			if removed == 0 {
				return fmt.Errorf("no token matched %q", prefix)
			}
			doc.Tokens = kept
			if err := writeTokensFile(base, doc); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "revoked %d token(s) matching %q\n", removed, prefix)
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&prefix, "token-prefix", "", "substring of the token to remove (use the suffix shown by `tokens list`)")
	_ = cmd.MarkFlagRequired("base")
	_ = cmd.MarkFlagRequired("token-prefix")
	return cmd
}
