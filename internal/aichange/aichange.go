// Package aichange defines the AIChange value type — the normalised
// representation of "what an AI tool (or human-with-AI-help) did in a PR".
// Every adapter in internal/ingest produces an AIChange; downstream packages
// (classify, scan, policy, bom) consume one.
package aichange

import (
	"encoding/json"
	"errors"
	"strconv"
)

// FileChangeKind enumerates the ways a file can change in a PR.
type FileChangeKind string

const (
	// FileAdded — the file did not exist before the change.
	FileAdded FileChangeKind = "ADDED"
	// FileModified — the file existed and was changed in place.
	FileModified FileChangeKind = "MODIFIED"
	// FileDeleted — the file existed before and is removed by the change.
	FileDeleted FileChangeKind = "DELETED"
)

// Valid reports whether k is one of the three defined values.
func (k FileChangeKind) Valid() bool {
	switch k {
	case FileAdded, FileModified, FileDeleted:
		return true
	}
	return false
}

// FileTouch describes one file's contribution to an AIChange.
//
// BeforeHash is empty for ADDED files; AfterHash is empty for DELETED files.
// Both are hex SHA-256 of the file content; the diff between them is
// implicit. (Themis never stores raw file content in the ledger — only hashes —
// to keep the ledger PII-free and reasonable in size.)
type FileTouch struct {
	Path       string         `json:"path"`
	ChangeKind FileChangeKind `json:"change_kind"`
	BeforeHash string         `json:"before_hash,omitempty"`
	AfterHash  string         `json:"after_hash,omitempty"`
}

// AIChange is the normalised input the rest of Themis reasons over.
// One AIChange = one PR (or one agent batch); many ledger events reference
// the same AIChange.
type AIChange struct {
	// PRID uniquely identifies the change in some external system, e.g.
	// "gh:tzone85/themis#42" or "vxd:bounty-2026-05-23-001".
	PRID string `json:"pr_id"`

	// Actor identifies who/what produced the change. Examples:
	// "claude_code", "cursor", "github_copilot", "vxd", "human:thandi".
	Actor string `json:"actor"`

	// TouchedFiles lists every file the change adds, modifies, or deletes.
	TouchedFiles []FileTouch `json:"touched_files"`

	// RawTranscriptHash is the SHA-256 hex of the AI tool transcript (Claude
	// Code session JSON, Cursor MCP log, …) — "" if none was attached.
	RawTranscriptHash string `json:"raw_transcript_hash,omitempty"`

	// Metadata carries adapter-specific key/value pairs. Plan-2 readers
	// MUST ignore unknown keys; producers MUST namespace their keys (e.g.
	// "claude_code:model", "vxd:bounty_id").
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ErrInvalidChangeKind indicates a FileTouch carried an unknown ChangeKind.
var ErrInvalidChangeKind = errors.New("aichange: invalid FileTouch.ChangeKind")

// Validate enforces minimum invariants. It is intentionally permissive
// because the rest of Themis tolerates partial evidence (see
// design spec §8 "INGEST_PARTIAL"); but a structurally broken AIChange
// must not propagate.
func (c AIChange) Validate() error {
	for i, ft := range c.TouchedFiles {
		if !ft.ChangeKind.Valid() {
			return errFileTouch{index: i, kind: ft.ChangeKind}
		}
	}
	return nil
}

type errFileTouch struct {
	index int
	kind  FileChangeKind
}

func (e errFileTouch) Error() string {
	return "aichange: TouchedFiles[" + strconv.Itoa(e.index) + "]: invalid ChangeKind " + string(e.kind)
}

func (e errFileTouch) Unwrap() error { return ErrInvalidChangeKind }

// MarshalJSON ensures TouchedFiles is always emitted as `[]`, never `null`,
// so downstream consumers can iterate without nil-checks.
func (c AIChange) MarshalJSON() ([]byte, error) {
	type alias AIChange
	a := alias(c)
	if a.TouchedFiles == nil {
		a.TouchedFiles = []FileTouch{}
	}
	return json.Marshal(a)
}
