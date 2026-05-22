package catalogue

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrDuplicateID is returned when the same id appears for two entities
// of the same kind (two services called "collector", two events called X).
var ErrDuplicateID = errors.New("catalogue: duplicate id")

// ErrMissingReference is returned when a service Produces/Consumes an event
// that does not appear under events/.
var ErrMissingReference = errors.New("catalogue: missing reference")

// Parse walks the EventCatalog tree rooted at path and returns a fully
// resolved CatalogueGraph. The graph's ContentHash is deterministic — for
// the same input bytes in the same logical structure it always returns
// the same hex string regardless of traversal order.
func Parse(root string) (CatalogueGraph, error) {
	g := CatalogueGraph{
		Domains:    map[string]Domain{},
		Services:   map[string]Service{},
		Events:     map[string]EventDef{},
		SourceRoot: root,
		SyncedAt:   time.Now().UTC(),
	}

	if err := parseDomains(filepath.Join(root, "domains"), &g); err != nil {
		return CatalogueGraph{}, err
	}
	if err := parseServices(filepath.Join(root, "services"), &g); err != nil {
		return CatalogueGraph{}, err
	}
	if err := parseEvents(filepath.Join(root, "events"), &g); err != nil {
		return CatalogueGraph{}, err
	}
	if err := resolveReferences(&g); err != nil {
		return CatalogueGraph{}, err
	}
	g.ContentHash = computeContentHash(g)
	return g, nil
}

func parseDomains(dir string, g *CatalogueGraph) error {
	return forEachIndex(dir, "domain", func(path string, fm frontMatter) error {
		if fm.ID == "" {
			return fmt.Errorf("%s: domain missing 'id' in front-matter", path)
		}
		if _, exists := g.Domains[fm.ID]; exists {
			return fmt.Errorf("%w: domain %q (at %s)", ErrDuplicateID, fm.ID, path)
		}
		refs := make([]ServiceRef, 0, len(fm.Services))
		for _, s := range fm.Services {
			refs = append(refs, ServiceRef{ID: s})
		}
		g.Domains[fm.ID] = Domain{ID: fm.ID, Name: nonEmpty(fm.Name, fm.ID), Services: refs}
		return nil
	})
}

func parseServices(dir string, g *CatalogueGraph) error {
	return forEachIndex(dir, "service", func(path string, fm frontMatter) error {
		if fm.ID == "" {
			return fmt.Errorf("%s: service missing 'id' in front-matter", path)
		}
		if _, exists := g.Services[fm.ID]; exists {
			return fmt.Errorf("%w: service %q (at %s)", ErrDuplicateID, fm.ID, path)
		}
		rel, _ := filepath.Rel(g.SourceRoot, path)
		g.Services[fm.ID] = Service{
			ID:         fm.ID,
			Name:       nonEmpty(fm.Name, fm.ID),
			Domain:     fm.Domain,
			Produces:   toEventRefs(fm.Produces),
			Consumes:   toEventRefs(fm.Consumes),
			SourcePath: rel,
		}
		return nil
	})
}

func parseEvents(dir string, g *CatalogueGraph) error {
	return forEachIndex(dir, "event", func(path string, fm frontMatter) error {
		if fm.ID == "" {
			return fmt.Errorf("%s: event missing 'id' in front-matter", path)
		}
		if _, exists := g.Events[fm.ID]; exists {
			return fmt.Errorf("%w: event %q (at %s)", ErrDuplicateID, fm.ID, path)
		}
		g.Events[fm.ID] = EventDef{
			ID:         fm.ID,
			Name:       nonEmpty(fm.Name, fm.ID),
			Domain:     fm.Domain,
			SchemaPath: fm.SchemaPath,
			Owners:     append([]string(nil), fm.Owners...),
			Version:    fm.Version,
		}
		return nil
	})
}

func resolveReferences(g *CatalogueGraph) error {
	for _, sid := range sortedKeys(g.Services) {
		s := g.Services[sid]
		for _, ref := range s.Produces {
			if _, ok := g.Events[ref.ID]; !ok {
				return fmt.Errorf("%w: service %q produces unknown event %q", ErrMissingReference, sid, ref.ID)
			}
		}
		for _, ref := range s.Consumes {
			if _, ok := g.Events[ref.ID]; !ok {
				return fmt.Errorf("%w: service %q consumes unknown event %q", ErrMissingReference, sid, ref.ID)
			}
		}
	}
	return nil
}

