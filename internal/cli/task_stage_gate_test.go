package cli

import (
	"bytes"
	"strings"
	"testing"

	"hop.top/kit/go/core/stage"
	"hop.top/tlc/internal/core"
)

// TestTaskCreate_StageGateBlocksArchived confirms task creation is
// refused on an archived scope, matching the "scope is read-only"
// arm of the policy table.
func TestTaskCreate_StageGateBlocksArchived(t *testing.T) {
	withTestLock(func() {
		_, _ = setupProjectScopedTestDir(t, "tlc-stage-task-", "hop-top/tlc")

		prev := core.DefaultStageReader
		core.DefaultStageReader = func(_ string) (stage.State, error) {
			return stage.State{Stage: stage.StageArchived}, nil
		}
		t.Cleanup(func() { core.DefaultStageReader = prev })

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "should-fail"})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected gate error on archived scope; got success:\n%s", buf.String())
		}
		if !strings.Contains(err.Error(), "archived") {
			t.Errorf("error should mention archived, got %q", err)
		}
	})
}

// TestTaskCreate_StageGateFeatureFreezeAllowsFix verifies that a fix-
// typed task linked to a fix track passes during feature_freeze, while
// an unparented feature task is blocked.
func TestTaskCreate_StageGateFeatureFreezeAllowsFix(t *testing.T) {
	withTestLock(func() {
		_, _ = setupProjectScopedTestDir(t, "tlc-stage-task-ff-", "hop-top/tlc")

		// Pre-create a fix track BEFORE flipping the gate so the parent
		// exists for the task.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		fixTrack := &core.Track{
			Slug:   "fix-track",
			Title:  "Fix track",
			Type:   "fix",
			Status: core.TrackStatusActive,
		}
		if err := core.NewTrackService(s, s).CreateTrack(t.Context(), fixTrack); err != nil {
			t.Fatalf("seed CreateTrack: %v", err)
		}
		_ = s.Close()

		prev := core.DefaultStageReader
		core.DefaultStageReader = func(_ string) (stage.State, error) {
			return stage.State{Stage: stage.StageFeatureFreeze}, nil
		}
		t.Cleanup(func() { core.DefaultStageReader = prev })

		// Fix-track-linked task: should pass.
		taskTrack = "fix-track"
		t.Cleanup(func() { taskTrack = "" })

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "create", "patch the bug", "--track", "fix-track",
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("feature_freeze should allow fix-track task, got %v", err)
		}
	})
}
