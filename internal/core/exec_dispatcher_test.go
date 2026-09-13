package core

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// recordingArgvRunner captures the opts the dispatcher hands over and
// answers with a canned result.
type recordingArgvRunner struct {
	opts ArgvOpts
	res  *ArgvResult
	err  error
}

func (r *recordingArgvRunner) Run(_ context.Context, opts ArgvOpts) (*ArgvResult, error) {
	r.opts = opts
	return r.res, r.err
}

func execKindTask(spec *ExecSpec) *Task {
	return &Task{ID: "task_x", Kind: TaskKindExec, Spec: &TaskSpec{Exec: spec}}
}

// TestExecDispatcher_PassesSpecToRunner: every exec.* field reaches the
// runner unchanged, with the dispatcher's DefaultCwd alongside.
func TestExecDispatcher_PassesSpecToRunner(t *testing.T) {
	r := &recordingArgvRunner{res: &ArgvResult{Stdout: "ok", DurationMs: 3}}
	d := &ExecDispatcher{DefaultCwd: "/repo", Runner: r}
	res, err := d.Dispatch(context.Background(), execKindTask(&ExecSpec{
		Argv: []string{"make", "test"}, Cwd: "sub", Env: map[string]string{"CI": "1"}, Timeout: "5m", StdoutMax: 512,
	}))
	if err != nil {
		t.Fatalf("Dispatch err = %v", err)
	}
	want := ArgvOpts{
		Argv: []string{"make", "test"}, Cwd: "sub", DefaultCwd: "/repo",
		Env: map[string]string{"CI": "1"}, Timeout: 5 * time.Minute, StdoutMax: 512,
	}
	if !reflect.DeepEqual(r.opts, want) {
		t.Errorf("runner opts = %+v; want %+v", r.opts, want)
	}
	if res.Status != AgentStatusSucceeded || res.Result["stdout"] != "ok" || !strings.Contains(res.Summary, "exit 0") {
		t.Errorf("result = %+v; want succeeded with the runner's output", res)
	}
}

func TestExecDispatcher_MapsOutcomes(t *testing.T) {
	cases := []struct {
		name        string
		res         *ArgvResult
		err         error
		wantStatus  AgentResultStatus
		wantSummary string
		wantErr     string
	}{
		{"nonzero exit fails", &ArgvResult{ExitCode: 2, Stderr: "boom\nmore"}, nil, AgentStatusFailed, "exit 2: boom", ""},
		{
			"timeout keeps partial output", &ArgvResult{Stdout: "partial", TimedOut: true},
			fmt.Errorf("%w after 1s", ErrArgvTimeout), AgentStatusTimeout, "timed out", "",
		},
		{"runner error names the task", nil, errors.New("pod create failed"), "", "", "task_x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &ExecDispatcher{Runner: &recordingArgvRunner{res: tc.res, err: tc.err}}
			res, err := d.Dispatch(context.Background(), execKindTask(&ExecSpec{Argv: []string{"x"}}))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "pod create failed") {
					t.Fatalf("err = %v; want one naming %q and the cause", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Dispatch err = %v", err)
			}
			if res.Status != tc.wantStatus || !strings.Contains(res.Summary, tc.wantSummary) {
				t.Errorf("result = %+v; want %s / %q", res, tc.wantStatus, tc.wantSummary)
			}
			if res.Result["stdout"] != tc.res.Stdout {
				t.Errorf("stored stdout = %v; want %q", res.Result["stdout"], tc.res.Stdout)
			}
		})
	}
}

// TestExecDispatcher_DefaultsToHost: a nil Runner is the host path.
func TestExecDispatcher_DefaultsToHost(t *testing.T) {
	res, err := NewExecDispatcher("").Dispatch(context.Background(), execKindTask(&ExecSpec{
		Argv: []string{"sh", "-c", "echo host"},
	}))
	if err != nil {
		t.Fatalf("Dispatch err = %v", err)
	}
	if res.Status != AgentStatusSucceeded || res.Result["stdout"] != "host\n" {
		t.Errorf("result = %+v; want the host's echo", res)
	}
}

func TestExecDispatcher_RejectsMissingArgv(t *testing.T) {
	d := &ExecDispatcher{Runner: &recordingArgvRunner{}}
	for _, task := range []*Task{
		{ID: "task_x", Kind: TaskKindExec},
		execKindTask(&ExecSpec{}),
	} {
		if _, err := d.Dispatch(context.Background(), task); err == nil || !strings.Contains(err.Error(), "no exec argv") {
			t.Errorf("err = %v; want the missing argv error", err)
		}
	}
}

func TestExecDispatcher_BadTimeoutIsAnError(t *testing.T) {
	d := &ExecDispatcher{Runner: &recordingArgvRunner{}}
	_, err := d.Dispatch(context.Background(), execKindTask(&ExecSpec{Argv: []string{"x"}, Timeout: "soon"}))
	if err == nil || !strings.Contains(err.Error(), "exec timeout") {
		t.Errorf("err = %v; want the timeout parse error", err)
	}
}

func TestHostArgvRunner_RunsOnHost(t *testing.T) {
	var runner ArgvRunner = HostArgvRunner{}
	res, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"sh", "-c", "exit 3"}})
	if err != nil || res.ExitCode != 3 {
		t.Errorf("res = %+v, err = %v; want exit 3", res, err)
	}
}
