// Tests for the --with-pod path of `tlc track execute`: exec-kind tasks
// run inside a pod driven through a scripted pod CLI, never on the host.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

const podCreatedReply = `{"name":"pod-exec","id":"c0ffee","status":"running"}`

type podReply struct {
	stdout, stderr string
	code           int
}

// scriptedPodRunner is a core.CommandRunner that records every pod CLI
// call and answers each with the next canned reply.
type scriptedPodRunner struct {
	calls   [][]string
	replies []podReply
}

func (r *scriptedPodRunner) Run(_ context.Context, name string, args ...string) (string, string, int, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(r.replies) == 0 {
		return "", "", 1, fmt.Errorf("unexpected pod call: %s %v", name, args)
	}
	reply := r.replies[0]
	r.replies = r.replies[1:]
	return reply.stdout, reply.stderr, reply.code, nil
}

// podRunReplies is one successful create, cp, exec, destroy conversation.
func podRunReplies(stdout string) []podReply {
	return []podReply{{stdout: podCreatedReply}, {}, {stdout: stdout}, {}}
}

// withScriptedPod swaps the exec pod shell factory for one over r and
// restores it on cleanup.
func withScriptedPod(t *testing.T, r *scriptedPodRunner) {
	t.Helper()
	prev := newExecPodShell
	newExecPodShell = func() *core.PodShell { return core.NewPodShell(r) }
	t.Cleanup(func() { newExecPodShell = prev })
}

// setWithPod passes --with-pod the way the command line does, so the
// flag counts as explicitly set.
func setWithPod(t *testing.T, value string) {
	t.Helper()
	if err := trackExecuteCmd.Flags().Set("with-pod", value); err != nil {
		t.Fatalf("set --with-pod=%s: %v", value, err)
	}
}

func TestTrackExecute_WithPodImageRunsExecTaskInPod(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		defer resetTrackExecuteFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		seedExecTrack(t, ctx, s, execTestTask("task_a", 1))

		r := &scriptedPodRunner{replies: podRunReplies("from pod\n")}
		withScriptedPod(t, r)
		resetTrackExecuteFlags()
		setWithPod(t, "ghcr.io/example/tools:1")

		out, err := execTrackCmd(t, execTestTrack)
		if err != nil {
			t.Fatalf("track execute --with-pod: %v\n%s", err, out)
		}
		got := mustGetTask(t, ctx, s, "task_a")
		if got.Status != core.StatusDone {
			t.Errorf("task_a = %q; want DONE\n%s", got.Status, out)
		}
		if got.Result == nil || got.Result["stdout"] != "from pod\n" || got.Result["exit_code"] != float64(0) {
			t.Errorf("task_a result = %v; want the pod's stdout and exit 0", got.Result)
		}
		if len(r.calls) != 4 {
			t.Fatalf("pod calls = %v; want create, cp, exec, destroy", r.calls)
		}
		create := strings.Join(r.calls[0], " ")
		if !strings.HasPrefix(create, "pod create") || !strings.Contains(create, "--image ghcr.io/example/tools:1") {
			t.Errorf("create call = %q; want the --with-pod image", create)
		}
		cwd, _ := os.Getwd()
		if cp := strings.Join(r.calls[1], " "); cp != "pod cp "+cwd+" pod-exec:/workspace" {
			t.Errorf("cp call = %q; want the working tree copied to /workspace", cp)
		}
		exec := strings.Join(r.calls[2], " ")
		if !strings.HasPrefix(exec, "pod exec pod-exec -- sh -c ") || !strings.HasSuffix(exec, " /workspace sh -c echo task_a") {
			t.Errorf("exec call = %q; want the literal argv behind the /workspace prologue", exec)
		}
		if destroy := strings.Join(r.calls[3], " "); destroy != "pod destroy pod-exec --yes" {
			t.Errorf("destroy call = %q", destroy)
		}
	})
}

// TestTrackExecute_WithPodTrueUsesAgentImage: a bare --with-pod borrows
// the --agent's image, so exec steps run where the agent steps do.
func TestTrackExecute_WithPodTrueUsesAgentImage(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, "agents:\n  claude:\n    image: ghcr.io/example/claude:latest\n")
		defer resetTrackExecuteFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		seedExecTrack(t, ctx, s, execTestTask("task_a", 1))

		r := &scriptedPodRunner{replies: podRunReplies("ok\n")}
		withScriptedPod(t, r)
		resetTrackExecuteFlags()
		trackExecuteAgent = "claude"
		setWithPod(t, "true")

		out, err := execTrackCmd(t, execTestTrack)
		if err != nil {
			t.Fatalf("track execute --with-pod --agent claude: %v\n%s", err, out)
		}
		if got := mustGetTask(t, ctx, s, "task_a"); got.Status != core.StatusDone {
			t.Errorf("task_a = %q; want DONE\n%s", got.Status, out)
		}
		if len(r.calls) == 0 || !strings.Contains(strings.Join(r.calls[0], " "), "--image ghcr.io/example/claude:latest") {
			t.Errorf("pod calls = %v; want create with the agent's image", r.calls)
		}
	})
}

func TestTrackExecute_WithPodTrueWithoutImageFails(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		defer resetTrackExecuteFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		seedExecTrack(t, ctx, s, execTestTask("task_a", 1))

		r := &scriptedPodRunner{}
		withScriptedPod(t, r)
		resetTrackExecuteFlags()
		setWithPod(t, "true")

		_, err = execTrackCmd(t, execTestTrack)
		if err == nil || !strings.Contains(err.Error(), "--with-pod=<image>") {
			t.Fatalf("err = %v; want the image hint", err)
		}
		if len(r.calls) != 0 {
			t.Errorf("pod calls = %v; want none", r.calls)
		}
		if got := mustGetTask(t, ctx, s, "task_a"); got.Status != core.StatusTodo {
			t.Errorf("task_a = %q; want untouched TODO", got.Status)
		}
	})
}

// TestTrackExecute_WithPodDefaultAndFalseStayOnHost: the flag's default
// governs agents only; exec tasks leave the host only when it is passed.
func TestTrackExecute_WithPodDefaultAndFalseStayOnHost(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		defer resetTrackExecuteFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		seedExecTrack(t, ctx, s)

		r := &scriptedPodRunner{}
		withScriptedPod(t, r)
		for i, value := range []string{"", "false"} {
			id := fmt.Sprintf("task_host_%d", i)
			if err := s.CreateTask(ctx, execTestTask(id, int64(i+1))); err != nil {
				t.Fatalf("CreateTask %s: %v", id, err)
			}
			resetTrackExecuteFlags()
			if value != "" {
				setWithPod(t, value)
			}
			out, err := execTrackCmd(t, execTestTrack)
			if err != nil {
				t.Fatalf("--with-pod=%q: %v\n%s", value, err, out)
			}
			got := mustGetTask(t, ctx, s, id)
			if got.Status != core.StatusDone || got.Result == nil || !strings.Contains(got.Result["stdout"].(string), id) {
				t.Errorf("--with-pod=%q: %s = %q result %v; want DONE with the host's echo", value, id, got.Status, got.Result)
			}
			if len(r.calls) != 0 {
				t.Errorf("--with-pod=%q touched the pod CLI: %v", value, r.calls)
			}
		}
	})
}
