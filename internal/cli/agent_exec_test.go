package cli

// Exec-path tests: parameters travel explicitly rather than through the
// agentRun* flag bindings, container runs are isolated per call, local
// runs get a per-run context directory, and the track dispatcher
// serializes local mode only.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// fakePodRunner answers the pod CLI calls PodShell makes. Each `create`
// mints a pod whose create args and image are remembered so `agent-run`
// can report which image it ran in. barrier, when > 0, holds every
// agent-run until that many are in flight, so a caller can prove two
// runs overlapped; a run that never reaches the barrier fails on ctx.
type fakePodRunner struct {
	mu       sync.Mutex
	pods     int
	creates  map[string][]string
	images   map[string]string
	destroys []string
	inflight int
	barrier  int
	gate     chan struct{}
}

func newFakePodRunner(barrier int) *fakePodRunner {
	return &fakePodRunner{
		creates: map[string][]string{},
		images:  map[string]string{},
		barrier: barrier,
		gate:    make(chan struct{}),
	}
}

func (f *fakePodRunner) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	if name != "pod" || len(args) == 0 {
		return "", "", 1, fmt.Errorf("unexpected command %s %v", name, args)
	}
	switch args[0] {
	case "--version":
		return "pod 0.0.0-test", "", 0, nil
	case "create":
		return f.create(args[1:])
	case "cp":
		// Downloads (pod:path → local) report no results file; uploads succeed.
		if strings.Contains(args[1], ":") {
			return "", "no such file", 1, nil
		}
		return "", "", 0, nil
	case "exec":
		return f.exec(ctx, args[1:])
	case "destroy":
		f.mu.Lock()
		f.destroys = append(f.destroys, args[1])
		f.mu.Unlock()
		return "", "", 0, nil
	}
	return "", "", 1, fmt.Errorf("unexpected pod args %v", args)
}

func (f *fakePodRunner) create(args []string) (string, string, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pods++
	pod := fmt.Sprintf("pod-%d", f.pods)
	f.creates[pod] = args
	f.images[pod] = argValue(args, "--image")
	out, _ := json.Marshal(map[string]string{"name": pod, "id": "id-" + pod, "status": "running"})
	return string(out), "", 0, nil
}

// exec handles `<pod> -- test -f <path>` (verify) and `<pod> -- agent-run`.
func (f *fakePodRunner) exec(ctx context.Context, args []string) (string, string, int, error) {
	pod := args[0]
	if len(args) >= 3 && args[2] == "test" {
		return "", "", 0, nil
	}
	f.mu.Lock()
	f.inflight++
	if f.barrier > 0 && f.inflight >= f.barrier {
		select {
		case <-f.gate:
		default:
			close(f.gate)
		}
	}
	image := f.images[pod]
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.inflight--
		f.mu.Unlock()
	}()
	if f.barrier > 0 {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return "", "", 1, fmt.Errorf("agent-run in %s never overlapped another run: %w", pod, ctx.Err())
		}
	}
	return fmt.Sprintf(`{"version":1,"status":"succeeded","summary":"ran in %s"}`, image), "", 0, nil
}

func (f *fakePodRunner) destroyed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.destroys...)
}

func argValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// TestExecuteContainer_ReadsParamsNotGlobals poisons every agentRun*
// binding and checks the pod is created from execParams alone.
func TestExecuteContainer_ReadsParamsNotGlobals(t *testing.T) {
	withTestLock(func() {
		defer resetAgentRunFlags()
		agentRunAgent, agentRunNetwork = "global-agent", "global-net"
		agentRunEnv, agentRunMounts = []string{"LEAK=1"}, []string{"/leak:/leak"}
		agentRunKeepPod, agentRunLocal = true, true

		fake := newFakePodRunner(0)
		p := execParams{
			agent:    "alpha",
			mounts:   parseMounts([]string{"/src:/dst:ro"}),
			env:      []string{"KEY=a"},
			network:  "net-a",
			repoRoot: "/workspace",
			runner:   fake,
		}
		cfg := &core.AgentConfig{Image: "img-a", Env: map[string]string{"BASE": "cfg"}}
		record := &core.AgentRunRecord{ID: "run-a", Agent: "alpha"}

		result, err := executeContainer(context.Background(), p, cfg, []byte(`{}`), record, core.NewResultCollector())
		if err != nil {
			t.Fatalf("executeContainer: %v", err)
		}
		if result.Agent != "alpha" || result.Status != core.AgentStatusSucceeded {
			t.Errorf("result = %+v; want agent alpha succeeded", result)
		}
		if record.ContainerID != "id-pod-1" {
			t.Errorf("record.ContainerID = %q; want id-pod-1", record.ContainerID)
		}
		joined := strings.Join(fake.creates["pod-1"], " ")
		for _, want := range []string{
			"--image img-a", "--mount /src:/dst:ro", "--env KEY=a", "--env BASE=cfg", "--network net-a",
			"--label tlc.agent=alpha", "--label tlc.run-id=run-a",
			"--env TLC_CONTEXT_PATH=/workspace/.tlc/context.json",
			"--env TLC_RESULTS_PATH=/workspace/.tlc/results.json",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("pod create lacks %q:\n%s", want, joined)
			}
		}
		for _, leak := range []string{"global", "LEAK", "/leak"} {
			if strings.Contains(joined, leak) {
				t.Errorf("pod create leaked flag binding %q:\n%s", leak, joined)
			}
		}
		if got := fake.destroyed(); len(got) != 1 || got[0] != "pod-1" {
			t.Errorf("destroys = %v; want [pod-1] (keepPod false)", got)
		}

		keep := newFakePodRunner(0)
		p.runner, p.keepPod = keep, true
		if _, err := executeContainer(context.Background(), p, cfg, []byte(`{}`), record, core.NewResultCollector()); err != nil {
			t.Fatalf("executeContainer keepPod: %v", err)
		}
		if got := keep.destroyed(); len(got) != 0 {
			t.Errorf("keepPod=true destroyed %v", got)
		}
	})
}

