# Plan 2 — Catalogue + Classifier

**Date:** 2026-05-23
**Depends on:** Plan 1 (Foundation) — shipped 2026-05-23.
**Scope:** Parse an EventCatalog repository tree into a `CatalogueGraph`. Implement
the pure `classify(AIChange, Graph) → Impact` function. Wire two new ledger event
kinds: `CATALOGUE_SYNCED`, `IMPACT_CLASSIFIED`. Property tests for the classifier.

## Architectural alignment (design spec §6.1, Appendix B)

- `internal/catalogue` — parses EventCatalog repo (Markdown front-matter + AsyncAPI + OpenAPI). Builds `CatalogueGraph`. Read-only.
- `internal/aichange` — `AIChange` value type. The single normalised representation of "what a PR/agent did". File touches + adapter metadata.
- `internal/classify` — pure: `(AIChange, CatalogueGraph) → Impact`.
- `internal/ledger` — extend with `CATALOGUE_SYNCED`, `IMPACT_CLASSIFIED` (kinds + projector noops + wiring-test entries).
- `themis catalogue sync` CLI — parses a tenant-configured catalogue path, emits `CATALOGUE_SYNCED` event.
- `themis classify` CLI — given an AIChange JSON file, classifies and emits `IMPACT_CLASSIFIED`.

## Source files this plan creates

| File | Purpose |
|---|---|
| `internal/aichange/aichange.go` | `AIChange` struct + JSON tags |
| `internal/aichange/aichange_test.go` | round-trip + validation tests |
| `internal/catalogue/graph.go` | `CatalogueGraph`, `Domain`, `Service`, `Event` value types |
| `internal/catalogue/parser.go` | Markdown front-matter + AsyncAPI parser |
| `internal/catalogue/parser_test.go` | parser fixture tests |
| `internal/catalogue/testdata/...` | mini EventCatalog tree |
| `internal/classify/impact.go` | `Impact`, `Kind` enum (SCHEMA_BREAKING etc.) |
| `internal/classify/classify.go` | pure `Classify()` function |
| `internal/classify/classify_test.go` | unit tests per `Kind` |
| `internal/classify/classify_property_test.go` | rapid-driven property tests |
| `internal/cli/catalogue_cmd.go` | `themis catalogue sync` |
| `internal/cli/classify_cmd.go` | `themis classify` |
| `internal/cli/*_cmd_test.go` | matching CLI tests |
| extend `internal/ledger/registry.go` | register `CATALOGUE_SYNCED`, `IMPACT_CLASSIFIED` |
| extend `internal/ledger/wiring_test.go` | require both kinds |

## Task list

### Task 1: `AIChange` value type

**Files:** Create `internal/aichange/aichange.go`, `internal/aichange/aichange_test.go`.

`AIChange` is the normalised representation. Plan-2 fields (more added later by adapter plans):
- `PRID string` — pull-request identifier (e.g. `gh:org/repo#123`)
- `Actor string` — `claude_code`, `vxd`, `human:thandi`, …
- `TouchedFiles []FileTouch` — each `{Path, ChangeKind: ADDED|MODIFIED|DELETED, BeforeHash, AfterHash string}`
- `RawTranscriptHash string` — SHA-256 of the AI transcript (or "" if none)
- `Metadata map[string]string` — adapter-specific

Tests cover JSON round-trip + nil-slice safe + zero-value valid.

### Task 2: `CatalogueGraph` value types

**Files:** Create `internal/catalogue/graph.go`, `internal/catalogue/graph_test.go`.

Types:
- `Domain { ID, Name string; Services []ServiceRef }`
- `Service { ID, Name, Domain string; Produces, Consumes []EventRef; SourcePath string }`
- `EventDef { ID, Name, Domain string; SchemaPath string; Owners []string; Version string }`
- `CatalogueGraph { Domains map[string]Domain; Services map[string]Service; Events map[string]EventDef; SourceRoot string; SyncedAt time.Time; ContentHash string }`

Methods:
- `(g CatalogueGraph) ConsumersOf(eventID) []Service`
- `(g CatalogueGraph) ProducerOf(eventID) (Service, bool)`
- `(g CatalogueGraph) DomainOfService(serviceID) (Domain, bool)`

### Task 3: Mini EventCatalog fixture

**Files:** Create `internal/catalogue/testdata/sample/domains/...`.

A 2-domain, 4-service, 6-event tree with Markdown front-matter following the
EventCatalog v2 layout. Used by all parser tests.

### Task 4: Parser — load fixture into a graph

**Files:** Create `internal/catalogue/parser.go`, `internal/catalogue/parser_test.go`.

Public API:
- `Parse(root string) (CatalogueGraph, error)`

Implementation reads each `index.md` under `domains/*/`, `services/*/`, `events/*/`,
extracts YAML front-matter (between `---` markers), produces typed structs. No
AsyncAPI deep-parsing yet — just the registered event names/owners. `ContentHash`
is a sha256 over the sorted-tuple representation so two graphs from the same
input always produce the same hash.

### Task 5: Parser handles malformed input

**Files:** Modify `parser.go`, `parser_test.go`.

Add tests: missing front-matter delimiters, unknown front-matter keys (warn, don't
fail), duplicate event id (error), missing referenced event (error). Failure modes
must return rich errors with file path + line.

