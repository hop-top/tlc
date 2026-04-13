package core

import (
	"context"
	"testing"
	"time"

	"hop.top/kit/domain"
)

// mockDomainRepo implements domain.Repository[Task] for testing.
type mockDomainRepo struct {
	tasks   map[string]*Task
	creates int
	updates int
}

func newMockDomainRepo() *mockDomainRepo {
	return &mockDomainRepo{tasks: make(map[string]*Task)}
}

func (m *mockDomainRepo) Create(_ context.Context, t *Task) error {
	m.creates++
	m.tasks[t.GetID()] = t
	return nil
}

func (m *mockDomainRepo) Get(_ context.Context, id string) (*Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *mockDomainRepo) List(_ context.Context, _ domain.Query) ([]Task, error) {
	var result []Task
	for _, t := range m.tasks {
		result = append(result, *t)
	}
	return result, nil
}

func (m *mockDomainRepo) Update(_ context.Context, t *Task) error {
	m.updates++
	m.tasks[t.GetID()] = t
	return nil
}

func (m *mockDomainRepo) Delete(_ context.Context, id string) error {
	delete(m.tasks, id)
	return nil
}

func TestTaskService_WithDomainRepo_Create(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	dr := newMockDomainRepo()
	svc := NewTaskService(repo, repo, WithDomainRepo(dr))

	ctx := context.Background()
	task := &Task{
		ID:        "T-0001",
		Title:     "domain test",
		Status:    StatusTodo,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := svc.CreateTask(ctx, task, "tester", "via domain"); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Domain repo should have received the create.
	if dr.creates != 1 {
		t.Errorf("domain repo creates = %d, want 1", dr.creates)
	}
	if _, ok := dr.tasks["T-0001"]; !ok {
		t.Error("task not found in domain repo")
	}

	// Log should still be recorded via logRepo.
	if len(repo.logs) != 1 {
		t.Errorf("logs = %d, want 1", len(repo.logs))
	}
	if repo.logs[0].Action != ActionCreated {
		t.Errorf("log action = %q, want %q", repo.logs[0].Action, ActionCreated)
	}
}

func TestTaskService_WithDomainRepo_Update(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	dr := newMockDomainRepo()
	svc := NewTaskService(repo, repo, WithDomainRepo(dr))

	ctx := context.Background()
	task := &Task{
		ID:        "T-0002",
		Title:     "original",
		Status:    StatusTodo,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Seed via domain repo directly.
	dr.tasks["T-0002"] = task

	task.Title = "updated"
	task.UpdatedAt = time.Now().UTC()
	if err := svc.UpdateTask(ctx, task, "tester", ""); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	if dr.updates != 1 {
		t.Errorf("domain repo updates = %d, want 1", dr.updates)
	}
	if dr.tasks["T-0002"].Title != "updated" {
		t.Errorf("title = %q, want %q", dr.tasks["T-0002"].Title, "updated")
	}
}

// mockDomainTrackRepo implements domain.Repository[Track] for testing.
type mockDomainTrackRepo struct {
	tracks  map[string]*Track
	creates int
	updates int
	deletes int
}

func newMockDomainTrackRepo() *mockDomainTrackRepo {
	return &mockDomainTrackRepo{tracks: make(map[string]*Track)}
}

func (m *mockDomainTrackRepo) Create(_ context.Context, t *Track) error {
	m.creates++
	m.tracks[t.GetID()] = t
	return nil
}

func (m *mockDomainTrackRepo) Get(_ context.Context, id string) (*Track, error) {
	t, ok := m.tracks[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *mockDomainTrackRepo) List(_ context.Context, _ domain.Query) ([]Track, error) {
	var result []Track
	for _, t := range m.tracks {
		result = append(result, *t)
	}
	return result, nil
}

func (m *mockDomainTrackRepo) Update(_ context.Context, t *Track) error {
	m.updates++
	m.tracks[t.GetID()] = t
	return nil
}

func (m *mockDomainTrackRepo) Delete(_ context.Context, id string) error {
	m.deletes++
	delete(m.tracks, id)
	return nil
}

func TestTrackService_WithDomainRepo_Create(t *testing.T) {
	trackRepo := newStubTrackRepo()
	taskRepo := NewMockRepository()
	dr := newMockDomainTrackRepo()
	svc := NewTrackService(trackRepo, taskRepo, WithDomainTrackRepo(dr))

	ctx := context.Background()
	track := &Track{
		ID:    "test-track",
		Title: "Test",
		Type:  TrackTypeFeature,
	}

	if err := svc.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	if dr.creates != 1 {
		t.Errorf("domain repo creates = %d, want 1", dr.creates)
	}
	if _, ok := dr.tracks[track.GetID()]; !ok {
		t.Errorf("track not found in domain repo (key=%q)", track.GetID())
	}
}

func TestTrackService_WithDomainRepo_Delete(t *testing.T) {
	trackRepo := newStubTrackRepo()
	taskRepo := NewMockRepository()
	dr := newMockDomainTrackRepo()
	svc := NewTrackService(trackRepo, taskRepo, WithDomainTrackRepo(dr))

	ctx := context.Background()
	if err := svc.DeleteTrack(ctx, "some-track"); err != nil {
		t.Fatalf("DeleteTrack: %v", err)
	}

	if dr.deletes != 1 {
		t.Errorf("domain repo deletes = %d, want 1", dr.deletes)
	}
}

// mockDomainFlowRunRepo implements domain.Repository[FlowRun] for testing.
type mockDomainFlowRunRepo struct {
	runs    map[string]*FlowRun
	creates int
	updates int
}

func newMockDomainFlowRunRepo() *mockDomainFlowRunRepo {
	return &mockDomainFlowRunRepo{runs: make(map[string]*FlowRun)}
}

func (m *mockDomainFlowRunRepo) Create(_ context.Context, r *FlowRun) error {
	m.creates++
	m.runs[r.GetID()] = r
	return nil
}

func (m *mockDomainFlowRunRepo) Get(_ context.Context, id string) (*FlowRun, error) {
	r, ok := m.runs[id]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *mockDomainFlowRunRepo) List(_ context.Context, _ domain.Query) ([]FlowRun, error) {
	var result []FlowRun
	for _, r := range m.runs {
		result = append(result, *r)
	}
	return result, nil
}

func (m *mockDomainFlowRunRepo) Update(_ context.Context, r *FlowRun) error {
	m.updates++
	m.runs[r.GetID()] = r
	return nil
}

func (m *mockDomainFlowRunRepo) Delete(_ context.Context, id string) error {
	delete(m.runs, id)
	return nil
}

func TestTaskService_WithFlowDomainRepo_PauseResume(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	fr := newMockDomainFlowRunRepo()

	// Seed a running flow run.
	fr.runs["run-001"] = &FlowRun{
		ID:     "run-001",
		FlowID: "flow-a",
		Status: FlowStatusRunning,
	}

	svc := NewTaskService(repo, repo, WithFlowDomainRepo(fr))
	ctx := context.Background()

	if err := svc.PauseFlowRun(ctx, "run-001", "tester", "testing pause"); err != nil {
		t.Fatalf("PauseFlowRun: %v", err)
	}

	if fr.runs["run-001"].Status != FlowStatusPaused {
		t.Errorf("status = %q, want %q", fr.runs["run-001"].Status, FlowStatusPaused)
	}
	if fr.updates != 1 {
		t.Errorf("domain repo updates = %d, want 1", fr.updates)
	}

	if err := svc.ResumeFlowRun(ctx, "run-001", "tester", "testing resume"); err != nil {
		t.Fatalf("ResumeFlowRun: %v", err)
	}

	if fr.runs["run-001"].Status != FlowStatusRunning {
		t.Errorf("status = %q, want %q", fr.runs["run-001"].Status, FlowStatusRunning)
	}
}

func TestTaskService_WithoutDomainRepo_FallsBack(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	svc := NewTaskService(repo, repo) // no domain repo

	ctx := context.Background()
	task := &Task{
		ID:        "T-0003",
		Title:     "fallback",
		Status:    StatusTodo,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := svc.CreateTask(ctx, task, "tester", "legacy path"); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Should use the legacy repo directly.
	if _, ok := repo.tasks["T-0003"]; !ok {
		t.Error("task not found in legacy repo")
	}
}
