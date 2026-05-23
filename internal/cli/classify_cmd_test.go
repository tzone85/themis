package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
	"github.com/tzone85/themis/internal/ledger"
)

// setupTenantWithSyncedCatalogue runs `tenant init` + `catalogue sync` so the
// tenant has both an events.jsonl and a catalogue.json snapshot ready.
func setupTenantWithSyncedCatalogue(t *testing.T) (base, id string) {
	t.Helper()
	base, id = setupTenantWithCatalogue(t)
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"catalogue", "sync", "--id", id, "--base", base, "--source", fixtureRoot(t)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("catalogue sync: %v", err)
	}
	return
}

func writeAIChange(t *testing.T, dir string, c aichange.AIChange) string {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "aichange.json")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClassifyCmd_DocOnly(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	change := aichange.AIChange{
		PRID:  "gh:test#1",
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "README.md", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	cp := writeAIChange(t, t.TempDir(), change)

	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"classify", "--id", id, "--base", base, "--aichange", cp})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("classify: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if got["kind"] != "DOC_ONLY" {
		t.Fatalf("kind = %v, want DOC_ONLY", got["kind"])
	}

	events, err := ledger.ReadAll(filepath.Join(base, "tenants", id, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.Kind != "IMPACT_CLASSIFIED" {
		t.Fatalf("last event Kind = %q, want IMPACT_CLASSIFIED", last.Kind)
	}
}

func TestClassifyCmd_SchemaBreaking(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	change := aichange.AIChange{
		PRID:  "gh:test#2",
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "events/PaymentReceived/schema.json", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	cp := writeAIChange(t, t.TempDir(), change)

	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"classify", "--id", id, "--base", base, "--aichange", cp})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("classify: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(out.Bytes(), &got)
	if got["kind"] != "SCHEMA_BREAKING" {
		t.Fatalf("kind = %v, want SCHEMA_BREAKING", got["kind"])
	}
}

func TestClassifyCmd_ConsumerTouch(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	change := aichange.AIChange{
		PRID:  "gh:test#3",
		Actor: "claude_code",
		TouchedFiles: []aichange.FileTouch{
			{Path: "services/dispatcher/handler.go", ChangeKind: aichange.FileModified, BeforeHash: "a", AfterHash: "b"},
		},
	}
	cp := writeAIChange(t, t.TempDir(), change)

	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"classify", "--id", id, "--base", base, "--aichange", cp})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("classify: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(out.Bytes(), &got)
	// dispatcher both consumes (PaymentReceived) and produces (PaymentDispatched),
	// so the classifier prefers producer-touch.
	if got["kind"] != "PRODUCER_TOUCH" {
		t.Fatalf("kind = %v, want PRODUCER_TOUCH (dispatcher both consumes+produces)", got["kind"])
	}
}

func TestClassifyCmd_RejectsBrokenAIChange(t *testing.T) {
	base, id := setupTenantWithSyncedCatalogue(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(p, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"classify", "--id", id, "--base", base, "--aichange", p})
	if err := cmd.Execute(); err == nil {
		t.Fatal("classify should reject malformed AIChange JSON")
	}
}

func TestClassifyCmd_RejectsMissingCatalogueSnapshot(t *testing.T) {
	base, id := setupTenantWithCatalogue(t) // no catalogue sync
	change := aichange.AIChange{PRID: "x", Actor: "y", TouchedFiles: []aichange.FileTouch{{Path: "README.md", ChangeKind: aichange.FileModified}}}
	cp := writeAIChange(t, t.TempDir(), change)
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"classify", "--id", id, "--base", base, "--aichange", cp})
	if err := cmd.Execute(); err == nil {
		t.Fatal("classify should fail when no catalogue snapshot exists")
	}
}
