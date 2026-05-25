package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPI_Dashboard_ServesHTML(t *testing.T) {
	base, _, _, _, _ := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html...", ct)
	}
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	if !strings.Contains(string(body[:n]), "THEMIS — audit dashboard") {
		t.Fatalf("body does not contain dashboard title:\n%s", string(body[:n]))
	}
}

func TestAPI_Dashboard_NotFoundOnSubpaths(t *testing.T) {
	base, _, _, _, _ := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/random/sub/path")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAPI_Dashboard_MethodNotAllowed(t *testing.T) {
	base, _, _, _, _ := seedTenantState(t)
	srv := httptest.NewServer(NewMux(base))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}
