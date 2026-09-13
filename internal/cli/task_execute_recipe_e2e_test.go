package cli

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// recipeQuickFixture is two exec steps the real executor can run on the
// host, reading the subject into the first one's argv.
const recipeQuickFixture = `recipe: quick
version: 1.0.0
requires: {subject: task}
steps:
  - id: a
    kind: exec
    exec: {argv: [sh, -c, "echo a {{subject.id}}"]}
  - id: b
    kind: exec
    exec: {argv: [sh, -c, "echo b"]}
    depends_on: [a]
`

func TestTaskExecuteRecipe_E2E_CreatesThenRuns(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantRecipe(t, "quick.yaml", recipeQuickFixture)
		s := openTestStorage(t)
		subject := seedSubjectTask(t, ctx, s, "Ship it", "")
		alias := formatTaskAlias(subject)

		out, err := runRecipeCLI(t, "task", "execute", alias, "--recipe", "quick")
		if err != nil {
			t.Fatalf("task execute --recipe: %v\n%s", err, out)
		}
		assertContains(t, out, "Materialized quick@1.0.0 for "+alias+": created 2 task(s)", "done")

		byStep := tasksByStep(allTasks(t, ctx, s))
		a, b := byStep["a"], byStep["b"]
		if a == nil || b == nil {
			t.Fatalf("steps missing: %v", byStep)
		}
		for _, task := range []*core.Task{a, b} {
			if task.Status != core.StatusDone {
				t.Errorf("%s = %q; want DONE\n%s", task.StepID, task.Status, out)
			}
		}
		if stdout, _ := a.Result["stdout"].(string); !strings.Contains(stdout, "a "+alias) {
			t.Errorf("a stdout = %q; want the subject rendered", stdout)
		}
		if strings.Index(out, "a "+alias) > strings.Index(out, "echo b") && strings.Contains(out, "echo b") {
			t.Errorf("b ran before its blocker a:\n%s", out)
		}
		got := mustGetTask(t, ctx, s, subject.ID)
		if got.Status != core.StatusDone {
			t.Errorf("subject = %q; want DONE once the run's leaves finished\n%s", got.Status, out)
		}
		if blocked := got.BlockedBy(); len(blocked) != 1 || blocked[0] != b.ID {
			t.Errorf("subject blocked_by = %v; want the leaf %s", blocked, b.ID)
		}
	})
}

func TestTaskExecuteRecipe_E2E_DryRunCreatesNothing(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		plantRecipe(t, "quick.yaml", recipeQuickFixture)
		s := openTestStorage(t)
		subject := seedSubjectTask(t, ctx, s, "Ship it", "")

		out, err := runRecipeCLI(t, "task", "execute", formatTaskAlias(subject), "--recipe", "quick", "--dry-run")
		if err != nil {
			t.Fatalf("task execute --dry-run: %v\n%s", err, out)
		}
		assertContains(t, out, "Dry run", "quick@1.0.0", "2 task(s)")
		if got := len(allTasks(t, ctx, s)); got != 1 {
			t.Errorf("%d tasks; want only the subject", got)
		}
		if got := mustGetTask(t, ctx, s, subject.ID); got.Status != core.StatusTodo || len(got.BlockedBy()) != 0 {
			t.Errorf("subject changed: %q blocked_by %v", got.Status, got.BlockedBy())
		}
	})
}
