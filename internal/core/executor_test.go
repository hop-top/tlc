package core

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	execTrack   = "track_1"
	execProject = "proj-a"
	execActor   = "executor-1"
)

var execNow = time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)

// fakeRunStore is an in-memory RecipeRunStore: enough for results
// lookups and subject completion.
type fakeRunStore struct {
	runs  map[string]*RecipeRun
	tasks map[string][]RecipeRunTask
}

func newFakeRunStore() *fakeRunStore {
	return &fakeRunStore{runs: map[string]*RecipeRun{}, tasks: map[string][]RecipeRunTask{}}
}

func (f *fakeRunStore) CreateRecipeRun(_ context.Context, run *RecipeRun) error {
	f.runs[run.ID] = run
	return nil
}

func (f *fakeRunStore) GetRecipeRun(_ context.Context, id string) (*RecipeRun, error) {
	return f.runs[id], nil
}

func (f *fakeRunStore) ListRecipeRuns(context.Context, RecipeRunQuery) ([]*RecipeRun, error) {
	return nil, nil
}
func (f *fakeRunStore) DeleteRecipeRun(context.Context, string) error { return nil }

func (f *fakeRunStore) AddRecipeRunTasks(_ context.Context, runID string, tasks []RecipeRunTask) error {
	for _, t := range tasks {
		t.RunID = runID
		f.tasks[runID] = append(f.tasks[runID], t)
	}
	return nil
}

func (f *fakeRunStore) ListRecipeRunTasks(_ context.Context, runID string) ([]RecipeRunTask, error) {
	return f.tasks[runID], nil
}

func (f *fakeRunStore) ListRecipeRunTasksByTrack(context.Context, string) ([]RecipeRunTask, error) {
	return nil, nil
}

// fakeDispatcher records dispatch order and peak concurrency; fn decides
// the outcome per task.
type fakeDispatcher struct {
	mu          sync.Mutex
	calls       []string
	inflight    int
	maxInflight int
	fn          func(t *Task) (*DispatchResult, error)
}

func (d *fakeDispatcher) Dispatch(_ context.Context, t *Task) (*DispatchResult, error) {
	d.mu.Lock()
	d.calls = append(d.calls, t.ID)
	d.inflight++
	if d.inflight > d.maxInflight {
		d.maxInflight = d.inflight
	}
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.inflight--
		d.mu.Unlock()
	}()
	if d.fn != nil {
		return d.fn(t)
	}
	return &DispatchResult{Status: AgentStatusSucceeded, Summary: "ok", Result: map[string]any{"exit_code": float64(0)}}, nil
}

func (d *fakeDispatcher) callsFor(id string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, c := range d.calls {
		if c == id {
			n++
		}
	}
	return n
}

type execFixture struct {
	repo  *MockRepository
	runs  *fakeRunStore
	disp  *fakeDispatcher
	exec  *Executor
	slept []time.Duration
}

func newExecFixture(t *testing.T, opts ExecutorOpts) *execFixture {
	t.Helper()
	f := &execFixture{repo: NewMockRepository(), runs: newFakeRunStore(), disp: &fakeDispatcher{}}
	if opts.Actor == "" {
		opts.Actor = execActor
	}
	opts.Now = func() time.Time { return execNow }
	opts.Sleep = func(_ context.Context, d time.Duration) error {
		f.slept = append(f.slept, d)
		return nil
	}
	opts.Out = io.Discard
	f.exec = NewExecutor(f.repo, f.repo, f.runs, DefaultWorkflow(), opts)
	f.exec.Register(TaskKindAgent, f.disp)
	return f
}

// task builds a recipe-born TODO task on the fixture track.
func (f *execFixture) task(id string, ordinal int, blockedBy ...string) *Task {
	track, project := execTrack, execProject
	t := &Task{
		ID: id, Seq: int64(ordinal), Title: id, Status: StatusTodo,
		TrackID: &track, ProjectID: &project,
		RunID: "run_1", StepID: id, StepOrdinal: ordinal,
		CreatedAt: execNow.Add(-time.Hour), UpdatedAt: execNow.Add(-time.Hour),
	}
	t.SetBlockedBy(blockedBy)
	return t
}

func (f *execFixture) add(t *testing.T, tasks ...*Task) {
	t.Helper()
	for _, task := range tasks {
		if err := f.repo.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}
}

func (f *execFixture) run(t *testing.T) *ExecReport {
	t.Helper()
	report, err := f.exec.RunTrack(context.Background(), execTrack, execProject)
	if err != nil {
		t.Fatalf("RunTrack: %v", err)
	}
	return report
}

