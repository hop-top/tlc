package core

import (
	"context"
	"time"
)

// RecipeRun is one materialization of a recipe: which recipe at which
// version and content hash, with which vars, for which subject, into
// which track. It is the ledger that reconcile keys off — a step whose
// task row is gone is not recreated because the run remembers it was
// created once — and what `recipe runs` reports on.
type RecipeRun struct {
	ID          string                 `json:"id" yaml:"id" table:"Run ID"`
	ProjectID   string                 `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	RecipeID    string                 `json:"recipe_id" yaml:"recipe_id" table:"Recipe"`
	Version     string                 `json:"version,omitempty" yaml:"version,omitempty" table:"Version"`
	Hash        string                 `json:"hash,omitempty" yaml:"hash,omitempty"`
	Vars        map[string]interface{} `json:"vars,omitempty" yaml:"vars,omitempty"`
	SubjectType string                 `json:"subject_type,omitempty" yaml:"subject_type,omitempty"` // task | track
	SubjectID   string                 `json:"subject_id,omitempty" yaml:"subject_id,omitempty"`
	TrackID     string                 `json:"track_id,omitempty" yaml:"track_id,omitempty" table:"Track"`
	Selection   string                 `json:"selection,omitempty" yaml:"selection,omitempty"` // raw --task selector
	DroppedDeps []string               `json:"dropped_deps,omitempty" yaml:"dropped_deps,omitempty"`
	ParentRun   string                 `json:"parent_run,omitempty" yaml:"parent_run,omitempty"` // set by reconcile
	CreatedBy   string                 `json:"created_by,omitempty" yaml:"created_by,omitempty" table:"By"`
	CreatedAt   time.Time              `json:"created_at" yaml:"created_at" table:"Created"`
}

// RecipeRunTask maps one step of a run to the task it created.
type RecipeRunTask struct {
	RunID  string `json:"run_id" yaml:"run_id"`
	StepID string `json:"step_id" yaml:"step_id"`
	TaskID string `json:"task_id" yaml:"task_id"`
}

// RecipeRunQuery filters ListRecipeRuns. Empty fields do not filter.
// Like task queries, results are scoped to the detected project unless
// ProjectID is set explicitly or AllProjects is true.
type RecipeRunQuery struct {
	RecipeID    string
	TrackID     string
	SubjectType string
	SubjectID   string
	ProjectID   string
	AllProjects bool
	Limit       int
	Offset      int
}

// RecipeRunStore is the ledger contract. Narrow on purpose: it is
// implemented by the SQLite store and asserted there, and is not part of
// Repository so mocks and library consumers are not forced to carry it.
type RecipeRunStore interface {
	CreateRecipeRun(ctx context.Context, run *RecipeRun) error
	// GetRecipeRun returns (nil, nil) when no run has that id.
	GetRecipeRun(ctx context.Context, id string) (*RecipeRun, error)
	// ListRecipeRuns returns runs newest first.
	ListRecipeRuns(ctx context.Context, query RecipeRunQuery) ([]*RecipeRun, error)
	// DeleteRecipeRun removes a run and its step→task rows.
	DeleteRecipeRun(ctx context.Context, id string) error
	// AddRecipeRunTasks records the tasks a run created, one per step.
	// RunID on each entry is taken from runID.
	AddRecipeRunTasks(ctx context.Context, runID string, tasks []RecipeRunTask) error
	// ListRecipeRunTasks returns a run's rows in insertion order.
	ListRecipeRunTasks(ctx context.Context, runID string) ([]RecipeRunTask, error)
	// ListRecipeRunTasksByTrack returns the rows of every run on a track,
	// runs oldest first, rows in insertion order — the reconcile input.
	ListRecipeRunTasksByTrack(ctx context.Context, trackID string) ([]RecipeRunTask, error)
}

// TaskClaimer is the compare-and-set the executor uses to take a task:
// exactly one claimant moves the row from `from` to `to`, stamping actor
// and claimed_at, and everyone else sees false rather than an error.
// A claim is refused when the row is not in `from`, is assigned to
// another actor, lives in another project, or does not exist.
type TaskClaimer interface {
	ClaimTask(ctx context.Context, id, projectID string, from, to TaskStatus, actor string, now time.Time) (bool, error)
}
