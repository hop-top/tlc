package cli

// `tlc status` prints task and overdue counts as bare facts. They used to
// be measured by taking len() of a capped slice (--limit 50 for the
// assigned counts, 1000 for overdue), so once a project outgrew the cap
// the numbers silently drifted low with no warning and exit 0.
//
// Observed on real data before the fix: 1077 active tasks, of which 4 were
// overdue, but only 3 fell inside the first 1000 rows by created_at DESC --
// status reported "Overdue: 3". The fixtures below reproduce that shape at
// smaller scale, seeding past both caps.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

const (
	// statusSeedInProgress and statusSeedTodo both exceed the old
	// per-status cap of 50.
	statusSeedInProgress = 70
	statusSeedTodo       = 80
	statusOldAssignedCap = 50
	statusSeedUser       = "counts-owner"
	statusProjectID      = "status-fixture"
)

// statusFixtureDB returns a private copy of the shared status fixture:
// assigned active tasks past the old cap, plus one overdue task placed last
// by created_at so any scan cap would miss it.
func statusFixtureDB(t *testing.T) string {
	t.Helper()

	total := statusSeedInProgress + statusSeedTodo
	base := time.Now().UTC().Add(-72 * time.Hour)
	user := statusSeedUser
	due := time.Now().UTC().Add(-48 * time.Hour)

	return seedTemplateDB(t, "status", e2eTaskSeed{
		// One extra row: the overdue straggler.
		Total:   total + 1,
		Project: statusProjectID,
		Mutate: func(i int, task *core.Task) {
			task.AssignedTo = &user

			if i == total {
				// Created oldest so it sorts last under created_at
				// DESC -- the position a scan cap drops first.
				oldest := base.Add(-24 * time.Hour)
				task.ID = "T-9999"
				task.Title = "overdue straggler"
				task.DueAt = &due
				task.CreatedAt = oldest
				task.UpdatedAt = oldest
				return
			}

			if i < statusSeedInProgress {
				task.Status = core.StatusInProgress
			}
			created := base.Add(time.Duration(i) * time.Minute)
			task.CreatedAt = created
			task.UpdatedAt = created
		},
	})
}

func statusFixture(t *testing.T) (bin, cwd string, env []string) {
	t.Helper()
	bin = buildTLCBinary(t)
	dbPath := statusFixtureDB(t)
	env = append(e2eEnv(t, t.TempDir(), dbPath), "TLC_USER="+statusSeedUser)
	return bin, t.TempDir(), env
}

// TestStatus_CountsAreNotCapped asserts the printed counts describe the
// whole match set, not the slice a cap happened to return.
func TestStatus_CountsAreNotCapped(t *testing.T) {
	bin, cwd, env := statusFixture(t)

	out, code := runTLC(t, bin, cwd, env, "status")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	var inProgress, todo int
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "in progress") {
			continue
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(line),
			"Tasks: %d in progress, %d todo (yours)", &inProgress, &todo); err != nil {
			t.Fatalf("parse %q: %v", line, err)
		}
		break
	}

	if inProgress != statusSeedInProgress {
		t.Errorf("in progress = %d, want %d\n%s", inProgress, statusSeedInProgress, out)
	}
	// The overdue straggler is TODO too, so it counts here.
	if wantTodo := statusSeedTodo + 1; todo != wantTodo {
		t.Errorf("todo = %d, want %d\n%s", todo, wantTodo, out)
	}
	// The defect's signature: a count pinned to the old cap.
	if inProgress == statusOldAssignedCap || todo == statusOldAssignedCap {
		t.Errorf("a count equals the old cap of %d -- slice measured, not match set\n%s",
			statusOldAssignedCap, out)
	}
}

// TestStatus_OverdueCountsWholeMatchSet pins the overdue line, the count
// that was demonstrably wrong on real data.
func TestStatus_OverdueCountsWholeMatchSet(t *testing.T) {
	bin, cwd, env := statusFixture(t)

	out, code := runTLC(t, bin, cwd, env, "status")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	// Exactly one task is overdue, and it is the one a scan cap would
	// have dropped, so its absence is the regression.
	if !strings.Contains(out, "Overdue:  1 task(s)") {
		t.Errorf("expected 'Overdue:  1 task(s)', got:\n%s", out)
	}
}

// failingStatusReader fails one of the three status queries and succeeds at
// the rest, so a partial-output bug is reproducible without a real
// SQLITE_BUSY.
type failingStatusReader struct {
	failCountByStatus bool
	failCountTasks    bool
	failList          bool
}

