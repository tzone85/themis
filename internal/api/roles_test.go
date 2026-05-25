package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/auth"
	"github.com/tzone85/themis/internal/ledger"
	"github.com/tzone85/themis/internal/policy"
)

// seedTenantWithStructuredTokens writes a tokens.yaml giving the test
// access tokens for every role tier. The legacy api-tokens file from
// seedTenantState would otherwise grant admin to all tokens — we delete
// it so role gates can be exercised.
func seedTenantWithStructuredTokens(t *testing.T) (base, id, prID string, tokens map[auth.Role]string) {
	t.Helper()
	base, id, prID, _, _ = seedTenantState(t)
	// Remove the legacy admin-fallback so role gates can fail closed.
	_ = os.Remove(filepath.Join(base, "tenants", id, "api-tokens"))

	tokens = map[auth.Role]string{
		auth.RoleRead:       "tok-read",
		auth.RoleDev:        "tok-dev",
		auth.RoleReviewer:   "tok-reviewer",
		auth.RoleCompliance: "tok-compliance",
		auth.RoleAdmin:      "tok-admin",
	}
	yaml := "tokens:\n"
	for role, tok := range tokens {
		yaml += "  - token: \"" + tok + "\"\n"
		yaml += "    tenant: \"" + id + "\"\n"
		yaml += "    role: \"" + string(role) + "\"\n"
	}
	if err := os.WriteFile(filepath.Join(base, "tenants", "tokens.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return
}

func TestRoles_ReadCanGetHealthCannotPOSTDecide(t *testing.T) {
	base, id, _, tokens := seedTenantWithStructuredTokens(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	// read can GET health.
	if status, _ := doReq(t, srv.Client(), http.MethodGet,
		srv.URL+"/v1/tenants/"+id+"/health", tokens[auth.RoleRead]); status != http.StatusOK {
		t.Fatalf("read GET health: %d, want 200", status)
	}

	// read CANNOT POST /decide → 403.
	body := decideRequest{
		AIChange: aichange.AIChange{
			PRID: "x", Actor: "claude_code",
			TouchedFiles: []aichange.FileTouch{{Path: "README.md", ChangeKind: aichange.FileModified}},
		},
		PolicyYAML: "version: 1\ndefault: ALLOW\n",
	}
	if status, _ := postJSON(t, srv.Client(),
		srv.URL+"/v1/tenants/"+id+"/decide", tokens[auth.RoleRead], body); status != http.StatusForbidden {
		t.Fatalf("read POST decide: %d, want 403", status)
	}
}

func TestRoles_DevCanPOSTDecideCannotApprove(t *testing.T) {
	base, id, _, tokens := seedTenantWithStructuredTokens(t)
	// Catalogue snapshot for the decide path.
	if err := os.WriteFile(filepath.Join(base, "tenants", id, "catalogue.json"),
		[]byte(`{"content_hash":"stub","domains":{},"services":{},"events":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	body := decideRequest{
		AIChange: aichange.AIChange{
			PRID: "gh:role-dev#1", Actor: "claude_code",
			TouchedFiles: []aichange.FileTouch{{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"}},
		},
		PolicyYAML: "version: 1\ndefault: ALLOW\n",
	}
	if status, raw := postJSON(t, srv.Client(),
		srv.URL+"/v1/tenants/"+id+"/decide", tokens[auth.RoleDev], body); status != http.StatusOK {
		t.Fatalf("dev POST decide: %d body %s", status, raw)
	}

	// dev CANNOT POST /approvals → 403.
	appReq := approvalRequest{PRID: "gh:role-dev#1", Approver: "x", Role: "senior", Action: "grant"}
	if status, _ := postJSON(t, srv.Client(),
		srv.URL+"/v1/tenants/"+id+"/approvals", tokens[auth.RoleDev], appReq); status != http.StatusForbidden {
		t.Fatalf("dev POST approvals: %d, want 403", status)
	}
}

func TestRoles_ReviewerCanGrantCannotOverride(t *testing.T) {
	base, id, _, tokens := seedTenantWithStructuredTokens(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	// Seed a REQUIRE_APPROVAL decision so the grant path is valid.
	prID := "gh:role-rev#1"
	storePath := filepath.Join(base, "tenants", id, "events.jsonl")
	store, _ := ledger.OpenStore(storePath)
	dec := policy.Decision{Verdict: policy.VerdictRequireApproval, RequiredApprovers: []policy.Approver{{Role: "senior"}}}
	payload, _ := json.Marshal(map[string]any{"pr_id": prID, "decision": dec})
	_, _ = store.Append(ledger.Event{Kind: "DECISION_ISSUED", Tenant: id, Payload: payload, PrevHash: store.LastHash()})
	_ = store.Close()

	// reviewer can grant.
	body := approvalRequest{PRID: prID, Approver: "human:rev", Role: "senior", Action: "grant"}
	if status, _ := postJSON(t, srv.Client(),
		srv.URL+"/v1/tenants/"+id+"/approvals", tokens[auth.RoleReviewer], body); status != http.StatusOK {
		t.Fatalf("reviewer POST approvals: %d, want 200", status)
	}

	// reviewer CANNOT POST /overrides → 403.
	overrideBody := map[string]any{
		"pr_id":     prID,
		"actor":     "human:a", "co_signer": "human:b",
		"reason":    "long enough reason " + strings.Repeat("x", 80),
	}
	if status, _ := postJSON(t, srv.Client(),
		srv.URL+"/v1/tenants/"+id+"/overrides", tokens[auth.RoleReviewer], overrideBody); status != http.StatusForbidden {
		t.Fatalf("reviewer POST overrides: %d, want 403", status)
	}
}

func TestRoles_ComplianceCannotAnchor(t *testing.T) {
	base, id, _, tokens := seedTenantWithStructuredTokens(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	// compliance can POST /overrides for a denied decision.
	if status, _ := postJSON(t, srv.Client(),
		srv.URL+"/v1/tenants/"+id+"/anchor", tokens[auth.RoleCompliance], map[string]string{}); status != http.StatusForbidden {
		t.Fatalf("compliance POST anchor: %d, want 403", status)
	}
}

func TestRoles_AdminCanDoAnything(t *testing.T) {
	base, id, _, tokens := seedTenantWithStructuredTokens(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	// admin can POST /anchor (chain must be intact — seed already valid).
	if status, raw := postJSON(t, srv.Client(),
		srv.URL+"/v1/tenants/"+id+"/anchor", tokens[auth.RoleAdmin], map[string]string{}); status != http.StatusOK {
		t.Fatalf("admin POST anchor: %d body %s", status, raw)
	}
}

func TestRoles_WrongTenantTokenRejected(t *testing.T) {
	base := t.TempDir()
	// Two tenants with disjoint tokens.
	if err := os.MkdirAll(filepath.Join(base, "tenants"), 0o700); err != nil {
		t.Fatal(err)
	}
	yaml := `tokens:
  - token: "tenant-A-tok"
    tenant: "alpha"
    role: "admin"
  - token: "tenant-B-tok"
    tenant: "beta"
    role: "admin"
`
	if err := os.WriteFile(filepath.Join(base, "tenants", "tokens.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(base, "tenants", id), 0o700); err != nil {
			t.Fatal(err)
		}
		// Empty events.jsonl so doctor doesn't error.
		_ = os.WriteFile(filepath.Join(base, "tenants", id, "events.jsonl"), nil, 0o600)
	}
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	// Tenant-B token presented for tenant-A → 401.
	status, _ := doReq(t, srv.Client(), http.MethodGet,
		srv.URL+"/v1/tenants/alpha/health", "tenant-B-tok")
	if status != http.StatusUnauthorized {
		t.Fatalf("cross-tenant token: %d, want 401", status)
	}
}

// keep unused-import linter quiet
var _ = url.QueryEscape
