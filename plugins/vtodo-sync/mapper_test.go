package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// TestTaskRoundTrip exercises the full pipeline:
//
//	plugin Task → core.Task → VCALENDAR → bytes → ParseVCalendar → core.Task → plugin Task
//
// Every mapped field on the way out must match what we put in.
func TestTaskRoundTrip(t *testing.T) {
	due := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	remind := time.Date(2026, 5, 14, 9, 0, 0, 0, time.UTC)
	created := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC)

	original := &Task{
		ID:          "task_01h455vb4pex5vsknk084sn02q",
		Title:       "Replace JWT signer",
		Description: "Rotate to ES256 across services",
		Status:      string(core.StatusInProgress),
		AssignedTo:  "alice",
		Tags:        []string{"security", "auth"},
		Priority:    string(core.PriorityP1),
		Effort:      string(core.EffortM),
		DueAt:       &due,
		RemindAt:    &remind,
		RRule:       "FREQ=DAILY;INTERVAL=2",
		Reference:   "tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn02q",
		CreatedAt:   created,
		UpdatedAt:   updated,
	}

	// Encode: plugin Task → core.Task → VCALENDAR bytes.
	coreIn := taskToCore(original)
	cal, err := vtodo.BuildVCalendar([]*core.Task{coreIn}, nil, nil)
	if err != nil {
		t.Fatalf("BuildVCalendar: %v", err)
	}
	var buf bytes.Buffer
	if err := cal.SerializeTo(&buf); err != nil {
		t.Fatalf("SerializeTo: %v", err)
	}

	// Decode: bytes → core.Task → plugin Task.
	res, err := vtodo.ParseVCalendar(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ParseVCalendar: %v", err)
	}
	if got, want := len(res.Tasks), 1; got != want {
		t.Fatalf("Tasks count = %d, want %d", got, want)
	}
	roundTripped := taskFromCore(res.Tasks[0])

	checkField(t, "ID", roundTripped.ID, original.ID)
	checkField(t, "Title", roundTripped.Title, original.Title)
	checkField(t, "Description", roundTripped.Description, original.Description)
	checkField(t, "Status", roundTripped.Status, original.Status)
	checkField(t, "AssignedTo", roundTripped.AssignedTo, original.AssignedTo)
	checkField(t, "Priority", roundTripped.Priority, original.Priority)
	checkField(t, "Effort", roundTripped.Effort, original.Effort)
	checkField(t, "RRule", roundTripped.RRule, original.RRule)
	checkField(t, "Reference", roundTripped.Reference, original.Reference)
	if !reflect.DeepEqual(roundTripped.Tags, original.Tags) {
		t.Errorf("Tags = %v, want %v", roundTripped.Tags, original.Tags)
	}
	checkTime(t, "DueAt", roundTripped.DueAt, original.DueAt)
	checkTime(t, "RemindAt", roundTripped.RemindAt, original.RemindAt)
	checkTime(t, "CreatedAt", &roundTripped.CreatedAt, &original.CreatedAt)
	checkTime(t, "UpdatedAt", &roundTripped.UpdatedAt, &original.UpdatedAt)
}

// TestTaskToCore_AllStatuses asserts string→typed status conversion is
// stable across every supported status value.
func TestTaskToCore_AllStatuses(t *testing.T) {
	cases := []struct {
		in   string
		want core.TaskStatus
	}{
		{"TODO", core.StatusTodo},
		{"IN_PROGRESS", core.StatusInProgress},
		{"DONE", core.StatusDone},
		{"SKIPPED", core.StatusSkipped},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := taskToCore(&Task{ID: "task_x", Status: c.in})
			if got.Status != c.want {
				t.Errorf("status %q → %q, want %q", c.in, got.Status, c.want)
			}
		})
	}
}

// TestTaskToCore_PointerFields confirms the empty-string optional fields
// stay nil on the core side (vs. pointing at empty strings, which would
// confuse downstream code that branches on != nil).
func TestTaskToCore_PointerFields(t *testing.T) {
	got := taskToCore(&Task{ID: "task_x", Title: "t"})

	if got.AssignedTo != nil {
		t.Errorf("AssignedTo = %q, want nil", *got.AssignedTo)
	}
	if got.ProjectID != nil {
		t.Errorf("ProjectID = %q, want nil", *got.ProjectID)
	}
	if got.TrackID != nil {
		t.Errorf("TrackID = %q, want nil", *got.TrackID)
	}
	if got.BlockedReason != nil {
		t.Errorf("BlockedReason = %q, want nil", *got.BlockedReason)
	}
	if got.DueAt != nil {
		t.Errorf("DueAt = %v, want nil", *got.DueAt)
	}
}

// TestTaskFromCore_PointerFields verifies the inverse: nil pointers on a
// core.Task become zero-valued (empty string) plain fields on the wire.
func TestTaskFromCore_PointerFields(t *testing.T) {
	got := taskFromCore(&core.Task{ID: "task_x", Title: "t"})

	if got.AssignedTo != "" {
		t.Errorf("AssignedTo = %q, want empty", got.AssignedTo)
	}
	if got.ProjectID != "" {
		t.Errorf("ProjectID = %q, want empty", got.ProjectID)
	}
	if got.TrackID != "" {
		t.Errorf("TrackID = %q, want empty", got.TrackID)
	}
}

// TestCloneMeta confirms cloneMeta returns an independent map.
func TestCloneMeta(t *testing.T) {
	src := map[string]interface{}{"k": "v"}
	dst := cloneMeta(src)

	if !reflect.DeepEqual(dst, src) {
		t.Fatalf("cloneMeta drift: %v vs %v", dst, src)
	}
	dst["k"] = "changed"
	if src["k"] == "changed" {
		t.Errorf("cloneMeta returned shared reference (mutation leaked)")
	}

	if cloneMeta(nil) != nil {
		t.Errorf("cloneMeta(nil) should return nil")
	}
}

// --- helpers ---

func checkField(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

func checkTime(t *testing.T, name string, got, want *time.Time) {
	t.Helper()
	switch {
	case got == nil && want == nil:
		return
	case got == nil:
		t.Errorf("%s = nil, want %v", name, *want)
	case want == nil:
		t.Errorf("%s = %v, want nil", name, *got)
	case !got.Equal(*want):
		t.Errorf("%s = %v, want %v", name, *got, *want)
	}
}
