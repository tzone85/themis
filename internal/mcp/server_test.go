package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubAPI returns a tiny httptest.Server that echoes back path-specific
// payloads — enough to drive every tool dispatch path without standing up
// a real Themis state directory.
func stubAPI(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	const token = "stub-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path":  r.URL.Path,
			"query": r.URL.RawQuery,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, token
}

func newServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	stub, token := stubAPI(t)
	return &Server{
		BaseURL:  stub.URL,
		Token:    token,
		TenantID: "acme",
	}, stub
}

// runRequest drives one JSON-RPC frame through Server.Run and returns the
// (single-line) response.
func runRequest(t *testing.T, s *Server, body map[string]any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	in := bytes.NewBuffer(append(raw, '\n'))
	out := &bytes.Buffer{}
	if err := s.Run(context.Background(), in, out); err != nil {
		t.Fatalf("run: %v", err)
	}
	var resp map[string]any
	dec := json.NewDecoder(out)
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("decode response: %v\nraw: %s", err, out.String())
	}
	return resp
}

func TestMCP_InitializeHandshake(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
	})
	result := resp["result"].(map[string]any)
	if result["protocolVersion"] != ProtocolVersion {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}
	info := result["serverInfo"].(map[string]any)
	if info["name"] != "themis-mcp" {
		t.Fatalf("serverInfo.name = %v", info["name"])
	}
}

func TestMCP_ListToolsAdvertisesFourTools(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list",
	})
	tools := resp["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 4 {
		t.Fatalf("tools = %d, want 4", len(tools))
	}
	names := map[string]bool{}
	for _, raw := range tools {
		names[raw.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"themis_health", "themis_decisions", "themis_bom", "themis_events"} {
		if !names[want] {
			t.Errorf("tools missing %q", want)
		}
	}
}

func TestMCP_HealthToolCall(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{
			"name":      "themis_health",
			"arguments": map[string]any{},
		},
	})
	content := resp["result"].(map[string]any)["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "/v1/tenants/acme/health") {
		t.Fatalf("expected path in response, got: %s", text)
	}
}

func TestMCP_DecisionsToolCall(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "tools/call",
		"params": map[string]any{
			"name":      "themis_decisions",
			"arguments": map[string]any{"pr_id": "gh:test#42"},
		},
	})
	text := resp["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "/v1/tenants/acme/decisions") {
		t.Fatalf("wrong path: %s", text)
	}
	if !strings.Contains(text, "gh%3Atest%2342") {
		t.Fatalf("pr_id not URL-encoded: %s", text)
	}
}

func TestMCP_DecisionsRequiresPRID(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "2.0", "id": 5, "method": "tools/call",
		"params": map[string]any{
			"name":      "themis_decisions",
			"arguments": map[string]any{},
		},
	})
	if resp["error"] == nil {
		t.Fatal("expected error for missing pr_id")
	}
}

func TestMCP_BOMToolRejectsShortHash(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "2.0", "id": 6, "method": "tools/call",
		"params": map[string]any{
			"name":      "themis_bom",
			"arguments": map[string]any{"hash": "too-short"},
		},
	})
	if resp["error"] == nil {
		t.Fatal("expected error for short hash")
	}
}

func TestMCP_EventsToolWithLimit(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "2.0", "id": 7, "method": "tools/call",
		"params": map[string]any{
			"name":      "themis_events",
			"arguments": map[string]any{"kind": "DECISION_ISSUED", "limit": 10},
		},
	})
	text := resp["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "kind=DECISION_ISSUED") || !strings.Contains(text, "limit=10") {
		t.Fatalf("query params missing: %s", text)
	}
}

func TestMCP_UnknownMethod(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "2.0", "id": 8, "method": "what/is/this",
	})
	if resp["error"] == nil {
		t.Fatal("expected error for unknown method")
	}
	e := resp["error"].(map[string]any)
	if int(e["code"].(float64)) != codeMethodNotFound {
		t.Fatalf("code = %v, want %d", e["code"], codeMethodNotFound)
	}
}

func TestMCP_UnknownTool(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "2.0", "id": 9, "method": "tools/call",
		"params": map[string]any{"name": "themis_phantom", "arguments": map[string]any{}},
	})
	if resp["error"] == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestMCP_RejectsNonJSONRPCVersion(t *testing.T) {
	s, _ := newServer(t)
	resp := runRequest(t, s, map[string]any{
		"jsonrpc": "1.0", "id": 10, "method": "initialize",
	})
	if resp["error"] == nil {
		t.Fatal("expected error for bad jsonrpc version")
	}
}

func TestMCP_NotificationProducesNoResponse(t *testing.T) {
	s, _ := newServer(t)
	in := bytes.NewBufferString(`{"jsonrpc":"2.0","method":"notifications/cancelled"}` + "\n")
	out := &bytes.Buffer{}
	if err := s.Run(context.Background(), in, out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty output for notification; got %q", out.String())
	}
}

func TestMCP_ParseErrorOnBadJSON(t *testing.T) {
	s, _ := newServer(t)
	in := bytes.NewBufferString("not json at all\n")
	out := &bytes.Buffer{}
	if err := s.Run(context.Background(), in, out); err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] == nil {
		t.Fatal("expected parse error response")
	}
}
