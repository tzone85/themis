package cli

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// freePort asks the kernel for an unused TCP port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestServe_ListensThenShutsDownOnContextCancel(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	_ = id
	port := freePort(t)

	cmd := &cobra.Command{}
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)

	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetContext(ctx)

	done := make(chan error, 1)
	go func() {
		done <- runServe(cmd, base, "127.0.0.1:"+itoaShim(port))
	}()

	// Wait until the server actually accepts connections.
	deadline := time.Now().Add(3 * time.Second)
	var resp *http.Response
	for time.Now().Before(deadline) {
		var err error
		resp, err = http.Get("http://127.0.0.1:" + itoaShim(port) + "/v1/health")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if resp == nil {
		t.Fatal("server never became reachable")
	}
	if resp.StatusCode != 200 {
		t.Errorf("health status = %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Cancel context → server should shut down cleanly.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServe returned non-nil error on clean shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServe did not exit within 5s after ctx cancel")
	}
}

func TestServe_PortAlreadyBoundReturnsError(t *testing.T) {
	base, _ := setupTenantWithSyncedCatalogue(t)

	// Hold the port hostage so runServe's Listen fails.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- runServe(cmd, base, "127.0.0.1:"+itoaShim(port))
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected listen-failure error from runServe")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runServe didn't surface listen failure within 3s")
	}
}

// itoaShim formats an int without pulling strconv into this file.
func itoaShim(n int) string {
	if n == 0 {
		return "0"
	}
	var b [10]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
