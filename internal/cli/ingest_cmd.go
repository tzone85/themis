package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/ingest"
	"github.com/tzone85/themis/internal/ledger"
)

func newIngestCmd() *cobra.Command {
	var id, base, adapterName, prID, workdir, baseRef, transcript, actor, out string
	var fileFlags []string

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Run an ingestion adapter to produce an AIChange JSON + emit INGEST_COMPLETED",
		Long: "Available adapters: " + strings.Join(ingest.All(), ", "),
		RunE: func(cmd *cobra.Command, args []string) error {
			adapter, ok := ingest.Resolve(adapterName)
			if !ok {
				return fmt.Errorf("unknown adapter %q (available: %s)", adapterName, strings.Join(ingest.All(), ", "))
			}

			files, ferr := parseFileFlags(fileFlags)
			if ferr != nil {
				return ferr
			}

			in := ingest.Inputs{
				PRID:           prID,
				ActorOverride:  actor,
				Workdir:        workdir,
				BaseRef:        baseRef,
				TranscriptPath: transcript,
				Files:          files,
			}

			ai, ierr := adapter.Ingest(in)
			if ierr != nil {
				if logErr := emitAdapterFailed(base, id, adapterName, prID, ierr); logErr != nil {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to record INGEST_ADAPTER_FAILED: %v\n", logErr)
				}
				return ierr
			}
			if err := ai.Validate(); err != nil {
				return fmt.Errorf("adapter produced invalid AIChange: %w", err)
			}

			raw, err := json.MarshalIndent(ai, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal aichange: %w", err)
			}

			outPath := out
			if outPath == "" {
				outPath = defaultIngestOutPath(base, id, prID)
			}
			if err := os.WriteFile(outPath, raw, 0o600); err != nil {
				return fmt.Errorf("write %s: %w", outPath, err)
			}

			if err := emitIngestCompleted(base, id, adapterName, prID, outPath, len(ai.TouchedFiles)); err != nil {
				return err
			}

			payload := map[string]any{
				"adapter":      adapterName,
				"pr_id":        prID,
				"aichange_path": outPath,
				"file_count":   len(ai.TouchedFiles),
				"actor":        ai.Actor,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(payload)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&adapterName, "adapter", "", "adapter name ("+strings.Join(ingest.All(), "|")+")")
	cmd.Flags().StringVar(&prID, "pr-id", "", "PR identifier (e.g. gh:org/repo#123)")
	cmd.Flags().StringVar(&workdir, "workdir", "", "(git_heuristic) checkout root")
	cmd.Flags().StringVar(&baseRef, "base-ref", "", "(git_heuristic) base ref to diff against (default HEAD~1)")
	cmd.Flags().StringVar(&transcript, "transcript", "", "(claude_code_transcript) transcript JSON path")
	cmd.Flags().StringVar(&actor, "actor", "", "actor override (manual: required, must start with 'human:')")
	cmd.Flags().StringArrayVar(&fileFlags, "file", nil, "(manual) repeatable: <path>=<beforeHash>,<afterHash>")
	cmd.Flags().StringVar(&out, "out", "", "output AIChange JSON path (default tenants/<id>/aichange/<pr>.json)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	_ = cmd.MarkFlagRequired("adapter")
	_ = cmd.MarkFlagRequired("pr-id")
	return cmd
}

// parseFileFlags converts repeated --file=path=before,after entries into
// the map shape expected by Inputs.Files.
func parseFileFlags(flags []string) (map[string][2]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	out := make(map[string][2]string, len(flags))
	for _, f := range flags {
		eq := strings.IndexByte(f, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("--file %q must be path=before,after", f)
		}
		path := f[:eq]
		rest := f[eq+1:]
		comma := strings.IndexByte(rest, ',')
		if comma < 0 {
			return nil, fmt.Errorf("--file %q must be path=before,after (missing comma)", f)
		}
		out[path] = [2]string{rest[:comma], rest[comma+1:]}
	}
	return out, nil
}

// defaultIngestOutPath builds the standard per-tenant location for AIChange
// JSON files. PR IDs are sanitised so they're safe to use in filenames.
func defaultIngestOutPath(base, id, prID string) string {
	safe := strings.NewReplacer("/", "_", "#", "_", ":", "_", " ", "_").Replace(prID)
	dir := tenantSubdir(base, id, "aichange")
	_ = os.MkdirAll(dir, 0o700)
	return dir + "/" + safe + ".json"
}

func tenantSubdir(base, id, name string) string {
	return base + "/tenants/" + id + "/" + name
}

func emitIngestCompleted(base, id, adapter, prID, path string, fileCount int) error {
	return appendIngestEvent(base, id, "INGEST_COMPLETED", map[string]any{
		"adapter":       adapter,
		"pr_id":         prID,
		"aichange_path": path,
		"file_count":    fileCount,
	})
}

func emitAdapterFailed(base, id, adapter, prID string, cause error) error {
	reason := cause.Error()
	if errors.Is(cause, ingest.ErrAdapterFailed) {
		reason = strings.TrimPrefix(reason, "ingest: adapter failed: ")
	}
	return appendIngestEvent(base, id, "INGEST_ADAPTER_FAILED", map[string]any{
		"adapter": adapter,
		"pr_id":   prID,
		"reason":  reason,
	})
}

func appendIngestEvent(base, id, kind string, payload map[string]any) error {
	events, _ := tenantPaths(base, id)
	s, err := ledger.OpenStore(events)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	raw, _ := json.Marshal(payload)
	e := ledger.Event{
		Kind:      kind,
		Tenant:    id,
		Timestamp: time.Now().UTC(),
		Payload:   raw,
		PrevHash:  s.LastHash(),
	}
	_, err = s.Append(e)
	return err
}
