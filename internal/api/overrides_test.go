package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/override"
	"github.com/tzone85/themis/internal/policy"
)

const overrideLongReason = "Catalogue server outage blocking the on-call rotation; need to merge a logging fix before the next deploy."

// seedTenantForOverride bakes in a DECISION_ISSUED with VerdictDeny so an
// override is meaningful.
func seedTenantForOverride(t *testing.T) (base, id, prID, token string) {
	t.Helper()
	base, id, token = seedTenantForDecide(t)
	prID = "gh:override#1"

	store, err := ledger.OpenStore(filepath.Join(base, "tenants", id, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	payload, _ := json.Marshal(map[string]any{
		"pr_id":    prID,
		"actor":    "claude_code",
		"decision": policy.Decision{Verdict: policy.VerdictDeny, RuleName: "secret detected"},
	})
	if _, err := store.Append(ledger.Event{
		Kind: "DECISION_ISSUED", Tenant: id, Timestamp: time.Now().UTC(), Payload: payload, PrevHash: store.LastHash(),
	}); err != nil {
		t.Fatal(err)
	}
	return
}

func TestAPI_OverrideInvoke_HappyPath(t *testing.T) {
	base, id, prID, token := seedTenantForOverride(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := overrideInvokeRequest{
		PRID: prID, Actor: "human:alice", CoSigner: "human:bob",
		Reason: overrideLongReason,
	}
	status, raw := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/overrides", token, body)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	var p override.InvokePayload
	_ = json.Unmarshal(raw, &p)
	if p.Actor != "human:alice" {
		t.Fatalf("actor = %q", p.Actor)
	}
}

func TestAPI_OverrideInvoke_RejectsShortReason(t *testing.T) {
	base, id, prID, token := seedTenantForOverride(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := overrideInvokeRequest{PRID: prID, Actor: "a", CoSigner: "b", Reason: "too short"}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/overrides", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", status)
	}
}

func TestAPI_OverrideStatus_ReportsActive(t *testing.T) {
	base, id, prID, token := seedTenantForOverride(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := overrideInvokeRequest{
		PRID: prID, Actor: "human:alice", CoSigner: "human:bob",
		Reason: overrideLongReason,
	}
	if status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/overrides", token, body); status != http.StatusOK {
		t.Fatalf("invoke pre-step: %d", status)
	}

	q := "?pr_id=" + url.QueryEscape(prID)
	status, raw := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/overrides"+q, token)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if !strings.Contains(string(raw), `"active":true`) {
		t.Fatalf("expected active=true: %s", raw)
	}
}

func TestAPI_OverrideClosePM_HappyPath(t *testing.T) {
	base, id, prID, token := seedTenantForOverride(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	invokeBody := overrideInvokeRequest{
		PRID: prID, Actor: "human:alice", CoSigner: "human:bob",
		Reason: overrideLongReason,
	}
	if status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/overrides", token, invokeBody); status != http.StatusOK {
		t.Fatalf("invoke: %d", status)
	}

	closeBody := overrideClosePMRequest{
		PRID: prID, Closer: "human:compliance",
		Notes: "post-mortem complete: root cause identified",
	}
	status, raw := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/overrides/postmortem", token, closeBody)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	if !strings.Contains(string(raw), `"postmortem_closed":true`) {
		t.Fatalf("expected closed: %s", raw)
	}
}

func TestAPI_OverrideClosePM_RequiresFields(t *testing.T) {
	base, id, _, token := seedTenantForOverride(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := overrideClosePMRequest{PRID: "x"} // missing closer/notes
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/overrides/postmortem", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", status)
	}
}

func TestAPI_OverrideClosePM_RejectsUnknownPR(t *testing.T) {
	base, id, _, token := seedTenantForOverride(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := overrideClosePMRequest{PRID: "nope", Closer: "c", Notes: "n"}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/overrides/postmortem", token, body)
	if status != http.StatusNotFound {
		t.Fatalf("status %d, want 404", status)
	}
}

func TestAPI_Overrides_AuthRequired(t *testing.T) {
	base, id, _, _ := seedTenantForOverride(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, _ := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/overrides?pr_id=x", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", status)
	}
}

func TestAPI_Overrides_UnknownSubrouteIs404(t *testing.T) {
	base, id, _, token := seedTenantForOverride(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, _ := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/overrides/phantom", token)
	if status != http.StatusNotFound {
		t.Fatalf("status %d, want 404", status)
	}
}
