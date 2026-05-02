package main

import (
	"time"

	"hop.top/tlc/internal/core"
)

// Task is the JSON-RPC wire shape exchanged between tlc and the plugin.
// It mirrors the contract used by plugins/github-sync so consumers can
// treat sync plugins uniformly. Only fields the vtodo encoder/decoder
// touches are kept here; tlc-internal-only fields (Seq, OriginSystem,
// LastSyncAt, Archived, etc.) are passed through Meta when needed.
type Task struct {
	ID            string                 `json:"id"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description,omitempty"`
	Status        string                 `json:"status"`
	AssignedTo    string                 `json:"assigned_to,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	Priority      string                 `json:"priority,omitempty"`
	Effort        string                 `json:"effort,omitempty"`
	BlockedReason *string                `json:"blocked_reason,omitempty"`
	DueAt         *time.Time             `json:"due_at,omitempty"`
	RemindAt      *time.Time             `json:"remind_at,omitempty"`
	RRule         string                 `json:"rrule,omitempty"`
	Reference     string                 `json:"reference,omitempty"`
	ProjectID     string                 `json:"project_id,omitempty"`
	TrackID       string                 `json:"track_id,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Meta          map[string]interface{} `json:"meta,omitempty"`
}

// taskToCore translates the JSON-RPC Task wire shape into the
// internal/core.Task model that the vtodo encoder consumes. This is a
// thin field-by-field copy — the heavy lifting (RFC 5545 wire format)
// happens inside internal/vtodo.
func taskToCore(t *Task) *core.Task {
	if t == nil {
		return nil
	}
	out := &core.Task{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      core.TaskStatus(t.Status),
		Tags:        append([]string(nil), t.Tags...),
		Priority:    core.Priority(t.Priority),
		Effort:      core.Effort(t.Effort),
		Reference:   t.Reference,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		DueAt:       cloneTime(t.DueAt),
		RemindAt:    cloneTime(t.RemindAt),
		RRule:       t.RRule,
		Meta:        cloneMeta(t.Meta),
	}
	if t.AssignedTo != "" {
		v := t.AssignedTo
		out.AssignedTo = &v
	}
	if t.ProjectID != "" {
		v := t.ProjectID
		out.ProjectID = &v
	}
	if t.TrackID != "" {
		v := t.TrackID
		out.TrackID = &v
	}
	if t.BlockedReason != nil {
		v := *t.BlockedReason
		out.BlockedReason = &v
	}
	return out
}

// taskFromCore translates a core.Task back into the JSON-RPC Task wire
// shape. Inverse of taskToCore.
func taskFromCore(t *core.Task) *Task {
	if t == nil {
		return nil
	}
	out := &Task{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		Tags:        append([]string(nil), t.Tags...),
		Priority:    string(t.Priority),
		Effort:      string(t.Effort),
		Reference:   t.Reference,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		DueAt:       cloneTime(t.DueAt),
		RemindAt:    cloneTime(t.RemindAt),
		RRule:       t.RRule,
		Meta:        cloneMeta(t.Meta),
	}
	if t.AssignedTo != nil {
		out.AssignedTo = *t.AssignedTo
	}
	if t.ProjectID != nil {
		out.ProjectID = *t.ProjectID
	}
	if t.TrackID != nil {
		out.TrackID = *t.TrackID
	}
	if t.BlockedReason != nil {
		v := *t.BlockedReason
		out.BlockedReason = &v
	}
	return out
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}

func cloneMeta(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
