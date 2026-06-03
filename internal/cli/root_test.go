package cli

import (
	"bytes"
	"runtime"
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

// TestRoot_VersionFlagPrintsFullBuildInfo asserts the four-field
// version surface (semver, commit, build date, Go runtime). Operators
// auditing a running deployment need all four to correlate a binary
// with a release and reproduce a build.
func TestRoot_VersionFlagPrintsFullBuildInfo(t *testing.T) {
	prevV, prevC, prevD := Version, Commit, Date
	Version, Commit, Date = "v9.9.9-test", "deadbee", "2026-06-03T00:00:00Z"
	t.Cleanup(func() { Version, Commit, Date = prevV, prevC, prevD })

	out := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	for _, want := range []string{"v9.9.9-test", "deadbee", "2026-06-03T00:00:00Z", runtime.Version()} {
		if !strings.Contains(got, want) {
			t.Errorf("--version output missing %q\nfull output: %q", want, got)
		}
	}
}
