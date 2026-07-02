package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseTaskRef_TypeID(t *testing.T) {
	repo := newTestRepo()
	id := NewTaskID()
	repo.tasks[id] = &Task{ID: id, Seq: 1}

	got, err := ParseTaskRef(context.Background(), repo, "", id)
	if err != nil {
		t.Fatalf("ParseTaskRef: %v", err)
	}
	if got != id {
		t.Errorf("got %q; want %q", got, id)
	}
}

func TestParseTaskRef_Alias(t *testing.T) {
	repo := newTestRepo()
	id := NewTaskID()
	repo.tasks[id] = &Task{ID: id, Seq: 42}

	got, err := ParseTaskRef(context.Background(), repo, "", "T-0042")
	if err != nil {
		t.Fatalf("ParseTaskRef: %v", err)
	}
	if got != id {
		t.Errorf("got %q; want %q", got, id)
	}
}

func TestParseTaskRef_BareDigits(t *testing.T) {
	repo := newTestRepo()
	id := NewTaskID()
	repo.tasks[id] = &Task{ID: id, Seq: 7}

	got, err := ParseTaskRef(context.Background(), repo, "", "7")
	if err != nil {
		t.Fatalf("ParseTaskRef: %v", err)
	}
	if got != id {
		t.Errorf("got %q; want %q", got, id)
	}
}

func TestParseTaskRef_LargeSeqOverflow(t *testing.T) {
	repo := newTestRepo()
	id := NewTaskID()
	repo.tasks[id] = &Task{ID: id, Seq: 1234567}

	got, err := ParseTaskRef(context.Background(), repo, "", "T-1234567")
	if err != nil {
		t.Fatalf("ParseTaskRef: %v", err)
	}
	if got != id {
		t.Errorf("got %q; want %q", got, id)
	}
}

func TestParseTaskRef_NotFound(t *testing.T) {
	repo := newTestRepo()
	_, err := ParseTaskRef(context.Background(), repo, "", "T-9999")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error; got %v", err)
	}
}

func TestParseTaskRef_Malformed(t *testing.T) {
	repo := newTestRepo()
	cases := []string{"foo", "task_", "T-", "  ", ""}
	for _, c := range cases {
		_, err := ParseTaskRef(context.Background(), repo, "", c)
		if err == nil {
			t.Errorf("ParseTaskRef(%q) expected error, got nil", c)
		}
	}
}

func TestParseTrackRef_TypeID(t *testing.T) {
	repo := newTestRepo()
	id := NewTrackID()
	repo.tracks[id] = &Track{ID: id, Slug: "foo"}

	got, err := ParseTrackRef(context.Background(), repo, "", id)
	if err != nil {
		t.Fatalf("ParseTrackRef: %v", err)
	}
	if got != id {
		t.Errorf("got %q; want %q", got, id)
	}
}

func TestParseTrackRef_Slug(t *testing.T) {
	repo := newTestRepo()
	id := NewTrackID()
	repo.tracks[id] = &Track{ID: id, Slug: "auth-rewrite"}

	got, err := ParseTrackRef(context.Background(), repo, "", "auth-rewrite")
	if err != nil {
		t.Fatalf("ParseTrackRef: %v", err)
	}
	if got != id {
		t.Errorf("got %q; want %q", got, id)
	}
}

func TestFormatTaskAlias(t *testing.T) {
	cases := []struct {
		seq  int64
		want string
	}{
		{1, "T-0001"},
		{42, "T-0042"},
		{9999, "T-9999"},
		{10000, "T-10000"},
		{1234567, "T-1234567"},
	}
	for _, c := range cases {
		got := FormatTaskAlias(&Task{Seq: c.seq})
		if got != c.want {
			t.Errorf("FormatTaskAlias(%d) = %q; want %q", c.seq, got, c.want)
		}
	}
}

// testRepo is an in-memory Repository+TrackRepository for ParseRef tests.
type testRepo struct {
	tasks  map[string]*Task
	tracks map[string]*Track
}

func newTestRepo() *testRepo {
	return &testRepo{
		tasks:  make(map[string]*Task),
		tracks: make(map[string]*Track),
	}
}

func (r *testRepo) GetNextSequenceID(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (r *testRepo) CreateTask(_ context.Context, _ *Task) error { return nil }
func (r *testRepo) GetTask(_ context.Context, id string) (*Task, error) {
	return r.tasks[id], nil
}

func (r *testRepo) GetTaskBySeq(_ context.Context, projectID string, seq int64) (*Task, error) {
	for _, t := range r.tasks {
		var pid string
		if t.ProjectID != nil {
			pid = *t.ProjectID
		}
		if pid == projectID && t.Seq == seq {
			return t, nil
		}
	}
	return nil, nil
}
func (r *testRepo) UpdateTask(_ context.Context, _ *Task) error { return nil }
func (r *testRepo) UpdateTaskWithLog(_ context.Context, _ *Task, _ *LogEntry) error {
	return nil
}
func (r *testRepo) ListTasks(_ context.Context, _ Query) ([]*Task, error)  { return nil, nil }
func (r *testRepo) DeleteTask(_ context.Context, _ string) error           { return nil }
func (r *testRepo) GetTasksNeedingPush(_ context.Context) ([]*Task, error) { return nil, nil }
func (r *testRepo) FindTaskByOrigin(_ context.Context, _, _ string) (*Task, error) {
	return nil, nil
}

func (r *testRepo) ArchiveTasks(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
func (r *testRepo) CreateFlowRun(_ context.Context, _ *FlowRun) error { return nil }
func (r *testRepo) GetFlowRun(_ context.Context, _ string) (*FlowRun, error) {
	return nil, nil
}
func (r *testRepo) UpdateFlowRun(_ context.Context, _ *FlowRun) error           { return nil }
func (r *testRepo) ListFlowRuns(_ context.Context, _ Query) ([]*FlowRun, error) { return nil, nil }

func (r *testRepo) CreateTrack(_ context.Context, _ *Track) error { return nil }
func (r *testRepo) GetTrack(_ context.Context, id string) (*Track, error) {
	return r.tracks[id], nil
}

func (r *testRepo) GetTrackBySlug(_ context.Context, projectID, slug string) (*Track, error) {
	for _, t := range r.tracks {
		var pid string
		if t.ProjectID != nil {
			pid = *t.ProjectID
		}
		if pid == projectID && t.Slug == slug {
			return t, nil
		}
	}
	return nil, nil
}
func (r *testRepo) UpdateTrack(_ context.Context, _ *Track) error { return nil }
func (r *testRepo) DeleteTrack(_ context.Context, _ string) error { return nil }
func (r *testRepo) ListTracks(_ context.Context, _ TrackQuery) ([]*Track, error) {
	return nil, nil
}
