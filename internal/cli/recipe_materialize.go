package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// recipePlan is a recipe resolved for one invocation: the steps to
// materialize, expanded, selected and rendered, plus what the run ledger
// records about how they were chosen.
type recipePlan struct {
	Recipe    *core.Recipe
	Steps     []core.RecipeStep
	Vars      map[string]any
	Subject   *recipeSubject
	RunID     string
	Selection string
	Dropped   map[string][]string
	Warnings  []string
}

// scope is the create-time template environment: vars, the subject when
// one is bound, and the run being recorded.
func (p *recipePlan) scope() core.Scope {
	sc := core.Scope{Vars: p.Vars, Run: map[string]any{"id": p.RunID, "recipe": p.Recipe.Name, "version": p.Recipe.Version}}
	if p.Subject != nil {
		sc.Subject = p.Subject.Fields
	}
	return sc
}

// renderRecipePlan binds vars, applies --task selection and renders the
// expanded steps. carried holds an earlier run's vars, overridden by
// --var.
func renderRecipePlan(
	cmd *cobra.Command, r *core.Recipe, expanded []core.RecipeStep, opts recipeCreateOpts,
	subj *recipeSubject, carried map[string]string,
) (*recipePlan, error) {
	vars, err := bindRecipeVars(cmd, r, opts, carried)
	if err != nil {
		return nil, err
	}
	plan := &recipePlan{Recipe: r, Vars: vars, Subject: subj, RunID: core.NewRecipeRunID(), Selection: strings.Join(opts.Tasks, ",")}
	sel, err := core.ParseTaskSelector(opts.Tasks)
	if err != nil {
		return nil, err //nolint:wrapcheck // names the offending --task piece and the accepted forms
	}
	selected, dropped, err := core.Select(expanded, sel, opts.WithDeps)
	if err != nil {
		return nil, err //nolint:wrapcheck // names the unknown ordinal or id
	}
	plan.Dropped = dropped
	for _, step := range selected {
		for _, dep := range dropped[step.ID] {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"step %s: dependency on unselected step %s dropped (add it to --task or pass --with-deps)", step.ID, dep,
			))
		}
	}
	plan.Steps, err = core.RenderSteps(selected, plan.scope())
	if err != nil {
		return nil, fmt.Errorf("recipe %s: %w", r.Name, err)
	}
	return plan, nil
}

// recipeTarget is where and how a plan is materialized.
type recipeTarget struct {
	TrackID   string
	TrackType string // stage-gate input; "" when trackless
	ProjectID string
	DryRun    bool
	Recreate  bool
	Reconcile bool // Reconcile against the track's ledger instead of a plain Materialize
}

// materializeRecipe runs the plan through the materializer with the CLI
// create gates installed. Plan warnings (dropped dependencies) lead the
// result's own.
func materializeRecipe(
	ctx context.Context, w io.Writer, s *storage.SQLiteStorage, plan *recipePlan, target recipeTarget, assign bool,
) (*core.MaterializeResult, error) {
	var engine *core.AssignmentEngine
	if assign {
		var err error
		if engine, err = loadAssignmentEngine(); err != nil {
			return nil, err
		}
	}
	warnings := append([]string(nil), plan.Warnings...)
	in := core.MaterializeInput{
		Recipe: plan.Recipe, Steps: plan.Steps, RunID: plan.RunID,
		TrackID: target.TrackID, ProjectID: target.ProjectID, By: core.GetCurrentUser(),
		Vars: plan.Vars, Selection: plan.Selection, DroppedDeps: plan.Dropped,
		Recreate: target.Recreate, DryRun: target.DryRun,
		PreCreate: recipeTaskGate(w, target.ProjectID, target.TrackType, engine, &warnings),
	}
	if plan.Subject != nil {
		in.SubjectType, in.SubjectID = plan.Subject.Type, plan.Subject.ID
	}
	m := &core.Materializer{Repo: s, IDGen: s, Runs: s}
	var res *core.MaterializeResult
	var err error
	if target.Reconcile {
		res, err = m.Reconcile(ctx, in)
	} else {
		res, err = m.Materialize(ctx, in)
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // materializer errors name the recipe and step
	}
	res.Warnings = append(warnings, res.Warnings...)
	return res, nil
}

// blockSubjectOnRunLeaves adds the run's leaves — tasks nothing else in
// the run depends on — to the subject task's blocked_by, so the subject
// waits for the work that decomposes it.
func blockSubjectOnRunLeaves(ctx context.Context, s *storage.SQLiteStorage, subj *recipeSubject, res *core.MaterializeResult) error {
	if subj == nil || subj.Type != core.SubjectTask || len(res.Created) == 0 {
		return nil
	}
	var runTasks []*core.Task
	for _, st := range append(append([]core.StepTask(nil), res.Created...), res.Skipped...) {
		if st.TaskID == "" {
			continue
		}
		task, err := s.GetTask(ctx, st.TaskID)
		if err != nil {
			return fmt.Errorf("load run task %s: %w", st.TaskID, err)
		}
		if task != nil {
			runTasks = append(runTasks, task)
		}
	}
	graph, err := core.NewDepGraph(runTasks)
	if err != nil {
		return fmt.Errorf("run dependency graph: %w", err)
	}
	subject, err := s.GetTask(ctx, subj.ID)
	if err != nil || subject == nil {
		return fmt.Errorf("reload subject %s: %w", subj.Display, err)
	}
	edges := subject.BlockedBy()
	for _, task := range runTasks {
		if len(graph.Dependents(task.ID)) == 0 {
			edges = append(edges, task.ID)
		}
	}
	subject.SetBlockedBy(edges)
	if err := s.UpdateTask(ctx, subject); err != nil {
		return fmt.Errorf("block subject %s on the run: %w", subj.Display, err)
	}
	return nil
}

