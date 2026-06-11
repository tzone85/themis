package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tzone85/themis/internal/aichange"
)

// ErrInvalidBaseRef is returned when --base-ref looks like a git option
// flag (e.g. "-HEAD", "--upload-pack=…"). Catching these at the adapter
// boundary means we don't depend on git's own option parser to reject
// them — future git versions or new global options can't widen the
// surface silently.
var ErrInvalidBaseRef = errors.New("ingest: invalid base ref")

// GitHeuristic ingests an AIChange from a real git working tree by diffing
// against a base ref. It's the universal adapter — even when no AI-specific
// transcript exists, a git diff is always available.
type GitHeuristic struct{}

// Name returns the adapter's stable name.
func (g *GitHeuristic) Name() string { return "git_heuristic" }

// Ingest runs `git diff --name-status` against in.BaseRef (default "HEAD~1")
// and converts each entry into a FileTouch with before/after content hashes.
func (g *GitHeuristic) Ingest(in Inputs) (aichange.AIChange, error) {
	if in.PRID == "" {
		return aichange.AIChange{}, fmt.Errorf("%w: git_heuristic requires --pr-id", ErrAdapterFailed)
	}
	if in.Workdir == "" {
		return aichange.AIChange{}, fmt.Errorf("%w: git_heuristic requires --workdir", ErrAdapterFailed)
	}
	baseRef := in.BaseRef
	if baseRef == "" {
		baseRef = "HEAD~1"
	}
	if !safeGitRef(baseRef) {
		return aichange.AIChange{}, fmt.Errorf("%w: base-ref %q: %w", ErrAdapterFailed, baseRef, ErrInvalidBaseRef)
	}

	// 1. List name-status entries. The `--` after refs is belt-and-braces:
	// even with safeGitRef in place, the separator makes the rev/pathspec
	// boundary explicit so a future caller can't accidentally widen the
	// pathspec into the rev slot.
	diffOut, err := runGit(in.Workdir, "diff", "--name-status", "--no-renames", baseRef+"..HEAD", "--")
	if err != nil {
		return aichange.AIChange{}, fmt.Errorf("%w: git diff: %v", ErrAdapterFailed, err)
	}
	entries, err := parseNameStatus(strings.TrimSpace(diffOut))
	if err != nil {
		return aichange.AIChange{}, fmt.Errorf("%w: parse diff: %v", ErrAdapterFailed, err)
	}
	if len(entries) == 0 {
		return aichange.AIChange{}, fmt.Errorf("%w: no changes between %s and HEAD", ErrAdapterFailed, baseRef)
	}

	// 2. Compute before/after content hashes per file.
	touches := make([]aichange.FileTouch, 0, len(entries))
	for _, e := range entries {
		ft := aichange.FileTouch{Path: e.path, ChangeKind: e.kind}
		switch e.kind {
		case aichange.FileAdded:
			h, herr := gitBlobHash(in.Workdir, "HEAD", e.path)
			if herr != nil {
				return aichange.AIChange{}, fmt.Errorf("%w: hash %s@HEAD: %v", ErrAdapterFailed, e.path, herr)
			}
			ft.AfterHash = h
		case aichange.FileDeleted:
			h, herr := gitBlobHash(in.Workdir, baseRef, e.path)
			if herr != nil {
				return aichange.AIChange{}, fmt.Errorf("%w: hash %s@%s: %v", ErrAdapterFailed, e.path, baseRef, herr)
			}
			ft.BeforeHash = h
		case aichange.FileModified:
			h1, herr := gitBlobHash(in.Workdir, baseRef, e.path)
			if herr != nil {
				return aichange.AIChange{}, fmt.Errorf("%w: hash %s@%s: %v", ErrAdapterFailed, e.path, baseRef, herr)
			}
			h2, herr := gitBlobHash(in.Workdir, "HEAD", e.path)
			if herr != nil {
				return aichange.AIChange{}, fmt.Errorf("%w: hash %s@HEAD: %v", ErrAdapterFailed, e.path, herr)
			}
			ft.BeforeHash = h1
			ft.AfterHash = h2
		}
		touches = append(touches, ft)
	}

	// 3. Sort for determinism.
	sort.SliceStable(touches, func(i, j int) bool { return touches[i].Path < touches[j].Path })

	// 4. Resolve actor from the latest commit's author.
	authorOut, _ := runGit(in.Workdir, "log", "-1", "--format=%ae")
	author := strings.TrimSpace(authorOut)
	actor := "human:unknown"
	if author != "" {
		actor = "human:" + author
	}
	if in.ActorOverride != "" {
		actor = in.ActorOverride
	}

	return aichange.AIChange{
		PRID:         in.PRID,
		Actor:        actor,
		TouchedFiles: touches,
		Metadata: map[string]string{
			"git:base_ref": baseRef,
			"git:workdir":  filepath.Clean(in.Workdir),
		},
	}, nil
}

type diffEntry struct {
	kind aichange.FileChangeKind
	path string
}

// parseNameStatus reads `git diff --name-status` output. Each line is a
// status code (A/M/D/...), a tab, then the path.
func parseNameStatus(out string) ([]diffEntry, error) {
	if out == "" {
		return nil, nil
	}
	lines := strings.Split(out, "\n")
	entries := make([]diffEntry, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, "\t", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed diff line: %q", ln)
		}
		kind := aichange.FileChangeKind("")
		switch parts[0][0] {
		case 'A':
			kind = aichange.FileAdded
		case 'M':
			kind = aichange.FileModified
		case 'D':
			kind = aichange.FileDeleted
		default:
			// Other statuses (C/R/T/U/X) are ignored at Plan 5; later plans
			// may map renames to delete+add pairs.
			continue
		}
		entries = append(entries, diffEntry{kind: kind, path: parts[1]})
	}
	return entries, nil
}

// gitBlobHash returns the SHA-256 of the file's content at the given ref.
// We hash the bytes directly rather than relying on git's own blob SHA so
// the hash is portable across git versions and matches what the AIChange
// schema expects (hex SHA-256).
func gitBlobHash(workdir, ref, path string) (string, error) {
	out, err := runGitBytes(workdir, "show", ref+":"+path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(out)
	return hex.EncodeToString(sum[:]), nil
}

// runGit executes git in the workdir and returns stdout as string.
func runGit(workdir string, args ...string) (string, error) {
	out, err := runGitBytes(workdir, args...)
	return string(out), err
}

// safeGitRef rejects ref strings shaped like a git option flag. Refs
// can legally contain a wide alphabet (slashes, dots, alnum, dashes
// after the first char) but a leading `-` always means "option" to
// git's argument parser. We also reject the literal `--` since it's
// the rev/pathspec separator and has no business being a ref.
//
// We do NOT try to validate the full git refname grammar — git itself
// enforces that. We just refuse the shapes that exec.Command's lack of
// shell semantics doesn't protect against.
func safeGitRef(ref string) bool {
	if ref == "" || ref == "--" {
		return false
	}
	if strings.HasPrefix(ref, "-") {
		return false
	}
	return true
}

// runGitBytes is the byte-stream variant of runGit.
func runGitBytes(workdir string, args ...string) ([]byte, error) {
	// #nosec G204 -- args originate from this package; workdir is operator-supplied per command invocation.
	cmd := exec.Command("git", args...)
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git %v exit %d: %s", args, ee.ExitCode(), string(ee.Stderr))
		}
		return nil, err
	}
	return out, nil
}
