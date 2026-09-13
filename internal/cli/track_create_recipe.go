package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	kitcli "hop.top/kit/go/console/cli"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// runTrackCreateRecipe is `track create --recipe`: the track comes from
// the argument or the recipe's track block, then the recipe is
// materialized into it.
func runTrackCreateRecipe(cmd *cobra.Command, args []string) error {
	opts := recipeOptsFromFlags()
	r, expanded, err := loadExpandedRecipe(opts.Recipe)
	if err != nil {
		return err
	}
	s, err := getStorageRaw()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	ctx := cmdContext(cmd)

	var subj *recipeSubject
	if opts.For != "" {
		if subj, err = resolveRecipeSubject(ctx, s, opts.For); err != nil {
			return err
		}
	}
	vars, err := bindRecipeVars(cmd, r, opts, nil)
	if err != nil {
		return err
	}
	track, err := trackFromRecipe(r, args, vars, subj)
	if err != nil {
		return err
	}
	if subj == nil && r.Requires.Subject == core.SubjectTrack {
		subj = subjectFromTrack(track)
	}
	if err := enforceRecipeSubject(r, subj, "pass --for <task>"); err != nil {
		return err
	}
	plan, err := renderRecipePlan(cmd, r, expanded, opts, subj, nil)
	if err != nil {
		return err
	}
	planMD, err := renderTrackPlan(r, plan)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	_, structured := structuredOutput()
	human := w
	if structured {
		human = io.Discard
	}
	target := recipeTarget{TrackID: track.ID, TrackType: track.Type, ProjectID: currentProjectID(), DryRun: kitcli.IsDryRun(cmd)}
	if target.DryRun {
		printTrackCreateDryRun(human, track)
	} else if err := createTrackWithPlan(ctx, human, s, track, planMD); err != nil {
		return err
	}
	res, err := materializeRecipe(ctx, human, s, plan, target, opts.Assign)
	if err != nil {
		return err
	}
	if !target.DryRun {
		if err := blockSubjectOnRunLeaves(ctx, s, subj, res); err != nil {
			return err
		}
	}
	rep := materializeReport{Verb: verbMaterialized, Track: track, DryRun: target.DryRun}
	if subj != nil && subj.Type == core.SubjectTask {
		rep.Subject = subj.Display
	}
	if err := printMaterializeSummary(ctx, w, s, plan, res, rep); err != nil {
		return err
	}
	if target.DryRun {
		return nil
	}
	return writeProjection()
}

// trackFromRecipe builds the track to create: the title argument, else
// the recipe's track.title rendered with the vars (and the --for
// subject); the type from --type, else track.type, else config.
func trackFromRecipe(r *core.Recipe, args []string, vars map[string]any, subj *recipeSubject) (*core.Track, error) {
	title := ""
	if len(args) > 0 {
		title = trimMatchingQuotes(args[0])
	}
	if title == "" {
		rendered, err := recipeTrackTitle(r, vars, subj)
		if err != nil {
			return nil, err
		}
		title = rendered
	}
	if title == "" {
		return nil, fmt.Errorf(
			"recipe %s names no track title; pass one as the argument or set track.title in the recipe", r.Name,
		)
	}
	trackType := trackCreateType
	if trackType == "" && r.Track != nil {
		trackType = r.Track.Type
	}
	track, err := newTrackFromFlags(title, trackType)
	if err != nil {
		return nil, err
	}
	// Minted here so the subject binding and the run ledger can name the
	// track before it is written; CreateTrack keeps a typeid it is given.
	track.ID = core.NewTrackID()
	return track, nil
}

// recipeTrackTitle renders the recipe's track.title against the vars and
// the --for subject, "" when the recipe declares none.
func recipeTrackTitle(r *core.Recipe, vars map[string]any, subj *recipeSubject) (string, error) {
	if r.Track == nil || r.Track.Title == "" {
		return "", nil
	}
	scope := core.Scope{Vars: vars, Run: map[string]any{"recipe": r.Name, "version": r.Version}}
	if subj != nil {
		scope.Subject = subj.Fields
	}
	rendered, err := core.Render(r.Track.Title, scope)
	if err != nil {
		return "", fmt.Errorf("recipe %s: track.title: %w", r.Name, err)
	}
	return rendered, nil
}

// renderTrackPlan renders the recipe's track.plan, "" when it has none.
func renderTrackPlan(r *core.Recipe, plan *recipePlan) (string, error) {
	if r.Track == nil || r.Track.Plan == "" {
		return "", nil
	}
	rendered, err := core.Render(r.Track.Plan, plan.scope())
	if err != nil {
		return "", fmt.Errorf("recipe %s: track.plan: %w", r.Name, err)
	}
	return rendered, nil
}

// createTrackWithPlan writes the track, reports it and scaffolds its
// directory with the rendered plan.
func createTrackWithPlan(ctx context.Context, w io.Writer, s *storage.SQLiteStorage, track *core.Track, planMD string) error {
	if err := newTrackService(s).CreateTrack(ctx, track); err != nil {
		return err //nolint:wrapcheck // the service names the slug or type that was refused
	}
	printTrackCreated(w, track)
	if configDir := resolveConfigDir(); configDir != "" {
		scaffoldTrackDirWithPlan(w, track, configDir, planMD)
	}
	return nil
}
