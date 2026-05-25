package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPI_Events_NewestFirstWithLimit(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	status, body := doReq(t, srv.Client(), http.MethodGet,
		srv.URL+"/v1/tenants/"+id+"/events?limit=2", token)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, body)
	}
	var out struct {
		Events   []map[string]any `json:"events"`
		Total    int              `json:"total"`
		Returned int              `json:"returned"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Returned != 2 {
		t.Fatalf("returned = %d, want 2", out.Returned)
	}
	if out.Total < 2 {
		t.Fatalf("total = %d, want ≥ 2", out.Total)
	}
	// seedTenantState appended in order: TENANT_INITIALISED, CATALOGUE_SYNCED, DECISION_ISSUED.
	// Newest-first → DECISION_ISSUED first, CATALOGUE_SYNCED next.
	if out.Events[0]["kind"] != "DECISION_ISSUED" {
		t.Errorf("events[0].kind = %v, want DECISION_ISSUED", out.Events[0]["kind"])
	}
}

func TestAPI_Events_KindFilter(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	status, body := doReq(t, srv.Client(), http.MethodGet,
		srv.URL+"/v1/tenants/"+id+"/events?kind=DECISION_ISSUED", token)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	var out struct {
		Events []map[string]any `json:"events"`
	}
	_ = json.Unmarshal(body, &out)
	if len(out.Events) != 1 {
		t.Fatalf("filtered events = %d, want 1", len(out.Events))
	}
	if out.Events[0]["kind"] != "DECISION_ISSUED" {
		t.Fatalf("kind filter leaked: %v", out.Events[0]["kind"])
	}
}

func TestAPI_Events_Pagination(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	status, body := doReq(t, srv.Client(), http.MethodGet,
		srv.URL+"/v1/tenants/"+id+"/events?limit=1&offset=1", token)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	var out struct {
		Events   []map[string]any `json:"events"`
		Total    int              `json:"total"`
		Returned int              `json:"returned"`
	}
	_ = json.Unmarshal(body, &out)
	if out.Returned != 1 {
		t.Fatalf("returned = %d, want 1", out.Returned)
	}
	// offset=1 means we skipped the newest event (DECISION_ISSUED).
	if out.Events[0]["kind"] != "CATALOGUE_SYNCED" {
		t.Errorf("offset=1 events[0].kind = %v, want CATALOGUE_SYNCED", out.Events[0]["kind"])
	}
}

func TestAPI_Events_LimitClamped(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, body := doReq(t, srv.Client(), http.MethodGet,
		srv.URL+"/v1/tenants/"+id+"/events?limit=999999", token)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	var out struct {
		Returned int `json:"returned"`
	}
	_ = json.Unmarshal(body, &out)
	if out.Returned > 500 {
		t.Fatalf("limit not clamped at 500: returned %d", out.Returned)
	}
}

func TestAPI_Events_BadLimitFallsBack(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, _ := doReq(t, srv.Client(), http.MethodGet,
		srv.URL+"/v1/tenants/"+id+"/events?limit=not-a-number", token)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (limit default fallback)", status)
	}
}

func TestAPI_Events_MethodNotAllowed(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/tenants/"+id+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}
