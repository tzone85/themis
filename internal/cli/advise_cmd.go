package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/advisor"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/mempalace"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
)

func newAdviseCmd() *cobra.Command {
	var id, base, prID, llmName string
	cmd := &cobra.Command{
		Use:   "advise",
		Short: "Draft an advisory note for the most recent DECISION_ISSUED matching --pr-id; write it to the tenant's Mempalace wing",
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := loadDecisionInput(base, id, prID)
			if err != nil {
				return err
			}
			out, err := advisor.Draft(context.Background(), resolveLLM(llmName), in)
			if err != nil {
				return err
			}
			body, err := json.Marshal(out)
			if err != nil {
				return err
			}
			path, err := mempalace.New(base).Write(mempalace.Drawer{
				Kind:        "advisor-note",
				Tenant:      id,
				Body:        body,
				Tags:        []string{prID, out.Summary.Verdict},
				Description: "advisor draft for " + prID,
			})
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{
				"backed_by":    out.BackedBy,
				"verdict":      out.Summary.Verdict,
				"note":         out.Text,
				"drawer_path":  path,
				"summary":      out.Summary,
			})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&prID, "pr-id", "", "PR identifier (matches DECISION_ISSUED.payload.pr_id)")
	cmd.Flags().StringVar(&llmName, "llm", "null", "LLM backend (null only at Plan 17; provider adapters land later)")
	for _, n := range []string{"id", "base", "pr-id"} {
		_ = cmd.MarkFlagRequired(n)
	}
	return cmd
}

// loadDecisionInput rebuilds the Input the advisor needs by walking back
// through the ledger to the most recent DECISION_ISSUED for prID.
func loadDecisionInput(base, id, prID string) (advisor.Input, error) {
	eventsPath, _ := tenantPaths(base, id)
	events, err := ledger.ReadAll(eventsPath)
	if err != nil {
		return advisor.Input{}, fmt.Errorf("read ledger: %w", err)
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "DECISION_ISSUED" {
			continue
		}
		var p struct {
			PRID     string           `json:"pr_id"`
			Actor    string           `json:"actor"`
			Impact   classify.Impact  `json:"impact"`
			Findings []scan.Finding   `json:"findings"`
			Decision policy.Decision  `json:"decision"`
		}
		if err := json.Unmarshal(events[i].Payload, &p); err != nil {
			continue
		}
		if p.PRID != prID {
			continue
		}
		return advisor.Input{
			PRID: p.PRID, Actor: p.Actor,
			Impact: p.Impact, Findings: p.Findings, Decision: p.Decision,
		}, nil
	}
	return advisor.Input{}, errors.New("no DECISION_ISSUED found for the supplied --pr-id")
}

// resolveLLM picks the advisor backend. Plan 17 ships NullLLM only; real
// providers land as drop-in implementations.
func resolveLLM(name string) advisor.LLM {
	switch name {
	case "", "null":
		return advisor.NullLLM{}
	}
	return advisor.NullLLM{}
}

// keep the linter quiet when the standalone filepath import is unused
// during refactors.
var _ = filepath.Join
