package scan

import (
	"sort"

	"github.com/tzone85/themis/internal/aichange"
)

// RunAll runs every supplied Scanner against the AIChange. Scanner failures
// are captured as a synthetic "scan_failure" finding rather than aborting
// the run — per design spec §8.1, "every failure becomes a ledger event".
// The returned slice is sorted deterministically (detector, file, line, kind)
// so the output is stable for golden tests + idempotent re-projection.
func RunAll(scanners []Scanner, c aichange.AIChange, fileBodies map[string][]byte) []Finding {
	all := make([]Finding, 0)
	for _, s := range scanners {
		findings, err := s.Scan(c, fileBodies)
		if err != nil {
			all = append(all, Finding{
				Kind:        "scan_failure",
				Severity:    SeverityHigh,
				Description: "scanner " + s.Name() + " failed: " + err.Error(),
				Detector:    s.Name(),
			})
			continue
		}
		all = append(all, findings...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Detector != all[j].Detector {
			return all[i].Detector < all[j].Detector
		}
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		return all[i].Kind < all[j].Kind
	})
	return all
}

// DefaultScanners returns the set of scanners enabled by default. Tests and
// the decide CLI use this; the eventual plugin marketplace will extend it.
func DefaultScanners() []Scanner {
	return []Scanner{
		NewSecretsScanner(),
		NewPIIScanner(),
		NewSupplyChainScanner(),
	}
}
