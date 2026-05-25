package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tzone85/themis/internal/incidents"
	"github.com/tzone85/themis/internal/ledger"
)

func TestAPI_Heartbeat_AppendsEnforcementMissing(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := heartbeatRequest{
		Repo: "gh:org/repo", ExpectedCheck: "themis-check",
		ReportedBy: "watchdog", LastSeen: "2026-05-24T10:00:00Z",
	}
	status, raw := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/heartbeat", token, body)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	var payload map[string]string
	_ = json.Unmarshal(raw, &payload)
	if payload["repo"] != "gh:org/repo" {
		t.Errorf("repo = %q", payload["repo"])
	}

	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if events[len(events)-1].Kind != "ENFORCEMENT_MISSING" {
		t.Fatalf("last event = %q, want ENFORCEMENT_MISSING", events[len(events)-1].Kind)
	}
}

func TestAPI_Heartbeat_RequiresFields(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := heartbeatRequest{Repo: "x"}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/heartbeat", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", status)
	}
}

func TestAPI_Heartbeat_AuthRequired(t *testing.T) {
	base, id, _, _, _ := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/heartbeat", "", heartbeatRequest{})
	if status != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", status)
	}
}

func TestAPI_Heartbeat_MethodNotAllowed(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, _ := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/heartbeat", token)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", status)
	}
}

func TestAPI_Anchor_AppendsTipHash(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := anchorRequest{Sink: "s3://test-bucket/themis/"}
	status, raw := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/anchor", token, body)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	if p["tip_hash"].(string) == "" {
		t.Fatalf("tip_hash empty: %+v", p)
	}
	if p["sink"] != "s3://test-bucket/themis/" {
		t.Errorf("sink = %v", p["sink"])
	}
}

func TestAPI_Anchor_RefusesOnBrokenChain(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	// Tamper the ledger so anchor refuses.
	eventsPath := filepath.Join(base, "tenants", id, "events.jsonl")
	raw, _ := os.ReadFile(eventsPath)
	raw[10] ^= 0x01
	_ = os.WriteFile(eventsPath, raw, 0o600)

	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, body := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/anchor", token, anchorRequest{})
	if status != http.StatusConflict {
		t.Fatalf("status %d body %s, want 409", status, body)
	}
}

func TestAPI_Incidents_ReturnsSidecarRows(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	// Seed an incident row directly.
	if err := incidents.Append(base, id, "ENFORCEMENT_MISSING", json.RawMessage(`{"repo":"gh:x/y"}`)); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	status, raw := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/incidents", token)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	if !strings.Contains(string(raw), "ENFORCEMENT_MISSING") {
		t.Fatalf("expected ENFORCEMENT_MISSING in response: %s", raw)
	}
}

func TestAPI_Incidents_EmptyWhenNoFile(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	status, raw := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/incidents", token)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if !strings.Contains(string(raw), `"count":0`) {
		t.Fatalf("expected count:0 in response: %s", raw)
	}
}

func TestAPI_Incidents_AuthRequired(t *testing.T) {
	base, id, _, _, _ := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, _ := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/incidents", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", status)
	}
}
