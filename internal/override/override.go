// Package override implements the emergency-override flow from design
// spec §9.1.1. An override is a deliberately constrained mechanism:
//
//   - the actor must be named,
//   - the reason must be ≥ 50 characters,
//   - a co-signer must be supplied,
//   - the override is time-boxed (default 24h),
//   - a post-mortem ledger event is automatically scheduled and
//     compliance MUST close it within 7 days.
//
// All validation and status computation lives in this pure package; CLI
// and API surfaces append the resulting ledger events.
package override

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tzone85/themis/internal/ledger"
)

// MinReasonLength is the shortest reason an operator may submit. Lifted
// directly from the design spec §9.1.1 ("Reason ≥ 50 characters").
const MinReasonLength = 50

// DefaultDuration is how long an override is active for unless the operator
// supplies a shorter --expires-at.
const DefaultDuration = 24 * time.Hour

// PostmortemWindow is how long a closer has to file the post-mortem before
// the system marks the override "post-mortem overdue".
const PostmortemWindow = 7 * 24 * time.Hour

// InvokePayload is the JSON shape of an EMERGENCY_OVERRIDE_INVOKED event.
type InvokePayload struct {
	PRID             string    `json:"pr_id"`
	Actor            string    `json:"actor"`
	CoSigner         string    `json:"co_signer"`
	Reason           string    `json:"reason"`
	Scope            string    `json:"scope,omitempty"`
	InvokedAt        time.Time `json:"invoked_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	PostmortemDueAt  time.Time `json:"postmortem_due_at"`
}

// PostmortemDuePayload is emitted alongside an invoke so the timeline shows
// the obligation explicitly without re-deriving it from InvokePayload.
type PostmortemDuePayload struct {
	PRID            string    `json:"pr_id"`
	DueAt           time.Time `json:"due_at"`
	InvokedAt       time.Time `json:"invoked_at"`
}

// PostmortemClosedPayload records the operator who closed the post-mortem.
type PostmortemClosedPayload struct {
	PRID     string    `json:"pr_id"`
	Closer   string    `json:"closer"`
	Notes    string    `json:"notes"`
	ClosedAt time.Time `json:"closed_at"`
}

// Status summarises the override state for a PR.
type Status struct {
	Active            bool      `json:"active"`
	Expired           bool      `json:"expired"`
	InvokedAt         time.Time `json:"invoked_at,omitempty"`
	ExpiresAt         time.Time `json:"expires_at,omitempty"`
	PostmortemDue     bool      `json:"postmortem_due"`
	PostmortemDueAt   time.Time `json:"postmortem_due_at,omitempty"`
	PostmortemClosed  bool      `json:"postmortem_closed"`
	PostmortemOverdue bool      `json:"postmortem_overdue"`
	Actor             string    `json:"actor,omitempty"`
	CoSigner          string    `json:"co_signer,omitempty"`
}

// Errors surfaced by Invoke validation. Wrapped for errors.Is routing.
var (
	ErrReasonTooShort    = errors.New("override: reason too short")
	ErrMissingField      = errors.New("override: missing required field")
	ErrExpiryInPast      = errors.New("override: expires_at must be in the future")
)

// ValidateInvoke runs the structural checks the design spec requires.
func ValidateInvoke(p InvokePayload, now time.Time) error {
	if strings.TrimSpace(p.PRID) == "" {
		return fmt.Errorf("%w: pr_id", ErrMissingField)
	}
	if strings.TrimSpace(p.Actor) == "" {
		return fmt.Errorf("%w: actor", ErrMissingField)
	}
	if strings.TrimSpace(p.CoSigner) == "" {
		return fmt.Errorf("%w: co_signer", ErrMissingField)
	}
	if p.Actor == p.CoSigner {
		return fmt.Errorf("%w: actor and co_signer must differ", ErrMissingField)
	}
	if len(strings.TrimSpace(p.Reason)) < MinReasonLength {
		return fmt.Errorf("%w: need ≥ %d chars, got %d", ErrReasonTooShort, MinReasonLength, len(strings.TrimSpace(p.Reason)))
	}
	if !p.ExpiresAt.After(now) {
		return ErrExpiryInPast
	}
	return nil
}

// BuildInvoke fills out the timestamps + post-mortem due window for an
// invocation. Caller is expected to validate first.
func BuildInvoke(p InvokePayload, now time.Time) (InvokePayload, PostmortemDuePayload) {
	invoke := p
	invoke.InvokedAt = now.UTC()
	if invoke.ExpiresAt.IsZero() {
		invoke.ExpiresAt = now.Add(DefaultDuration).UTC()
	}
	invoke.PostmortemDueAt = now.Add(PostmortemWindow).UTC()
	due := PostmortemDuePayload{
		PRID:      invoke.PRID,
		DueAt:     invoke.PostmortemDueAt,
		InvokedAt: invoke.InvokedAt,
	}
	return invoke, due
}

// Compute returns the current override state for prID. Multiple invocations
// for the same PR are allowed (re-invoke after expiry); Compute considers
// only the most recent one.
func Compute(events []ledger.Event, prID string, now time.Time) Status {
	st := Status{}
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		switch e.Kind {
		case "EMERGENCY_OVERRIDE_INVOKED":
			var p InvokePayload
			if err := json.Unmarshal(e.Payload, &p); err != nil || p.PRID != prID {
				continue
			}
			st.InvokedAt = p.InvokedAt
			st.ExpiresAt = p.ExpiresAt
			st.PostmortemDueAt = p.PostmortemDueAt
			st.Actor = p.Actor
			st.CoSigner = p.CoSigner
			st.Active = !now.After(p.ExpiresAt)
			st.Expired = !st.Active
			st.PostmortemDue = true
		case "OVERRIDE_POSTMORTEM_CLOSED":
			var p PostmortemClosedPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil || p.PRID != prID {
				continue
			}
			st.PostmortemClosed = true
		}
		if st.PostmortemDue && st.PostmortemClosed {
			break
		}
	}
	if st.PostmortemDue && !st.PostmortemClosed && now.After(st.PostmortemDueAt) {
		st.PostmortemOverdue = true
	}
	return st
}

// BuildClosed constructs the PostmortemClosedPayload for the ledger event.
func BuildClosed(prID, closer, notes string, now time.Time) PostmortemClosedPayload {
	return PostmortemClosedPayload{
		PRID: prID, Closer: closer, Notes: notes, ClosedAt: now.UTC(),
	}
}