func (f *execFixture) status(id string) TaskStatus { return f.repo.Tasks[id].Status }

func (f *execFixture) actions(id string) []string {
	var out []string
	for _, l := range f.repo.logs {
		if l.TaskID == id {
			out = append(out, l.Action)
		}
	}
	return out
}

func hasAction(actions []string, want string) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}

// TestExecutor_CompletesTodoTaskThroughClaim is the regression guard for
// the pre-executor bug: the default state machine forbids TODO→DONE, so a
// dispatch that skips the claim can never complete a task. The executor
// must claim first (TODO→IN_PROGRESS, CLAIMED logged) and then complete.
func TestExecutor_CompletesTodoTaskThroughClaim(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	f.add(t, f.task("task_a", 1))

	report := f.run(t)

	if got := f.status("task_a"); got != StatusDone {
		t.Fatalf("status = %q; want DONE", got)
	}
	if !hasAction(f.actions("task_a"), ActionClaimed) || !hasAction(f.actions("task_a"), ActionDone) {
		t.Errorf("log actions = %v; want CLAIMED then DONE", f.actions("task_a"))
	}
	if len(report.Done) != 1 || report.Done[0] != "task_a" {
		t.Errorf("report.Done = %v; want [task_a]", report.Done)
	}
	got := f.repo.Tasks["task_a"]
	if got.ClaimedAt != nil {
		t.Errorf("ClaimedAt = %v after completion; want nil", got.ClaimedAt)
	}
	if got.Result["exit_code"] != float64(0) {
		t.Errorf("Result = %v; want the dispatcher's result stored", got.Result)
	}
}

func TestExecutor_RunsInDependencyOrderByOrdinal(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	// c (ordinal 3) and b (ordinal 2) both wait on a; b must go first.
	f.add(t, f.task("task_c", 3, "task_a"), f.task("task_a", 1), f.task("task_b", 2, "task_a"))

	f.run(t)

	want := []string{"task_a", "task_b", "task_c"}
	if strings.Join(f.disp.calls, ",") != strings.Join(want, ",") {
		t.Errorf("dispatch order = %v; want %v", f.disp.calls, want)
	}
	for _, id := range want {
		if f.status(id) != StatusDone {
			t.Errorf("%s = %q; want DONE", id, f.status(id))
		}
	}
}

func TestExecutor_StrictModeSkipsTasksWithoutRunID(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	extra := f.task("task_extra", 2)
	extra.RunID, extra.StepID = "", ""
	f.add(t, f.task("task_a", 1), extra)

	f.run(t)
	if f.status("task_extra") != StatusTodo || f.disp.callsFor("task_extra") != 0 {
		t.Errorf("strict mode dispatched a task without run_id: %q", f.status("task_extra"))
	}

	p := newExecFixture(t, ExecutorOpts{Permissive: true})
	extra2 := p.task("task_extra", 2)
	extra2.RunID, extra2.StepID = "", ""
	p.add(t, p.task("task_a", 1), extra2)
	p.run(t)
	if p.status("task_extra") != StatusDone {
		t.Errorf("permissive mode left task_extra %q; want DONE", p.status("task_extra"))
	}
}

// TestExecutor_WhenSkipsOrBlocks: a false `when` skips the task (deps
// treat SKIPPED as satisfied); an unevaluable one blocks it with the
// evaluator's error as the reason.
func TestExecutor_WhenSkipsOrBlocks(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	f.disp.fn = func(task *Task) (*DispatchResult, error) {
		return &DispatchResult{Status: AgentStatusSucceeded, Summary: "ok", Result: map[string]any{"exit_code": float64(1)}}, nil
	}
	fix := f.task("task_fix", 2, "task_test")
	fix.Spec = &TaskSpec{When: "results.task_test.exit_code == 0"}
	retest := f.task("task_retest", 3, "task_fix")
	broken := f.task("task_broken", 4)
	broken.Spec = &TaskSpec{When: "results.nope.exit_code == 0"}
	f.add(t, f.task("task_test", 1), fix, retest, broken)

	report := f.run(t)

	if f.status("task_fix") != StatusSkipped || !hasAction(f.actions("task_fix"), ActionSkipped) {
		t.Errorf("task_fix = %q, actions %v; want SKIPPED", f.status("task_fix"), f.actions("task_fix"))
	}
	if f.status("task_retest") != StatusDone {
		t.Errorf("task_retest = %q; want DONE (skipped dep satisfies)", f.status("task_retest"))
	}
	b := f.repo.Tasks["task_broken"]
	if b.BlockedReason == nil || !strings.HasPrefix(*b.BlockedReason, "when:") || b.Status != StatusTodo {
		t.Errorf("task_broken = %q / %v; want TODO blocked with a when: reason", b.Status, b.BlockedReason)
	}
	if len(report.Skipped) != 1 || len(report.Blocked) != 1 {
		t.Errorf("report skipped/blocked = %v/%v", report.Skipped, report.Blocked)
	}
}

