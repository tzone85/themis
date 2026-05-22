package catalogue

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// TestPropContentHash_DependsOnlyOnSemantics asserts that two on-disk
// representations of the same logical catalogue produce the same
// ContentHash. We achieve this by:
//
//  1. parsing the canonical fixture,
//  2. cloning every fixture file into a fresh temp dir,
//  3. touching every cloned file with a different mtime + ordering,
//  4. re-parsing the clone,
//  5. asserting ContentHash matches.
func TestPropContentHash_DependsOnlyOnSemantics(t *testing.T) {
	canonicalRoot := filepath.Join("testdata", "sample")
	g1, err := Parse(canonicalRoot)
	if err != nil {
		t.Fatal(err)
	}

	cloneRoot := t.TempDir()
	if err := copyTree(canonicalRoot, cloneRoot); err != nil {
		t.Fatal(err)
	}
	// Touch files in reverse-lexical order with shifted mtimes — any
	// implementation that hashed traversal order would now produce a
	// different hash.
	if err := touchTreeReverse(cloneRoot); err != nil {
		t.Fatal(err)
	}

	g2, err := Parse(cloneRoot)
	if err != nil {
		t.Fatal(err)
	}
	if g1.ContentHash != g2.ContentHash {
		t.Fatalf("ContentHash diverged after structural-only re-shuffle:\n  canonical=%s\n  shuffled =%s",
			g1.ContentHash, g2.ContentHash)
	}
}

// TestPropContentHash_ChangesOnFieldEdit confirms ContentHash IS sensitive
// to semantic changes — the negative direction of the same property.
func TestPropContentHash_ChangesOnFieldEdit(t *testing.T) {
	root := t.TempDir()
	if err := copyTree(filepath.Join("testdata", "sample"), root); err != nil {
		t.Fatal(err)
	}
	g1, err := Parse(root)
	if err != nil {
		t.Fatal(err)
	}

	// Bump the version on PaymentReceived.
	target := filepath.Join(root, "events", "PaymentReceived", "index.md")
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, replaceFirst(body, []byte(`"1.0.0"`), []byte(`"2.0.0"`)), 0o600); err != nil {
		t.Fatal(err)
	}

	g2, err := Parse(root)
	if err != nil {
		t.Fatal(err)
	}
	if g1.ContentHash == g2.ContentHash {
		t.Fatal("ContentHash should change when a field changes")
	}
}

func TestPropContentHash_HexShape(t *testing.T) {
	g, err := Parse(filepath.Join("testdata", "sample"))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.ContentHash) != hex.EncodedLen(sha256.Size) {
		t.Fatalf("ContentHash length = %d, want %d", len(g.ContentHash), hex.EncodedLen(sha256.Size))
	}
	if _, err := hex.DecodeString(g.ContentHash); err != nil {
		t.Fatalf("ContentHash not valid hex: %v", err)
	}
}

// copyTree recursively copies src into dst.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o600)
	})
}

// touchTreeReverse re-stamps mtime on every file in reverse-sorted order
// so that a traversal that depended on mtime would yield a different order.
func touchTreeReverse(root string) error {
	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	base := time.Unix(1_700_000_000, 0)
	for i, f := range files {
		when := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(f, when, when); err != nil {
			return err
		}
	}
	return nil
}

func replaceFirst(haystack, needle, replacement []byte) []byte {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			out := make([]byte, 0, len(haystack)-len(needle)+len(replacement))
			out = append(out, haystack[:i]...)
			out = append(out, replacement...)
			out = append(out, haystack[i+len(needle):]...)
			return out
		}
	}
	return haystack
}
