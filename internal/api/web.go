package api

import (
	"embed"
	"net/http"
)

//go:embed web/index.html
var dashboardFS embed.FS

// handleDashboard serves the embedded single-page dashboard at `/`. The
// dashboard is a vanilla-JS bundle so the binary stays a single Go static
// binary with no Node toolchain dependency.
func (s *server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	body, err := dashboardFS.ReadFile("web/index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}
