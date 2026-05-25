package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tzone85/themis/internal/ledger"
)

// Version is the embedded server build version. ldflags-injectable so the
// /v1/health endpoint can advertise something useful in production.
var Version = "dev"

// NewMux constructs the HTTP route table rooted at base (the Themis state
// directory). Returns a *http.ServeMux that the caller wraps in their own
// http.Server (decoupling listen/serve from routing).
func NewMux(base string) *http.ServeMux {
	mux := http.NewServeMux()
	srv := &server{base: base}

	mux.HandleFunc("/v1/health", srv.handleHealth)

	// Tenant-scoped endpoints. We pattern-match in code rather than relying
	// on Go 1.22's tree router so behaviour is identical across Go versions
	// the project supports.
	mux.HandleFunc("/v1/tenants/", srv.handleTenantRoute)

	// Embedded dashboard at /.
	mux.HandleFunc("/", srv.handleDashboard)
	return mux
}

type server struct {
	base string
}

// writeJSON serialises body as JSON with status code.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleHealth is the unauthenticated heartbeat endpoint.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	count := s.tenantCount()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        Version,
		"tenants_count":  count,
	})
}

// tenantCount counts entries under tenants/. Missing dir → 0.
func (s *server) tenantCount() int {
	entries, err := os.ReadDir(filepath.Join(s.base, "tenants"))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n
}

// handleTenantRoute dispatches /v1/tenants/{id}/... to the per-tenant
// endpoint. All paths here require Bearer auth.
func (s *server) handleTenantRoute(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	id, action := parts[0], parts[1]

	if err := RequireToken(s.base, id, r); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch action {
	case "health":
		s.handleTenantHealth(w, r, id)
	case "decisions":
		s.handleDecisions(w, r, id)
	case "decide":
		s.handleDecide(w, r, id)
	case "events":
		s.handleEvents(w, r, id)
	case "approvals":
		s.handleApprovals(w, r, id)
	case "overrides":
		sub := ""
		if len(parts) >= 3 {
			sub = parts[2]
		}
		s.handleOverrides(w, r, id, sub)
	case "boms":
		if len(parts) < 3 || parts[2] == "" {
			writeError(w, http.StatusNotFound, "missing bom hash")
			return
		}
		s.handleBOM(w, r, id, parts[2])
	default:
		writeError(w, http.StatusNotFound, "unknown tenant action")
	}
}

// handleTenantHealth reports per-tenant ledger health.
func (s *server) handleTenantHealth(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	eventsPath := filepath.Join(s.base, "tenants", id, "events.jsonl")
	rep, err := ledger.Doctor(eventsPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event_count":  rep.EventCount,
		"chain_intact": rep.ChainIntact,
		"chain_error":  rep.ChainError,
		"last_hash":    rep.LastHash,
	})
}

// handleDecisions returns the most recent DECISION_ISSUED matching ?pr_id=.
func (s *server) handleDecisions(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	prID := r.URL.Query().Get("pr_id")
	if prID == "" {
		writeError(w, http.StatusBadRequest, "missing pr_id query parameter")
		return
	}
	eventsPath := filepath.Join(s.base, "tenants", id, "events.jsonl")
	events, err := ledger.ReadAll(eventsPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind != "DECISION_ISSUED" {
			continue
		}
		var probe struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(events[i].Payload, &probe); err != nil {
			continue
		}
		if probe.PRID == prID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(events[i].Payload)
			return
		}
	}
	writeError(w, http.StatusNotFound, "no decision for pr_id")
}

// handleBOM streams the stored BOM artefact for a given content hash, plus
// (when ".sig" suffix is requested) the hex signature sidecar.
func (s *server) handleBOM(w http.ResponseWriter, r *http.Request, id, target string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	bomsDir := filepath.Join(s.base, "tenants", id, "bom")

	var (
		filePath    string
		contentType string
	)
	switch {
	case strings.HasSuffix(target, ".sig"):
		hash := strings.TrimSuffix(target, ".sig")
		if !safeBOMHash(hash) {
			writeError(w, http.StatusBadRequest, "bom hash must be 64 hex characters")
			return
		}
		filePath = filepath.Join(bomsDir, hash+".bom.json.sig")
		contentType = "text/plain; charset=utf-8"
	default:
		if !safeBOMHash(target) {
			writeError(w, http.StatusBadRequest, "bom hash must be 64 hex characters")
			return
		}
		filePath = filepath.Join(bomsDir, target+".bom.json")
		contentType = "application/json"
	}

	body, err := os.ReadFile(filePath) // #nosec G304 -- path validated to be inside bom dir + safeBOMHash().
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "bom not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// safeBOMHash gates file lookups so path traversal is impossible.
func safeBOMHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