func TestExecutor_RetryThenBlockOnExhaustion(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	f.disp.fn = func(*Task) (*DispatchResult, error) {
		return &DispatchResult{Status: AgentStatusFailed, Summary: "boom"}, nil
	}
	a := f.task("task_a", 1)
	a.Spec = &TaskSpec{Retry: &RetrySpec{MaxAttempts: 2, Backoff: "30s"}}
	f.add(t, a)

	report := f.run(t)

	got := f.repo.Tasks["task_a"]
	if f.disp.callsFor("task_a") != 2 || got.Attempts != 2 {
		t.Errorf("dispatches/attempts = %d/%d; want 2/2", f.disp.callsFor("task_a"), got.Attempts)
	}
	if got.Status != StatusTodo || got.BlockedReason == nil || !strings.Contains(*got.BlockedReason, "×2") {
		t.Errorf("after exhaustion: status %q reason %v; want TODO blocked '… failed ×2 …'", got.Status, got.BlockedReason)
	}
	if !hasAction(f.actions("task_a"), ActionRetry) || !hasAction(f.actions("task_a"), ActionBlocked) {
		t.Errorf("actions = %v; want RETRY then BLOCKED", f.actions("task_a"))
	}
	if len(f.slept) != 1 || f.slept[0] != 30*time.Second {
		t.Errorf("backoff sleeps = %v; want [30s] (once, between the two attempts)", f.slept)
	}
	if len(report.Blocked) != 1 {
		t.Errorf("report.Blocked = %v", report.Blocked)
	}
}

func TestExecutor_RetrySucceedsOnLaterAttempt(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	calls := 0
	f.disp.fn = func(*Task) (*DispatchResult, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("transient")
		}
		return &DispatchResult{Status: AgentStatusSucceeded, Summary: "ok"}, nil
	}
	a := f.task("task_a", 1)
	a.Spec = &TaskSpec{Retry: &RetrySpec{MaxAttempts: 3}}
	f.add(t, a)

	f.run(t)

	got := f.repo.Tasks["task_a"]
	if got.Status != StatusDone || got.Attempts != 2 || got.BlockedReason != nil {
		t.Errorf("task_a = %q attempts %d reason %v; want DONE after 2 failed attempts", got.Status, got.Attempts, got.BlockedReason)
	}
}

func TestExecutor_GateFailureIsAFailure(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	f.exec.gate = func(_ context.Context, g *StepGate, out map[string]any, _ string) error {
		return errors.New("contract " + g.Contract + " violated")
	}
	a := f.task("task_a", 1)
	a.Spec = &TaskSpec{Gate: &StepGate{Contract: "lint-clean"}}
	f.add(t, a)

	f.run(t)

	got := f.repo.Tasks["task_a"]
	if got.Status != StatusTodo || got.BlockedReason == nil || !strings.Contains(*got.BlockedReason, "lint-clean") {
		t.Errorf("gate failure: status %q reason %v; want blocked naming the contract", got.Status, got.BlockedReason)
	}
	if got.Result == nil {
		t.Error("result must be stored even when the gate fails")
	}
}

func TestExecutor_HumanGateWaits(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	signoff := f.task("task_signoff", 2, "task_a")
	signoff.Kind = TaskKindHuman
	signoff.Spec = &TaskSpec{Human: &HumanSpec{Assignee: "@lead"}}
	after := f.task("task_after", 3, "task_signoff")
	f.add(t, f.task("task_a", 1), signoff, after)

	report := f.run(t)

	if f.status("task_a") != StatusDone {
		t.Errorf("task_a = %q; want DONE before the gate", f.status("task_a"))
	}
	if f.status("task_signoff") != StatusTodo || f.disp.callsFor("task_signoff") != 0 {
		t.Errorf("human task touched: %q, dispatched %d", f.status("task_signoff"), f.disp.callsFor("task_signoff"))
	}
	if f.status("task_after") != StatusTodo {
		t.Errorf("task_after = %q; want TODO behind the gate", f.status("task_after"))
	}
	if len(report.WaitingHuman) != 1 || report.WaitingHuman[0] != "task_signoff" {
		t.Errorf("report.WaitingHuman = %v; want [task_signoff]", report.WaitingHuman)
	}
}

