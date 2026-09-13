package cli

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

var recipeDiffRecipe string

// Step states reported by recipe diff.
const (
	diffStateMaterialized = "materialized" // in the recipe, ledger row, task present
	diffStateMissing      = "missing"      // in the recipe, no ledger row
	diffStateDeleted      = "deleted"      // ledger row, task gone
	diffStateExtra        = "extra"        // task or ledger row of the run chain, step not in the recipe
	diffNoOrdinal         = "-"
	diffHashExcerpt       = 12
)

var recipeDiffCmd = &cobra.Command{
	Use:   "diff <track>",
	Short: "Compare a track's tasks with the recipe that materialized them",
	Long: `Walk the recipe's expanded steps against the run ledger of a track and
report, per step, whether it is materialized (its task exists), missing
(never materialized), deleted (materialized, task since removed) or extra
(a task of the run chain that the recipe no longer declares).

The recipe is the one behind the track's latest run, resolved through the
search path; --recipe names another reference or a file path. The header
notes when the file's version or content hash differs from what the run
recorded. Nothing is changed; reconcile with track create --recipe.`,
	Args: cobra.ExactArgs(1),
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: runRecipeDiff,
}

// recipeDiffRow is one step of the comparison.
type recipeDiffRow struct {
	Ordinal string `table:"#" json:"ordinal,omitempty" yaml:"ordinal,omitempty"`
	Step    string `table:"STEP" json:"step" yaml:"step"`
	Kind    string `table:"KIND" json:"kind,omitempty" yaml:"kind,omitempty"`
	Task    string `table:"TASK" json:"task,omitempty" yaml:"task,omitempty"`
	Status  string `table:"STATUS" json:"status,omitempty" yaml:"status,omitempty"`
	State   string `table:"STATE" json:"state" yaml:"state"`
}

// recipeDiffView is the structured report.
type recipeDiffView struct {
	Recipe     string          `json:"recipe" yaml:"recipe"`
	Version    string          `json:"version" yaml:"version"`
	Hash       string          `json:"hash" yaml:"hash"`
	Track      string          `json:"track" yaml:"track"`
	Run        string          `json:"run" yaml:"run"`
	RunVersion string          `json:"run_version" yaml:"run_version"`
	RunHash    string          `json:"run_hash" yaml:"run_hash"`
	Drift      []string        `json:"drift,omitempty" yaml:"drift,omitempty"`
	Steps      []recipeDiffRow `json:"steps" yaml:"steps"`
}

func runRecipeDiff(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	s, err := getStorageRaw()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	track, runs, err := trackRuns(ctx, s, args[0])
	if err != nil {
		return err
	}
	ref := recipeDiffRecipe
	if ref == "" {
		ref = runs[0].RecipeID
	}
	r, steps, err := loadExpandedRecipe(ref)
	if err != nil {
		return fmt.Errorf("%w; pass --recipe <path> to compare against a file outside the search path", err)
	}
	chain := runsOfRecipe(runs, r.Name)
	if len(chain) == 0 {
		return fmt.Errorf(
			"track %s has no run of recipe %s; its latest run used %s", formatTrackAlias(track), r.Name, runs[0].RecipeID,
		)
	}
	view, err := buildRecipeDiff(ctx, s, track, r, steps, chain)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	format := viper.GetString("output.format")
	switch format {
	case formatJSON, formatYAML:
		return renderRecipeOutput(out, format, view)
	}
	printRecipeDiffHeader(out, view)
	return renderRecipeTable(out, format, view.Steps)
}

// trackRuns resolves the track and lists its runs newest first.
func trackRuns(ctx context.Context, s *storage.SQLiteStorage, ref string) (*core.Track, []*core.RecipeRun, error) {
	id, err := resolveTrackID(ctx, s, ref)
	if err != nil {
		return nil, nil, err
	}
	track, err := s.GetTrack(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get track %s: %w", id, err)
	}
	if track == nil {
		return nil, nil, newTrackNotFoundError("track %q not found; run 'tlc track list' to see available tracks", ref)
	}
	runs, err := s.ListRecipeRuns(ctx, core.RecipeRunQuery{TrackID: track.ID, ProjectID: trackProjectID(track), AllProjects: true})
	if err != nil {
		return nil, nil, fmt.Errorf("list runs of track %s: %w", formatTrackAlias(track), err)
	}
	if len(runs) == 0 {
		return nil, nil, fmt.Errorf(
			"track %s has no recipe runs; materialize a recipe into it with 'tlc track create --recipe' first",
			formatTrackAlias(track),
		)
	}
	return track, runs, nil
}

func trackProjectID(track *core.Track) string {
	if track.ProjectID == nil {
		return ""
	}
	return *track.ProjectID
}

func runsOfRecipe(runs []*core.RecipeRun, name string) []*core.RecipeRun {
	var out []*core.RecipeRun
	for _, run := range runs {
		if run.RecipeID == name {
			out = append(out, run)
		}
	}
	return out
}