// TestExecuteLocal_PerRunContextDir runs a script agent twice and checks
// each run got its own context/results paths (announced via env), that
// results were collected from the per-run file, and that nothing was
// written to the shared .tlc/context.json.
func TestExecuteLocal_PerRunContextDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script agent fixture is POSIX-only")
	}
	withTestLock(func() {
		defer resetAgentRunFlags()
		agentRunAgent, agentRunEnv = "global-agent", []string{"OUT=/nonexistent/leak"}

		repo := t.TempDir()
		out := filepath.Join(repo, "out")
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		script := filepath.Join(repo, "agent.sh")
		body := "#!/bin/sh\n" +
			"printf '%s\\n' \"$TLC_CONTEXT_PATH\" > \"$OUT/ctx-path\"\n" +
			"printf '%s\\n' \"$TLC_RESULTS_PATH\" > \"$OUT/res-path\"\n" +
			"cp \"$TLC_CONTEXT_PATH\" \"$OUT/context.json\"\n" +
			"run=$(basename \"$(dirname \"$TLC_RESULTS_PATH\")\")\n" +
			"printf '{\"version\":1,\"status\":\"succeeded\",\"summary\":\"file\",\"outputs\":{\"run\":\"%s\"}}' \"$run\" > \"$TLC_RESULTS_PATH\"\n" +
			"echo '{\"version\":1,\"status\":\"succeeded\",\"summary\":\"from stdout\"}'\n"
		if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}

		p := execParams{
			agent:    "alpha",
			local:    true,
			env:      []string{"OUT=" + out},
			repoRoot: repo,
			runsDir:  filepath.Join(repo, ".tlc", "runs"),
		}
		cfg := &core.AgentConfig{Binary: script}

		for _, runID := range []string{"run-1", "run-2"} {
			record := &core.AgentRunRecord{ID: runID, Agent: "alpha"}
			contextJSON := []byte(`{"version":1,"task_id":"` + runID + `"}`)
			result, err := executeLocal(context.Background(), p, cfg, contextJSON, record, core.NewResultCollector())
			if err != nil {
				t.Fatalf("%s: executeLocal: %v", runID, err)
			}
			runDir := filepath.Join(p.runsDir, runID)
			if got := readTrim(t, filepath.Join(out, "ctx-path")); got != filepath.Join(runDir, "context.json") {
				t.Errorf("%s: TLC_CONTEXT_PATH = %q; want per-run path under %s", runID, got, runDir)
			}
			if got := readTrim(t, filepath.Join(out, "res-path")); got != filepath.Join(runDir, "results.json") {
				t.Errorf("%s: TLC_RESULTS_PATH = %q; want per-run path under %s", runID, got, runDir)
			}
			if got := readTrim(t, filepath.Join(out, "context.json")); got != string(contextJSON) {
				t.Errorf("%s: agent read context %q; want %q", runID, got, contextJSON)
			}
			if result.Agent != "alpha" || result.Summary != "from stdout" || result.Outputs["run"] != runID {
				t.Errorf("%s: result = %+v; want agent alpha, stdout summary, outputs.run=%s", runID, result, runID)
			}
			if _, err := os.Stat(runDir); !os.IsNotExist(err) {
				t.Errorf("%s: run dir %s not removed after a collected run (stat err %v)", runID, runDir, err)
			}
		}
		if _, err := os.Stat(filepath.Join(repo, ".tlc", "context.json")); !os.IsNotExist(err) {
			t.Errorf("shared .tlc/context.json was written (stat err %v)", err)
		}
	})
}

