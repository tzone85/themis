package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/catalogue"
	"github.com/tzone85/themis/internal/ledger"
)

const decidePolicyYAML = `version: 1
default: REQUIRE_APPROVAL
rules:
  - name: doc-only allowed
    when:
      impact.kind: [DOC_ONLY]
    then:
      verdict: ALLOW
  - name: secrets block
    when:
      findings.kind: secret
    then:
      verdict: DENY
      reason: secret detected
`

// seedTenantForDecide builds a tenant that has init+catalogue snapshot+tokens,
// minus any prior decisions. Returns base, id, token.
func seedTenantForDecide(t *testing.T) (base, id, token string) {
	t.Helper()
	base = t.TempDir()
	id = "acme"
	token = "decide-token"

	if err := os.MkdirAll(filepath.Join(base, "tenants", id), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "tenants", id, "api-tokens"), []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Seed a minimal events.jsonl with TENANT_INITIALISED so the ledger is non-empty.
	storePath := filepath.Join(base, "tenants", id, "events.jsonl")
	store, err := ledger.OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"id": id})
	if _, err := store.Append(ledger.Event{Kind: "TENANT_INITIALISED", Tenant: id, Payload: payload, PrevHash: store.LastHash()}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	// Stub catalogue snapshot.
	g := catalogue.CatalogueGraph{ContentHash: "stub", Services: map[string]catalogue.Service{}, Events: map[string]catalogue.EventDef{}, Domains: map[string]catalogue.Domain{}}
	raw, _ := json.Marshal(g)
	if err := os.WriteFile(filepath.Join(base, "tenants", id, "catalogue.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return
}

func postJSON(t *testing.T, c *http.Client, url, token string, body any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func TestAPI_Decide_DocOnlyAllows(t *testing.T) {
	base, id, token := seedTenantForDecide(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := decideRequest{
		AIChange: aichange.AIChange{
			PRID: "gh:test#api-decide-1", Actor: "claude_code",
			TouchedFiles: []aichange.FileTouch{{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"}},
		},
		PolicyYAML: decidePolicyYAML,
	}
	status, raw := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/decide", token, body)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	dec := out["decision"].(map[string]any)
	if dec["verdict"] != "ALLOW" {
		t.Fatalf("verdict = %v, want ALLOW", dec["verdict"])
	}
}

func TestAPI_Decide_SecretInWorkdirDenies(t *testing.T) {
	base, id, token := seedTenantForDecide(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := decideRequest{
		AIChange: aichange.AIChange{
			PRID: "gh:test#api-decide-2", Actor: "claude_code",
			TouchedFiles: []aichange.FileTouch{{Path: "src/leak.go", ChangeKind: aichange.FileAdded, AfterHash: "h"}},
		},
		PolicyYAML: decidePolicyYAML,
		WorkdirFiles: map[string]string{
			"src/leak.go": base64.StdEncoding.EncodeToString([]byte("aws = AKIAIOSFODNN7EXAMPLE\n")),
		},
	}
	status, raw := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/decide", token, body)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	dec := out["decision"].(map[string]any)
	if dec["verdict"] != "DENY" {
		t.Fatalf("verdict = %v, want DENY (secret detected)", dec["verdict"])
	}

	// Ledger should now contain a SCAN_FINDING + DECISION_ISSUED.
	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Kind] = true
	}
	if !seen["SCAN_FINDING"] || !seen["DECISION_ISSUED"] {
		t.Fatalf("expected SCAN_FINDING + DECISION_ISSUED in ledger; saw %v", seen)
	}
}

func TestAPI_Decide_RequiresAuth(t *testing.T) {
	base, id, _ := seedTenantForDecide(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/decide", "", decideRequest{PolicyYAML: decidePolicyYAML})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestAPI_Decide_MethodNotAllowed(t *testing.T) {
	base, id, token := seedTenantForDecide(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, _ := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/decide", token)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", status)
	}
}

func TestAPI_Decide_InvalidJSONBodyIs400(t *testing.T) {
	base, id, token := seedTenantForDecide(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/tenants/"+id+"/decide", bytes.NewBufferString("{not-json"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAPI_Decide_MissingPolicyIs400(t *testing.T) {
	base, id, token := seedTenantForDecide(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	body := decideRequest{AIChange: aichange.AIChange{PRID: "x", Actor: "y"}}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/decide", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestAPI_Decide_BadPolicyEmitsPolicyInvalid(t *testing.T) {
	base, id, token := seedTenantForDecide(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	body := decideRequest{
		AIChange:   aichange.AIChange{PRID: "x", Actor: "y"},
		PolicyYAML: "default: SHRUG\n", // no version → invalid
	}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/decide", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	events, _ := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	sawPolicyInvalid := false
	for _, e := range events {
		if e.Kind == "POLICY_INVALID" {
			sawPolicyInvalid = true
		}
	}
	if !sawPolicyInvalid {
		t.Fatal("ledger should contain POLICY_INVALID after malformed policy")
	}
}

func TestAPI_Decide_BadBase64Is400(t *testing.T) {
	base, id, token := seedTenantForDecide(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	body := decideRequest{
		AIChange:   aichange.AIChange{PRID: "x", Actor: "y", TouchedFiles: []aichange.FileTouch{{Path: "a", ChangeKind: aichange.FileAdded, AfterHash: "h"}}},
		PolicyYAML: decidePolicyYAML,
		WorkdirFiles: map[string]string{"a": "###not-base64###"},
	}
	status, raw := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/decide", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d body %s, want 400", status, raw)
	}
}

func TestAPI_Decide_InvalidAIChangeIs400(t *testing.T) {
	base, id, token := seedTenantForDecide(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	body := decideRequest{
		AIChange: aichange.AIChange{
			PRID: "x", Actor: "y",
			TouchedFiles: []aichange.FileTouch{{Path: "a", ChangeKind: aichange.FileChangeKind("BOGUS")}},
		},
		PolicyYAML: decidePolicyYAML,
	}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/decide", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}
