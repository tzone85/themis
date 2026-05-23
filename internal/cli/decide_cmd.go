package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/catalogue"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
)

func newDecideCmd() *cobra.Command {
	var id, base, changePath, policyPath, cataloguePath, workdir string
	cmd := &cobra.Command{
		Use:   "decide",
		Short: "Classify + scan + apply policy; emit SCAN_FINDING + DECISION_ISSUED events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecide(cmd, id, base, changePath, policyPath, cataloguePath, workdir)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&changePath, "aichange", "", "path to AIChange JSON file")
	cmd.Flags().StringVar(&policyPath, "policy", "", "path to policy YAML")
	cmd.Flags().StringVar(&cataloguePath, "catalogue", "", "path to catalogue snapshot JSON (defaults to tenant's catalogue.json)")
	cmd.Flags().StringVar(&workdir, "workdir", "", "directory whose files mirror the AIChange's AfterHash side; scanners read content from here. Optional — when omitted, scanners run with no bodies.")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	_ = cmd.MarkFlagRequired("aichange")
	_ = cmd.MarkFlagRequired("policy")
	return cmd
}

func runDecide(cmd *cobra.Command, id, base, changePath, policyPath, cataloguePath, workdir string) error {
	registry := ledger.DefaultRegistry()
	for _, kind := range []string{"SCAN_FINDING", "DECISION_ISSUED", "POLICY_INVALID"} {
		if _, ok := registry.Projector(kind); !ok {
			return fmt.Errorf("ledger: required kind %q not registered", kind)
		}
	}

	// Catalogue.
	if cataloguePath == "" {
		cataloguePath = snapshotPath(base, id)
	}
	rawCat, err := os.ReadFile(cataloguePath) // #nosec G304 -- tenant-scoped path or explicit flag.
	if err != nil {
		return fmt.Errorf("read catalogue snapshot %s: %w", cataloguePath, err)
	}
	var g catalogue.CatalogueGraph
	if err := json.Unmarshal(rawCat, &g); err != nil {
		return fmt.Errorf("parse catalogue snapshot: %w", err)
	}

	// AIChange.
	rawCh, err := os.ReadFile(changePath) // #nosec G304 -- operator-supplied path.
	if err != nil {
		return fmt.Errorf("read aichange %s: %w", changePath, err)
	}
	var c aichange.AIChange
	if err := json.Unmarshal(rawCh, &c); err != nil {
		return fmt.Errorf("parse aichange: %w", err)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("invalid aichange: %w", err)
	}

	// Policy.
	rawPol, err := os.ReadFile(policyPath) // #nosec G304 -- operator-supplied path.
	if err != nil {
		return fmt.Errorf("read policy %s: %w", policyPath, err)
	}
	p, err := policy.Parse(rawPol)
	if err != nil {
		// Best-effort: log POLICY_INVALID before returning so the audit
		// trail includes the failure (design spec §8.2 "fail closed").
		if storeErr := emitPolicyInvalid(base, id, policyPath, err); storeErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to record POLICY_INVALID: %v\n", storeErr)
		}
		return err
	}

	// Classify.
	imp := classify.Classify(c, g)

	// Scan.
	bodies, err := readBodies(workdir, c)
	if err != nil {
		return fmt.Errorf("read workdir bodies: %w", err)
	}
	findings := scan.RunAll(scan.DefaultScanners(), c, bodies)

	// Open ledger once for the cluster of events we'll emit.
	events, _ := tenantPaths(base, id)
	s, err := ledger.OpenStore(events)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	// Emit one SCAN_FINDING per finding (per spec §6.1 "each finding stored
	// individually so adding/removing scanners doesn't invalidate history").
	for _, f := range findings {
		payload, err := json.Marshal(map[string]any{"pr_id": c.PRID, "finding": f})
		if err != nil {
			return fmt.Errorf("marshal finding: %w", err)
		}
		e := ledger.Event{
			Kind:      "SCAN_FINDING",
			Tenant:    id,
			Timestamp: time.Now().UTC(),
			Payload:   payload,
			PrevHash:  s.LastHash(),
		}
		if _, err := s.Append(e); err != nil {
			return fmt.Errorf("append SCAN_FINDING: %w", err)
		}
	}

	// Decide.
	decision := policy.Decide(c, imp, findings, p)

	payload, err := json.Marshal(map[string]any{
		"pr_id":    c.PRID,
		"actor":    c.Actor,
		"impact":   imp,
		"findings": findings,
		"decision": decision,
	})
	if err != nil {
		return fmt.Errorf("marshal decision payload: %w", err)
	}
	e := ledger.Event{
		Kind:      "DECISION_ISSUED",
		Tenant:    id,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		PrevHash:  s.LastHash(),
	}
	if _, err := s.Append(e); err != nil {
		return fmt.Errorf("append DECISION_ISSUED: %w", err)
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(decision)
}

// readBodies pulls AfterHash-side file content from workdir for every
// ADDED/MODIFIED FileTouch. Missing files are tolerated — scanners report
// no findings for paths they can't read.
func readBodies(workdir string, c aichange.AIChange) (map[string][]byte, error) {
	out := map[string][]byte{}
	if workdir == "" {
		return out, nil
	}
	for _, ft := range c.TouchedFiles {
		if ft.ChangeKind == aichange.FileDeleted {
			continue
		}
		path := filepath.Join(workdir, ft.Path)
		body, err := os.ReadFile(path) // #nosec G304 -- workdir is operator-supplied per command invocation.
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		out[ft.Path] = body
	}
	return out, nil
}

func emitPolicyInvalid(base, id, policyPath string, parseErr error) error {
	events, _ := tenantPaths(base, id)
	s, err := ledger.OpenStore(events)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	payload, _ := json.Marshal(map[string]string{"policy_path": policyPath, "error": parseErr.Error()})
	e := ledger.Event{
		Kind:      "POLICY_INVALID",
		Tenant:    id,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		PrevHash:  s.LastHash(),
	}
	_, err = s.Append(e)
	return err
}
