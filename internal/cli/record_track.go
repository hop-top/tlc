package cli

import (
	"reflect"
	"time"

	"hop.top/tlc/internal/core"
)

// trackProgressRecord carries progress as numbers; Text is the display
// string the table's Progress column shows ("0/4 (P1/2)").
type trackProgressRecord struct {
	Done         int                  `json:"done"`
	Total        int                  `json:"total"`
	Percent      int                  `json:"percent"`
	CurrentPhase int                  `json:"current_phase"`
	TotalPhases  int                  `json:"total_phases"`
	Phases       []core.PhaseProgress `json:"phases"`
	Text         string               `json:"text"`
}

// trackRecord is the one structured shape of a track, shared by
// `track list` and `track show`.
type trackRecord struct {
	ID         string                 `json:"id"`
	DisplayID  string                 `json:"display_id"`
	Seq        int64                  `json:"seq"`
	Slug       string                 `json:"slug"`
	Title      string                 `json:"title"`
	Type       string                 `json:"type"`
	Status     string                 `json:"status"`
	State      []string               `json:"state"`
	AssignedTo *string                `json:"assigned_to"`
	ProjectID  *string                `json:"project_id"`
	Progress   trackProgressRecord    `json:"progress"`
	DueAt      *time.Time             `json:"due_at"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Meta       map[string]interface{} `json:"meta"`
	PlanMap    map[int]string         `json:"plan_mapping"`
}

// trackShowRecord is trackRecord plus the detail-only parts of `show`.
type trackShowRecord struct {
	trackRecord
	Tasks             []taskRecord            `json:"tasks"`
	ExecutionStrategy *core.ExecutionStrategy `json:"execution_strategy"`
}

var (
	trackRecordFields     = fieldsOf(reflect.TypeOf(trackRecord{}))
	trackShowRecordFields = fieldsOf(reflect.TypeOf(trackShowRecord{}))
)

// trackColumnFields maps every `track list` table column key to the JSON
// field carrying it; see taskColumnFields.
var trackColumnFields = map[string]string{
	"id":       "display_id",
	"slug":     "slug",
	"project":  "project_id",
	"title":    "title",
	"type":     "type",
	"status":   "status",
	"state":    "state",
	"progress": "progress",
	"assignee": "assigned_to",
}

func newTrackRecord(t *core.Track, state []core.TrackStateFlag, p core.TrackProgress) trackRecord {
	flags := make([]string, len(state))
	for i, f := range state {
		flags[i] = string(f)
	}
	pct := 0
	if p.TotalTasks > 0 {
		pct = p.CompletedTasks * 100 / p.TotalTasks
	}
	phases := p.Phases
	if phases == nil {
		phases = []core.PhaseProgress{}
	}
	meta := t.Meta
	if meta == nil {
		meta = map[string]interface{}{}
	}
	pm := t.PlanMapping
	if pm == nil {
		pm = map[int]string{}
	}
	assigned := t.AssignedTo
	if assigned != nil && *assigned == "" {
		assigned = nil
	}
	return trackRecord{
		ID:         t.ID,
		DisplayID:  formatTrackDisplayID(t),
		Seq:        t.Seq,
		Slug:       t.Slug,
		Title:      t.Title,
		Type:       t.Type,
		Status:     string(t.Status),
		State:      flags,
		AssignedTo: assigned,
		ProjectID:  t.ProjectID,
		Progress: trackProgressRecord{
			Done:         p.CompletedTasks,
			Total:        p.TotalTasks,
			Percent:      pct,
			CurrentPhase: p.CurrentPhase,
			TotalPhases:  p.TotalPhases,
			Phases:       phases,
			Text:         core.FormatProgress(p),
		},
		DueAt:     t.DueAt,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
		Meta:      meta,
		PlanMap:   pm,
	}
}
