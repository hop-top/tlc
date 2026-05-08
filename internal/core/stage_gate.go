package core

import (
	"errors"
	"fmt"

	"hop.top/kit/go/core/stage"
)

// StageGateError is returned by GateTrackCreate / GateTaskCreate when a
// scope's current Stage forbids the requested mutation. The scope's
// stage.Read result is NOT exposed on the error itself — callers that
// need the underlying State should call stage.Read directly. Wrapping
// the user-facing message here keeps cli error rendering predictable.
type StageGateError struct {
	Scope string
	Stage stage.Stage
	Op    string // "track.create" | "task.create"
	Kind  string // "track" | "task"
	// TrackType is set on task.create so the error message can explain
	// why a feature task was blocked while a fix task would have passed.
	TrackType string
	Message   string
}

// Error implements error. The message follows tlc's "agent-first
// actionable" convention from AGENTS.md: name the subject, state the
// reason, suggest a next step.
func (e *StageGateError) Error() string {
	return fmt.Sprintf(
		"%s blocked: scope %q is in stage %q; %s",
		e.Op, e.Scope, e.Stage, e.Message,
	)
}

// IsStageGateError unwraps to the sentinel for cli exit-code routing.
func IsStageGateError(err error) bool {
	var sge *StageGateError
	return errors.As(err, &sge)
}

// stageReader is the seam between core/stage and tlc gating. Tests
// inject a stub; production wires stage.Read directly via
// DefaultStageReader.
type stageReader func(scope string) (stage.State, error)

// DefaultStageReader resolves the active stage for a scope by calling
// kit/core/stage.Read. Wrapped here so tests can swap it without
// touching the real projects.yaml on disk.
var DefaultStageReader stageReader = stage.Read

// GateTrackCreate returns a non-nil error when the supplied scope's
// stage forbids creating a new track of the supplied type. The scope
// is the projects.yaml entry name (typically core.DetectProject().
// ProjectID); an empty scope short-circuits to allow so callers
// outside any project context aren't gated.
//
// Policy table (matches kit/runtime/policy/stage.yaml defaults):
//
//	active            → allow all
//	public_feedback   → allow only trackType == "feedback"
//	feature_freeze    → deny (no new tracks)
//	maintenance       → deny (no new tracks)
//	sunset            → deny (no creates)
//	archived          → deny (read-only scope)
//
// Empty / missing Stage on the scope is treated as StageActive so
// adopters who haven't opted into stage.yaml see no behaviour change.
func GateTrackCreate(scope, trackType string) error {
	if scope == "" {
		return nil
	}
	st, err := DefaultStageReader(scope)
	if err != nil {
		return fmt.Errorf("stage gate: read scope %q: %w", scope, err)
	}
	switch st.Stage {
	case "", stage.StageActive:
		return nil
	case stage.StagePublicFeedback:
		if trackType == "feedback" {
			return nil
		}
		return &StageGateError{
			Scope:     scope,
			Stage:     st.Stage,
			Op:        "track.create",
			Kind:      "track",
			TrackType: trackType,
			Message: "only feedback-typed tracks may be created in public_feedback; " +
				"create with --type feedback or change the scope stage via 'tlc stage set'",
		}
	case stage.StageFeatureFreeze:
		return &StageGateError{
			Scope: scope, Stage: st.Stage, Op: "track.create", Kind: "track",
			TrackType: trackType,
			Message: "no new tracks during feature_freeze (fix/chore tasks ok); " +
				"change the scope stage via 'tlc stage set' to unblock",
		}
	case stage.StageMaintenance:
		return &StageGateError{
			Scope: scope, Stage: st.Stage, Op: "track.create", Kind: "track",
			TrackType: trackType,
			Message: "no new tracks during maintenance (fix/chore/docs tasks only); " +
				"change the scope stage via 'tlc stage set' to unblock",
		}
	case stage.StageSunset:
		return &StageGateError{
			Scope: scope, Stage: st.Stage, Op: "track.create", Kind: "track",
			TrackType: trackType,
			Message: "scope is in sunset; no new entities allowed (updates/deletes ok)",
		}
	case stage.StageArchived:
		return &StageGateError{
			Scope: scope, Stage: st.Stage, Op: "track.create", Kind: "track",
			TrackType: trackType,
			Message: "scope is archived; all mutations blocked",
		}
	default:
		// Unknown Stage value — treat as deny so a forward-compatible
		// stage doesn't silently downgrade enforcement. Adopters that
		// pin to a kit version with the unknown stage will get a
		// matching arm above.
		return &StageGateError{
			Scope: scope, Stage: st.Stage, Op: "track.create", Kind: "track",
			TrackType: trackType,
			Message: "unrecognized stage; refusing track create defensively",
		}
	}
}

