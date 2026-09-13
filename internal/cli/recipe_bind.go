package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// recipeSubject is the task or track a recipe is applied to: what the
// run records and what subject.* renders from.
type recipeSubject struct {
	Type    string // core.SubjectTask or core.SubjectTrack
	ID      string // durable id, recorded on the run
	TrackID string // the task's track, or the track itself; "" when trackless
	Display string // T-NNNN or the track slug, for messages
	Task    *core.Task
	Fields  map[string]any
}

// resolveRecipeSubject resolves a --for reference. Task-shaped references
// (task_<typeid>, T-NNNN, bare digits) are tasks; anything else is tried
// as a track (typeid, L-NNNN, slug) and then as a literal task id.
func resolveRecipeSubject(ctx context.Context, s *storage.SQLiteStorage, ref string) (*recipeSubject, error) {
	ref = strings.TrimSpace(ref)
	candidate := strings.TrimPrefix(ref, "@")
	if core.IsTaskID(candidate) || taskRefAliasRe.MatchString(candidate) || taskRefDigitsRe.MatchString(candidate) {
		id, err := parseTaskRefForCLI(ctx, s, ref)
		if err != nil {
			return nil, err
		}
		task, err := s.GetTask(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("--for %s: load task: %w", ref, err)
		}
		if task == nil {
			return nil, fmt.Errorf("--for %s: task not found; run 'tlc task list' to see available tasks", ref)
		}
		return subjectFromTask(ctx, s, task), nil
	}
	if trackID, err := core.ParseTrackRef(ctx, s, currentProjectID(), ref); err == nil {
		track, err := s.GetTrack(ctx, trackID)
		if err != nil {
			return nil, fmt.Errorf("--for %s: load track: %w", ref, err)
		}
		if track != nil {
			return subjectFromTrack(track), nil
		}
	}
	task, err := s.GetTask(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("--for %s: load task: %w", ref, err)
	}
	if task != nil {
		return subjectFromTask(ctx, s, task), nil
	}
	return nil, fmt.Errorf(
		"--for %s: no task or track by that reference; run 'tlc task list' or 'tlc track list'", ref,
	)
}

// subjectFromTask binds subject.* from a task. The track field carries
// the track's slug when it resolves, else the raw id.
func subjectFromTask(ctx context.Context, s *storage.SQLiteStorage, task *core.Task) *recipeSubject {
	subj := &recipeSubject{Type: core.SubjectTask, ID: task.ID, Display: formatTaskAlias(task), Task: task}
	trackField := ""
	if task.TrackID != nil && *task.TrackID != "" {
		subj.TrackID = *task.TrackID
		trackField = subj.TrackID
		if track, err := s.GetTrack(ctx, subj.TrackID); err == nil && track != nil {
			trackField = formatTrackAlias(track)
		}
	}
	project := ""
	if task.ProjectID != nil {
		project = *task.ProjectID
	}
	subj.Fields = map[string]any{
		"id": subj.Display, "title": task.Title, "description": task.Description,
		"track": trackField, "tags": strings.Join(task.Tags, ","), "project": project,
	}
	return subj
}

// subjectFromTrack binds subject.* from a track; id and track are both
// the slug, description and tags are empty.
func subjectFromTrack(track *core.Track) *recipeSubject {
	project := ""
	if track.ProjectID != nil {
		project = *track.ProjectID
	}
	slug := formatTrackAlias(track)
	return &recipeSubject{
		Type: core.SubjectTrack, ID: track.ID, TrackID: track.ID, Display: slug,
		Fields: map[string]any{
			"id": slug, "title": track.Title, "description": "", "track": slug, "tags": "", "project": project,
		},
	}
}

// enforceRecipeSubject checks requires.subject against what was bound.
// fix names how the caller supplies a subject ("pass --for <task>").
func enforceRecipeSubject(r *core.Recipe, subj *recipeSubject, fix string) error {
	want := r.Requires.Subject
	if want == "" || want == core.SubjectNone {
		return nil
	}
	if subj == nil {
		return fmt.Errorf("recipe %s requires a %s subject; %s", r.Name, want, fix)
	}
	if subj.Type != want {
		return fmt.Errorf("recipe %s requires a %s subject, but %s is a %s; %s", r.Name, want, subj.Display, subj.Type, fix)
	}
	return nil
}

// bindRecipeVars resolves the recipe's vars: carried values (an earlier
// run's) under --var, then the --infer stub, then the missing required
// ones — prompted for on a terminal, otherwise reported together with
// the --var fix.
func bindRecipeVars(cmd *cobra.Command, r *core.Recipe, opts recipeCreateOpts, carried map[string]string) (map[string]any, error) {
	provided, err := core.ParseVarFlags(opts.Vars)
	if err != nil {
		return nil, err //nolint:wrapcheck // the parser names the flag and the expected form
	}
	for k, v := range carried {
		if _, declared := r.Vars[k]; declared && !hasVar(provided, k) {
			provided[k] = v
		}
	}
	if opts.Infer {
		return nil, fmt.Errorf("--infer is not implemented yet; pass the values with --var key=value")
	}
	if missing := missingRecipeVars(r, provided); len(missing) > 0 && interactiveAvailable(cmd) {
		if err := promptRecipeVars(cmd, r, missing, provided); err != nil {
			return nil, err
		}
	}
	vars, err := core.ResolveRecipeVars(r, provided)
	if err != nil {
		return nil, err //nolint:wrapcheck // names every missing var and the --var fix
	}
	return vars, nil
}

func hasVar(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}

// missingRecipeVars lists the required vars without a value, sorted.
func missingRecipeVars(r *core.Recipe, provided map[string]string) []string {
	var missing []string
	for name, def := range r.Vars {
		if def.Required && def.Default == nil && !hasVar(provided, name) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

// promptRecipeVars asks for each missing var on the terminal.
func promptRecipeVars(cmd *cobra.Command, r *core.Recipe, missing []string, provided map[string]string) error {
	for _, name := range missing {
		val := ""
		input := huh.NewInput().Title(name).Description(r.Vars[name].Description).Value(&val).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("%s is required", name)
				}
				return nil
			})
		if err := huh.NewForm(huh.NewGroup(input)).WithOutput(cmd.OutOrStdout()).Run(); err != nil {
			return fmt.Errorf("var %s: aborted; pass it with --var %s=<value>", name, name)
		}
		provided[name] = val
	}
	return nil
}
