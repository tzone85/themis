package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/override"
)

type overrideInvokeRequest struct {
	PRID       string `json:"pr_id"`
	Actor      string `json:"actor"`
	CoSigner   string `json:"co_signer"`
	Reason     string `json:"reason"`
	Scope      string `json:"scope,omitempty"`
	TTLMinutes int    `json:"ttl_minutes,omitempty"`
}

type overrideClosePMRequest struct {
	PRID   string `json:"pr_id"`
	Closer string `json:"closer"`
	Notes  string `json:"notes"`
}

// handleOverrides dispatches /v1/tenants/{id}/overrides{,/postmortem}.
func (s *server) handleOverrides(w http.ResponseWriter, r *http.Request, id string, sub string) {
	switch sub {
	case "":
		switch r.Method {
		case http.MethodGet:
			s.handleOverrideStatus(w, r, id)
		case http.MethodPost:
			s.handleOverrideInvoke(w, r, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, "GET or POST")
		}
	case "postmortem":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		s.handleOverrideClosePM(w, r, id)
	default:
		writeError(w, http.StatusNotFound, "unknown overrides sub-route")
	}
}

func (s *server) handleOverrideStatus(w http.ResponseWriter, r *http.Request, id string) {
	prID := r.URL.Query().Get("pr_id")
	if prID == "" {
		writeError(w, http.StatusBadRequest, "missing pr_id query parameter")
		return
	}
	events, err := s.readTenantEvents(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	st := override.Compute(events, prID, time.Now().UTC())
	writeJSON(w, http.StatusOK, st)
}

func (s *server) handleOverrideInvoke(w http.ResponseWriter, r *http.Request, id string) {
	defer func() { _ = r.Body.Close() }()

	var req overrideInvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	now := time.Now().UTC()
	expires := now.Add(override.DefaultDuration)
	if req.TTLMinutes > 0 {
		expires = now.Add(time.Duration(req.TTLMinutes) * time.Minute)
	}
	payload := override.InvokePayload{
		PRID: req.PRID, Actor: req.Actor, CoSigner: req.CoSigner,
		Reason: req.Reason, Scope: req.Scope, ExpiresAt: expires,
	}
	if err := override.ValidateInvoke(payload, now); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	invoke, due := override.BuildInvoke(payload, now)

	store, err := s.openTenantStore(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "open store: "+err.Error())
		return
	}
	defer func() { _ = store.Close() }()

	ip, _ := json.Marshal(invoke)
	if _, err := store.Append(ledger.Event{
		Kind: "EMERGENCY_OVERRIDE_INVOKED", Tenant: id, Timestamp: now, Payload: ip, PrevHash: store.LastHash(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "append invoke: "+err.Error())
		return
	}
	dp, _ := json.Marshal(due)
	if _, err := store.Append(ledger.Event{
		Kind: "OVERRIDE_POSTMORTEM_DUE", Tenant: id, Timestamp: now, Payload: dp, PrevHash: store.LastHash(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "append postmortem-due: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, invoke)
}

func (s *server) handleOverrideClosePM(w http.ResponseWriter, r *http.Request, id string) {
	defer func() { _ = r.Body.Close() }()

	var req overrideClosePMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.PRID) == "" || strings.TrimSpace(req.Closer) == "" || strings.TrimSpace(req.Notes) == "" {
		writeError(w, http.StatusBadRequest, "pr_id, closer, notes are required")
		return
	}

	now := time.Now().UTC()
	events, err := s.readTenantEvents(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	st := override.Compute(events, req.PRID, now)
	if !st.PostmortemDue {
		writeError(w, http.StatusNotFound, "no override post-mortem due for pr_id")
		return
	}
	if st.PostmortemClosed {
		writeError(w, http.StatusConflict, "post-mortem already closed")
		return
	}

	store, err := s.openTenantStore(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "open store: "+err.Error())
		return
	}
	defer func() { _ = store.Close() }()

	payload, _ := json.Marshal(override.BuildClosed(req.PRID, req.Closer, req.Notes, now))
	if _, err := store.Append(ledger.Event{
		Kind: "OVERRIDE_POSTMORTEM_CLOSED", Tenant: id, Timestamp: now, Payload: payload, PrevHash: store.LastHash(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "append closed: "+err.Error())
		return
	}
	refreshed, _ := s.readTenantEvents(id)
	writeJSON(w, http.StatusOK, override.Compute(refreshed, req.PRID, now))
	_ = errors.New // keep imports tidy
}
