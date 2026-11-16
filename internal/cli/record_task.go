package cli

import (
	"context"
	"reflect"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// taskRecord is the one structured shape of a task: `task list` and
// `task show` both emit it, in every structured format. No field is
// omitted when unset; it is null (or an empty array/object for
// collections).
type taskRecord struct {
	ID            string                 `json:"id"`
	DisplayID     string                 `json:"display_id"`
	Seq           int64                  `json:"seq"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description"`
	Status        core.TaskStatus        `json:"status"`
	Kind          *string                `json:"kind"`
	AssignedTo    *string                `json:"assigned_to"`
	Priority      *string                `json:"priority"`
	Effort        *string                `json:"effort"`
	Tags          []string               `json:"tags"`
	TrackID       *string                `json:"track_id"`
	DueAt         *time.Time             `json:"due_at"`
	RemindAt      *time.Time             `json:"remind_at"`
	RRule         *string                `json:"rrule"`
	NoAutoRemind  bool                   `json:"no_auto_remind"`
	BlockedBy     []blockerJSON          `json:"blocked_by"`
	BlockedReason *string                `json:"blocked_reason"`
	StaleTimeout  *string                `json:"stale_timeout"`
	StaleFiredAt  *time.Time             `json:"stale_fired_at"`
	Reference     string                 `json:"reference"`
	ProjectID     *string                `json:"project_id"`
	Archived      bool                   `json:"archived"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	OriginSystem  *string                `json:"origin_system"`
	LastSyncAt    *time.Time             `json:"last_sync_at"`
	Meta          map[string]interface{} `json:"meta"`
	Spec          *core.TaskSpec         `json:"spec"`
	Result        map[string]interface{} `json:"result"`
	Attempts      int                    `json:"attempts"`
	ClaimedAt     *time.Time             `json:"claimed_at"`
	RunID         *string                `json:"run_id"`
	StepID        *string                `json:"step_id"`
	StepOrdinal   int                    `json:"step_ordinal"`
}

// taskRecordFields is the documented field set, in output order.
var taskRecordFields = fieldsOf(reflect.TypeOf(taskRecord{}))

// taskColumnFields maps every `task list` table column key to the JSON
// field that carries it. A column without an entry here (or whose field is
// not in taskRecord) fails TestTaskColumnsHaveJSONFields, so a new column
// cannot ship without structured output.
var taskColumnFields = map[string]string{
	"id":       "display_id",
	"project":  "project_id",
	"title":    "title",
	"status":   "status",
	"priority": "priority",
	"assigned": "assigned_to",
	"track":    "track_id",
	"effort":   "effort",
	"due":      "due_at",
	"stale":    "stale_timeout",
	"blocked":  "blocked_by",
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// newTaskRecord builds the record. blockers are the resolved blocked_by
// entries (nil for none).
func newTaskRecord(t *core.Task, blockers []blockerJSON) taskRecord {
	tags := t.Tags
	if tags == nil {
		tags = []string{}
	}
	meta := t.Meta
	if meta == nil {
		meta = map[string]interface{}{}
	}
	if blockers == nil {
		blockers = []blockerJSON{}
	}
	var stale *string
	if t.StaleTimeout != nil {
		s := t.StaleTimeout.String()
		stale = &s
	}
	return taskRecord{
		ID:            t.ID,
		DisplayID:     taskDisplayRef(t),
		Seq:           t.Seq,
		Title:         t.Title,
		Description:   t.Description,
		Status:        t.Status,
		Kind:          nilIfEmpty(string(t.Kind)),
		AssignedTo:    t.AssignedTo,
		Priority:      nilIfEmpty(string(t.Priority)),
		Effort:        nilIfEmpty(string(t.Effort)),
		Tags:          tags,
		TrackID:       t.TrackID,
		DueAt:         t.DueAt,
		RemindAt:      t.RemindAt,
		RRule:         nilIfEmpty(t.RRule),
		NoAutoRemind:  t.NoAutoRemind,
		BlockedBy:     blockers,
		BlockedReason: t.BlockedReason,
		StaleTimeout:  stale,
		StaleFiredAt:  t.StaleFiredAt,
		Reference:     t.Reference,
		ProjectID:     t.ProjectID,
		Archived:      t.Archived,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
		OriginSystem:  t.OriginSystem,
		LastSyncAt:    t.LastSyncAt,
		Meta:          meta,
		Spec:          t.Spec,
		Result:        t.Result,
		Attempts:      t.Attempts,
		ClaimedAt:     t.ClaimedAt,
		RunID:         nilIfEmpty(t.RunID),
		StepID:        nilIfEmpty(t.StepID),
		StepOrdinal:   t.StepOrdinal,
	}
}

// taskRecords builds records for a list. Blockers resolve through the
// same resolver `task show` uses, so both views carry identical
// blocked_by entries. Storage opens lazily, only when a task has edges.
func taskRecords(tasks []*core.Task) []taskRecord {
	out := make([]taskRecord, len(tasks))
	ctx := context.Background()
	var st *storage.SQLiteStorage
	tried := false
	defer func() {
		if st != nil {
			_ = st.Close()
		}
	}()
	for i, t := range tasks {
		refs := t.BlockedBy()
		var bl []blockerJSON
		if len(refs) > 0 {
			if !tried {
				tried = true
				st, _ = getStorageRaw() //nolint:errcheck // nil st degrades to unresolved edges
			}
			var sums []relatedTaskSummary
			if st != nil {
				sums = resolveBlockedBySummaries(ctx, st, st, refs)
			} else {
				sums = make([]relatedTaskSummary, len(refs))
			}
			bl = blockerSummariesToJSON(refs, sums)
		}
		out[i] = newTaskRecord(t, bl)
	}
	return out
}
