package core

import (
	"errors"
	"strings"
	"testing"

	"hop.top/kit/go/core/stage"
)

// withStubReader swaps DefaultStageReader for the duration of a test.
func withStubReader(t *testing.T, fn stageReader) {
	t.Helper()
	prev := DefaultStageReader
	DefaultStageReader = fn
	t.Cleanup(func() { DefaultStageReader = prev })
}

func stubStage(s stage.Stage) stageReader {
	return func(_ string) (stage.State, error) {
		return stage.State{Stage: s}, nil
	}
}

// TestGateTrackCreate_AllowsActive verifies the active stage (and the
// empty/missing zero-value State) lets every track type through.
func TestGateTrackCreate_AllowsActive(t *testing.T) {
	withStubReader(t, stubStage(stage.StageActive))

	for _, tt := range []string{"feature", "fix", "chore", "feedback"} {
		if err := GateTrackCreate("hop-top/tlc", tt); err != nil {
			t.Errorf("active stage blocked %q: %v", tt, err)
		}
	}
}

// TestGateTrackCreate_EmptyScopeAllows confirms an empty scope (no
// project context) bypasses the gate so adopters running tlc outside
// any registered project see no change.
func TestGateTrackCreate_EmptyScopeAllows(t *testing.T) {
	called := false
	withStubReader(t, func(_ string) (stage.State, error) {
		called = true
		return stage.State{Stage: stage.StageArchived}, nil
	})

	if err := GateTrackCreate("", "feature"); err != nil {
		t.Errorf("empty scope should bypass gate, got %v", err)
	}
	if called {
		t.Error("stage reader should not be called for empty scope")
	}
}

// TestGateTrackCreate_MissingStageTreatedAsActive confirms zero-value
// State (Stage == "") behaves like StageActive.
func TestGateTrackCreate_MissingStageTreatedAsActive(t *testing.T) {
	withStubReader(t, func(_ string) (stage.State, error) {
		return stage.State{}, nil
	})

	if err := GateTrackCreate("hop-top/tlc", "feature"); err != nil {
		t.Errorf("zero-State should allow create, got %v", err)
	}
}

// TestGateTrackCreate_PublicFeedbackOnlyFeedbackTracks verifies the
// public_feedback policy permits only `feedback` tracks.
func TestGateTrackCreate_PublicFeedbackOnlyFeedbackTracks(t *testing.T) {
	withStubReader(t, stubStage(stage.StagePublicFeedback))

	if err := GateTrackCreate("scope", "feedback"); err != nil {
		t.Errorf("public_feedback should allow feedback, got %v", err)
	}

	for _, tt := range []string{"feature", "fix", "chore", "refactor"} {
		err := GateTrackCreate("scope", tt)
		if err == nil {
			t.Errorf("public_feedback should block %q", tt)
			continue
		}
		var sge *StageGateError
		if !errors.As(err, &sge) {
			t.Errorf("expected StageGateError for %q, got %T", tt, err)
		}
	}
}

// TestGateTrackCreate_DenyStages verifies feature_freeze, maintenance,
// sunset, and archived all reject every track type.
func TestGateTrackCreate_DenyStages(t *testing.T) {
	denyCases := []stage.Stage{
		stage.StageFeatureFreeze,
		stage.StageMaintenance,
		stage.StageSunset,
		stage.StageArchived,
	}
	for _, st := range denyCases {
		t.Run(string(st), func(t *testing.T) {
			withStubReader(t, stubStage(st))
			for _, tt := range []string{"feature", "fix", "chore"} {
				if err := GateTrackCreate("scope", tt); err == nil {
					t.Errorf("%s should block %q", st, tt)
				}
			}
		})
	}
}

// TestGateTaskCreate_FeatureFreezeAllowsFixChore exercises the
// feature_freeze rule that permits fix/chore tasks.
func TestGateTaskCreate_FeatureFreezeAllowsFixChore(t *testing.T) {
	withStubReader(t, stubStage(stage.StageFeatureFreeze))

	for _, tt := range []string{"fix", "chore", "docs"} {
		if err := GateTaskCreate("scope", tt); err != nil {
			t.Errorf("feature_freeze should allow task on %s track, got %v", tt, err)
		}
	}

	for _, tt := range []string{"feature", "refactor", ""} {
		err := GateTaskCreate("scope", tt)
		if err == nil {
			t.Errorf("feature_freeze should block task on %q track", tt)
		}
	}
}

// TestGateTaskCreate_PublicFeedbackAllowsAll confirms public_feedback
// gates only track creation, not tasks.
func TestGateTaskCreate_PublicFeedbackAllowsAll(t *testing.T) {
	withStubReader(t, stubStage(stage.StagePublicFeedback))

	for _, tt := range []string{"feature", "fix", "feedback"} {
		if err := GateTaskCreate("scope", tt); err != nil {
			t.Errorf("public_feedback should allow task creation, got %v on %q", err, tt)
		}
	}
}

// TestGateTaskCreate_ArchivedRejects ensures archived blocks tasks
// regardless of trackType.
func TestGateTaskCreate_ArchivedRejects(t *testing.T) {
	withStubReader(t, stubStage(stage.StageArchived))

	for _, tt := range []string{"fix", "chore", "feature", "docs"} {
		err := GateTaskCreate("scope", tt)
		if err == nil {
			t.Errorf("archived should block task on %q", tt)
			continue
		}
		var sge *StageGateError
		if !errors.As(err, &sge) {
			t.Errorf("expected StageGateError, got %T", err)
		}
		if !IsStageGateError(err) {
			t.Error("IsStageGateError should match")
		}
	}
}

// TestStageGateError_Message verifies the error message includes the
// scope, stage, and op so cli renderings stay actionable per AGENTS.md.
func TestStageGateError_Message(t *testing.T) {
	err := &StageGateError{
		Scope: "hop-top/tlc",
		Stage: stage.StageMaintenance,
		Op:    "track.create",
		Kind:  "track",
		Message: "no new tracks during maintenance (fix/chore/docs tasks only); " +
			"change the scope stage via 'tlc stage set' to unblock",
	}
	want := `track.create blocked: scope "hop-top/tlc" is in stage "maintenance"`
	if got := err.Error(); !strings.Contains(got, want) {
		t.Errorf("error message = %q\nwant prefix %q", got, want)
	}
	if !strings.Contains(err.Error(), "tlc stage set") {
		t.Error("message should suggest the next-step command")
	}
}

// TestGateTrackCreate_ReadError surfaces the underlying error wrapped
// with the scope so adopters can debug a misconfigured projects.yaml.
func TestGateTrackCreate_ReadError(t *testing.T) {
	withStubReader(t, func(_ string) (stage.State, error) {
		return stage.State{}, errors.New("yaml: malformed")
	})

	err := GateTrackCreate("scope", "feature")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "scope") || !strings.Contains(err.Error(), "yaml: malformed") {
		t.Errorf("error should include scope + cause, got %q", err)
	}
}

