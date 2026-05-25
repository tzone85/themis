package scan

// PackageOracle is the abstraction the supply-chain scanners (slopsquat,
// hallucinated_imports) consult. Production deployments wire in a real
// data source (e.g. periodic dumps from npm/PyPI/Go module registries);
// the bundled StaticOracle ships an offline allowlist large enough to be
// useful as a default and tractable for unit tests.
//
// Two ecosystems share the same shape so adding new ones (cargo, rubygems)
// is just an extra map entry.
type PackageOracle interface {
	// Knows reports whether name is in the wider ecosystem at all.
	// Returning false means "this package does not exist as far as we
	// can tell" → hallucinated-import territory.
	Knows(ecosystem, name string) bool

	// Popular reports whether name is in the curated popular set.
	// Slopsquat detection only ever compares against popular packages —
	// confusable-with-obscure-package risk is below noise floor.
	Popular(ecosystem, name string) bool

	// DistanceToPopular returns the Levenshtein distance + the closest
	// popular package name. ok=false when no popular packages exist for
	// the ecosystem (in which case slopsquat checks are a no-op).
	DistanceToPopular(ecosystem, name string) (distance int, nearest string, ok bool)
}

// StaticOracle is the bundled default. Lists are deliberately small —
// "we know about these"; everything else we say "unknown". That biases
// the scanner toward false positives in production but those land as
// findings, not blockers, and the policy decides what to do with them.
type StaticOracle struct {
	// known is the ecosystem → set-of-known-names map.
	known map[string]map[string]struct{}
	// popular is the ecosystem → set-of-popular-names map (subset of known).
	popular map[string]map[string]struct{}
}

// NewStaticOracle returns the bundled default oracle.
func NewStaticOracle() *StaticOracle {
	npmPopular := []string{
		"react", "lodash", "express", "axios", "moment", "chalk", "debug",
		"commander", "yargs", "tslib", "typescript", "next", "vue", "vite",
		"webpack", "eslint", "prettier", "jest", "mocha", "ava", "request",
	}
	pypiPopular := []string{
		"requests", "numpy", "pandas", "django", "flask", "pytest", "boto3",
		"sqlalchemy", "scipy", "matplotlib", "tensorflow", "pytorch", "scikit-learn",
		"pillow", "lxml", "beautifulsoup4", "fastapi", "uvicorn", "httpx",
	}
	goPopular := []string{
		"github.com/spf13/cobra", "github.com/stretchr/testify",
		"github.com/sirupsen/logrus", "github.com/pkg/errors",
		"github.com/spf13/viper", "github.com/google/uuid",
		"github.com/gorilla/mux", "github.com/gin-gonic/gin",
		"github.com/labstack/echo", "github.com/go-redis/redis",
	}

	o := &StaticOracle{
		known:   map[string]map[string]struct{}{},
		popular: map[string]map[string]struct{}{},
	}
	o.seed("npm", npmPopular)
	o.seed("pypi", pypiPopular)
	o.seed("go", goPopular)
	return o
}

func (o *StaticOracle) seed(ecosystem string, names []string) {
	if o.known[ecosystem] == nil {
		o.known[ecosystem] = map[string]struct{}{}
	}
	if o.popular[ecosystem] == nil {
		o.popular[ecosystem] = map[string]struct{}{}
	}
	for _, n := range names {
		o.known[ecosystem][n] = struct{}{}
		o.popular[ecosystem][n] = struct{}{}
	}
}

// AddKnown lets tests + future feed adapters register additional non-popular
// packages so they pass the hallucination check without inflating the
// popular set used for slopsquat distance.
func (o *StaticOracle) AddKnown(ecosystem string, names ...string) {
	if o.known[ecosystem] == nil {
		o.known[ecosystem] = map[string]struct{}{}
	}
	for _, n := range names {
		o.known[ecosystem][n] = struct{}{}
	}
}

// Knows implements PackageOracle.
func (o *StaticOracle) Knows(ecosystem, name string) bool {
	if m, ok := o.known[ecosystem]; ok {
		_, found := m[name]
		return found
	}
	return false
}

// Popular implements PackageOracle.
func (o *StaticOracle) Popular(ecosystem, name string) bool {
	if m, ok := o.popular[ecosystem]; ok {
		_, found := m[name]
		return found
	}
	return false
}

// DistanceToPopular implements PackageOracle. Linear scan is fine: the
// popular set is curated and stays small.
func (o *StaticOracle) DistanceToPopular(ecosystem, name string) (int, string, bool) {
	m, ok := o.popular[ecosystem]
	if !ok || len(m) == 0 {
		return 0, "", false
	}
	best := -1
	nearest := ""
	for candidate := range m {
		d := levenshtein(name, candidate)
		if best == -1 || d < best {
			best = d
			nearest = candidate
		}
	}
	return best, nearest, true
}

// levenshtein is the standard DP edit distance (insert/delete/substitute
// each cost 1). Implemented inline so the scan package has no external
// dependency for a small, hot function.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			m := ins
			if del < m {
				m = del
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