// materializeView is the structured form of a materialization.
type materializeView struct {
	core.MaterializeResult
	Recipe  string `json:"recipe" yaml:"recipe"`
	Version string `json:"version" yaml:"version"`
	TrackID string `json:"track_id,omitempty" yaml:"track_id,omitempty"`
	Track   string `json:"track,omitempty" yaml:"track,omitempty"`
	Subject string `json:"subject,omitempty" yaml:"subject,omitempty"`
	DryRun  bool   `json:"dry_run,omitempty" yaml:"dry_run,omitempty"`
}

// materializeReport is what the summary says about where a plan went.
type materializeReport struct {
	Verb    string // "Materialized" or "Reconciled"
	Track   *core.Track
	Subject string
	DryRun  bool
}

// structuredOutput reports whether --format asks for json or yaml.
func structuredOutput() (string, bool) {
	format := viper.GetString("output.format")
	return format, format == formatJSON || format == formatYAML
}

// printMaterializeSummary renders the result as text or, under --format
// json|yaml, as one document.
func printMaterializeSummary(
	ctx context.Context, w io.Writer, s core.Repository, plan *recipePlan, res *core.MaterializeResult, rep materializeReport,
) error {
	if format, structured := structuredOutput(); structured {
		view := materializeView{
			MaterializeResult: *res, Recipe: plan.Recipe.Name, Version: plan.Recipe.Version,
			Subject: rep.Subject, DryRun: rep.DryRun,
		}
		if rep.Track != nil {
			view.TrackID, view.Track = rep.Track.ID, formatTrackAlias(rep.Track)
		}
		return renderRecipeOutput(w, format, view)
	}
	_, _ = fmt.Fprintf(w, "%s: created %d task(s)\n", summaryHead(plan, rep), len(res.Created))
	byStep := make(map[string]core.RecipeStep, len(plan.Steps))
	for _, step := range plan.Steps {
		byStep[step.ID] = step
	}
	for _, c := range res.Created {
		alias := "-"
		if c.TaskID != "" {
			alias = taskAliasOf(ctx, s, c.TaskID)
		}
		step := byStep[c.StepID]
		_, _ = fmt.Fprintf(w, "  %s  %s  (%s)\n", alias, c.StepID, step.EffectiveKind())
	}
	if len(res.Skipped) > 0 {
		_, _ = fmt.Fprintf(w, "Skipped (%d):\n", len(res.Skipped))
		for _, sk := range res.Skipped {
			_, _ = fmt.Fprintf(w, "  %s  %s\n", taskAliasOf(ctx, s, sk.TaskID), sk.StepID)
		}
	}
	if len(res.Warnings) > 0 {
		_, _ = fmt.Fprintln(w, "Warnings:")
		for _, warning := range res.Warnings {
			_, _ = fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
	return nil
}

// summaryHead is the first line's subject: verb, recipe key, subject and
// track, e.g. "Materialized review@1.0.0 for T-0042 into L-0003".
func summaryHead(plan *recipePlan, rep materializeReport) string {
	head := rep.Verb + " " + plan.Recipe.Key()
	preposition := "into"
	if rep.Verb == verbReconciled {
		preposition = "on"
	}
	if rep.DryRun {
		head = "Dry run — would " + strings.ToLower(strings.TrimSuffix(rep.Verb, "d")) + " " + plan.Recipe.Key()
	}
	if rep.Subject != "" {
		head += " for " + rep.Subject
	}
	if rep.Track != nil {
		display := formatTrackAlias(rep.Track) // no sequence yet in a dry run
		if rep.Track.Seq > 0 {
			display = formatTrackDisplayID(rep.Track)
		}
		head += " " + preposition + " " + display
	}
	return head
}

// Summary verbs; the dry-run form drops the trailing "d".
const (
	verbMaterialized = "Materialized"
	verbReconciled   = "Reconciled"
)

// taskAliasOf renders a task id as its display alias when the row loads.
func taskAliasOf(ctx context.Context, s core.Repository, id string) string {
	if task, err := s.GetTask(ctx, id); err == nil && task != nil {
		return formatTaskAlias(task)
	}
	return id
}

// trackTypeOf returns a track's type for the stage gate, "" when the
// track is unknown or unset.
func trackTypeOf(ctx context.Context, s *storage.SQLiteStorage, trackID string) string {
	if trackID == "" {
		return ""
	}
	if track, err := s.GetTrack(ctx, trackID); err == nil && track != nil {
		return track.Type
	}
	return ""
}

// createdTaskIDs lists the ids a pass created.
func createdTaskIDs(res *core.MaterializeResult) []string {
	ids := make([]string, 0, len(res.Created))
	for _, c := range res.Created {
		if c.TaskID != "" {
			ids = append(ids, c.TaskID)
		}
	}
	return ids
}
