// Command genfixtures-vtodo regenerates the .ics test fixtures under
// tests/fixtures/vtodo. Run from the repo root:
//
//	go run ./cmd/genfixtures-vtodo
//
// This is a developer-only helper, not shipped in releases.
package main

import (
	"fmt"
	"os"
	"time"

	vstar "hop.top/vstar"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

var fixedTime = time.Date(2026, 5, 2, 14, 30, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func write(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, len(content))
}

// mustSerialize encodes cal via vtodo.Serialize and panics on error.
// Replaces *ics.Calendar.Serialize() from the pre-vstar codec era.
func mustSerialize(cal vstar.Calendar) string {
	s, err := vtodo.Serialize(cal)
	if err != nil {
		panic(err)
	}
	return s
}

func main() {
	dir := "tests/fixtures/vtodo"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}

	assignee := "alice"
	due := fixedTime.Add(24 * time.Hour)

	base := &core.Task{
		ID:          "task_01h455vb4pex5vsknk084sn02q",
		Seq:         42,
		Title:       "Replace JWT signer",
		Description: "Rotate to ES256 across services",
		Status:      core.StatusInProgress,
		AssignedTo:  &assignee,
		Tags:        []string{"security", "auth"},
		Reference:   "tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn02q",
		Effort:      core.EffortM,
		Priority:    core.PriorityP1,
		CreatedAt:   fixedTime,
		UpdatedAt:   fixedTime.Add(time.Hour),
		DueAt:       &due,
	}

	// 1. single-task.ics
	cal, _ := vtodo.BuildVCalendar([]*core.Task{base}, nil, nil)
	write(dir+"/single-task.ics", mustSerialize(cal))

	// 2. track-with-tasks.ics
	track := &core.Track{
		ID:        "track_01h455vbqkfsn02nk084ksn02q",
		Slug:      "auth-rewrite",
		Title:     "Auth rewrite",
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusActive,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
	tt1 := *base
	tt1.TrackID = ptr(track.ID)
	tt2 := *base
	tt2.ID = "task_01h455vb4pex5vsknk084sn0aw"
	tt2.Title = "Audit token claims"
	tt2.TrackID = ptr(track.ID)
	tt3 := *base
	tt3.ID = "task_01h455vb4pex5vsknk084sn0ax"
	tt3.Title = "Bench rotation throughput"
	tt3.TrackID = ptr(track.ID)
	cal2, _ := vtodo.BuildVCalendar(
		[]*core.Task{&tt1, &tt2, &tt3},
		[]*core.Track{track}, nil,
	)
	write(dir+"/track-with-tasks.ics", mustSerialize(cal2))

	// 3. recurring-rrule.ics
	rec := *base
	rec.RRule = "FREQ=DAILY;INTERVAL=2"
	cal3, _ := vtodo.BuildVCalendar([]*core.Task{&rec}, nil, nil)
	write(dir+"/recurring-rrule.ics", mustSerialize(cal3))

	// 4. with-dependencies.ics
	blockerID := "task_01h455vb4pex5vsknk084sn0az"
	blocker := *base
	blocker.ID = blockerID
	blocker.Title = "Provision rotation key"

	dep := *base
	dep.ID = "task_01h455vb4pex5vsknk084sn0aa"
	dep.Title = "Wire signer to new key"
	dep.Meta = map[string]interface{}{
		"blocked_by": []string{blockerID},
	}
	cal4, _ := vtodo.BuildVCalendar([]*core.Task{&blocker, &dep}, nil, nil)
	write(dir+"/with-dependencies.ics", mustSerialize(cal4))

	// 5. with-logs.ics
	log1 := &core.LogEntry{
		TaskID:    base.ID,
		Timestamp: fixedTime,
		By:        "alice",
		Action:    "CLAIMED",
		Note:      "starting work on signer rotation",
	}
	log2 := &core.LogEntry{
		TaskID:    base.ID,
		Timestamp: fixedTime.Add(2 * time.Hour),
		By:        "alice",
		Action:    "PROGRESS",
		Note:      "tests passing for ES256 path",
	}
	cal5, _ := vtodo.BuildVCalendar(
		[]*core.Task{base}, nil, []*core.LogEntry{log1, log2},
		vtodo.WithIncludeLogs(true),
	)
	write(dir+"/with-logs.ics", mustSerialize(cal5))
}
