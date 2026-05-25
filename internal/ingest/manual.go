package ingest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tzone85/themis/internal/aichange"
)

// Manual is the "operator declares the change shape" adapter. Used for
// emergency overrides and retroactive PRs where no machine record exists.
//
// Required Inputs: PRID, ActorOverride (must start with "human:"), Files.
type Manual struct{}

// Name returns the adapter's stable name.
func (m *Manual) Name() string { return "manual_attestation" }

// Ingest validates the operator-supplied attestation and builds an AIChange.
func (m *Manual) Ingest(in Inputs) (aichange.AIChange, error) {
	if in.PRID == "" {
		return aichange.AIChange{}, fmt.Errorf("%w: manual_attestation requires --pr-id", ErrAdapterFailed)
	}
	if !strings.HasPrefix(in.ActorOverride, "human:") {
		return aichange.AIChange{}, fmt.Errorf("%w: manual_attestation requires --actor starting with 'human:' (got %q)", ErrAdapterFailed, in.ActorOverride)
	}
	if len(in.Files) == 0 {
		return aichange.AIChange{}, fmt.Errorf("%w: manual_attestation requires at least one --file entry", ErrAdapterFailed)
	}
	// Sort paths for deterministic output — re-running with the same input
	// must produce identical AIChange bytes (audit replay).
	paths := make([]string, 0, len(in.Files))
	for p := range in.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	touches := make([]aichange.FileTouch, 0, len(paths))
	for _, p := range paths {
		hashes := in.Files[p]
		kind := inferChangeKind(hashes[0], hashes[1])
		touches = append(touches, aichange.FileTouch{
			Path:       p,
			ChangeKind: kind,
			BeforeHash: hashes[0],
			AfterHash:  hashes[1],
		})
	}

	return aichange.AIChange{
		PRID:         in.PRID,
		Actor:        in.ActorOverride,
		TouchedFiles: touches,
		Metadata:     copyMetadata(in.Extra),
	}, nil
}

// inferChangeKind picks ADDED/MODIFIED/DELETED based on which hashes are set.
func inferChangeKind(before, after string) aichange.FileChangeKind {
	switch {
	case before == "" && after != "":
		return aichange.FileAdded
	case before != "" && after == "":
		return aichange.FileDeleted
	default:
		return aichange.FileModified
	}
}

// copyMetadata returns a defensive copy so the caller's map isn't shared.
func copyMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
