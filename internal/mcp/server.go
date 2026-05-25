// Package mcp implements a minimal Model Context Protocol bridge over
// stdio JSON-RPC. It exposes a curated, read-only subset of the Themis
// REST API as MCP "tools" so Claude Code, Cursor, and VXD can query
// Themis from inside an agent loop — the "agentic-first surface"
// pillar of the design spec (§5.1).
//
// Only the protocol methods Themis needs are implemented:
//
//   - initialize        (handshake)
//   - tools/list        (advertises the four read-only tools)
//   - tools/call        (dispatches to themis_{health,decisions,bom,catalogue})
//
// Notifications and unknown methods are answered with a JSON-RPC error so
// poorly-behaved clients can't crash the agent. The bridge speaks to an
// already-running themis API server — it never opens the ledger directly,
// so tenant isolation, auth, and audit logging remain centralised in the
// REST surface.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ProtocolVersion advertised in the `initialize` response. MCP clients are
// expected to negotiate down to versions they recognise; we ship a single
// stable identifier here.
const ProtocolVersion = "2024-11-05"

// Server is the MCP bridge. It is safe to construct without networking
// — calls only happen when Run() is invoked.
type Server struct {
	// BaseURL is the running Themis REST endpoint (e.g. http://127.0.0.1:8787).
	BaseURL string
	// Token is the per-tenant Bearer token.
	Token string
	// TenantID scopes every tool call to a single tenant.
	TenantID string
	// HTTPClient lets tests swap in a stub. Defaults to http.DefaultClient.
	HTTPClient *http.Client
}

// Run reads JSON-RPC messages from in, dispatches them, and writes
// responses to out. Returns nil when in reaches EOF.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	if s.HTTPClient == nil {
		s.HTTPClient = http.DefaultClient
	}
	scanner := bufio.NewScanner(in)
	// Allow large JSON-RPC frames (BOM payloads can be > 64k).
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		resp := s.handleLine(line)
		if resp == nil {
			continue
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
		if _, err := out.Write(append(raw, '\n')); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
	}
	return scanner.Err()
}

// request is the JSON-RPC 2.0 request shape (params is method-specific).
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// response is the JSON-RPC 2.0 response shape.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC standard error codes (subset).
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInternalError  = -32603
)

// handleLine parses one JSON-RPC frame and returns the response. Returns
// nil when the frame is a notification (no id field) — JSON-RPC mandates
// no response in that case.
func (s *Server) handleLine(line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return errResp(nil, codeParseError, "parse error: "+err.Error())
	}
	if req.JSONRPC != "2.0" {
		return errResp(req.ID, codeInvalidRequest, "jsonrpc must be \"2.0\"")
	}
	// Notification — no id field → no response.
	if len(req.ID) == 0 {
		return nil
	}

	switch req.Method {
	case "initialize":
		return ok(req.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{"name": "themis-mcp", "version": "0.1.0"},
		})
	case "tools/list":
		return ok(req.ID, map[string]any{"tools": toolDescriptors()})
	case "tools/call":
		return s.handleToolCall(req.ID, req.Params)
	default:
		return errResp(req.ID, codeMethodNotFound, "unknown method: "+req.Method)
	}
}

// toolCallParams matches MCP's tools/call params shape.
type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolCall(id, raw json.RawMessage) *response {
	var p toolCallParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return errResp(id, codeInvalidRequest, "tools/call: bad params: "+err.Error())
	}

	apiPath, err := dispatchTool(p.Name, p.Arguments, s.TenantID)
	if err != nil {
		return errResp(id, codeInvalidRequest, err.Error())
	}

	body, httpErr := s.httpGET(apiPath)
	if httpErr != nil {
		return errResp(id, codeInternalError, httpErr.Error())
	}

	// MCP responses wrap content; we return raw JSON inside a single text part
	// so clients can pretty-print the payload without further protocol work.
	return ok(id, map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(body)},
		},
	})
}

// dispatchTool translates an MCP tool name + arguments into an API path.
// Centralising the mapping here keeps the surface auditable: the set of
// REST endpoints the MCP bridge can reach is exactly what's listed here.
func dispatchTool(name string, args json.RawMessage, tenantID string) (string, error) {
	switch name {
	case "themis_health":
		return "/v1/tenants/" + url.PathEscape(tenantID) + "/health", nil
	case "themis_decisions":
		var a struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", errors.New("themis_decisions: invalid arguments")
		}
		if a.PRID == "" {
			return "", errors.New("themis_decisions: pr_id required")
		}
		return "/v1/tenants/" + url.PathEscape(tenantID) + "/decisions?pr_id=" + url.QueryEscape(a.PRID), nil
	case "themis_bom":
		var a struct {
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", errors.New("themis_bom: invalid arguments")
		}
		if len(a.Hash) != 64 {
			return "", errors.New("themis_bom: hash must be 64 hex chars")
		}
		return "/v1/tenants/" + url.PathEscape(tenantID) + "/boms/" + a.Hash, nil
	case "themis_events":
		// Optional kind + limit; both are passed through as query params.
		var a struct {
			Kind  string `json:"kind,omitempty"`
			Limit int    `json:"limit,omitempty"`
		}
		if len(args) > 0 {
			if err := json.Unmarshal(args, &a); err != nil {
				return "", errors.New("themis_events: invalid arguments")
			}
		}
		q := url.Values{}
		if a.Kind != "" {
			q.Set("kind", a.Kind)
		}
		if a.Limit > 0 {
			q.Set("limit", fmt.Sprintf("%d", a.Limit))
		}
		path := "/v1/tenants/" + url.PathEscape(tenantID) + "/events"
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
		return path, nil
	default:
		return "", errors.New("unknown tool: " + name)
	}
}

// httpGET fetches apiPath from the configured Themis base URL with the
// configured bearer token. Returns the response body bytes on 2xx; an
// error containing the status code otherwise.
func (s *Server) httpGET(apiPath string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(s.BaseURL, "/")+apiPath, nil)
	if err != nil {
		return nil, err
	}
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("themis api %s returned %d: %s", apiPath, resp.StatusCode, string(body))
	}
	return body, nil
}

// toolDescriptors enumerates the MCP tools we expose. Schemas use the same
// JSON-Schema dialect MCP standardises on.
func toolDescriptors() []map[string]any {
	return []map[string]any{
		{
			"name":        "themis_health",
			"description": "Return ledger health (event count, chain status, last hash) for the current tenant.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "themis_decisions",
			"description": "Return the most recent DECISION_ISSUED payload for a given pull-request identifier.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pr_id": map[string]any{"type": "string"},
				},
				"required": []string{"pr_id"},
			},
		},
		{
			"name":        "themis_bom",
			"description": "Return the canonical, signed AI-Bill-of-Materials body for a content hash.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hash": map[string]any{"type": "string", "minLength": 64, "maxLength": 64},
				},
				"required": []string{"hash"},
			},
		},
		{
			"name":        "themis_events",
			"description": "List ledger events (newest first), optionally filtered by kind.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":  map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 500},
				},
			},
		},
	}
}

// ok constructs a successful JSON-RPC response with result.
func ok(id json.RawMessage, result any) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}

// errResp constructs a JSON-RPC error response.
func errResp(id json.RawMessage, code int, msg string) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}
