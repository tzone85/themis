package api

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/catalogue"
	"github.com/tzone85/themis/internal/classify"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/policy"
	"github.com/tzone85/themis/internal/scan"
)

// seedTenantState constructs a fully-populated tenant directory:
//
//	tenants/<id>/events.jsonl (TENANT_INITIALISED, CATALOGUE_SYNCED, DECISION_ISSUED)
//	tenants/<id>/catalogue.json
//	tenants/<id>/bom/<hash>.bom.json  (+ .sig)
//	tenants/<id>/api-tokens
//
// We bypass the CLI here so the api package depends only on the data layer.
func seedTenantState(t *testing.T) (base, id, prID, bomHash, token string) {
	t.Helper()
	base = t.TempDir()
	id = "acme"
	prID = "gh:test#api-1"
	token = "test-bearer-token"

	// Tenant tree.
	for _, sub := range []string{"", "bom"} {
		if err := os.MkdirAll(filepath.Join(base, "tenants", id, sub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "tenants", id, "api-tokens"), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Append-only events.
	eventsPath := filepath.Join(base, "tenants", id, "events.jsonl")
	s, err := ledger.OpenStore(eventsPath)
	if err != nil {
		t.Fatal(err)
	}

	append := func(kind string, payload map[string]any) {
		raw, _ := json.Marshal(payload)
		_, err := s.Append(ledger.Event{
			Kind:     kind,
			Tenant:   id,
			Payload:  raw,
			PrevHash: s.LastHash(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	append("TENANT_INITIALISED", map[string]any{"id": id})
	append("CATALOGUE_SYNCED", map[string]any{"content_hash": "stub"})
	decisionPayload := map[string]any{
		"pr_id":    prID,
		"actor":    "claude_code",
		"impact":   classify.Impact{Kind: classify.KindDocOnly, Reason: "docs"},
		"findings": []scan.Finding{},
		"decision": policy.Decision{Verdict: policy.VerdictAllow, RuleName: "test"},
	}
	append("DECISION_ISSUED", decisionPayload)
	_ = s.Close()

	// Stub catalogue snapshot — the API doesn't read it, but tests in this
	// file assert non-empty tenant state.
	cgRaw, _ := json.Marshal(catalogue.CatalogueGraph{ContentHash: "stub"})
	if err := os.WriteFile(filepath.Join(base, "tenants", id, "catalogue.json"), cgRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	// Fake BOM artefact + signature.
	bomBody := []byte(`{"schema_version":"themis.bom.v1","pr_id":"` + prID + `"}`)
	bomHash = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if err := os.WriteFile(filepath.Join(base, "tenants", id, "bom", bomHash+".bom.json"), bomBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "tenants", id, "bom", bomHash+".bom.json.sig"), []byte(hex.EncodeToString([]byte("fake-signature-bytes"))), 0o600); err != nil {
		t.Fatal(err)
	}

	return
}

func newClient(t *testing.T, base string) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	return srv, srv.Client()
}

func doReq(t *testing.T, c *http.Client, method, url, token string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func TestAPI_Health_NoAuthRequired(t *testing.T) {
	base, _, _, _, _ := seedTenantState(t)
	srv, c := newClient(t, base)
	status, body := doReq(t, c, http.MethodGet, srv.URL+"/v1/health", "")
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out["tenants_count"].(float64) < 1 {
		t.Fatalf("tenants_count should be ≥ 1: %v", out)
	}
}

func TestAPI_Health_MethodNotAllowed(t *testing.T) {
	base, _, _, _, _ := seedTenantState(t)
	srv, c := newClient(t, base)
	status, _ := doReq(t, c, http.MethodPost, srv.URL+"/v1/health", "")
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", status)
	}
}

func TestAPI_TenantHealth_RequiresAuth(t *testing.T) {
	base, id, _, _, _ := seedTenantState(t)
	srv, c := newClient(t, base)
	status, _ := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/health", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestAPI_TenantHealth_HappyPath(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv, c := newClient(t, base)
	status, body := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/health", token)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out["chain_intact"] != true {
		t.Fatalf("chain_intact = %v, want true", out["chain_intact"])
	}
	if out["event_count"].(float64) != 3 {
		t.Fatalf("event_count = %v, want 3", out["event_count"])
	}
}

func TestAPI_Decisions_ReturnsMatchingPRID(t *testing.T) {
	base, id, prID, _, token := seedTenantState(t)
	srv, c := newClient(t, base)
	status, body := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/decisions?pr_id="+url.QueryEscape(prID), token)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, body)
	}
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out["pr_id"] != prID {
		t.Fatalf("pr_id = %v", out["pr_id"])
	}
	dec := out["decision"].(map[string]any)
	if dec["verdict"] != "ALLOW" {
		t.Fatalf("verdict = %v", dec["verdict"])
	}
}

func TestAPI_Decisions_MissingPRIDIs400(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv, c := newClient(t, base)
	status, _ := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/decisions", token)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestAPI_Decisions_UnknownPRIDIs404(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv, c := newClient(t, base)
	status, _ := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/decisions?pr_id=nope", token)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestAPI_BOM_ReturnsBody(t *testing.T) {
	base, id, _, bomHash, token := seedTenantState(t)
	srv, c := newClient(t, base)
	status, body := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/boms/"+bomHash, token)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, body)
	}
	var probe map[string]any
	_ = json.Unmarshal(body, &probe)
	if probe["schema_version"] != "themis.bom.v1" {
		t.Fatalf("schema_version = %v", probe["schema_version"])
	}
}

func TestAPI_BOMSig_ReturnsSignatureSidecar(t *testing.T) {
	base, id, _, bomHash, token := seedTenantState(t)
	srv, c := newClient(t, base)
	status, body := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/boms/"+bomHash+".sig", token)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if len(body) == 0 {
		t.Fatal("empty signature body")
	}
}

func TestAPI_BOM_RejectsPathTraversal(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv, c := newClient(t, base)
	status, _ := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/boms/..%2Fevents.jsonl", token)
	if status != http.StatusBadRequest && status != http.StatusNotFound {
		t.Fatalf("path-traversal: status = %d", status)
	}
}

func TestAPI_BOM_UnknownHashIs404(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv, c := newClient(t, base)
	status, _ := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/boms/"+
		"0000000000000000000000000000000000000000000000000000000000000000", token)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestAPI_UnknownTenantAction_Is404(t *testing.T) {
	base, id, _, _, token := seedTenantState(t)
	srv, c := newClient(t, base)
	status, _ := doReq(t, c, http.MethodGet, srv.URL+"/v1/tenants/"+id+"/phantom", token)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// TestAPI_TenantRoute_RejectsMalformedID locks in defense-in-depth: a
// malformed tenant ID (uppercase, leading dash, dot, oversized, …) must
// be rejected at the route boundary BEFORE the auth check touches the
// filesystem. Go's mux normalises `..` so true path traversal is already
// blocked; this gate covers everything else the validID regex catches.
func TestAPI_TenantRoute_RejectsMalformedID(t *testing.T) {
	base, _, _, _, token := seedTenantState(t)
	srv, c := newClient(t, base)
	for _, bad := range []string{
		"ACME",                  // uppercase
		"-acme",                 // leading dash
		"acme.corp",             // dot
		"acme_corp",             // underscore
		"acme corp",             // space (url-encoded by the server)
		string(make([]byte, 64)) + "x", // > 63 chars
	} {
		t.Run(bad, func(t *testing.T) {
			// url.PathEscape so URL parsers in mux see the raw bytes,
			// not a 400 from net/url parse.
			status, _ := doReq(t, c, http.MethodGet,
				srv.URL+"/v1/tenants/"+url.PathEscape(bad)+"/health", token)
			if status != http.StatusBadRequest {
				t.Fatalf("malformed id %q: status = %d, want 400", bad, status)
			}
		})
	}
}
