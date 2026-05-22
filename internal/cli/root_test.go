package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRoot_VersionFlagPrintsVersion(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "themis") {
		t.Fatalf("--version output missing 'themis': %q", out.String())
	}
}
