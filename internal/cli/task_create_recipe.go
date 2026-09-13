package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	kitcli "hop.top/kit/go/console/cli"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// runTaskCreateRecipe is `task create --recipe`: the recipe's steps
// become tasks in the --track, in the --for subject's track, or
// trackless.
func runTaskCreateRecipe(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf(
			"task create --recipe takes no title: the recipe's steps name the tasks; drop %q", args[0],
		)
	}
	opts := recipeOptsFromFlags()
	r, expanded, err := loadExpandedRecipe(opts.Recipe)
	if err != nil {
		return err
	}
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	ctx := cmdContext(cmd)
	w := cmd.OutOrStdout()
	_, structured := structuredOutput()
	human := w
	if structured {
		human = io.Discard
	}

	var subj *recipeSubject
	if opts.For != "" {
		if subj, err = resolveRecipeSubject(ctx, s, opts.For); err != nil {
			return err
		}
	}
	if err := enforceRecipeSubject(r, subj, "pass --for <task>"); err != nil {
		return err
	}
	target := recipeTarget{ProjectID: currentProjectID(), DryRun: kitcli.IsDryRun(cmd)}
	if subj != nil {
		target.TrackID = subj.TrackID
	}
	if taskTrack != "" {
		if target.TrackID, err = resolveOrCreateTrack(ctx, human, s, taskTrack); err != nil {
			return err
		}
	}
	target.TrackType = trackTypeOf(ctx, s, target.TrackID)

	plan, err := renderRecipePlan(cmd, r, expanded, opts, subj, nil)
	if err != nil {
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
	rep := materializeReport{Verb: verbMaterialized, DryRun: target.DryRun}
	if subj != nil && subj.Type == core.SubjectTask {
		rep.Subject = subj.Display
	}
	if target.TrackID != "" {
		rep.Track, _ = s.GetTrack(ctx, target.TrackID) //nolint:errcheck // display only; the id is still printed on a miss
	}
	if err := printMaterializeSummary(ctx, w, s, plan, res, rep); err != nil {
		return err
	}
	if target.DryRun {
		return nil
	}
	return writeProjection()
}

// resolveOrCreateTrack resolves a --track reference, offering to create
// the track when nothing matches — the same path `task create` takes for
// a hand-made task.
func resolveOrCreateTrack(ctx context.Context, w io.Writer, s *storage.SQLiteStorage, ref string) (string, error) {
	resolved, err := resolveTrackID(ctx, s, ref)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, ErrTrackNotFound) {
		return "", err
	}
	return maybeAutoCreateTrackFromWriter(ctx, w, s, ref)
}
