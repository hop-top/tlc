package workspace

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// --- mocks ---

type mockReader struct {
	tasks []*core.Task
	err   error
}

func (m *mockReader) ListTasks(_ context.Context, _ core.Query) ([]*core.Task, error) {
	return m.tasks, m.err
}

func (m *mockReader) GetTask(_ context.Context, id string) (*core.Task, error) {
	for _, t := range m.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockReader) GetTaskLogs(_ context.Context, _ string) ([]*core.LogEntry, error) {
	return nil, nil
}

func (m *mockReader) CountTasks(_ context.Context, _ core.Query) (int, error) {
	return len(m.tasks), nil
}

type mockSource struct {
	reader  *mockReader
	openErr error
	closed  bool
}

func (m *mockSource) OpenReadOnly() (core.TaskReader, error) {
	if m.openErr != nil {
		return nil, m.openErr
	}
	return m.reader, nil
}

func (m *mockSource) Close() error {
	m.closed = true
	return nil
}

type mockOpener struct {
	sources map[string]*mockSource
}

func (m *mockOpener) Open(proj core.RegisteredProject) (ProjectSource, error) {
	src, ok := m.sources[proj.ProjectID]
	if !ok {
		return nil, errors.New("unknown project")
	}
	return src, nil
}

// --- helpers ---

func strPtr(s string) *string { return &s }

func mkTask(id, title string, status core.TaskStatus, created time.Time) *core.Task {
	return &core.Task{
		ID:        id,
		Title:     title,
		Status:    status,
		CreatedAt: created,
		UpdatedAt: created,
	}
}

func mkProj(id string) core.RegisteredProject {
	return core.RegisteredProject{ProjectID: id, Status: "active"}
}

// --- tests ---

