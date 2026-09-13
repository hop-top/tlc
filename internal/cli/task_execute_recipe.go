package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// runTaskExecRecipe is `task execute <task> --recipe`: the recipe is
// materialized for the task as subject, into the task's track, the task
// is blocked on the run's leaves, and the created tasks run through the
// executor — which completes the subject once the leaves are done.
func runTaskExecRecipe(ctx context.Context, cmd *cobra.Command, s *storage.SQLiteStorage, subject *core.Task) error {
	opts := recipeOptsFromFlags()
	r, expanded, err := loadExpandedRecipe(opts.Recipe)
	if err != nil {
		return err
	}
	subj := subjectFromTask(ctx, s, subject)
	if err := enforceRecipeSubject(r, subj, "use 'tlc track execute <track> --recipe "+r.Name+"'"); err != nil {
		return err
	}
	plan, err := renderRecipePlan(cmd, r, expanded, opts, subj, nil)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	target := recipeTarget{
		TrackID: subj.TrackID, TrackType: trackTypeOf(ctx, s, subj.TrackID),
		ProjectID: currentProjectID(), DryRun: taskExecDryRun,
	}
	res, err := materializeRecipe(ctx, w, s, plan, target, opts.Assign)
	if err != nil {
		return err
	}
	rep := materializeReport{Verb: verbMaterialized, Subject: subj.Display, DryRun: target.DryRun}
	if target.TrackID != "" {
		rep.Track, _ = s.GetTrack(ctx, target.TrackID) //nolint:errcheck // display only; the id is still printed on a miss
	}
	if err := printMaterializeSummary(ctx, w, s, plan, res, rep); err != nil {
		return err
	}
	if target.DryRun {
		return nil
	}
	if err := blockSubjectOnRunLeaves(ctx, s, subj, res); err != nil {
		return err
	}
	if err := writeProjection(); err != nil {
		return err
	}
	ids := createdTaskIDs(res)
	if len(ids) == 0 {
		return nil
	}

	registry, err := loadAgentRegistry(taskExecTrustProject)
	if err != nil {
		return err
	}
	agent, _ := parseAgentExpr(taskExecAgentExpr)
	executor := newTrackExecutor(s, core.DefaultWorkflow(), core.ExecutorOpts{
		Actor: core.GetCurrentUser(), Concurrency: 1, EvaKey: os.Getenv("EVA_KEY"), Out: w,
	}, registry, executorSetup{agent: agent, withPod: taskExecWithPod, ctxtRefs: taskExecCtxtRefs})
	if taskExecTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, taskExecTimeout)
		defer cancel()
	}
	_, _ = fmt.Fprintf(w, "Executing %d task(s) for %s\n", len(ids), subj.Display)
	report, err := executor.RunTasks(ctx, ids)
	if err != nil {
		return fmt.Errorf("execute recipe %s for %s: %w", plan.Recipe.Key(), subj.Display, err)
	}
	printExecReport(ctx, w, s, "Task "+subj.Display, report)
	return nil
}
