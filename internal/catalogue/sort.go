package catalogue

import "sort"

// sortedKeys returns the keys of m in lexical order. Used to make
// graph traversal deterministic regardless of Go's randomised map order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