// frontMatter is the union of all keys we accept across domain/service/event
// index.md files. Unknown keys are silently tolerated by yaml.v3 unless
// strict mode is enabled — which we deliberately do not enable here, so
// catalogues that carry extra metadata (EventCatalog plugins often do) can
// still parse.
type frontMatter struct {
	ID         string   `yaml:"id"`
	Name       string   `yaml:"name"`
	Domain     string   `yaml:"domain"`
	Services   []string `yaml:"services"`
	Produces   []string `yaml:"produces"`
	Consumes   []string `yaml:"consumes"`
	SchemaPath string   `yaml:"schema_path"`
	Owners     []string `yaml:"owners"`
	Version    string   `yaml:"version"`
}

// forEachIndex walks dir, finds each `*/index.md`, extracts YAML front-matter,
// and invokes fn. dirKind is used for error messages only.
func forEachIndex(dir, dirKind string, fn func(path string, fm frontMatter) error) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // empty catalogue section is allowed
		}
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		indexPath := filepath.Join(dir, e.Name(), "index.md")
		if _, err := os.Stat(indexPath); err != nil {
			continue
		}
		fm, err := readFrontMatter(indexPath)
		if err != nil {
			return fmt.Errorf("%s %s: %w", dirKind, indexPath, err)
		}
		if err := fn(indexPath, fm); err != nil {
			return err
		}
	}
	return nil
}

// readFrontMatter pulls the YAML block between leading `---\n` and the next
// `\n---\n`. Returns an error if the delimiters are missing.
func readFrontMatter(path string) (frontMatter, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path comes from forEachIndex, rooted at user-supplied catalogue dir.
	if err != nil {
		return frontMatter{}, fmt.Errorf("read: %w", err)
	}
	const delim = "---"
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if !bytes.HasPrefix(trimmed, []byte(delim)) {
		return frontMatter{}, fmt.Errorf("missing leading '---' delimiter")
	}
	rest := bytes.TrimPrefix(trimmed, []byte(delim))
	// Skip newline immediately after the delimiter.
	rest = bytes.TrimLeft(rest, "\r\n")
	end := bytes.Index(rest, []byte("\n"+delim))
	if end < 0 {
		return frontMatter{}, fmt.Errorf("missing closing '---' delimiter")
	}
	yamlBlock := rest[:end]
	var fm frontMatter
	if err := yaml.Unmarshal(yamlBlock, &fm); err != nil {
		return frontMatter{}, fmt.Errorf("yaml unmarshal: %w", err)
	}
	return fm, nil
}

func toEventRefs(ids []string) []EventRef {
	if len(ids) == 0 {
		return nil
	}
	out := make([]EventRef, 0, len(ids))
	for _, id := range ids {
		out = append(out, EventRef{ID: id})
	}
	return out
}

func nonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// computeContentHash returns a hex SHA-256 over a canonical, order-independent
// representation of the graph. Reordering files on disk does not change it.
func computeContentHash(g CatalogueGraph) string {
	var buf bytes.Buffer
	buf.WriteString("DOMAINS\n")
	for _, id := range sortedKeys(g.Domains) {
		d := g.Domains[id]
		fmt.Fprintf(&buf, "  %s|%s\n", d.ID, d.Name)
		// services are referenced from Domain.Services; sort for stability.
		svcIDs := make([]string, 0, len(d.Services))
		for _, s := range d.Services {
			svcIDs = append(svcIDs, s.ID)
		}
		sortStrings(svcIDs)
		for _, sid := range svcIDs {
			fmt.Fprintf(&buf, "    svc=%s\n", sid)
		}
	}
	buf.WriteString("SERVICES\n")
	for _, id := range sortedKeys(g.Services) {
		s := g.Services[id]
		fmt.Fprintf(&buf, "  %s|%s|%s|%s\n", s.ID, s.Name, s.Domain, s.SourcePath)
		prod := refIDs(s.Produces)
		sortStrings(prod)
		for _, p := range prod {
			fmt.Fprintf(&buf, "    produces=%s\n", p)
		}
		cons := refIDs(s.Consumes)
		sortStrings(cons)
		for _, c := range cons {
			fmt.Fprintf(&buf, "    consumes=%s\n", c)
		}
	}
	buf.WriteString("EVENTS\n")
	for _, id := range sortedKeys(g.Events) {
		e := g.Events[id]
		fmt.Fprintf(&buf, "  %s|%s|%s|%s|%s\n", e.ID, e.Name, e.Domain, e.Version, e.SchemaPath)
		owners := append([]string(nil), e.Owners...)
		sortStrings(owners)
		for _, o := range owners {
			fmt.Fprintf(&buf, "    owner=%s\n", o)
		}
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:])
}

func refIDs(refs []EventRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

// sortStrings is in this file (rather than sort.go) to keep the canonical
// hash logic self-contained — it's the load-bearing piece for determinism.
func sortStrings(s []string) {
	// insertion sort is fine: graphs are small.
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