### Task 6: Catalogue ContentHash determinism property test

**Files:** Create `internal/catalogue/parser_property_test.go`.

Property: for any directory traversal order, `Parse(root).ContentHash` is the same.
Sanity: clone the fixture into a temp dir, shuffle filenames via mtime, re-parse,
hash unchanged.

### Task 7: `Impact` value type + `Kind` enum

**Files:** Create `internal/classify/impact.go`, `internal/classify/impact_test.go`.

```go
type Kind string

const (
    KindSchemaBreaking Kind = "SCHEMA_BREAKING"
    KindNewEvent       Kind = "NEW_EVENT"
    KindConsumerTouch  Kind = "CONSUMER_TOUCH"
    KindProducerTouch  Kind = "PRODUCER_TOUCH"
    KindDocOnly        Kind = "DOC_ONLY"
    KindOffCatalogue   Kind = "OFF_CATALOGUE"
    KindNonContract    Kind = "NON_CONTRACT"
)

type Impact struct {
    Kind           Kind
    Domain         string
    Service        string
    EventID        string
    Reason         string
    AffectedEvents []string
    AffectedConsumers []string
}
```

Tests: `Kind.String()`, `(Impact).IsContract()` helper, JSON round-trip.

### Task 8: `Classify` — happy paths per kind

**Files:** Create `internal/classify/classify.go`, `internal/classify/classify_test.go`.

`func Classify(c AIChange, g CatalogueGraph) Impact`

Logic (priority order — first match wins):
1. Any touched file is an `events/*.schema.json` that already exists in `g.Events` with a changed schema → `SCHEMA_BREAKING`. (For Plan 2: detect "AfterHash != BeforeHash + path matches a registered schema".)
2. Any touched ADDED file under `events/*` not in `g.Events` → `NEW_EVENT`.
3. Any touched MODIFIED file under `services/*` where service consumes events → `CONSUMER_TOUCH`.
4. Any touched MODIFIED file under `services/*` where service produces events → `PRODUCER_TOUCH`.
5. All touches under `docs/**` or `README*` or `*.md` → `DOC_ONLY`.
6. Touches under paths not present in the graph at all → `OFF_CATALOGUE`.
7. Otherwise → `NON_CONTRACT`.

One test per kind. Each test constructs a minimal graph + AIChange and asserts kind + reason + affected sets.

### Task 9: `Classify` is deterministic — property test

**Files:** Create `internal/classify/classify_property_test.go`.

Property: for the same `(AIChange, CatalogueGraph)`, `Classify` returns the same
`Impact` bytes when JSON-marshalled. Run 200 rapid iterations with synthesised
inputs.

### Task 10: `Classify` is monotone in evidence

**Files:** Modify `internal/classify/classify_property_test.go`.

Adding a touched file never *downgrades* an Impact's severity (where severity =
the ordinal in the priority list above). Property: `Classify(c, g)` ≤
`Classify(c+extraFile, g)` in severity rank.

### Task 11: Register `CATALOGUE_SYNCED` + `IMPACT_CLASSIFIED` in DefaultRegistry

**Files:** Modify `internal/ledger/registry.go`, `internal/ledger/wiring_test.go`.

Add both kinds with `noopProject` projectors. Extend wiring test's `want` list.

### Task 12: `themis catalogue sync` CLI command

**Files:** Create `internal/cli/catalogue_cmd.go`, `internal/cli/catalogue_cmd_test.go`.

`themis catalogue sync --id <tenant> --base <state> --source <path>`

Parses the catalogue at `--source`, emits `CATALOGUE_SYNCED` event with payload:
```json
{"source": "<path>", "content_hash": "<hex>", "synced_at": "<rfc3339>", "domains": <int>, "services": <int>, "events": <int>}
```

Test:
- Happy path: pointer at testdata fixture → CATALOGUE_SYNCED event written, contains correct counts.
- Repeat: re-running with same content → emits a second event with the *same* content_hash (downstream consumers can dedupe).

### Task 13: `themis classify` CLI command

**Files:** Create `internal/cli/classify_cmd.go`, `internal/cli/classify_cmd_test.go`.

`themis classify --id <tenant> --base <state> --aichange <path> --catalogue <path>`

Reads an `AIChange` JSON file + a catalogue snapshot (from `catalogue sync`),
classifies, emits `IMPACT_CLASSIFIED` event with the full `Impact` as payload,
prints the Impact JSON to stdout.

Tests: doc-only AIChange → `DOC_ONLY`; AIChange touching a service that consumes
an event → `CONSUMER_TOUCH`; AIChange touching a removed event schema →
`SCHEMA_BREAKING`.

### Task 14: Wiring guard — `themis classify` rejects unregistered kinds

**Files:** Modify `internal/cli/classify_cmd_test.go`.

Test that if (hypothetically) the registry lost `IMPACT_CLASSIFIED`, the classify
command would refuse to emit — proves the wiring test is meaningful.

### Task 15: Update README changelog

**Files:** Modify `README.md` (Changelog section).

Append "Plan 2 (Catalogue + Classifier)" sub-section with the new packages,
event kinds, and CLI commands.

### Task 16: Run full `make ci`

**Files:** none (verification).

Expect: vet → lint → test → cover → vulncheck → cover_check PASS.