var errStatusQuery = errors.New("database is locked")

func (f failingStatusReader) CountTasksByStatus(
	_ context.Context, _ core.Query,
) (map[string]int, error) {
	if f.failCountByStatus {
		return nil, errStatusQuery
	}
	return map[string]int{string(core.StatusInProgress): 3, string(core.StatusTodo): 7}, nil
}

func (f failingStatusReader) CountTasks(_ context.Context, _ core.Query) (int, error) {
	if f.failCountTasks {
		return 0, errStatusQuery
	}
	return 2, nil
}

func (f failingStatusReader) ListTasks(_ context.Context, _ core.Query) ([]*core.Task, error) {
	if f.failList {
		return nil, errStatusQuery
	}
	return nil, nil
}

// TestCollectStatusSnapshot_FailsBeforeAnyOutput is the F4 regression.
//
// Every count now runs before the first line is written. Writing the
// Project/User/Tasks lines and only then returning an error left a
// half-written snapshot on stdout next to a non-zero exit -- and a caller
// reading those lines cannot tell the report was cut short. Gathering first
// makes it all-or-nothing.
func TestCollectStatusSnapshot_FailsBeforeAnyOutput(t *testing.T) {
	cases := map[string]failingStatusReader{
		"AssignedCountsFail": {failCountByStatus: true},
		"OverdueCountFails":  {failCountTasks: true},
		"PreviewListFails":   {failList: true},
	}

	for name, reader := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := collectStatusSnapshot(context.Background(), reader, "someone")
			if err == nil {
				t.Fatal("expected an error; a failed count must not be silently omitted")
			}
			// The repo's error convention: what failed, why, and a
			// next step.
			if !strings.Contains(err.Error(), errStatusQuery.Error()) {
				t.Errorf("error should wrap the cause, got: %v", err)
			}
			if !strings.Contains(err.Error(), ";") {
				t.Errorf("error should carry a next-step clause, got: %v", err)
			}
		})
	}
}

// TestWriteStatusSnapshot_WritesNothingOnFailure is the load-bearing half of
// F4: not merely that the error surfaces, but that stdout stays empty when it
// does. A header plus a Tasks line followed by a non-zero exit is a truncated
// report a caller cannot detect.
func TestWriteStatusSnapshot_WritesNothingOnFailure(t *testing.T) {
	proj := &core.ProjectDetection{ProjectID: statusProjectID, InProject: true}

	for name, reader := range map[string]failingStatusReader{
		"AssignedCountsFail": {failCountByStatus: true},
		"OverdueCountFails":  {failCountTasks: true},
		"PreviewListFails":   {failList: true},
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			err := writeStatusSnapshot(context.Background(), &buf, reader, proj, "someone")
			if err == nil {
				t.Fatal("expected an error")
			}
			if buf.Len() != 0 {
				t.Errorf("wrote %d bytes before failing -- partial snapshot:\n%s", buf.Len(), buf.String())
			}
		})
	}
}

// TestWriteStatusSnapshot_WritesEverythingOnSuccess is the other side: the
// gather-first ordering must not have dropped a line.
func TestWriteStatusSnapshot_WritesEverythingOnSuccess(t *testing.T) {
	proj := &core.ProjectDetection{ProjectID: statusProjectID, InProject: true}

	var buf bytes.Buffer
	if err := writeStatusSnapshot(
		context.Background(), &buf, failingStatusReader{}, proj, "someone",
	); err != nil {
		t.Fatalf("writeStatusSnapshot: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"Project:  " + statusProjectID,
		"User:     someone",
		"Tasks:    3 in progress, 7 todo (yours)",
		"Overdue:  2 task(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n%s", want, out)
		}
	}
}

// TestStatus_SnapshotOrderIsStable pins the printed shape, so gathering
// before writing did not reorder the report.
func TestStatus_SnapshotOrderIsStable(t *testing.T) {
	bin, cwd, env := statusFixture(t)

	out, code := runTLC(t, bin, cwd, env, "status")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	order := []string{"Project:", "User:", "Tasks:", "Overdue:"}
	prev := -1
	for _, want := range order {
		at := strings.Index(out, want)
		if at < 0 {
			t.Fatalf("missing %q line\n%s", want, out)
		}
		if at < prev {
			t.Errorf("%q out of order\n%s", want, out)
		}
		prev = at
	}
}
