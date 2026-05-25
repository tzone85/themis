package override

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tzone85/themis/internal/ledger"
)

func mustMarshal(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestValidateInvoke_HappyPath(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	p := InvokePayload{
		PRID:      "x#1",
		Actor:     "human:alice",
		CoSigner:  "human:bob",
		Reason:    strings.Repeat("urgent fix for compliance audit blocker; need to merge before EOD ", 1),
		ExpiresAt: now.Add(time.Hour),
	}
	if err := ValidateInvoke(p, now); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestValidateInvoke_RejectsShortReason(t *testing.T) {
	now := time.Now()
	p := InvokePayload{
		PRID:      "x", Actor: "a", CoSigner: "b",
		Reason:    "too short",
		ExpiresAt: now.Add(time.Hour),
	}
	if err := ValidateInvoke(p, now); !errors.Is(err, ErrReasonTooShort) {
		t.Fatalf("expected ErrReasonTooShort, got %v", err)
	}
}

func TestValidateInvoke_RejectsMissingFields(t *testing.T) {
	now := time.Now()
	base := InvokePayload{
		PRID: "x", Actor: "a", CoSigner: "b",
		Reason:    strings.Repeat("x", MinReasonLength),
		ExpiresAt: now.Add(time.Hour),
	}
	for _, m := range []func(*InvokePayload){
		func(p *InvokePayload) { p.PRID = "" },
		func(p *InvokePayload) { p.Actor = "" },
		func(p *InvokePayload) { p.CoSigner = "" },
	} {
		variant := base
		m(&variant)
		if err := ValidateInvoke(variant, now); !errors.Is(err, ErrMissingField) {
			t.Errorf("missing field not detected: variant=%+v err=%v", variant, err)
		}
	}
}

func TestValidateInvoke_RejectsActorEqualsCoSigner(t *testing.T) {
	now := time.Now()
	p := InvokePayload{
		PRID: "x", Actor: "human:alice", CoSigner: "human:alice",
		Reason:    strings.Repeat("x", MinReasonLength),
		ExpiresAt: now.Add(time.Hour),
	}
	if err := ValidateInvoke(p, now); !errors.Is(err, ErrMissingField) {
		t.Fatalf("expected ErrMissingField for actor==co_signer, got %v", err)
	}
}

func TestValidateInvoke_RejectsExpiryInPast(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	p := InvokePayload{
		PRID: "x", Actor: "a", CoSigner: "b",
		Reason:    strings.Repeat("x", MinReasonLength),
		ExpiresAt: now.Add(-time.Hour),
	}
	if err := ValidateInvoke(p, now); !errors.Is(err, ErrExpiryInPast) {
		t.Fatalf("expected ErrExpiryInPast, got %v", err)
	}
}

func TestBuildInvoke_DefaultsExpiryToTwentyFourHours(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	p := InvokePayload{PRID: "x", Actor: "a", CoSigner: "b"}
	out, due := BuildInvoke(p, now)
	if !out.ExpiresAt.Equal(now.Add(DefaultDuration).UTC()) {
		t.Fatalf("ExpiresAt = %v, want %v", out.ExpiresAt, now.Add(DefaultDuration))
	}
	if !due.DueAt.Equal(now.Add(PostmortemWindow).UTC()) {
		t.Fatalf("post-mortem due = %v, want %v", due.DueAt, now.Add(PostmortemWindow))
	}
}

func TestCompute_ActiveBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	events := []ledger.Event{
		{Kind: "EMERGENCY_OVERRIDE_INVOKED", Payload: mustMarshal(InvokePayload{
			PRID: "x#1", Actor: "a", CoSigner: "b",
			InvokedAt: now, ExpiresAt: now.Add(time.Hour),
			PostmortemDueAt: now.Add(PostmortemWindow),
		})},
	}
	st := Compute(events, "x#1", now.Add(30*time.Minute))
	if !st.Active || st.Expired {
		t.Fatalf("expected active+not expired: %+v", st)
	}
	if !st.PostmortemDue {
		t.Fatal("PostmortemDue should be true after invoke")
	}
	if st.PostmortemOverdue {
		t.Fatal("PostmortemOverdue should be false before window")
	}
}

func TestCompute_ExpiredAfterTTL(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	events := []ledger.Event{
		{Kind: "EMERGENCY_OVERRIDE_INVOKED", Payload: mustMarshal(InvokePayload{
			PRID: "x#1", InvokedAt: now, ExpiresAt: now.Add(time.Hour),
			PostmortemDueAt: now.Add(PostmortemWindow),
		})},
	}
	st := Compute(events, "x#1", now.Add(2*time.Hour))
	if st.Active || !st.Expired {
		t.Fatalf("expected expired: %+v", st)
	}
}

func TestCompute_PostmortemOverdue(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	events := []ledger.Event{
		{Kind: "EMERGENCY_OVERRIDE_INVOKED", Payload: mustMarshal(InvokePayload{
			PRID: "x#1", InvokedAt: now, ExpiresAt: now.Add(time.Hour),
			PostmortemDueAt: now.Add(PostmortemWindow),
		})},
	}
	st := Compute(events, "x#1", now.Add(PostmortemWindow+24*time.Hour))
	if !st.PostmortemOverdue {
		t.Fatal("PostmortemOverdue should be true past due_at")
	}
}

func TestCompute_PostmortemClosedClearsOverdue(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	events := []ledger.Event{
		{Kind: "EMERGENCY_OVERRIDE_INVOKED", Payload: mustMarshal(InvokePayload{
			PRID: "x#1", InvokedAt: now, ExpiresAt: now.Add(time.Hour),
			PostmortemDueAt: now.Add(PostmortemWindow),
		})},
		{Kind: "OVERRIDE_POSTMORTEM_CLOSED", Payload: mustMarshal(PostmortemClosedPayload{
			PRID: "x#1", Closer: "human:compliance", Notes: "fixed root cause",
			ClosedAt: now.Add(2 * 24 * time.Hour),
		})},
	}
	st := Compute(events, "x#1", now.Add(PostmortemWindow+24*time.Hour))
	if !st.PostmortemClosed || st.PostmortemOverdue {
		t.Fatalf("expected closed+not overdue: %+v", st)
	}
}

func TestBuildClosed_PopulatesFields(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	p := BuildClosed("x#1", "human:compliance", "done", now)
	if p.PRID != "x#1" || p.Closer != "human:compliance" || p.Notes != "done" {
		t.Fatalf("payload = %+v", p)
	}
	if !p.ClosedAt.Equal(now.UTC()) {
		t.Fatalf("ClosedAt = %v", p.ClosedAt)
	}
}

func TestCompute_IgnoresOtherPRs(t *testing.T) {
	now := time.Now()
	events := []ledger.Event{
		{Kind: "EMERGENCY_OVERRIDE_INVOKED", Payload: mustMarshal(InvokePayload{
			PRID: "OTHER#9", InvokedAt: now, ExpiresAt: now.Add(time.Hour),
		})},
	}
	st := Compute(events, "x#1", now)
	if st.Active || st.PostmortemDue {
		t.Fatalf("override from other PR leaked: %+v", st)
	}
}
