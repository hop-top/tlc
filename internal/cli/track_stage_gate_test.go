package cli

import (
	"bytes"
	"strings"
	"testing"

	"hop.top/kit/go/core/stage"
	"hop.top/tlc/internal/core"
)

// TestTrackCreate_StageGateBlocksFeatureFreeze proves the cli layer
// short-circuits a feature_freeze scope before any storage write hits.
// Swaps core.DefaultStageReader so the test stays hermetic w.r.t. the
// real projects.yaml on disk.
func TestTrackCreate_StageGateBlocksFeatureFreeze(t *testing.T) {
	withTestLock(func() {
		_, _ = setupProjectScopedTestDir(t, "tlc-stage-track-", "hop-top/tlc")

		prev := core.DefaultStageReader
		core.DefaultStageReader = func(_ string) (stage.State, error) {
			return stage.State{Stage: stage.StageFeatureFreeze}, nil
		}
		t.Cleanup(func() { core.DefaultStageReader = prev })

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "Stage gated", "--type", "feature",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected stage gate error; got success:\n%s", buf.String())
		}
		if !strings.Contains(err.Error(), "feature_freeze") {
			t.Errorf("error should mention feature_freeze, got %q", err)
		}
		if !core.IsStageGateError(err) {
			t.Errorf("error should be StageGateError, got %T", err)
		}

		// Verify the track was NOT persisted.
		s, _ := getStorageRaw()
		defer s.Close()
		tr, _ := s.GetTrack(t.Context(), "stage-gated")
		if tr != nil {
			t.Errorf("track should not have been created; got %+v", tr)
		}
	})
}

// TestTrackCreate_StageGateAllowsActive confirms the gate is a no-op
// in the default active stage so existing behaviour is preserved.
func TestTrackCreate_StageGateAllowsActive(t *testing.T) {
	withTestLock(func() {
		_, _ = setupProjectScopedTestDir(t, "tlc-stage-track-", "hop-top/tlc")

		prev := core.DefaultStageReader
		core.DefaultStageReader = func(_ string) (stage.State, error) {
			return stage.State{Stage: stage.StageActive}, nil
		}
		t.Cleanup(func() { core.DefaultStageReader = prev })

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "Active stage", "--type", "feature",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("active stage should allow create, got %v", err)
		}
		if !strings.Contains(buf.String(), "active-stage") {
			t.Errorf("expected slug in output, got:\n%s", buf.String())
		}
	})
}

// TestTrackCreate_StageGatePublicFeedbackAllowsFeedbackOnly proves the
// public_feedback rule from the policy table — feedback tracks pass,
// feature tracks bounce.
func TestTrackCreate_StageGatePublicFeedbackAllowsFeedbackOnly(t *testing.T) {
	withTestLock(func() {
		_, _ = setupProjectScopedTestDir(t, "tlc-stage-pf-", "hop-top/tlc")

		prev := core.DefaultStageReader
		core.DefaultStageReader = func(_ string) (stage.State, error) {
			return stage.State{Stage: stage.StagePublicFeedback}, nil
		}
		t.Cleanup(func() { core.DefaultStageReader = prev })

		// Feature track is rejected.
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs([]string{"track", "create", "Should fail", "--type", "feature"})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("public_feedback should block --type feature")
		}
	})
}
