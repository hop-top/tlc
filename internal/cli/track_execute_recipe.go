package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// reconcileTrackRecipe is the `--recipe` half of `track execute`: the
// steps the track's ledger does not cover yet are materialized before
// the readiness loop runs. Vars come from the latest run of the recipe
// on the track, overridden by --var; so does the subject.
func reconcileTrackRecipe(
	ctx context.Context, cmd *cobra.Command, s *storage.SQLiteStorage, track *core.Track, projectID string, dryRun bool,
) error {
	opts := recipeOptsFromFlags()
	r, expanded, err := loadExpandedRecipe(opts.Recipe)
	if err != nil {
		return err
	}
	latest, err := latestRecipeRun(ctx, s, r, track.ID, projectID)
	if err != nil {
		return err
	}
	subj, err := reconcileSubject(ctx, s, r, latest, track)
	if err != nil {
		return err
	}
	plan, err := renderRecipePlan(cmd, r, expanded, opts, subj, carriedVars(r, latest))
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	target := recipeTarget{
		TrackID: track.ID, TrackType: track.Type, ProjectID: projectID,
		DryRun: dryRun, Recreate: opts.Recreate, Reconcile: true,
	}
	res, err := materializeRecipe(ctx, w, s, plan, target, false)
	if err != nil {
		return err
	}
	rep := materializeReport{Verb: verbReconciled, Track: track, DryRun: dryRun}
	if subj != nil && subj.Type == core.SubjectTask {
		rep.Subject = subj.Display
	}
	if err := printMaterializeSummary(ctx, w, s, plan, res, rep); err != nil {
		return err
	}
	if dryRun || len(res.Created) == 0 {
		return nil
	}
	if err := blockSubjectOnRunLeaves(ctx, s, subj, res); err != nil {
		return err
	}
	return writeProjection()
}

// latestRecipeRun returns the newest run of the recipe on the track,
// nil when it was never materialized there.
func latestRecipeRun(ctx context.Context, s *storage.SQLiteStorage, r *core.Recipe, trackID, projectID string) (*core.RecipeRun, error) {
	runs, err := s.ListRecipeRuns(ctx, core.RecipeRunQuery{
		RecipeID: r.Name, TrackID: trackID, ProjectID: projectID, AllProjects: true, Limit: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("list runs of recipe %s on track %s: %w", r.Name, trackID, err)
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return runs[0], nil
}

// reconcileSubject rebinds the subject the latest run recorded; without
// a run, a recipe that requires a track subject gets the track itself.
func reconcileSubject(
	ctx context.Context, s *storage.SQLiteStorage, r *core.Recipe, latest *core.RecipeRun, track *core.Track,
) (*recipeSubject, error) {
	var subj *recipeSubject
	switch {
	case latest != nil && latest.SubjectType == core.SubjectTask:
		task, err := s.GetTask(ctx, latest.SubjectID)
		if err != nil {
			return nil, fmt.Errorf("load subject %s of run %s: %w", latest.SubjectID, latest.ID, err)
		}
		if task == nil {
			return nil, fmt.Errorf(
				"subject task %s of run %s no longer exists; materialize the recipe again with "+
					"'tlc task create --recipe %s --for <task>'", latest.SubjectID, latest.ID, r.Name,
			)
		}
		subj = subjectFromTask(ctx, s, task)
	case latest != nil && latest.SubjectType == core.SubjectTrack:
		subj = subjectFromTrack(track)
	case latest == nil && r.Requires.Subject == core.SubjectTrack:
		subj = subjectFromTrack(track)
	}
	fix := fmt.Sprintf("materialize it first with 'tlc task create --recipe %s --for <task>'", r.Name)
	if err := enforceRecipeSubject(r, subj, fix); err != nil {
		return nil, err
	}
	return subj, nil
}

// carriedVars turns the latest run's vars into --var form, keeping only
// the ones the recipe still declares.
func carriedVars(r *core.Recipe, latest *core.RecipeRun) map[string]string {
	if latest == nil {
		return nil
	}
	out := make(map[string]string, len(latest.Vars))
	for k, v := range latest.Vars {
		if _, declared := r.Vars[k]; declared {
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}
