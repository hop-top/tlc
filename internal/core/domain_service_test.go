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
