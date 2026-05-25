package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/catalogue"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/pipeline"
	"github.com/tzone85/themis/internal/policy"
)

// decideRequest is the JSON body shape POST /v1/tenants/{id}/decide accepts.
type decideRequest struct {
	AIChange     aichange.AIChange `json:"ai_change"`
	PolicyYAML   string            `json:"policy_yaml"`
	// WorkdirFiles maps PR path → base64-encoded file body. Optional; when
	// absent, scanners run with no body content and report no scanner-level
	// findings for those paths.
	WorkdirFiles map[string]string `json:"workdir_files,omitempty"`
}

// handleDecide is the POST handler that mirrors `themis decide` over HTTP.
func (s *server) handleDecide(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req decideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if err := req.AIChange.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid ai_change: "+err.Error())
		return
	}
	if req.PolicyYAML == "" {
		writeError(w, http.StatusBadRequest, "policy_yaml is required")
		return
	}

	bodies, err := decodeBodies(req.WorkdirFiles)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	pol, err := policy.Parse([]byte(req.PolicyYAML))
	if err != nil {
		if logErr := s.emitPolicyInvalidEvent(id, err); logErr != nil {
			// Best-effort: ledger logging failure doesn't change the HTTP response.
			_ = logErr
		}
		writeError(w, http.StatusBadRequest, "invalid policy: "+err.Error())
		return
	}

	g, err := s.loadCatalogueSnapshot(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load catalogue: "+err.Error())
		return
	}

	store, err := s.openTenantStore(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "open store: "+err.Error())
		return
	}
	defer func() { _ = store.Close() }()

	result, err := pipeline.Run(store, id, req.AIChange, g, pol, bodies, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pipeline: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// decodeBodies converts the base64-encoded map in the request into raw bytes.
func decodeBodies(in map[string]string) (map[string][]byte, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string][]byte, len(in))
	for path, b64 := range in {
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, errors.New("workdir_files[" + path + "]: invalid base64")
		}
		out[path] = raw
	}
	return out, nil
}

// loadCatalogueSnapshot reads tenants/<id>/catalogue.json.
func (s *server) loadCatalogueSnapshot(id string) (catalogue.CatalogueGraph, error) {
	raw, err := os.ReadFile(filepath.Join(s.base, "tenants", id, "catalogue.json")) // #nosec G304 -- tenant-scoped path.
	if err != nil {
		return catalogue.CatalogueGraph{}, err
	}
	var g catalogue.CatalogueGraph
	if err := json.Unmarshal(raw, &g); err != nil {
		return catalogue.CatalogueGraph{}, err
	}
	return g, nil
}

// openTenantStore opens the per-tenant events.jsonl.
func (s *server) openTenantStore(id string) (*ledger.Store, error) {
	return ledger.OpenStore(filepath.Join(s.base, "tenants", id, "events.jsonl"))
}

// emitPolicyInvalidEvent best-effort logs a POLICY_INVALID event when the
// request carries malformed policy YAML.
func (s *server) emitPolicyInvalidEvent(id string, parseErr error) error {
	store, err := s.openTenantStore(id)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	payload, _ := json.Marshal(map[string]string{
		"source": "api",
		"error":  parseErr.Error(),
	})
	_, err = store.Append(ledger.Event{
		Kind:      "POLICY_INVALID",
		Tenant:    id,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		PrevHash:  store.LastHash(),
	})
	return err
}
