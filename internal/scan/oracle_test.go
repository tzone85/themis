package scan

import "testing"

func TestStaticOracle_KnowsPopular(t *testing.T) {
	o := NewStaticOracle()
	if !o.Knows("npm", "react") || !o.Popular("npm", "react") {
		t.Fatal("react should be known + popular in npm")
	}
	if !o.Knows("pypi", "requests") || !o.Popular("pypi", "requests") {
		t.Fatal("requests should be known + popular in pypi")
	}
}

func TestStaticOracle_UnknownEcosystem(t *testing.T) {
	o := NewStaticOracle()
	if o.Knows("rubygems", "rails") {
		t.Fatal("unknown ecosystem must return false")
	}
}

func TestStaticOracle_UnknownPackage(t *testing.T) {
	o := NewStaticOracle()
	if o.Knows("npm", "totally-made-up-package-name-xyz") {
		t.Fatal("nonexistent package should not be Known")
	}
}

func TestStaticOracle_DistanceToPopular(t *testing.T) {
	o := NewStaticOracle()
	// "reactt" → react, distance 1.
	d, nearest, ok := o.DistanceToPopular("npm", "reactt")
	if !ok {
		t.Fatal("expected ok=true for npm")
	}
	if nearest != "react" || d != 1 {
		t.Fatalf("nearest=%q d=%d, want react/1", nearest, d)
	}
}

func TestStaticOracle_DistanceUnknownEcosystem(t *testing.T) {
	o := NewStaticOracle()
	if _, _, ok := o.DistanceToPopular("rubygems", "rails"); ok {
		t.Fatal("unknown ecosystem must yield ok=false")
	}
}

func TestStaticOracle_AddKnownLiftsHallucination(t *testing.T) {
	o := NewStaticOracle()
	o.AddKnown("npm", "internal-tool")
	if !o.Knows("npm", "internal-tool") {
		t.Fatal("AddKnown should register the name")
	}
	if o.Popular("npm", "internal-tool") {
		t.Fatal("AddKnown must NOT add to popular set")
	}
}

func TestLevenshtein_Identical(t *testing.T) {
	if levenshtein("react", "react") != 0 {
		t.Fatal("identical strings should be distance 0")
	}
}

func TestLevenshtein_EmptyToNonEmpty(t *testing.T) {
	if levenshtein("", "abc") != 3 || levenshtein("abc", "") != 3 {
		t.Fatal("empty→3-char string distance must be 3")
	}
}

func TestLevenshtein_KnownPairs(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"gumbo", "gambol", 2},
		{"react", "reactt", 1},
		{"requests", "reqests", 1},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
