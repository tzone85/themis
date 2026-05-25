package api

import (
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/tzone85/themis/internal/ledger"
)

// handleEvents returns a paginated, newest-first slice of the tenant's
// ledger events. Query params:
//
//	limit  — max events to return (default 50, max 500)
//	kind   — when set, only events whose Kind exactly matches are returned
//	offset — number of records to skip (newest-first ordering)
//
// The response carries `{events: [...], total: int, returned: int}`.
func (s *server) handleEvents(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}
	kind := r.URL.Query().Get("kind")

	eventsPath := filepath.Join(s.base, "tenants", id, "events.jsonl")
	events, err := ledger.ReadAll(eventsPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter (newest-first) by kind, then apply offset/limit.
	filtered := make([]ledger.Event, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		if kind != "" && events[i].Kind != kind {
			continue
		}
		filtered = append(filtered, events[i])
	}

	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	if offset > len(filtered) {
		offset = len(filtered)
	}
	page := filtered[offset:end]

	writeJSON(w, http.StatusOK, map[string]any{
		"events":   page,
		"total":    len(filtered),
		"returned": len(page),
	})
}

func parseIntDefault(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

