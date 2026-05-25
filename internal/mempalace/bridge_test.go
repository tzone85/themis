package mempalace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestWrite_ComputesContentAddressedKey(t *testing.T) {
	b := New(t.TempDir())
	body := json.RawMessage(`{"verdict":"ALLOW"}`)
	d := Drawer{Kind: "decision", Tenant: "acme", Body: body}

	first, err := b.Write(d)
	if err != nil {
		t.Fatal(err)
	}
	// Re-writing the same body must land at the same path.
	second, err := b.Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("content-addressed writes diverged: %q vs %q", first, second)
	}
}

func TestWrite_RejectsMissingFields(t *testing.T) {
	b := New(t.TempDir())
	cases := []Drawer{
		{Kind: "", Tenant: "acme", Body: json.RawMessage(`{}`)},
		{Kind: "decision", Tenant: "", Body: json.RawMessage(`{}`)},
		{Kind: "decision", Tenant: "acme"}, // no body
	}
	for _, d := range cases {
		if _, err := b.Write(d); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for %+v, got %v", d, err)
		}
	}
}

func TestReadWrite_RoundTrip(t *testing.T) {
	b := New(t.TempDir())
	d := Drawer{
		Kind: "bom", Tenant: "acme",
		Body: json.RawMessage(`{"schema_version":"themis.bom.v1"}`),
		Tags: []string{"audit", "q1-2026"},
		Description: "audit packet",
	}
	path, err := b.Write(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	got, err := b.Read(d.Tenant, d.Kind, sha256Key(d.Body))
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "audit packet" || len(got.Tags) != 2 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestList_ReturnsKeys(t *testing.T) {
	b := New(t.TempDir())
	keys := []string{}
	for _, body := range []string{`{"v":1}`, `{"v":2}`, `{"v":3}`} {
		out, err := b.Write(Drawer{Kind: "decision", Tenant: "acme", Body: json.RawMessage(body)})
		if err != nil {
			t.Fatal(err)
		}
		// extract the basename without extension as the key
		base := filepath.Base(out)
		keys = append(keys, base[:len(base)-5])
	}
	got, err := b.List("acme", "decision")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("List = %d, want 3 (%+v)", len(got), got)
	}
	sort.Strings(got)
	sort.Strings(keys)
	for i := range keys {
		if got[i] != keys[i] {
			t.Fatalf("List[%d] = %q, want %q", i, got[i], keys[i])
		}
	}
}

func TestList_MissingDirReturnsEmpty(t *testing.T) {
	b := New(t.TempDir())
	out, err := b.List("acme", "decision")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty list, got %+v", out)
	}
}

func TestWingDir_PerTenant(t *testing.T) {
	b := New("/srv/themis")
	if b.WingDir("alpha") == b.WingDir("beta") {
		t.Fatal("WingDir must differ per tenant")
	}
}

// sha256Key mirrors the bridge's internal keying for the round-trip test.
func sha256Key(body []byte) string {
	d := Drawer{Body: body}
	tmp := New("/tmp")
	d.Kind = "decision"
	d.Tenant = "x"
	// Use the bridge to compute the key by writing into a temp dir, then
	// reading the basename back. (Cheap shortcut so the test doesn't
	// duplicate the sha256 logic.)
	path, err := tmp.Write(d)
	if err != nil {
		panic(err)
	}
	base := filepath.Base(path)
	_ = os.RemoveAll(filepath.Dir(filepath.Dir(filepath.Dir(path)))) // clean up /tmp leakage
	return base[:len(base)-5]
}