// TestExecutor_WaitPollsUntilCancelled: with Wait set the executor sleeps
// Poll and re-checks instead of returning; canceling the context ends it.
func TestExecutor_WaitPollsUntilCancelled(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{Wait: true, Poll: 7 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	polls := 0
	f.exec.opts.Sleep = func(_ context.Context, d time.Duration) error {
		polls++
		if d != 7*time.Second {
			t.Errorf("poll sleep = %v; want 7s", d)
		}
		if polls == 2 {
			cancel()
		}
		return nil
	}
	signoff := f.task("task_signoff", 1)
	signoff.Kind = TaskKindHuman
	f.add(t, signoff)

	report, err := f.exec.RunTrack(ctx, execTrack, execProject)
	if err != nil {
		t.Fatalf("RunTrack: %v", err)
	}
	if polls != 2 || len(report.WaitingHuman) != 1 {
		t.Errorf("polls = %d, waiting = %v; want 2 polls then exit on cancel", polls, report.WaitingHuman)
	}
}

func TestExecutor_HumanTimeoutPolicy(t *testing.T) {
	for _, tc := range []struct {
		policy     string
		wantStatus TaskStatus
		wantAction string
	}{
		{"approve", StatusDone, ActionApproved},
		{"reject", StatusTodo, ActionRejected},
	} {
		f := newExecFixture(t, ExecutorOpts{})
		h := f.task("task_h", 1)
		h.Kind = TaskKindHuman
		h.Spec = &TaskSpec{Human: &HumanSpec{Timeout: "30m", OnTimeout: tc.policy}}
		h.CreatedAt = execNow.Add(-time.Hour) // overdue
		fresh := f.task("task_fresh", 2)
		fresh.Kind = TaskKindHuman
		fresh.Spec = &TaskSpec{Human: &HumanSpec{Timeout: "2h", OnTimeout: tc.policy}}
		f.add(t, h, fresh)

		f.run(t)

		if f.status("task_h") != tc.wantStatus || !hasAction(f.actions("task_h"), tc.wantAction) {
			t.Errorf("%s: task_h = %q actions %v; want %q with %s", tc.policy, f.status("task_h"), f.actions("task_h"), tc.wantStatus, tc.wantAction)
		}
		if tc.policy == "reject" {
			if r := f.repo.Tasks["task_h"].BlockedReason; r == nil || !strings.Contains(*r, "timeout") {
				t.Errorf("reject: blocked reason = %v; want timeout", r)
			}
		}
		if f.status("task_fresh") != StatusTodo {
			t.Errorf("%s: not-yet-overdue human task changed to %q", tc.policy, f.status("task_fresh"))
		}
	}
}

func TestExecutor_ReclaimStaleClaim(t *testing.T) {
	dead := "dead-executor"
	stale := execNow.Add(-2 * time.Hour)

	f := newExecFixture(t, ExecutorOpts{})
	a := f.task("task_a", 1)
	a.Status, a.AssignedTo, a.ClaimedAt = StatusInProgress, &dead, &stale
	f.add(t, a)
	f.run(t)
	if f.disp.callsFor("task_a") != 0 || f.status("task_a") != StatusInProgress {
		t.Errorf("without --reclaim a foreign claim was touched: %q, dispatched %d", f.status("task_a"), f.disp.callsFor("task_a"))
	}

	r := newExecFixture(t, ExecutorOpts{Reclaim: time.Hour})
	b := r.task("task_a", 1)
	b.Status, b.AssignedTo, b.ClaimedAt = StatusInProgress, &dead, &stale
	r.add(t, b)
	report := r.run(t)
	got := r.repo.Tasks["task_a"]
	if got.Status != StatusDone || !hasAction(r.actions("task_a"), ActionReclaimed) {
		t.Errorf("reclaim: status %q actions %v; want DONE with RECLAIMED", got.Status, r.actions("task_a"))
	}
	if len(report.Reclaimed) != 1 {
		t.Errorf("report.Reclaimed = %v", report.Reclaimed)
	}

	// A recent foreign claim is left alone even with --reclaim.
	recent := execNow.Add(-5 * time.Minute)
	n := newExecFixture(t, ExecutorOpts{Reclaim: time.Hour})
	c := n.task("task_a", 1)
	c.Status, c.AssignedTo, c.ClaimedAt = StatusInProgress, &dead, &recent
	n.add(t, c)
	n.run(t)
	if n.disp.callsFor("task_a") != 0 {
		t.Error("recent foreign claim was reclaimed")
	}
}

func TestExecutor_NoDispatcherForKindBlocks(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	a := f.task("task_a", 1)
	a.Kind = TaskKindExec
	a.Spec = &TaskSpec{Exec: &ExecSpec{Argv: []string{"true"}}}
	f.add(t, a)

	report := f.run(t)

	got := f.repo.Tasks["task_a"]
	if got.BlockedReason == nil || !strings.Contains(*got.BlockedReason, "exec") || len(report.Blocked) != 1 {
		t.Errorf("exec task without a dispatcher: reason %v, report %v; want blocked naming the kind", got.BlockedReason, report.Blocked)
	}
}

func TestExecutor_ConcurrencyBound(t *testing.T) {
	for _, n := range []int{1, 2} {
		f := newExecFixture(t, ExecutorOpts{Concurrency: n})
		gate := make(chan struct{})
		var once sync.Once
		f.disp.fn = func(*Task) (*DispatchResult, error) {
			// Hold every dispatch until all in this batch have started, so
			// the peak in-flight count reflects the semaphore, not timing.
			f.disp.mu.Lock()
			inflight := f.disp.inflight
			f.disp.mu.Unlock()
			if inflight >= n {
				once.Do(func() { close(gate) })
			}
			<-gate
			return &DispatchResult{Status: AgentStatusSucceeded, Summary: "ok"}, nil
		}
		f.add(t, f.task("task_a", 1), f.task("task_b", 2), f.task("task_c", 3), f.task("task_d", 4))

		f.run(t)

		if f.disp.maxInflight != n {
			t.Errorf("Concurrency %d: peak in-flight = %d", n, f.disp.maxInflight)
		}
		for _, id := range []string{"task_a", "task_b", "task_c", "task_d"} {
			if f.status(id) != StatusDone {
				t.Errorf("Concurrency %d: %s = %q", n, id, f.status(id))
			}
		}
	}
}

// TestExecutor_SubjectAutoCompletes: when every leaf of a run is done the
// subject task completes with a provenance note — unless another actor
// holds it.
func TestExecutor_SubjectAutoCompletes(t *testing.T) {
	newSubjectFixture := func(t *testing.T, assignee string) *execFixture {
		f := newExecFixture(t, ExecutorOpts{})
		project := execProject
		subject := &Task{ID: "task_subject", Title: "subject", Status: StatusTodo, ProjectID: &project, CreatedAt: execNow, UpdatedAt: execNow}
		if assignee != "" {
			subject.AssignedTo = &assignee
		}
		a, b := f.task("task_a", 1), f.task("task_b", 2, "task_a")
		for _, task := range []*Task{a, b} {
			task.SetProvenance(TaskProvenance{Recipe: "code-review", RecipeVersion: "1.2.0", Subject: "task_subject"})
		}
		f.add(t, subject, a, b)
		_ = f.runs.AddRecipeRunTasks(context.Background(), "run_1", []RecipeRunTask{{StepID: "task_a", TaskID: "task_a"}, {StepID: "task_b", TaskID: "task_b"}})
		return f
	}

	f := newSubjectFixture(t, "")
	f.run(t)
	subject := f.repo.Tasks["task_subject"]
	if subject.Status != StatusDone {
		t.Fatalf("subject = %q; want DONE once every leaf is done", subject.Status)
	}
	noted := false
	for _, l := range f.repo.logs {
		if l.TaskID == "task_subject" && strings.Contains(l.Note, "code-review@1.2.0") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("subject completion note lacks recipe@version: %+v", f.actions("task_subject"))
	}

	g := newSubjectFixture(t, "someone-else")
	g.run(t)
	if g.status("task_subject") != StatusTodo {
		t.Errorf("subject claimed by another actor was completed: %q", g.status("task_subject"))
	}
}

// TestExecutor_ResultsEnvIsScopedToRun: `when` sees results of the same
// run only, keyed by step id.
func TestExecutor_ResultsEnvIsScopedToRun(t *testing.T) {
	f := newExecFixture(t, ExecutorOpts{})
	other := f.task("task_other", 1)
	other.RunID, other.StepID, other.Status = "run_2", "test", StatusDone
	other.Result = map[string]any{"exit_code": float64(0)}
	dep := f.task("task_dep", 2)
	dep.Spec = &TaskSpec{When: "results.test.exit_code == 0"}
	f.add(t, other, dep)

	f.run(t)

	got := f.repo.Tasks["task_dep"]
	if got.BlockedReason == nil || !strings.Contains(*got.BlockedReason, "test") {
		t.Errorf("results from another run leaked into `when`: status %q reason %v", got.Status, got.BlockedReason)
	}
}