// GateTaskCreate is the analogue for `tlc task create`. trackType is
// the parent track's Type when known (so "feature_freeze" can still
// permit fix/chore/docs tasks); empty trackType represents an
// unparented task and is treated as a feature task for stage rules
// since tlc's predominant task style is feature work.
//
// Policy table:
//
//	active            → allow
//	public_feedback   → allow (tasks aren't gated; only track-type is)
//	feature_freeze    → deny if trackType in {feature, refactor};
//	                    allow fix/chore/docs/feedback
//	maintenance       → deny if trackType in {feature, refactor};
//	                    allow fix/chore/docs
//	sunset            → deny (no new entities)
//	archived          → deny (read-only)
func GateTaskCreate(scope, trackType string) error {
	if scope == "" {
		return nil
	}
	st, err := DefaultStageReader(scope)
	if err != nil {
		return fmt.Errorf("stage gate: read scope %q: %w", scope, err)
	}
	switch st.Stage {
	case "", stage.StageActive, stage.StagePublicFeedback:
		return nil
	case stage.StageFeatureFreeze:
		if !isMaintenanceFriendlyType(trackType) && !isDocsType(trackType) {
			return &StageGateError{
				Scope: scope, Stage: st.Stage, Op: "task.create", Kind: "task",
				TrackType: trackType,
				Message: "only fix/chore/docs tasks allowed during feature_freeze; " +
					"link the task to a fix/chore track or change the scope stage",
			}
		}
		return nil
	case stage.StageMaintenance:
		if !isMaintenanceFriendlyType(trackType) && !isDocsType(trackType) {
			return &StageGateError{
				Scope: scope, Stage: st.Stage, Op: "task.create", Kind: "task",
				TrackType: trackType,
				Message: "only fix/chore/docs tasks allowed during maintenance; " +
					"link the task to a fix/chore track or change the scope stage",
			}
		}
		return nil
	case stage.StageSunset:
		return &StageGateError{
			Scope: scope, Stage: st.Stage, Op: "task.create", Kind: "task",
			TrackType: trackType,
			Message: "scope is in sunset; no new entities allowed (updates/deletes ok)",
		}
	case stage.StageArchived:
		return &StageGateError{
			Scope: scope, Stage: st.Stage, Op: "task.create", Kind: "task",
			TrackType: trackType,
			Message: "scope is archived; all mutations blocked",
		}
	default:
		return &StageGateError{
			Scope: scope, Stage: st.Stage, Op: "task.create", Kind: "task",
			TrackType: trackType,
			Message: "unrecognized stage; refusing task create defensively",
		}
	}
}

// isMaintenanceFriendlyType reports whether the supplied track type is
// permitted during feature_freeze / maintenance. The list mirrors
// kit/runtime/policy/stage.yaml: fix and chore are always ok.
func isMaintenanceFriendlyType(t string) bool {
	switch t {
	case "fix", "chore":
		return true
	}
	return false
}

// isDocsType returns true for documentation work, which kit policy
// permits during maintenance but not feature_freeze. tlc's track-type
// vocabulary doesn't ship "docs" by default, but adopters extend it via
// config.yaml — keep the discriminator separate from the fix/chore arm
// so the per-stage rules stay readable.
func isDocsType(t string) bool {
	return t == "docs"
}
