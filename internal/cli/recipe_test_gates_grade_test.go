package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/flowtest"
	"hop.top/tlc/internal/storage"
)

// gradeTask is one task of a synthetic run: the step, what it waits on and
// where it ended up.
type gradeTask struct {
	step      string
	blockedBy []string
	status    core.TaskStatus
	blocked   string // BlockedReason
	human     bool
}

// gradeRun stores the tasks and returns their ids in order.
func gradeRun(t *testing.T, tasks ...gradeTask) (*storage.SQLiteStorage, []string) {
	t.Helper()
	inRepoRoot(t)
	store, err := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "grade.sqlite"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	project := currentProjectID()
	now := time.Now()
	ids := make([]string, 0, len(tasks))
	for i, gt := range tasks {
		task := &core.Task{
			ID: "task_grade_" + gt.step, Title: gt.step, Status: gt.status,
			StepID: gt.step, StepOrdinal: i + 1, CreatedAt: now, UpdatedAt: now,
			ProjectID: &project,
		}
		if gt.human {
			task.Kind = core.TaskKindHuman
		}
		if gt.blocked != "" {
			reason := gt.blocked
			task.BlockedReason = &reason
		}
		if len(gt.blockedBy) > 0 {
			blockers := make([]string, len(gt.blockedBy))
			for j, b := range gt.blockedBy {
				blockers[j] = "task_grade_" + b
			}
			task.SetBlockedBy(blockers)
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("create %s: %v", gt.step, err)
		}
		ids = append(ids, task.ID)
	}
	return store, ids
}

func rejected(reason string) string { return "rejected: " + reason }

func TestGradeRecipeRun_Gates(t *testing.T) {
	approve := flowtest.GateDecision{Decision: flowtest.GateApprove}
	reject := flowtest.GateDecision{Decision: flowtest.GateReject, Reason: "no"}
	gateRun := func(d flowtest.GateDecision) flowtest.Run {
		return flowtest.Run{Gates: map[string]flowtest.GateDecision{"gate": d}}
	}

	tests := []struct {
		name     string
		tasks    []gradeTask
		run      flowtest.Run
		done     []string // steps the executor reported done
		wantCode int
		wantOut  string
	}{
		{
			name: "reject as declared, dependents never reached, passes",
			tasks: []gradeTask{
				{step: "prep", status: core.StatusDone},
				{step: "gate", blockedBy: []string{"prep"}, status: core.StatusTodo, blocked: rejected("no"), human: true},
				{step: "next", blockedBy: []string{"gate"}, status: core.StatusTodo},
				{step: "last", blockedBy: []string{"next"}, status: core.StatusTodo},
			},
			run: gateRun(reject), done: []string{"prep"},
			wantOut: "result: PASS",
		},
		{
			name: "approve as declared passes",
			tasks: []gradeTask{
				{step: "prep", status: core.StatusDone},
				{step: "gate", blockedBy: []string{"prep"}, status: core.StatusDone, human: true},
				{step: "next", blockedBy: []string{"gate"}, status: core.StatusDone},
			},
			run: gateRun(approve), done: []string{"prep", "gate", "next"},
			wantOut: "result: PASS",
		},
		{
			name: "dependent of a rejected gate that ran anyway fails",
			tasks: []gradeTask{
				{step: "gate", status: core.StatusTodo, blocked: rejected("no"), human: true},
				{step: "next", blockedBy: []string{"gate"}, status: core.StatusDone},
			},
			run: gateRun(reject), done: []string{"next"},
			wantCode: recipeTestExitFailed, wantOut: `"next" ran behind rejected gate "gate"`,
		},
		{
			name: "transitive dependent that ran anyway fails",
			tasks: []gradeTask{
				{step: "gate", status: core.StatusTodo, blocked: rejected("no"), human: true},
				{step: "next", blockedBy: []string{"gate"}, status: core.StatusTodo},
				{step: "last", blockedBy: []string{"next"}, status: core.StatusDone},
			},
			run: gateRun(reject), done: []string{"last"},
			wantCode: recipeTestExitFailed, wantOut: `"last" ran behind rejected gate "gate"`,
		},
		{
			name: "declared reject but the gate was approved fails",
			tasks: []gradeTask{
				{step: "gate", status: core.StatusDone, human: true},
			},
			run: gateRun(reject), done: []string{"gate"},
			wantCode: recipeTestExitFailed, wantOut: `gate "gate": declared reject`,
		},
		{
			name: "declared approve but the gate was rejected fails",
			tasks: []gradeTask{
				{step: "gate", status: core.StatusTodo, blocked: rejected("timeout"), human: true},
			},
			run:      gateRun(approve),
			wantCode: recipeTestExitFailed, wantOut: `gate "gate": declared approve`,
		},
		{
			name: "unfinished task outside the rejected gate's dependents fails",
			tasks: []gradeTask{
				{step: "gate", status: core.StatusTodo, blocked: rejected("no"), human: true},
				{step: "aside", status: core.StatusTodo},
			},
			run:      gateRun(reject),
			wantCode: recipeTestExitFailed, wantOut: "result: FAIL (1 of 2 tasks finished",
		},
		{
			name: "task stuck behind another blocked task is not excused by a reject",
			tasks: []gradeTask{
				{step: "gate", status: core.StatusTodo, blocked: rejected("no"), human: true},
				{step: "stuck", status: core.StatusTodo, blocked: "attempts exhausted"},
			},
			run:      gateRun(reject),
			wantCode: recipeTestExitFailed, wantOut: "result: FAIL",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, ids := gradeRun(t, tc.tasks...)
			report := &core.ExecReport{}
			for _, step := range tc.done {
				report.Done = append(report.Done, "task_grade_"+step)
			}
			code, out := grade(t, store, tc.run, ids, report)
			if code != tc.wantCode {
				t.Errorf("exit = %d; want %d\n%s", code, tc.wantCode, out)
			}
			if !strings.Contains(out, tc.wantOut) {
				t.Errorf("output = %q; want %q", out, tc.wantOut)
			}
		})
	}
}
