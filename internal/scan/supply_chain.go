package scan

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tzone85/themis/internal/aichange"
)

// SlopsquatThreshold is the maximum Levenshtein distance at which a
// non-popular package is flagged as confusable with a popular one. 3 is
// the upper bound for "near-miss" — beyond that the names diverge enough
// that false positives outpace genuine slopsquats.
const SlopsquatThreshold = 3

// SupplyChainScanner runs both slopsquat detection and hallucinated-import
// detection against parsed manifest files. Two scanners share one struct
// because they share the parser, but emit findings of different kinds so
// policy YAML can target them independently.
type SupplyChainScanner struct {
	Oracle PackageOracle
}

// NewSupplyChainScanner returns a scanner backed by the bundled StaticOracle.
func NewSupplyChainScanner() *SupplyChainScanner {
	return &SupplyChainScanner{Oracle: NewStaticOracle()}
}

// Name implements Scanner.
func (s *SupplyChainScanner) Name() string { return "supply_chain" }

// Scan implements Scanner. Each manifest file (package.json, requirements.txt,
// go.mod) is parsed into a list of {ecosystem, name, line}, then every name
// is checked against the oracle for both hallucination and slopsquat.
func (s *SupplyChainScanner) Scan(c aichange.AIChange, fileBodies map[string][]byte) ([]Finding, error) {
	out := make([]Finding, 0)
	oracle := s.Oracle
	if oracle == nil {
		oracle = NewStaticOracle()
	}
	for _, ft := range c.TouchedFiles {
		if ft.ChangeKind == aichange.FileDeleted {
			continue
		}
		body, ok := fileBodies[ft.Path]
		if !ok {
			continue
		}
		eco, names := parseManifest(ft.Path, body)
		if eco == "" {
			continue
		}
		for _, n := range names {
			out = append(out, s.evaluate(oracle, eco, n, ft.Path)...)
		}
	}
	return out, nil
}

func (s *SupplyChainScanner) evaluate(oracle PackageOracle, eco string, p pkgRef, file string) []Finding {
	out := []Finding{}

	// Skip popular packages outright — they are themselves the
	// reference point.
	if oracle.Popular(eco, p.Name) {
		return out
	}

	// Hallucination: unknown to the oracle entirely.
	if !oracle.Knows(eco, p.Name) {
		out = append(out, Finding{
			Kind:        "hallucinated_import",
			Severity:    SeverityCritical,
			File:        file,
			Line:        p.Line,
			Description: "package " + p.Name + " (" + eco + ") is unknown to the package oracle on line " + itoa(p.Line),
			Detector:    "supply_chain",
		})
		// A hallucinated package is also confusable with whatever is
		// closest — emit a slopsquat finding only if the distance is
		// suspicious. Otherwise the hallucination finding is enough.
	}

	// Slopsquat: small edit distance to a popular package.
	d, nearest, ok := oracle.DistanceToPopular(eco, p.Name)
	if ok && nearest != p.Name && d > 0 && d <= SlopsquatThreshold {
		out = append(out, Finding{
			Kind:        "slopsquat",
			Severity:    SeverityHigh,
			File:        file,
			Line:        p.Line,
			Description: "package " + p.Name + " (" + eco + ") is " + itoa(d) + " edit(s) from popular package " + nearest,
			Detector:    "supply_chain",
		})
	}
	return out
}

// --- manifest parsing -------------------------------------------------------

type pkgRef struct {
	Name string
	Line int
}

// parseManifest returns (ecosystem, [{name,line}…]) for a supported manifest.
// Empty ecosystem means "not a manifest we recognise".
func parseManifest(path string, body []byte) (string, []pkgRef) {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case base == "package.json":
		return "npm", parsePackageJSON(body)
	case base == "requirements.txt":
		return "pypi", parseRequirementsTxt(body)
	case base == "go.mod":
		return "go", parseGoMod(body)
	}
	return "", nil
}

// parsePackageJSON extracts the union of dependencies + devDependencies keys.
// Line numbers are 0 because package.json doesn't carry useful line context
// without a full position-tracking parser; the scanner falls back to file-level.
func parsePackageJSON(body []byte) []pkgRef {
	var doc struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil
	}
	out := make([]pkgRef, 0, len(doc.Dependencies)+len(doc.DevDependencies))
	for name := range doc.Dependencies {
		out = append(out, pkgRef{Name: name})
	}
	for name := range doc.DevDependencies {
		out = append(out, pkgRef{Name: name})
	}
	return out
}

// reqLine matches `package-name[ extras ][==1.2.3]` etc.
var reqLine = regexp.MustCompile(`^([A-Za-z0-9._\-]+)`)

// parseRequirementsTxt parses pip-style lines. Blank lines and comments skipped.
func parseRequirementsTxt(body []byte) []pkgRef {
	out := []pkgRef{}
	for i, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		if m := reqLine.FindStringSubmatch(line); m != nil {
			out = append(out, pkgRef{Name: strings.ToLower(m[1]), Line: i + 1})
		}
	}
	return out
}

// goModRequire matches `<import-path> v1.2.3` lines inside a go.mod require block.
var goModRequire = regexp.MustCompile(`^\s+([^\s]+)\s+v\d`)

// parseGoMod returns module paths from require() blocks.
func parseGoMod(body []byte) []pkgRef {
	out := []pkgRef{}
	for i, raw := range strings.Split(string(body), "\n") {
		if m := goModRequire.FindStringSubmatch(raw); m != nil {
			out = append(out, pkgRef{Name: m[1], Line: i + 1})
		}
	}
	return out
}