func readTrim(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(data))
}

func seedAgentTask(t *testing.T, ctx context.Context, s *storage.SQLiteStorage, id string, seq int64, agent string) *core.Task {
	t.Helper()
	now := time.Now().UTC()
	task := &core.Task{
		ID: id, Seq: seq, Title: "agent " + id, Status: core.StatusTodo,
		Kind: core.TaskKindAgent, Spec: &core.TaskSpec{Agent: agent},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask %s: %v", id, err)
	}
	return task
}

func newTestDispatcher(t *testing.T, s *storage.SQLiteStorage, p execParams) *agentDispatcher {
	t.Helper()
	registry := core.NewAgentRegistry()
	if err := registry.LoadDefaults(); err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	return &agentDispatcher{
		s:        s,
		registry: registry,
		params:   p,
		updater:  core.NewStateUpdater(core.NewTaskService(s, s), GetEventBus()),
	}
}

// TestAgentDispatcher_ContainerDispatchesOverlap runs two agent tasks
// with different agents through one dispatcher at once. The fake pod
// holds each agent-run until both are in flight, so a dispatcher that
// still queues container runs times out; each result must name its own
// agent's image, so params cannot cross between the two runs.
func TestAgentDispatcher_ContainerDispatchesOverlap(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, "agents:\n  alpha:\n    image: img-alpha\n  beta:\n    image: img-beta\n")

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		tasks := []*core.Task{
			seedAgentTask(t, ctx, s, "task_alpha", 1, "alpha"),
			seedAgentTask(t, ctx, s, "task_beta", 2, "beta"),
		}

		fake := newFakePodRunner(2)
		d := newTestDispatcher(t, s, execParams{repoRoot: "/workspace", runner: fake})

		runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		type outcome struct {
			res *core.DispatchResult
			err error
		}
		results := map[string]outcome{}
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, task := range tasks {
			wg.Add(1)
			go func(task *core.Task) {
				defer wg.Done()
				res, err := d.Dispatch(runCtx, task)
				mu.Lock()
				results[task.ID] = outcome{res, err}
				mu.Unlock()
			}(task)
		}
		wg.Wait()

		for id, image := range map[string]string{"task_alpha": "img-alpha", "task_beta": "img-beta"} {
			o := results[id]
			if o.err != nil {
				t.Fatalf("%s: %v (container dispatches must overlap, not queue)", id, o.err)
			}
			if o.res.Status != core.AgentStatusSucceeded || o.res.Summary != "ran in "+image {
				t.Errorf("%s: result = %+v; want succeeded in %s (params crossed between dispatches)", id, o.res, image)
			}
		}
		if got := fake.destroyed(); len(got) != 2 {
			t.Errorf("destroys = %v; want both pods torn down", got)
		}
	})
}

// TestAgentDispatcher_LocalGuard proves the guard is held for the whole
// of a local dispatch and never for a container one.
func TestAgentDispatcher_LocalGuard(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantAgentsYAML(t, "agents:\n  alpha:\n    image: img-alpha\n    binary: /bin/true\n")

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		task := seedAgentTask(t, ctx, s, "task_alpha", 1, "alpha")

		for _, local := range []bool{true, false} {
			d := newTestDispatcher(t, s, execParams{local: local, repoRoot: repoRootForMode(local)})
			started, release := make(chan struct{}), make(chan struct{})
			var seen execParams
			d.exec = func(_ context.Context, p execParams, _ string, _ *core.AgentContext, _ *core.AgentConfig, _ *core.StateUpdater) (*core.AgentResult, error) {
				seen = p
				close(started)
				<-release
				return &core.AgentResult{Version: core.AgentResultVersion, Status: core.AgentStatusSucceeded, Summary: "ok"}, nil
			}
			done := make(chan error, 1)
			go func() {
				_, err := d.Dispatch(ctx, task)
				done <- err
			}()
			<-started

			free := d.localMu.TryLock()
			if free {
				d.localMu.Unlock()
			}
			if local && free {
				t.Errorf("local dispatch ran without holding the local guard")
			}
			if !local && !free {
				t.Errorf("container dispatch held the local guard")
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatalf("local=%v: Dispatch: %v", local, err)
			}
			if seen.agent != "alpha" || seen.local != local {
				t.Errorf("local=%v: exec saw params %+v; want agent alpha in the same mode", local, seen)
			}
		}
	})
}
