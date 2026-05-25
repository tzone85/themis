package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/tzone85/themis/internal/incidents"
	"github.com/tzone85/themis/internal/ledger"
)

type heartbeatRequest struct {
	Repo          string `json:"repo"`
	ExpectedCheck string `json:"expected_check"`
	LastSeen      string `json:"last_seen,omitempty"`
	ReportedBy    string `json:"reported_by"`
}

type anchorRequest struct {
	Sink string `json:"sink,omitempty"`
}

// handleHeartbeat records an ENFORCEMENT_MISSING ledger event.
func (s *server) handleHeartbeat(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Repo) == "" || strings.TrimSpace(req.ExpectedCheck) == "" || strings.TrimSpace(req.ReportedBy) == "" {
		writeError(w, http.StatusBadRequest, "repo, expected_check, reported_by are required")
		return
	}

	now := time.Now().UTC()
	payload := map[string]string{
		"repo":           req.Repo,
		"expected_check": req.ExpectedCheck,
		"reported_by":    req.ReportedBy,
		"reported_at":    now.Format(time.RFC3339Nano),
	}
	if req.LastSeen != "" {
		payload["last_seen"] = req.LastSeen
	}
	raw, _ := json.Marshal(payload)

	store, err := s.openTenantStore(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "open store: "+err.Error())
		return
	}
	defer func() { _ = store.Close() }()

	if _, err := store.Append(ledger.Event{
		Kind: "ENFORCEMENT_MISSING", Tenant: id, Timestamp: now, Payload: raw, PrevHash: store.LastHash(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "append: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// handleAnchor records a LEDGER_ANCHOR event with the current tip hash.
func (s *server) handleAnchor(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req anchorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Body is optional; treat decode failure as "no body" only when EOF/empty.
		if r.ContentLength != 0 {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}

	eventsPath := filepath.Join(s.base, "tenants", id, "events.jsonl")
	rep, err := ledger.Doctor(eventsPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "doctor: "+err.Error())
		return
	}
	if !rep.ChainIntact {
		writeError(w, http.StatusConflict, "chain not intact: "+rep.ChainError)
		return
	}

	store, err := s.openTenantStore(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "open store: "+err.Error())
		return
	}
	defer func() { _ = store.Close() }()

	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{
		"tip_hash":    rep.LastHash,
		"event_count": rep.EventCount,
		"anchored_at": now.Format(time.RFC3339Nano),
		"sink":        req.Sink,
	})
	if _, err := store.Append(ledger.Event{
		Kind: "LEDGER_ANCHOR", Tenant: id, Timestamp: now, Payload: payload, PrevHash: store.LastHash(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "append: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tip_hash":    rep.LastHash,
		"event_count": rep.EventCount,
		"anchored_at": now.Format(time.RFC3339Nano),
		"sink":        req.Sink,
	})
}

// handleIncidents reads the sidecar incidents.jsonl and returns rows as JSON.
func (s *server) handleIncidents(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	recs, err := incidents.ReadAll(s.base, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"incidents": recs,
		"count":     len(recs),
	})
}
