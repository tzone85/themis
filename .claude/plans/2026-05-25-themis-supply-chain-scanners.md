# Plan 13 — Supply-chain scanners (slopsquat + hallucinated imports)

**Date:** 2026-05-25
**Depends on:** Plans 1-12.
**Scope:** Add the two outstanding scanners from design spec §6.1 — both
detect AI-supply-chain risks where an LLM suggests a package name that is
either a typo-squat of a popular package (slopsquat) or doesn't exist at
all (hallucinated import).

Implementation strategy: each scanner ships with a small bundled allowlist
of known-popular packages per ecosystem (npm, pypi, go) so the scanner
runs offline. Production deployments swap in a richer feed via a
`PackageOracle` interface; the bundled list is enough for tests and a
useful default.

## Tasks

### T1: `internal/scan/oracle.go`

`PackageOracle` interface:

```go
type PackageOracle interface {
    Knows(ecosystem, name string) bool   // package exists in the wider ecosystem
    Popular(ecosystem, name string) bool // package is in the popular-N list
    DistanceToPopular(ecosystem, name string) (int, string, bool)
}
```

Bundled `StaticOracle` carries a small allowlist per ecosystem (a few
dozen entries each — enough to test slopsquat + hallucination logic).

### T2: `internal/scan/slopsquat.go`

For each touched `package.json` / `requirements.txt` / `go.mod` (Plan 13
inspects raw content via the existing fileBodies map):

1. Parse the package names.
2. For each name, compute `DistanceToPopular`. If < 3 and not itself
   popular → `slopsquat` finding (high severity).

### T3: `internal/scan/hallucinated.go`

Same parse, but: if `Knows(name)` returns false → `hallucinated_import`
finding (critical severity).

### T4: Wire both into `DefaultScanners()`

### T5: Tests (unit + table-driven on the bundled oracle)

### T6: README Plan 13 changelog

### T7: `make ci` pass