func TestQueryAcross_ZeroProjects(t *testing.T) {
	tasks, err := QueryAcross(context.Background(), nil, core.Query{}, &mockOpener{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestQueryAcross_MergesMultipleProjects(t *testing.T) {
	now := time.Now()
	opener := &mockOpener{sources: map[string]*mockSource{
		"alpha": {reader: &mockReader{tasks: []*core.Task{
			mkTask("a1", "Alpha One", core.StatusTodo, now.Add(-2*time.Hour)),
		}}},
		"beta": {reader: &mockReader{tasks: []*core.Task{
			mkTask("b1", "Beta One", core.StatusDone, now.Add(-1*time.Hour)),
			mkTask("b2", "Beta Two", core.StatusInProgress, now),
		}}},
	}}

	projects := []core.RegisteredProject{mkProj("alpha"), mkProj("beta")}
	tasks, err := QueryAcross(context.Background(), projects, core.Query{}, opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestQueryAcross_SkipsFailedProject(t *testing.T) {
	now := time.Now()
	opener := &mockOpener{sources: map[string]*mockSource{
		"good": {reader: &mockReader{tasks: []*core.Task{
			mkTask("g1", "Good Task", core.StatusTodo, now),
		}}},
		"bad": {openErr: errors.New("db corrupt")},
	}}

	projects := []core.RegisteredProject{mkProj("good"), mkProj("bad")}
	tasks, err := QueryAcross(context.Background(), projects, core.Query{}, opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "g1" {
		t.Fatalf("expected task g1, got %s", tasks[0].ID)
	}
}

func TestQueryAcross_SkipsUnknownProject(t *testing.T) {
	opener := &mockOpener{sources: map[string]*mockSource{}}
	projects := []core.RegisteredProject{mkProj("missing")}
	tasks, err := QueryAcross(context.Background(), projects, core.Query{}, opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestQueryAcross_SortByCreatedAtAsc(t *testing.T) {
	now := time.Now()
	opener := &mockOpener{sources: map[string]*mockSource{
		"p1": {reader: &mockReader{tasks: []*core.Task{
			mkTask("t3", "Third", core.StatusTodo, now),
			mkTask("t1", "First", core.StatusTodo, now.Add(-2*time.Hour)),
		}}},
		"p2": {reader: &mockReader{tasks: []*core.Task{
			mkTask("t2", "Second", core.StatusTodo, now.Add(-1*time.Hour)),
		}}},
	}}

	projects := []core.RegisteredProject{mkProj("p1"), mkProj("p2")}
	tasks, err := QueryAcross(context.Background(), projects, core.Query{
		SortBy:        "created_at",
		SortDirection: "asc",
	}, opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].ID != "t1" || tasks[1].ID != "t2" || tasks[2].ID != "t3" {
		t.Fatalf("wrong order: %s, %s, %s", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestQueryAcross_SortByTitleDesc(t *testing.T) {
	now := time.Now()
	opener := &mockOpener{sources: map[string]*mockSource{
		"p": {reader: &mockReader{tasks: []*core.Task{
			mkTask("a", "Apple", core.StatusTodo, now),
			mkTask("c", "Cherry", core.StatusTodo, now),
			mkTask("b", "Banana", core.StatusTodo, now),
		}}},
	}}

	tasks, err := QueryAcross(context.Background(),
		[]core.RegisteredProject{mkProj("p")},
		core.Query{SortBy: "title", SortDirection: "desc"}, opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tasks[0].Title != "Cherry" || tasks[1].Title != "Banana" || tasks[2].Title != "Apple" {
		t.Fatalf("wrong order: %s, %s, %s", tasks[0].Title, tasks[1].Title, tasks[2].Title)
	}
}

func TestQueryAcross_LimitOffset(t *testing.T) {
	now := time.Now()
	opener := &mockOpener{sources: map[string]*mockSource{
		"p": {reader: &mockReader{tasks: []*core.Task{
			mkTask("t1", "One", core.StatusTodo, now.Add(-3*time.Hour)),
			mkTask("t2", "Two", core.StatusTodo, now.Add(-2*time.Hour)),
			mkTask("t3", "Three", core.StatusTodo, now.Add(-1*time.Hour)),
			mkTask("t4", "Four", core.StatusTodo, now),
		}}},
	}}

	tasks, err := QueryAcross(context.Background(),
		[]core.RegisteredProject{mkProj("p")},
		core.Query{SortBy: "created_at", SortDirection: "asc", Limit: 2, Offset: 1},
		opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].ID != "t2" || tasks[1].ID != "t3" {
		t.Fatalf("wrong tasks: %s, %s", tasks[0].ID, tasks[1].ID)
	}
}

func TestQueryAcross_SetsProjectID(t *testing.T) {
	now := time.Now()
	existing := "already-set"
	opener := &mockOpener{sources: map[string]*mockSource{
		"proj-a": {reader: &mockReader{tasks: []*core.Task{
			mkTask("t1", "No PID", core.StatusTodo, now),
			{
				ID: "t2", Title: "Has PID", Status: core.StatusTodo,
				CreatedAt: now, UpdatedAt: now,
				ProjectID: &existing,
			},
		}}},
	}}

	tasks, err := QueryAcross(context.Background(),
		[]core.RegisteredProject{mkProj("proj-a")},
		core.Query{}, opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Task without ProjectID gets it set.
	if tasks[0].ProjectID == nil || *tasks[0].ProjectID != "proj-a" {
		t.Fatalf("expected ProjectID 'proj-a' on t1, got %v", tasks[0].ProjectID)
	}
	// Task with existing ProjectID keeps it.
	if *tasks[1].ProjectID != "already-set" {
		t.Fatalf("expected ProjectID 'already-set' on t2, got %s", *tasks[1].ProjectID)
	}
}

func TestQueryAcross_OffsetBeyondLength(t *testing.T) {
	now := time.Now()
	opener := &mockOpener{sources: map[string]*mockSource{
		"p": {reader: &mockReader{tasks: []*core.Task{
			mkTask("t1", "One", core.StatusTodo, now),
		}}},
	}}

	tasks, err := QueryAcross(context.Background(),
		[]core.RegisteredProject{mkProj("p")},
		core.Query{Offset: 100}, opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tasks != nil {
		t.Fatalf("expected nil, got %d tasks", len(tasks))
	}
}

func TestQueryAcross_ListError(t *testing.T) {
	opener := &mockOpener{sources: map[string]*mockSource{
		"p": {reader: &mockReader{err: errors.New("query failed")}},
	}}

	tasks, err := QueryAcross(context.Background(),
		[]core.RegisteredProject{mkProj("p")},
		core.Query{}, opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestSortTasks_DefaultsToCreatedAtAsc(t *testing.T) {
	now := time.Now()
	tasks := []*core.Task{
		mkTask("b", "B", core.StatusTodo, now),
		mkTask("a", "A", core.StatusTodo, now.Add(-1*time.Hour)),
	}
	sortTasks(tasks, "", "")
	if tasks[0].ID != "a" {
		t.Fatalf("expected 'a' first, got %s", tasks[0].ID)
	}
}

// T-0598: workspace mode preserves IN_PROGRESS-first ordering.
// After QueryAcross merges tasks from multiple projects, a stable sort
// with StatusPriority = "IN_PROGRESS" must bubble IN_PROGRESS tasks to
// the top while preserving the base sort order within each status group.
// This mirrors the post-merge sort in runTaskListWorkspace (task_list.go).
func TestQueryAcross_StatusPriorityPreservesINPROGRESSFirst(t *testing.T) {
	now := time.Now()
	opener := &mockOpener{sources: map[string]*mockSource{
		"proj-a": {reader: &mockReader{tasks: []*core.Task{
			mkTask("a1", "A TODO", core.StatusTodo, now.Add(-3*time.Hour)),
			mkTask("a2", "A IN_PROGRESS", core.StatusInProgress, now.Add(-2*time.Hour)),
		}}},
		"proj-b": {reader: &mockReader{tasks: []*core.Task{
			mkTask("b1", "B TODO", core.StatusTodo, now.Add(-1*time.Hour)),
			mkTask("b2", "B IN_PROGRESS", core.StatusInProgress, now),
		}}},
	}}

	projects := []core.RegisteredProject{mkProj("proj-a"), mkProj("proj-b")}
	q := core.Query{
		SortBy:         "created_at",
		SortDirection:  "desc",
		StatusPriority: string(core.StatusInProgress),
	}

	tasks, err := QueryAcross(context.Background(), projects, q, opener)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}

	// QueryAcross ignores StatusPriority; the caller (runTaskListWorkspace)
	// applies a post-merge stable sort. Use the same sort.SliceStable call
	// as task_list.go to avoid reimplementing the comparator.
	prio := core.TaskStatus(q.StatusPriority)
	sort.SliceStable(tasks, func(i, j int) bool {
		ip := tasks[i].Status == prio
		jp := tasks[j].Status == prio
		return ip && !jp
	})

	// First two must be IN_PROGRESS.
	if tasks[0].Status != core.StatusInProgress {
		t.Errorf("tasks[0].Status = %q, want IN_PROGRESS", tasks[0].Status)
	}
	if tasks[1].Status != core.StatusInProgress {
		t.Errorf("tasks[1].Status = %q, want IN_PROGRESS", tasks[1].Status)
	}
	// Last two must be TODO.
	if tasks[2].Status != core.StatusTodo {
		t.Errorf("tasks[2].Status = %q, want TODO", tasks[2].Status)
	}
	if tasks[3].Status != core.StatusTodo {
		t.Errorf("tasks[3].Status = %q, want TODO", tasks[3].Status)
	}

	// Within the IN_PROGRESS group, base sort (created_at desc) preserved:
	// b2 (newest) before a2 (older).
	if tasks[0].ID != "b2" {
		t.Errorf("tasks[0].ID = %q, want b2 (newest IN_PROGRESS)", tasks[0].ID)
	}
	if tasks[1].ID != "a2" {
		t.Errorf("tasks[1].ID = %q, want a2 (older IN_PROGRESS)", tasks[1].ID)
	}
}

func TestSourceClose(t *testing.T) {
	now := time.Now()
	src := &mockSource{reader: &mockReader{tasks: []*core.Task{
		mkTask("t1", "Task", core.StatusTodo, now),
	}}}
	opener := &mockOpener{sources: map[string]*mockSource{"p": src}}

	_, _ = QueryAcross(context.Background(),
		[]core.RegisteredProject{mkProj("p")},
		core.Query{}, opener)

	if !src.closed {
		t.Fatal("expected source to be closed after query")
	}
}
