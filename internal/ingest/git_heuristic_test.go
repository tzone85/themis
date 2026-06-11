package ingest

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tzone85/themis/internal/aichange"
)

// gitAvailable skips the test if `git` is missing from PATH.
func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping git_heuristic test")
	}
}

// setupGitRepo creates a fresh repo with two commits: an initial state and
// a follow-up that adds, modifies, and deletes files. Returns the workdir.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	gitAvailable(t)
	dir := t.TempDir()

	gitOK := func(args ...string) {
		t.Helper()
		// #nosec G204 -- args fixed by this helper, not user input.
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	gitOK("init", "-q", "--initial-branch=main")
	gitOK("config", "user.email", "thandi@example.test")
	gitOK("config", "user.name", "Thandi")
	gitOK("config", "commit.gpgsign", "false")

	write("keep.go", "package x\n")
	write("delete-me.go", "package x\n// gone next commit\n")
	gitOK("add", ".")
	gitOK("commit", "-q", "-m", "initial")

	write("keep.go", "package x\n// modified\n")
	write("new.go", "package x\n// added\n")
	if err := os.Remove(filepath.Join(dir, "delete-me.go")); err != nil {
		t.Fatal(err)
	}
	gitOK("add", "-A")
	gitOK("commit", "-q", "-m", "second")

	return dir
}

func TestGitHeuristic_HappyPath(t *testing.T) {
	dir := setupGitRepo(t)
	got, err := (&GitHeuristic{}).Ingest(Inputs{
		PRID:    "gh:test#git-1",
		Workdir: dir,
		BaseRef: "HEAD~1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Actor != "human:thandi@example.test" {
		t.Errorf("actor = %q, want human:thandi@example.test", got.Actor)
	}
	if len(got.TouchedFiles) != 3 {
		t.Fatalf("files = %d, want 3 (add, modify, delete)", len(got.TouchedFiles))
	}

	wantKinds := map[string]aichange.FileChangeKind{
		"delete-me.go": aichange.FileDeleted,
		"keep.go":      aichange.FileModified,
		"new.go":       aichange.FileAdded,
	}
	for _, ft := range got.TouchedFiles {
		if wantKinds[ft.Path] != ft.ChangeKind {
			t.Errorf("%s: got %q, want %q", ft.Path, ft.ChangeKind, wantKinds[ft.Path])
		}
		// Hash invariants:
		if ft.ChangeKind == aichange.FileAdded && (ft.BeforeHash != "" || ft.AfterHash == "") {
			t.Errorf("%s ADDED: hashes = %q/%q", ft.Path, ft.BeforeHash, ft.AfterHash)
		}
		if ft.ChangeKind == aichange.FileDeleted && (ft.BeforeHash == "" || ft.AfterHash != "") {
			t.Errorf("%s DELETED: hashes = %q/%q", ft.Path, ft.BeforeHash, ft.AfterHash)
		}
		if ft.ChangeKind == aichange.FileModified && (ft.BeforeHash == "" || ft.AfterHash == "" || ft.BeforeHash == ft.AfterHash) {
			t.Errorf("%s MODIFIED: hashes = %q/%q", ft.Path, ft.BeforeHash, ft.AfterHash)
		}
	}
}

func TestGitHeuristic_RequiresPRID(t *testing.T) {
	gitAvailable(t)
	_, err := (&GitHeuristic{}).Ingest(Inputs{Workdir: t.TempDir()})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("missing prid should ErrAdapterFailed, got %v", err)
	}
}

func TestGitHeuristic_RequiresWorkdir(t *testing.T) {
	_, err := (&GitHeuristic{}).Ingest(Inputs{PRID: "x"})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("missing workdir should ErrAdapterFailed, got %v", err)
	}
}

func TestGitHeuristic_NotAGitRepo(t *testing.T) {
	gitAvailable(t)
	_, err := (&GitHeuristic{}).Ingest(Inputs{PRID: "x", Workdir: t.TempDir()})
	if !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("non-repo should ErrAdapterFailed, got %v", err)
	}
}

// TestGitHeuristic_RejectsOptionLikeBaseRef locks in defense-in-depth
// against git argument injection: a --base-ref that starts with `-` (or
// looks like an option flag) must be refused at the adapter boundary
// BEFORE it reaches git. git itself happens to reject most of these
// today, but relying on a downstream tool's error handling is fragile
// — future git versions or subcommand changes could turn a rejected
// flag into an accepted one.
//
// The assertion wraps both ErrAdapterFailed AND ErrInvalidBaseRef so
// the test fails if validation is removed or git starts accepting the
// flag silently.
func TestGitHeuristic_RejectsOptionLikeBaseRef(t *testing.T) {
	dir := setupGitRepo(t)
	for _, bad := range []string{
		"-HEAD",
		"--upload-pack=evil",
		"--exec-path=/tmp/evil",
		"--",
	} {
		t.Run(bad, func(t *testing.T) {
			_, err := (&GitHeuristic{}).Ingest(Inputs{
				PRID:    "x",
				Workdir: dir,
				BaseRef: bad,
			})
			if !errors.Is(err, ErrInvalidBaseRef) {
				t.Fatalf("baseref %q should ErrInvalidBaseRef, got %v", bad, err)
			}
		})
	}
}

func TestGitHeuristic_RespectsActorOverride(t *testing.T) {
	dir := setupGitRepo(t)
	got, err := (&GitHeuristic{}).Ingest(Inputs{
		PRID:          "x",
		Workdir:       dir,
		BaseRef:       "HEAD~1",
		ActorOverride: "claude_code",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Actor != "claude_code" {
		t.Errorf("ActorOverride ignored: actor = %q", got.Actor)
	}
}
