package core_test

import (
	"errors"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestValidateTrackTransition_SameStatus(t *testing.T) {
	for _, s := range []core.TrackStatus{
		core.TrackStatusPending,
		core.TrackStatusActive,
		core.TrackStatusCompleted,
		core.TrackStatusArchived,
	} {
		if err := core.ValidateTrackTransition(s, s, 0, false); err != nil {
			t.Errorf("same-status %s should be no-op, got: %v", s, err)
		}
	}
}

func TestValidateTrackTransition_PendingToActive(t *testing.T) {
	// With linked tasks — allowed
	if err := core.ValidateTrackTransition(
		core.TrackStatusPending, core.TrackStatusActive, 3, false,
	); err != nil {
		t.Fatalf("expected allowed, got: %v", err)
	}

	// Without linked tasks — rejected
	err := core.ValidateTrackTransition(
		core.TrackStatusPending, core.TrackStatusActive, 0, false,
	)
	if err == nil {
		t.Fatal("expected error for 0 linked tasks")
	}
	var te core.ErrInvalidTrackTransition
	if !errors.As(err, &te) {
		t.Fatalf("expected ErrInvalidTrackTransition, got %T", err)
	}
}

func TestValidateTrackTransition_ActiveToCompleted(t *testing.T) {
	// All tasks terminal — allowed
	if err := core.ValidateTrackTransition(
		core.TrackStatusActive, core.TrackStatusCompleted, 5, true,
	); err != nil {
		t.Fatalf("expected allowed, got: %v", err)
	}

	// Non-terminal tasks — rejected
	err := core.ValidateTrackTransition(
		core.TrackStatusActive, core.TrackStatusCompleted, 5, false,
	)
	if err == nil {
		t.Fatal("expected error for non-terminal tasks")
	}
}

func TestValidateTrackTransition_PendingToAbandoned(t *testing.T) {
	if err := core.ValidateTrackTransition(
		core.TrackStatusPending, core.TrackStatusAbandoned, 0, false,
	); err != nil {
		t.Fatalf("pending → abandoned should be allowed, got: %v", err)
	}
}

func TestValidateTrackTransition_CompletedToAbandoned(t *testing.T) {
	if err := core.ValidateTrackTransition(
		core.TrackStatusCompleted, core.TrackStatusAbandoned, 5, true,
	); err != nil {
		t.Fatalf("completed → abandoned should be allowed, got: %v", err)
	}
}

func TestValidateTrackTransition_ActiveToAbandoned(t *testing.T) {
	if err := core.ValidateTrackTransition(
		core.TrackStatusActive, core.TrackStatusAbandoned, 3, false,
	); err != nil {
		t.Fatalf("expected allowed, got: %v", err)
	}
}

func TestValidateTrackTransition_CompletedToArchived(t *testing.T) {
	if err := core.ValidateTrackTransition(
		core.TrackStatusCompleted, core.TrackStatusArchived, 5, true,
	); err != nil {
		t.Fatalf("expected allowed, got: %v", err)
	}
}

func TestValidateTrackTransition_AbandonedToArchived(t *testing.T) {
	if err := core.ValidateTrackTransition(
		core.TrackStatusAbandoned, core.TrackStatusArchived, 0, false,
	); err != nil {
		t.Fatalf("expected allowed, got: %v", err)
	}
}

func TestValidateTrackTransition_ArchivedIsTerminal(t *testing.T) {
	for _, next := range []core.TrackStatus{
		core.TrackStatusPending,
		core.TrackStatusActive,
		core.TrackStatusCompleted,
		core.TrackStatusAbandoned,
	} {
		err := core.ValidateTrackTransition(
			core.TrackStatusArchived, next, 5, true,
		)
		if err == nil {
			t.Errorf("expected error for archived → %s", next)
		}
	}
}

func TestValidateTrackTransition_DisallowedTransitions(t *testing.T) {
	cases := []struct {
		from core.TrackStatus
		to   core.TrackStatus
	}{
		{core.TrackStatusPending, core.TrackStatusCompleted},
		{core.TrackStatusPending, core.TrackStatusArchived},
		{core.TrackStatusActive, core.TrackStatusPending},
		{core.TrackStatusCompleted, core.TrackStatusActive},
		{core.TrackStatusAbandoned, core.TrackStatusActive},
	}
	for _, tc := range cases {
		err := core.ValidateTrackTransition(tc.from, tc.to, 5, true)
		if err == nil {
			t.Errorf("expected error for %s → %s", tc.from, tc.to)
		}
	}
}

func TestValidateTrackTransition_UnknownStatus(t *testing.T) {
	err := core.ValidateTrackTransition("bogus", core.TrackStatusActive, 1, false)
	if err == nil {
		t.Fatal("expected error for unknown status")
	}

	err = core.ValidateTrackTransition(core.TrackStatusPending, "bogus", 1, false)
	if err == nil {
		t.Fatal("expected error for unknown target status")
	}
}

func TestErrInvalidTrackTransition_Error(t *testing.T) {
	// With allowed list
	e := core.ErrInvalidTrackTransition{
		From:    core.TrackStatusActive,
		To:      core.TrackStatusPending,
		Msg:     "transition not allowed",
		Allowed: []core.TrackStatus{core.TrackStatusCompleted, core.TrackStatusAbandoned},
	}
	msg := e.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}

	// Without allowed list
	e2 := core.ErrInvalidTrackTransition{
		From: core.TrackStatusArchived,
		To:   core.TrackStatusActive,
		Msg:  "archived is terminal",
	}
	msg2 := e2.Error()
	if msg2 == "" {
		t.Fatal("expected non-empty error message")
	}
}
