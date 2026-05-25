package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/tzone85/themis/internal/approvals"
	"github.com/tzone85/themis/internal/ledger"
)

// approvalRequest is the POST body shape: action is "grant" or "deny".
type approvalRequest struct {
	PRID     string `json:"pr_id"`
	Approver string `json:"approver"`
	Role     string `json:"role"`
	Action   string `json:"action"`
	Comment  string `json:"comment,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func (s *server) handleApprovals(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		s.handleApprovalStatus(w, r, id)
	case http.MethodPost:
		s.handleApprovalSubmit(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST")
	}
}

func (s *server) handleApprovalStatus(w http.ResponseWriter, r *http.Request, id string) {
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
	st := approvals.Compute(events, prID)
	writeJSON(w, http.StatusOK, st)
}

func (s *server) handleApprovalSubmit(w http.ResponseWriter, r *http.Request, id string) {
	defer func() { _ = r.Body.Close() }()

	var req approvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.PRID == "" || req.Approver == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "pr_id, approver, role are required")
		return
	}
	if req.Action != "grant" && req.Action != "deny" {
		writeError(w, http.StatusBadRequest, "action must be \"grant\" or \"deny\"")
		return
	}
	if req.Action == "deny" && req.Reason == "" {
		writeError(w, http.StatusBadRequest, "reason is required for deny")
		return
	}

	events, err := s.readTenantEvents(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	pre := approvals.Compute(events, req.PRID)
	if pre.Decision.Verdict == "" {
		writeError(w, http.StatusNotFound, "no DECISION_ISSUED found for this pr_id")
		return
	}
	if pre.Finalised {
		writeError(w, http.StatusConflict, "decision already finalised")
		return
	}

	store, err := s.openTenantStore(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "open store: "+err.Error())
		return
	}
	defer func() { _ = store.Close() }()

	now := time.Now().UTC()
	var kind string
	var payload []byte
	switch req.Action {
	case "grant":
		kind = "APPROVAL_GRANTED"
		payload, _ = json.Marshal(approvals.GrantPayload{
			PRID:      req.PRID,
			Approver:  req.Approver,
			Role:      req.Role,
			Comment:   req.Comment,
			GrantedAt: now,
		})
	case "deny":
		kind = "APPROVAL_DENIED"
		payload, _ = json.Marshal(approvals.DenyPayload{
			PRID:     req.PRID,
			Approver: req.Approver,
			Role:     req.Role,
			Reason:   req.Reason,
			DeniedAt: now,
		})
	}

	if _, err := store.Append(ledger.Event{
		Kind: kind, Tenant: id, Timestamp: now, Payload: payload, PrevHash: store.LastHash(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "append "+kind+": "+err.Error())
		return
	}

	// Re-read to compute fresh status + finalise if applicable.
	events2, err := s.readTenantEvents(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "re-read: "+err.Error())
		return
	}
	st := approvals.Compute(events2, req.PRID)
	if _, ready := approvals.CanFinalise(st); ready {
		finalised := approvals.BuildFinalised(st, req.PRID, now)
		body, _ := json.Marshal(finalised)
		if _, err := store.Append(ledger.Event{
			Kind: "DECISION_FINALISED", Tenant: id, Timestamp: now, Payload: body, PrevHash: store.LastHash(),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "append DECISION_FINALISED: "+err.Error())
			return
		}
		st.Finalised = true
		st.FinalVerdict = finalised.FinalVerdict
	}
	writeJSON(w, http.StatusOK, st)
}

// readTenantEvents is a small helper centralising the events.jsonl path.
func (s *server) readTenantEvents(id string) ([]ledger.Event, error) {
	return ledger.ReadAll(filepath.Join(s.base, "tenants", id, "events.jsonl"))
}
