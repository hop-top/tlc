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

// exportTime pins the export clock. DTSTAMP is each entity's own
// last-modified instant, so every fixture entity below carries
// UpdatedAt and the clock is only a backstop against a timestamp-less
// entity making a regeneration non-deterministic.
var exportTime = fixedTime

func ptr[T any](v T) *T { return &v }

// emit builds the calendar for one fixture and writes it. A build error
// stops the run with the fixture named: a fixture that cannot be built
// must not be left stale on disk as if it had been regenerated.
func emit(path string, tasks []*core.Task, tracks []*core.Track, logs []*core.LogEntry, opts ...vtodo.Option) {
	cal, err := vtodo.BuildVCalendar(tasks, tracks, logs, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: build calendar: %v\n", path, err)
		os.Exit(1)
	}
	write(path, cal)
}

// write gates cal, encodes it and writes the fixture. A calendar
// that fails the validation gate is never written: the golden documents
// are the conformance claim, so they must pass what the tests enforce.
func write(path string, cal vstar.Calendar) {
	gate(path, cal)
	s, err := vtodo.Serialize(cal)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil { //nolint:gosec // G306: committed golden fixture, world-readable like the rest of the repo
		panic(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, len(s))
}

// gate runs the V* validation gate (docs/VSTAR-CONFORMANCE.md,
// "Validation gate") and exits non-zero on a blocking diagnostic.
// Allowed errors and warnings go to stderr so a regeneration shows
// exactly what the fixtures carry.
func gate(path string, cal vstar.Calendar) {
	r := vtodo.ValidateExport(cal)
	for _, d := range r.Warnings {
		fmt.Fprintf(os.Stderr, "%s: warning %s %s: %s\n", path, d.Code, d.Path, d.Message)
	}
	for _, d := range r.Allowed {
		fmt.Fprintf(os.Stderr, "%s: allowed %s %s: %s\n", path, d.Code, d.Path, d.Message)
	}
	if err := r.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		os.Exit(1)
	}
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
	emit(dir+"/single-task.ics", []*core.Task{base}, nil, nil, vtodo.WithExportTime(exportTime))

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
	emit(
		dir+"/track-with-tasks.ics",
		[]*core.Task{&tt1, &tt2, &tt3},
		[]*core.Track{track}, nil,
		vtodo.WithExportTime(exportTime),
	)

	// 3. recurring-rrule.ics
	rec := *base
	rec.RRule = "FREQ=DAILY;INTERVAL=2"
	emit(dir+"/recurring-rrule.ics", []*core.Task{&rec}, nil, nil, vtodo.WithExportTime(exportTime))

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
	emit(dir+"/with-dependencies.ics", []*core.Task{&blocker, &dep}, nil, nil, vtodo.WithExportTime(exportTime))

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
	emit(
		dir+"/with-logs.ics",
		[]*core.Task{base}, nil, []*core.LogEntry{log1, log2},
		vtodo.WithIncludeLogs(true),
		vtodo.WithExportTime(exportTime),
	)

	// 6. recipe-run.ics: a playthrough (recipe run) through a track, one
	// assignment it materialized, that assignment's turns from the log
	// (one closed, one open) and its X-TLC-RUN edge back to the run.
	// The track is dated so the fixture carries no VS040.
	runTrack := *track
	runTrack.DueAt = ptr(fixedTime.Add(7 * 24 * time.Hour))
	run := &core.RecipeRun{
		ID:        "run_01h455vb4pex5vsknk084sn0r1",
		RecipeID:  "auth-rotation",
		Version:   "1.0.0",
		Hash:      "sha256:0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0",
		TrackID:   runTrack.ID,
		CreatedBy: "alice",
		CreatedAt: fixedTime.Add(-time.Hour),
	}
	step := *base
	step.TrackID = ptr(runTrack.ID)
	step.RunID = run.ID
	step.StepID = "rotate-signer"
	step.ClaimedAt = ptr(fixedTime.Add(4 * time.Hour))
	claim1 := &core.LogEntry{TaskID: step.ID, Timestamp: fixedTime, By: "alice", Action: "CLAIMED", Note: "first attempt"}
	release := &core.LogEntry{TaskID: step.ID, Timestamp: fixedTime.Add(time.Hour), By: "alice", Action: "RELEASED", Note: "handing back"}
	claim2 := &core.LogEntry{TaskID: step.ID, Timestamp: fixedTime.Add(4 * time.Hour), By: "alice", Action: "CLAIMED", Note: "second attempt"}
	emit(
		dir+"/recipe-run.ics",
		[]*core.Task{&step}, []*core.Track{&runTrack}, []*core.LogEntry{claim1, release, claim2},
		vtodo.WithIncludeLogs(true),
		vtodo.WithRecipeRuns([]*core.RecipeRun{run}),
		vtodo.WithExportTime(exportTime),
	)
}
