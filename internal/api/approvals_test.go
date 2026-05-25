package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/tzone85/themis/internal/approvals"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/policy"
)

// seedTenantWithRequireApproval seeds a tenant whose latest DECISION_ISSUED
// is REQUIRE_APPROVAL with one "senior" role required. Returns base, id,
// prID, token.
func seedTenantWithRequireApproval(t *testing.T) (base, id, prID, token string) {
	t.Helper()
	base, id, token = seedTenantForDecide(t)
	prID = "gh:approval#1"

	storePath := filepath.Join(base, "tenants", id, "events.jsonl")
	store, err := ledger.OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	dec := policy.Decision{
		Verdict:           policy.VerdictRequireApproval,
		RequiredApprovers: []policy.Approver{{Role: "senior"}},
		RuleName:          "schema-breaking",
	}
	payload, _ := json.Marshal(map[string]any{
		"pr_id":    prID,
		"actor":    "claude_code",
		"decision": dec,
	})
	if _, err := store.Append(ledger.Event{
		Kind: "DECISION_ISSUED", Tenant: id, Timestamp: time.Now().UTC(), Payload: payload, PrevHash: store.LastHash(),
	}); err != nil {
		t.Fatal(err)
	}
	return
}

func TestAPI_Approvals_GrantFinalises(t *testing.T) {
	base, id, prID, token := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := approvalRequest{
		PRID:     prID,
		Approver: "human:alice",
		Role:     "senior",
		Action:   "grant",
		Comment:  "looks ok",
	}
	status, raw := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/approvals", token, body)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	var st approvals.Status
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	if !st.Finalised || st.FinalVerdict != policy.VerdictAllow {
		t.Fatalf("status not finalised allow: %+v", st)
	}
}

func TestAPI_Approvals_DenyFinalisesDeny(t *testing.T) {
	base, id, prID, token := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := approvalRequest{
		PRID:     prID,
		Approver: "human:alice",
		Role:     "senior",
		Action:   "deny",
		Reason:   "schema break unsafe",
	}
	status, raw := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/approvals", token, body)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	var st approvals.Status
	_ = json.Unmarshal(raw, &st)
	if !st.Finalised || st.FinalVerdict != policy.VerdictDeny {
		t.Fatalf("expected finalised DENY, got %+v", st)
	}
}

func TestAPI_Approvals_StatusEndpoint(t *testing.T) {
	base, id, prID, token := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	q := "?pr_id=" + url.QueryEscape(prID)
	status, raw := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/approvals"+q, token)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, raw)
	}
	var st approvals.Status
	_ = json.Unmarshal(raw, &st)
	if st.Finalised {
		t.Fatal("status pre-grant should not be finalised")
	}
}

func TestAPI_Approvals_StatusRequiresPRID(t *testing.T) {
	base, id, _, token := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	status, _ := doReq(t, srv.Client(), http.MethodGet, srv.URL+"/v1/tenants/"+id+"/approvals", token)
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", status)
	}
}

func TestAPI_Approvals_AuthRequired(t *testing.T) {
	base, id, _, _ := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/approvals", "", approvalRequest{})
	if status != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", status)
	}
}

func TestAPI_Approvals_RejectsBadAction(t *testing.T) {
	base, id, prID, token := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	body := approvalRequest{PRID: prID, Approver: "a", Role: "b", Action: "shrug"}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/approvals", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", status)
	}
}

func TestAPI_Approvals_DenyRequiresReason(t *testing.T) {
	base, id, prID, token := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	body := approvalRequest{PRID: prID, Approver: "a", Role: "b", Action: "deny"}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/approvals", token, body)
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", status)
	}
}

func TestAPI_Approvals_UnknownPRIs404(t *testing.T) {
	base, id, _, token := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	body := approvalRequest{PRID: "no-such", Approver: "a", Role: "b", Action: "grant"}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/approvals", token, body)
	if status != http.StatusNotFound {
		t.Fatalf("status %d, want 404", status)
	}
}

func TestAPI_Approvals_AlreadyFinalisedIs409(t *testing.T) {
	base, id, prID, token := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := approvalRequest{PRID: prID, Approver: "human:alice", Role: "senior", Action: "grant"}
	if status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/approvals", token, body); status != http.StatusOK {
		t.Fatalf("first grant: %d", status)
	}
	status, _ := postJSON(t, srv.Client(), srv.URL+"/v1/tenants/"+id+"/approvals", token, body)
	if status != http.StatusConflict {
		t.Fatalf("second grant: %d, want 409", status)
	}
}

func TestAPI_Approvals_MethodNotAllowed(t *testing.T) {
	base, id, _, token := seedTenantWithRequireApproval(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)
	status, _ := doReq(t, srv.Client(), http.MethodDelete, srv.URL+"/v1/tenants/"+id+"/approvals", token)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", status)
	}
}
