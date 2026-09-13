package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

var (
	recipeRunsAllProjects bool
	recipeRunsTrack       string
)

const recipeRunCreatedLayout = "2006-01-02 15:04"

var recipeRunsCmd = &cobra.Command{
	Use:   "runs [<recipe>[@<version>]]",
	Short: "List recipe runs: which recipe created tasks, where and when",
	Long: `List the run ledger newest first: one row per materialization with the
recipe and version it used, the track it wrote into, the subject it was
applied to, the number of tasks it created, and who ran it.

Runs are scoped to the detected project unless --all-projects is set.
Name a recipe (optionally pinned as name@version) to keep its runs only;
--track keeps the runs of one track. --format json or yaml prints the
full ledger rows.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: recipeNameCompletion,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: runRecipeRuns,
}

// recipeRunRow is the table view of a run.
type recipeRunRow struct {
	Run     string `table:"RUN"`
	Recipe  string `table:"RECIPE"`
	Version string `table:"VERSION"`
	Track   string `table:"TRACK"`
	Subject string `table:"SUBJECT"`
	Tasks   int    `table:"TASKS"`
	Created string `table:"CREATED"`
	By      string `table:"BY"`
}

// recipeRunView is the json/yaml view: the ledger row plus its task count.
type recipeRunView struct {
	core.RecipeRun `yaml:",inline"`
	Tasks          int `json:"tasks" yaml:"tasks"`
}

func runRecipeRuns(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	s, err := getStorageRaw()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	query := core.RecipeRunQuery{AllProjects: recipeRunsAllProjects}
	var version string
	if len(args) == 1 {
		query.RecipeID, version, _ = strings.Cut(args[0], "@")
	}
	if recipeRunsTrack != "" {
		if query.TrackID, err = resolveTrackID(ctx, s, recipeRunsTrack); err != nil {
			return err
		}
	}
	runs, err := s.ListRecipeRuns(ctx, query)
	if err != nil {
		return fmt.Errorf("list recipe runs: %w", err)
	}
	views, err := recipeRunViews(ctx, s, runs, version)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	format := viper.GetString("output.format")
	switch format {
	case formatJSON, formatYAML:
		return renderRecipeOutput(out, format, views)
	}
	if len(views) == 0 {
		_, _ = fmt.Fprintln(out, "No recipe runs found")
		return nil
	}
	return renderRecipeTable(out, format, recipeRunRows(ctx, s, views))
}

// recipeRunViews pairs each run with its task count, keeping only the
// pinned version when one was given.
func recipeRunViews(ctx context.Context, s *storage.SQLiteStorage, runs []*core.RecipeRun, version string) ([]recipeRunView, error) {
	views := make([]recipeRunView, 0, len(runs))
	for _, run := range runs {
		if version != "" && run.Version != version {
			continue
		}
		rows, err := s.ListRecipeRunTasks(ctx, run.ID)
		if err != nil {
			return nil, fmt.Errorf("list tasks of run %s: %w", run.ID, err)
		}
		views = append(views, recipeRunView{RecipeRun: *run, Tasks: len(rows)})
	}
	return views, nil
}

func recipeRunRows(ctx context.Context, s *storage.SQLiteStorage, views []recipeRunView) []recipeRunRow {
	labels := &refLabels{s: s, tracks: map[string]string{}}
	rows := make([]recipeRunRow, len(views))
	for i, v := range views {
		rows[i] = recipeRunRow{
			Run:     v.ID,
			Recipe:  v.RecipeID,
			Version: v.Version,
			Track:   labels.track(ctx, v.TrackID),
			Subject: labels.subject(ctx, v.SubjectType, v.SubjectID),
			Tasks:   v.Tasks,
			Created: v.CreatedAt.UTC().Format(recipeRunCreatedLayout),
			By:      v.CreatedBy,
		}
	}
	return rows
}

// refLabels renders track and task ids as their display aliases, caching
// track lookups across rows.
type refLabels struct {
	s      *storage.SQLiteStorage
	tracks map[string]string
}

func (l *refLabels) track(ctx context.Context, id string) string {
	if id == "" {
		return ""
	}
	if label, ok := l.tracks[id]; ok {
		return label
	}
	label := id
	if track, err := l.s.GetTrack(ctx, id); err == nil && track != nil {
		label = formatTrackAlias(track)
	}
	l.tracks[id] = label
	return label
}

func (l *refLabels) subject(ctx context.Context, kind, id string) string {
	switch {
	case id == "":
		return ""
	case kind == core.SubjectTrack:
		return kind + " " + l.track(ctx, id)
	case kind == core.SubjectTask:
		if task, err := l.s.GetTask(ctx, id); err == nil && task != nil && task.Seq > 0 {
			return kind + " " + core.FormatTaskAlias(task)
		}
	}
	return strings.TrimSpace(kind + " " + id)
}

func init() {
	recipeRunsCmd.Flags().BoolVar(&recipeRunsAllProjects, "all-projects", false, "Show runs from all projects")
	recipeRunsCmd.Flags().StringVar(&recipeRunsTrack, "track", "", "Only runs that wrote into this track")
}
