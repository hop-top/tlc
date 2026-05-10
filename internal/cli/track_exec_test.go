// Tests for `tlc track exec`. Sandboxed end-to-end: HOME and storage
// both temp, agents.yaml planted via plantAgentsYAML.
package cli

import (
	"bytes"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestTrackExec_CtxtFlagReachesContext is a regression for the
// previously-broken path: `tlc track exec --ctxt …` would silently
// drop the refs because the shared runner rebuilt context from
// task-exec globals. Now `BuildForTrack` is called with
// `trackExecCtxtRefs` and the prebuilt context is threaded into
// taskExecForTrack. The dry-run output surfaces the refs so we can
// pin the wiring without running an actual agent.
func TestTrackExec_CtxtFlagReachesContext(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, `agents:
  claude:
    image: ghcr.io/example/claude:latest
`)
		defer resetTrackExecFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		trackID := "track_test"
		track := &core.Track{
			ID:     trackID,
			Slug:   "test-track",
			Title:  "ctxt regression",
			Status: core.TrackStatusActive,
		}
		if err := s.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}

		task := &core.Task{
			ID:      "T-0010",
			Seq:     10,
			Title:   "task in track",
			Status:  core.StatusTodo,
			TrackID: &trackID,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		resetTrackExecFlags()
		trackExecAgent = "claude"
		trackExecDryRun = true
		trackExecCtxtRefs = []string{"engineering?tag=runtime"}

		cmd := trackExecCmd
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{trackID})

		if err := cmd.RunE(cmd, []string{trackID}); err != nil {
			t.Fatalf("track exec: %v\n%s", err, buf.String())
		}

		out := buf.String()
		if !strings.Contains(out, "engineering?tag=runtime") {
			t.Errorf("dry-run did not surface --ctxt refs:\n%s", out)
		}
	})
}
