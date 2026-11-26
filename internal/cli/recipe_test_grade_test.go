package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/flowtest"
	"hop.top/tlc/internal/storage"
)

// gradeFixture stores one task per step and returns their ids, so
// gradeRecipeRun reads them back the way a real run does.
func gradeFixture(t *testing.T, steps ...string) (*storage.SQLiteStorage, []string) {
	t.Helper()
	inRepoRoot(t)
	store, err := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "grade.sqlite"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	project := currentProjectID()
	now := time.Now()
	ids := make([]string, 0, len(steps))
	for i, step := range steps {
		task := &core.Task{
			ID: "task_grade_" + step, Title: step, Status: core.StatusTodo,
			StepID: step, StepOrdinal: i + 1, CreatedAt: now, UpdatedAt: now,
			ProjectID: &project,
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("create %s: %v", step, err)
		}
		ids = append(ids, task.ID)
	}
	return store, ids
}

func grade(
	t *testing.T, store *storage.SQLiteStorage, fixture flowtest.Run, ids []string, report *core.ExecReport,
) (int, string) {
	t.Helper()
	buf := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	code, err := gradeRecipeRun(context.Background(), cmd, store, fixture, ids, report)
	if err != nil {
		t.Fatalf("gradeRecipeRun: %v", err)
	}
	return code, buf.String()
}

func TestGradeRecipeRun_WhenSkippedIsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		report   func(ids []string) *core.ExecReport
		wantCode int
		wantOut  string
	}{
		{
			name:     "all done passes",
			report:   func(ids []string) *core.ExecReport { return &core.ExecReport{Done: ids} },
			wantCode: 0, wantOut: "result: PASS",
		},
		{
			// The regression: a step skipped by a false when: is expected.
			name: "done and when-skipped passes",
			report: func(ids []string) *core.ExecReport {
				return &core.ExecReport{Done: ids[:2], Skipped: ids[2:]}
			},
			wantCode: 0, wantOut: "result: PASS",
		},
		{
			name: "blocked task still fails",
			report: func(ids []string) *core.ExecReport {
				return &core.ExecReport{Done: ids[:1], Skipped: ids[1:2], Blocked: ids[2:]}
			},
			wantCode: recipeTestExitFailed, wantOut: "result: FAIL (2 of 3 tasks finished: 1 done, 1 skipped)",
		},
		{
			name: "task left waiting on a human still fails",
			report: func(ids []string) *core.ExecReport {
				return &core.ExecReport{Done: ids[:1], Skipped: ids[1:2], WaitingHuman: ids[2:]}
			},
			wantCode: recipeTestExitFailed, wantOut: "result: FAIL (2 of 3 tasks finished: 1 done, 1 skipped)",
		},
		{
			name: "task neither done nor skipped still fails",
			report: func(ids []string) *core.ExecReport {
				return &core.ExecReport{Done: ids[:2]}
			},
			wantCode: recipeTestExitFailed, wantOut: "result: FAIL (2 of 3 tasks finished: 2 done, 0 skipped)",
		},
		{
			// Counts must not be fooled by a stray id outside the run.
			name: "foreign skipped id does not mask a missing task",
			report: func(ids []string) *core.ExecReport {
				return &core.ExecReport{Done: ids[:2], Skipped: []string{"task_elsewhere"}}
			},
			wantCode: recipeTestExitFailed, wantOut: "result: FAIL",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, ids := gradeFixture(t, "a", "b", "c")
			code, out := grade(t, store, flowtest.Run{}, ids, tc.report(ids))
			if code != tc.wantCode {
				t.Errorf("exit = %d; want %d\n%s", code, tc.wantCode, out)
			}
			if !strings.Contains(out, tc.wantOut) {
				t.Errorf("output = %q; want %q", out, tc.wantOut)
			}
		})
	}
}

// A run that expects a failure still fails when every task finished,
// skipped ones included: the skip does not turn into a pass.
func TestGradeRecipeRun_SkipDoesNotSatisfyExpectedFailure(t *testing.T) {
	store, ids := gradeFixture(t, "a", "b")
	report := &core.ExecReport{Done: ids[:1], Skipped: ids[1:]}
	code, out := grade(t, store, flowtest.Run{ExpectedExit: 1}, ids, report)
	if code != recipeTestExitFailed || !strings.Contains(out, "expected exit 1") {
		t.Errorf("exit = %d, output = %q; want %d naming the expectation", code, out, recipeTestExitFailed)
	}
}