// buildRecipeDiff joins the recipe's steps with the ledger rows of the
// run chain and the track's tasks.
func buildRecipeDiff(
	ctx context.Context, s *storage.SQLiteStorage, track *core.Track,
	r *core.Recipe, steps []core.RecipeStep, chain []*core.RecipeRun,
) (*recipeDiffView, error) {
	latest := chain[0]
	view := &recipeDiffView{
		Recipe: r.Name, Version: r.Version, Hash: r.Hash, Track: formatTrackAlias(track),
		Run: latest.ID, RunVersion: latest.Version, RunHash: latest.Hash, Drift: recipeDriftLines(r, latest),
	}
	ledger, err := chainLedger(ctx, s, track.ID, chain)
	if err != nil {
		return nil, err
	}
	tasks, err := trackTasksByID(ctx, s, track)
	if err != nil {
		return nil, err
	}
	inRecipe := make(map[string]bool, len(steps))
	for _, step := range steps {
		inRecipe[step.ID] = true
		row, err := diffStepRow(ctx, s, step, ledger, tasks)
		if err != nil {
			return nil, err
		}
		view.Steps = append(view.Steps, row)
	}
	for _, stepID := range ledger.order {
		if inRecipe[stepID] {
			continue
		}
		row := recipeDiffRow{Ordinal: diffNoOrdinal, Step: stepID, State: diffStateExtra}
		fillDiffTask(&row, tasks[ledger.taskOf[stepID]])
		view.Steps = append(view.Steps, row)
	}
	return view, nil
}

// diffStepRow classifies one recipe step. A task the ledger maps the
// step to may have left the track, so it is read directly when the track
// listing does not carry it.
func diffStepRow(
	ctx context.Context, s *storage.SQLiteStorage, step core.RecipeStep, ledger *chainLedgerView, tasks map[string]*core.Task,
) (recipeDiffRow, error) {
	row := recipeDiffRow{Ordinal: fmt.Sprint(step.Ordinal), Step: step.ID, Kind: string(step.EffectiveKind())}
	taskID, recorded := ledger.taskOf[step.ID]
	if !recorded {
		row.State = diffStateMissing
		return row, nil
	}
	task := tasks[taskID]
	if task == nil {
		var err error
		if task, err = s.GetTask(ctx, taskID); err != nil {
			return row, fmt.Errorf("step %s: get task %s: %w", step.ID, taskID, err)
		}
	}
	if task == nil {
		row.State = diffStateDeleted
		return row, nil
	}
	row.State = diffStateMaterialized
	fillDiffTask(&row, task)
	return row, nil
}

func fillDiffTask(row *recipeDiffRow, task *core.Task) {
	if task == nil {
		return
	}
	row.Task = core.FormatTaskDisplay(task)
	row.Status = string(task.Status)
	if row.Kind == "" {
		row.Kind = string(task.EffectiveKind())
	}
}

// chainLedgerView is the step→task map of a run chain, later runs
// winning, with step ids in first-seen order.
type chainLedgerView struct {
	taskOf map[string]string
	order  []string
}

func chainLedger(ctx context.Context, s *storage.SQLiteStorage, trackID string, chain []*core.RecipeRun) (*chainLedgerView, error) {
	mine := make(map[string]bool, len(chain))
	for _, run := range chain {
		mine[run.ID] = true
	}
	rows, err := s.ListRecipeRunTasksByTrack(ctx, trackID)
	if err != nil {
		return nil, fmt.Errorf("list run tasks of track %s: %w", trackID, err)
	}
	view := &chainLedgerView{taskOf: map[string]string{}}
	for _, row := range rows {
		if !mine[row.RunID] {
			continue
		}
		if _, seen := view.taskOf[row.StepID]; !seen {
			view.order = append(view.order, row.StepID)
		}
		view.taskOf[row.StepID] = row.TaskID
	}
	return view, nil
}

func trackTasksByID(ctx context.Context, s *storage.SQLiteStorage, track *core.Track) (map[string]*core.Task, error) {
	tasks, err := s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "track_id", Operator: core.OpEq, Value: track.ID},
			{Field: "project_id", Operator: core.OpEq, Value: trackProjectID(track)},
		},
		AllProjects: true, IncludeArchived: true,
	})
	if err != nil {
		return nil, fmt.Errorf("list tasks of track %s: %w", formatTrackAlias(track), err)
	}
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Seq < tasks[j].Seq })
	byID := make(map[string]*core.Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}
	return byID, nil
}

// recipeDriftLines reports where the recipe file differs from what the
// run recorded.
func recipeDriftLines(r *core.Recipe, run *core.RecipeRun) []string {
	var out []string
	if run.Version != r.Version {
		out = append(out, fmt.Sprintf("version: run used %s, file is %s", run.Version, r.Version))
	}
	if run.Hash != r.Hash {
		out = append(out, fmt.Sprintf("content: run used %s, file is %s", shortHash(run.Hash), shortHash(r.Hash)))
	}
	return out
}

func shortHash(h string) string {
	if len(h) > diffHashExcerpt {
		return h[:diffHashExcerpt] + "…"
	}
	if h == "" {
		return "(none)"
	}
	return h
}

func printRecipeDiffHeader(out io.Writer, view *recipeDiffView) {
	_, _ = fmt.Fprintf(out, "Recipe %s@%s vs run %s (%s) on track %s\n", view.Recipe, view.Version, view.Run, view.RunVersion, view.Track)
	for _, line := range view.Drift {
		_, _ = fmt.Fprintf(out, "  drift: %s\n", line)
	}
	_, _ = fmt.Fprintln(out)
}

func init() {
	recipeDiffCmd.Flags().StringVar(&recipeDiffRecipe, "recipe", "", "Recipe to compare (name, name@version or path; default: the latest run's)")
}
