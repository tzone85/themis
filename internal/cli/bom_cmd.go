package cli

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/bom"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
	"github.com/tzone85/themis/internal/sign"
)

func newBOMCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "bom", Short: "Build and sign AI Bills of Materials"}
	cmd.AddCommand(newBOMBuildCmd(), newBOMSignCmd())
	return cmd
}

func newBOMBuildCmd() *cobra.Command {
	var id, base, prID string
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a BOM for the most recent DECISION_ISSUED matching --pr-id",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, _, err := buildBOMFromLedger(base, id, prID)
			if err != nil {
				return err
			}
			canon, err := bom.Canonical(b)
			if err != nil {
				return err
			}
			if err := appendBOMBuiltEvent(base, id, b); err != nil {
				return err
			}
			_, _ = cmd.OutOrStdout().Write(canon)
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&prID, "pr-id", "", "PR identifier (matches DECISION_ISSUED.payload.pr_id)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	_ = cmd.MarkFlagRequired("pr-id")
	return cmd
}

func newBOMSignCmd() *cobra.Command {
	var id, base, prID, signerMode, oidcSubject, oidcIssuer string
	cmd := &cobra.Command{
		Use:   "sign",
		Short: "Build + sign a BOM with the tenant's signer (local ed25519 or cosign-keyless-stub); emit BOM_SIGNED",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, hash, err := buildBOMFromLedger(base, id, prID)
			if err != nil {
				return err
			}
			canon, err := bom.Canonical(b)
			if err != nil {
				return err
			}

			signer, err := sign.Resolve(sign.Mode(signerMode), sign.ResolveOptions{
				LocalKeyDir: filepath.Join(base, "tenants", id, "keys"),
				OIDCSubject: oidcSubject,
				OIDCIssuer:  oidcIssuer,
			})
			if err != nil {
				return err
			}
			bundle, err := signer.Sign(canon)
			if err != nil {
				return err
			}

			bomDir := filepath.Join(base, "tenants", id, "bom")
			if err := os.MkdirAll(bomDir, 0o700); err != nil {
				return fmt.Errorf("mkdir %s: %w", bomDir, err)
			}
			bomFile := filepath.Join(bomDir, hash+".bom.json")
			sigFile := bomFile + ".sig"
			bundleFile := bomFile + ".bundle.json"
			if err := os.WriteFile(bomFile, canon, 0o600); err != nil {
				return fmt.Errorf("write bom: %w", err)
			}
			if err := os.WriteFile(sigFile, []byte(hex.EncodeToString(bundle.Signature)), 0o600); err != nil {
				return fmt.Errorf("write signature: %w", err)
			}
			bundleBytes, _ := json.MarshalIndent(bundle, "", "  ")
			if err := os.WriteFile(bundleFile, bundleBytes, 0o600); err != nil {
				return fmt.Errorf("write bundle: %w", err)
			}

			if err := appendBOMSignedEvent(base, id, b, hash, bundle.Signature, bundle.PublicKey); err != nil {
				return err
			}

			out := map[string]string{
				"bom_hash":       hash,
				"signature_hex":  hex.EncodeToString(bundle.Signature),
				"public_key_hex": hex.EncodeToString(bundle.PublicKey),
				"signer_mode":    string(bundle.Mode),
				"rekor_url":      bundle.RekorURL,
				"bom_path":       bomFile,
				"signature_path": sigFile,
				"bundle_path":    bundleFile,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "tenant id")
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&prID, "pr-id", "", "PR identifier (matches DECISION_ISSUED.payload.pr_id)")
	cmd.Flags().StringVar(&signerMode, "signer", "local-ed25519", "signer mode: local-ed25519 | cosign-keyless-stub")
	cmd.Flags().StringVar(&oidcSubject, "oidc-subject", "", "OIDC subject (required for cosign-keyless-stub)")
	cmd.Flags().StringVar(&oidcIssuer, "oidc-issuer", "https://oidc.example.com", "OIDC issuer URL (cosign-keyless-stub)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("base")
	_ = cmd.MarkFlagRequired("pr-id")
	return cmd
}

// buildBOMFromLedger walks the per-tenant ledger to find the most recent
// DECISION_ISSUED for prID, plus the surrounding SCAN_FINDINGs, and
// reconstructs a BOM ready to be signed.
func buildBOMFromLedger(base, id, prID string) (bom.BOM, string, error) {
	eventsPath, _ := tenantPaths(base, id)
	events, err := ledger.ReadAll(eventsPath)
	if err != nil {
		return bom.BOM{}, "", fmt.Errorf("read ledger: %w", err)
	}

	// Walk backward to find the most recent DECISION_ISSUED for prID.
	decisionIdx := -1
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "DECISION_ISSUED" {
			continue
		}
		var payload struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(events[i].Payload, &payload); err == nil && payload.PRID == prID {
			decisionIdx = i
			break
		}
	}
	if decisionIdx < 0 {
		return bom.BOM{}, "", errors.New("no DECISION_ISSUED event found for the supplied --pr-id")
	}

	decisionEvent := events[decisionIdx]
	var dPayload struct {
		PRID     string            `json:"pr_id"`
		Actor    string            `json:"actor"`
		AIChange aichange.AIChange `json:"ai_change"`
		Impact   classify.Impact   `json:"impact"`
		Findings []scan.Finding    `json:"findings"`
		Decision policy.Decision   `json:"decision"`
	}
	if err := json.Unmarshal(decisionEvent.Payload, &dPayload); err != nil {
		return bom.BOM{}, "", fmt.Errorf("parse DECISION_ISSUED payload: %w", err)
	}

	// The AIChange is embedded in the DECISION_ISSUED payload (pipeline.Run
	// writes it there). Fall back to a stub for ledgers written by older
	// versions that did not include it — the PRID + Actor are always
	// available from the decision payload itself.
	aiChange := dPayload.AIChange
	if aiChange.PRID == "" {
		aiChange.PRID = prID
		aiChange.Actor = dPayload.Actor
	}

	b := bom.BOM{
		SchemaVersion: bom.CurrentSchemaVersion,
		PRID:          dPayload.PRID,
		Tenant:        id,
		Actor:         dPayload.Actor,
		BuiltAt:       time.Unix(0, 0).UTC(), // overwritten below
		AIChange:      aiChange,
		Impact:        dPayload.Impact,
		Findings:      dPayload.Findings,
		Decision:      dPayload.Decision,
		LedgerTip:     "", // overwritten below
	}
	// Tip = hash of the most recent event in the ledger.
	if len(events) > 0 {
		tipHash, hashErr := events[len(events)-1].ContentHash()
		if hashErr != nil {
			return bom.BOM{}, "", fmt.Errorf("compute ledger tip: %w", hashErr)
		}
		b.LedgerTip = tipHash
	}
	// BuiltAt is set deterministically to the DECISION_ISSUED timestamp so
	// re-running `bom build` reproduces the same canonical bytes.
	b.BuiltAt = decisionEvent.Timestamp.UTC()

	hash, err := bom.Hash(b)
	if err != nil {
		return bom.BOM{}, "", err
	}
	return b, hash, nil
}

func appendBOMBuiltEvent(base, id string, b bom.BOM) error {
	hash, err := bom.Hash(b)
	if err != nil {
		return err
	}
	events, _ := tenantPaths(base, id)
	s, err := ledger.OpenStore(events)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	payload, _ := json.Marshal(map[string]any{
		"pr_id":          b.PRID,
		"schema_version": b.SchemaVersion,
		"bom_hash":       hash,
	})
	e := ledger.Event{
		Kind:      "BOM_BUILT",
		Tenant:    id,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		PrevHash:  s.LastHash(),
	}
	_, err = s.Append(e)
	return err
}

func appendBOMSignedEvent(base, id string, b bom.BOM, hash string, signature, publicKey []byte) error {
	events, _ := tenantPaths(base, id)
	s, err := ledger.OpenStore(events)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	payload, _ := json.Marshal(map[string]any{
		"pr_id":          b.PRID,
		"bom_hash":       hash,
		"signature_hex":  hex.EncodeToString(signature),
		"public_key_hex": hex.EncodeToString(publicKey),
	})
	e := ledger.Event{
		Kind:      "BOM_SIGNED",
		Tenant:    id,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		PrevHash:  s.LastHash(),
	}
	_, err = s.Append(e)
	return err
}
