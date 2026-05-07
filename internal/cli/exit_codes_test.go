package cli

import (
	"errors"
	"fmt"
	"testing"

	"hop.top/kit/go/console/output"
	"hop.top/kit/go/runtime/domain"
	"hop.top/kit/go/runtime/policy"
	"hop.top/tlc/internal/uri"
)

// TestExitCodeFor pins the exit-code classification per docs/exit-codes.md
// (T-1104). Adding a new error class? Add a row here.
func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil = success", err: nil, want: ExitOK},
		{name: "bare error = generic", err: errors.New("oops"), want: ExitGeneric},

		{name: "not-found sentinel", err: ErrNotFound, want: ExitNotFound},
		{name: "wrapped not-found sentinel", err: fmt.Errorf("task %s: %w", "T-1", ErrNotFound), want: ExitNotFound},
		{name: "uri.ErrTaskNotFound (typed)", err: &uri.ErrTaskNotFound{ID: "T-9999"}, want: ExitNotFound},
		{name: "uri.ErrTaskNotFound joined", err: errors.Join(&uri.ErrTaskNotFound{ID: "T-9"}, errors.New("other")), want: ExitNotFound},
		{name: "uri.ErrProjectNotFound", err: &uri.ErrProjectNotFound{ProjectID: "missing/proj"}, want: ExitNotFound},
		{name: "ErrTrackNotFound sentinel", err: ErrTrackNotFound, want: ExitNotFound},
		{name: "trackNotFoundError typed (Unwrap → sentinel)", err: newTrackNotFoundError("track %q not found", "missing"), want: ExitNotFound},

		{name: "domain.ErrConflict", err: domain.ErrConflict, want: ExitConflict},
		{name: "wrapped ErrConflict", err: fmt.Errorf("save: %w", domain.ErrConflict), want: ExitConflict},
		{name: "policy.PolicyDeniedError", err: &policy.PolicyDeniedError{Message: "denied"}, want: ExitConflict},

		{name: "ErrUnauthorized sentinel", err: ErrUnauthorized, want: ExitUnauthorized},
		{name: "wrapped ErrUnauthorized", err: fmt.Errorf("login: %w", ErrUnauthorized), want: ExitUnauthorized},

		{name: "ExitCodeError forwards code", err: &ExitCodeError{Code: 42, Message: "x"}, want: 42},
		{name: "output.Error with explicit code wins", err: &output.Error{ExitCode: 7}, want: 7},
		{name: "output.Error with zero code falls through", err: &output.Error{}, want: ExitGeneric},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := exitCodeFor(tc.err)
			if got != tc.want {
				t.Errorf("exitCodeFor(%v) = %d; want %d", tc.err, got, tc.want)
			}
		})
	}
}

// TestErrTaskNotFound_AsCLIError ensures uri.ErrTaskNotFound is rendered
// through kit's middleware as a NOT_FOUND envelope (exit 3).
func TestErrTaskNotFound_AsCLIError(t *testing.T) {
	e := &uri.ErrTaskNotFound{ID: "T-9999"}
	envelope := e.AsCLIError()
	if envelope.Code != output.CodeNotFound {
		t.Errorf("Code = %q; want %q", envelope.Code, output.CodeNotFound)
	}
	if envelope.ExitCode != ExitNotFound {
		t.Errorf("ExitCode = %d; want %d", envelope.ExitCode, ExitNotFound)
	}
}

// TestTrackNotFoundError_UnwrapsToSentinel keeps existing errors.Is callers
// working: track_update.go and task_create.go both check for the sentinel.
func TestTrackNotFoundError_UnwrapsToSentinel(t *testing.T) {
	e := newTrackNotFoundError("track %q not found", "x")
	if !errors.Is(e, ErrTrackNotFound) {
		t.Errorf("errors.Is(e, ErrTrackNotFound) = false; want true")
	}
}
