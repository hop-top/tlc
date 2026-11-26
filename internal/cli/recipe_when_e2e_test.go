package cli

import (
	"strings"
	"testing"
)

// recipeWhenFixture has a vars-only `when:` on both deploy branches, a
// join that depends on both, and a `when:` that reads a run-time result.
const recipeWhenFixture = `recipe: deploy
version: 1.0.0
vars:
  target: {required: true}
steps:
  - id: validate
    title: Validate
  - id: deploy-staging
    title: Deploy staging
    depends_on: [validate]
    when: "vars.target == 'staging'"
  - id: deploy-prod
    title: Deploy production
    depends_on: [validate]
    when: "vars.target == 'production'"
  - id: report
    title: Report
    depends_on: [deploy-staging, deploy-prod]
  - id: follow-up
    title: Follow up
    depends_on: [report]
    when: "results.report.exit_code == 0"
`

func TestTrackCreateRecipe_E2E_CreateTimeWhen(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "deploy.yaml", recipeWhenFixture)

		out, err := runRecipeCLI(t, "track", "create", "Ship", "--recipe", "deploy", "--var", "target=staging")
		if err != nil {
			t.Fatalf("track create --recipe: %v\n%s", err, out)
		}
		assertContains(t, out, "created 4 task(s)", "deploy-staging", "Warnings:", "step deploy-prod: not materialized")
		if strings.Contains(out, "deploy-prod  (") {
			t.Errorf("deploy-prod listed as created:\n%s", out)
		}

		s := openTestStorage(t)
		track := mustTrack(t, ctx, s, "ship")
		byStep := tasksByStep(tasksOfTrack(t, ctx, s, track.ID))
		if len(byStep) != 4 || byStep["deploy-prod"] != nil {
			t.Fatalf("steps = %v; want deploy-prod not materialized", byStep)
		}
		// The dependent of the dropped step keeps its other edge only,
		// as it does after a run-time skip (a skipped blocker is terminal).
		if got := byStep["report"].BlockedBy(); len(got) != 1 || got[0] != byStep["deploy-staging"].ID {
			t.Errorf("report blocked_by = %v; want only deploy-staging", got)
		}
		// A true condition keeps its run-time gate; a results condition is untouched.
		if sp := byStep["deploy-staging"].Spec; sp == nil || sp.When != "vars.target == 'staging'" {
			t.Errorf("deploy-staging spec = %+v; want the when kept", sp)
		}
		if sp := byStep["follow-up"].Spec; sp == nil || sp.When != "results.report.exit_code == 0" {
			t.Errorf("follow-up spec = %+v; want the results when untouched", sp)
		}
		runs := recipeRunsOf(t, ctx, s, "deploy", track.ID)
		if len(runs) != 1 || len(runs[0].DroppedDeps) != 1 || runs[0].DroppedDeps[0] != "report:deploy-prod" {
			t.Fatalf("runs = %+v; want the dropped edge recorded", runs)
		}
		if rows, err := s.ListRecipeRunTasks(ctx, runs[0].ID); err != nil || len(rows) != 4 {
			t.Errorf("run tasks = %v (%v); want 4 rows", rows, err)
		}

		// Reconcile with the same vars neither creates the dropped step nor reports it.
		out, err = runRecipeCLI(t, "track", "execute", "ship", "--recipe", "deploy")
		if err != nil {
			t.Fatalf("reconcile: %v\n%s", err, out)
		}
		assertContains(t, out, "created 0 task(s)")
		if got := len(tasksOfTrack(t, ctx, s, track.ID)); got != 4 {
			t.Errorf("track has %d tasks after reconcile; want 4", got)
		}
	})
}

func TestTrackCreateRecipe_E2E_CreateTimeWhenDryRun(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "deploy.yaml", recipeWhenFixture)

		out, err := runRecipeCLI(t, "track", "create", "Ship", "--recipe", "deploy", "--var", "target=production", "--dry-run")
		if err != nil {
			t.Fatalf("dry-run: %v\n%s", err, out)
		}
		assertContains(t, out, "created 4 task(s)", "deploy-prod", "step deploy-staging: not materialized")
		if strings.Contains(out, "deploy-staging  (") {
			t.Errorf("dry run lists the dropped step as created:\n%s", out)
		}
		s := openTestStorage(t)
		if n := len(allTasks(t, ctx, s)); n != 0 {
			t.Errorf("dry run wrote %d tasks", n)
		}
	})
}

func TestTrackCreateRecipe_E2E_WhenTrueMaterializes(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "deploy.yaml", recipeWhenFixture)

		out, err := runRecipeCLI(t, "track", "create", "Ship", "--recipe", "deploy", "--var", "target=production")
		if err != nil {
			t.Fatalf("track create: %v\n%s", err, out)
		}
		s := openTestStorage(t)
		byStep := tasksByStep(tasksOfTrack(t, ctx, s, mustTrack(t, ctx, s, "ship").ID))
		if byStep["deploy-prod"] == nil || byStep["deploy-staging"] != nil {
			t.Fatalf("steps = %v; want deploy-prod only", byStep)
		}
		if sp := byStep["deploy-prod"].Spec; sp == nil || sp.When != "vars.target == 'production'" {
			t.Errorf("deploy-prod spec = %+v; want the when kept for the executor", sp)
		}
	})
}

func TestTrackCreateRecipe_E2E_WhenUndefinedVarNotDropped(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "typo.yaml", `recipe: typo
version: 1.0.0
steps:
  - id: a
    title: A
    when: "vars.missing == 'x'"
`)
		out, err := runRecipeCLI(t, "track", "create", "Typo", "--recipe", "typo")
		if err != nil {
			t.Fatalf("track create: %v\n%s", err, out)
		}
		assertContains(t, out, "created 1 task(s)", "left to the executor")
		s := openTestStorage(t)
		if byStep := tasksByStep(tasksOfTrack(t, ctx, s, mustTrack(t, ctx, s, "typo").ID)); byStep["a"] == nil {
			t.Errorf("an unevaluable when dropped the step")
		}
	})
}

func TestRecipeDiff_E2E_CreateTimeWhenNotDrift(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "deploy.yaml", recipeWhenFixture)
		if out, err := runRecipeCLI(t, "track", "create", "Ship", "--recipe", "deploy", "--var", "target=staging"); err != nil {
			t.Fatalf("track create: %v\n%s", err, out)
		}
		out, err := runRecipeCmd(t, "diff", "ship")
		if err != nil {
			t.Fatalf("recipe diff: %v\n%s", err, out)
		}
		if row := diffRow(t, out, "deploy-prod"); strings.Contains(row, "missing") || !strings.Contains(row, "dropped") {
			t.Errorf("deploy-prod row = %q; want dropped, not missing", row)
		}
		for _, step := range []string{"validate", "deploy-staging", "report", "follow-up"} {
			if row := diffRow(t, out, step); !strings.Contains(row, "materialized") {
				t.Errorf("%s row = %q; want materialized", step, row)
			}
		}
	})
}
