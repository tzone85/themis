package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPolicyYAML = `version: 1
default: REQUIRE_APPROVAL
required_approvers_for_default:
  - role: senior
rules:
  - name: doc-only allowed
    when:
      impact.kind: [DOC_ONLY]
    then:
      verdict: ALLOW
`

func TestPolicyLint_AcceptsValidPolicy(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "themis.yaml")
	if err := os.WriteFile(p, []byte(validPolicyYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"policy", "lint", "--file", p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("policy lint: %v", err)
	}
	if !strings.Contains(out.String(), "ok") {
		t.Errorf("expected 'ok' in output, got %q", out.String())
	}
}

func TestPolicyLint_RejectsInvalidPolicy(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "themis.yaml")
	if err := os.WriteFile(p, []byte("default: SHRUG\n"), 0o600); err != nil { // no version
		t.Fatal(err)
	}
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"policy", "lint", "--file", p})
	if err := cmd.Execute(); err == nil {
		t.Fatal("policy lint should reject invalid policy")
	}
}

func TestPolicyLint_RejectsMissingFile(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"policy", "lint", "--file", filepath.Join(t.TempDir(), "nope.yaml")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("policy lint should fail on missing file")
	}
}
