package core

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"
)

// mockBus records published events for assertions.
type mockBus struct {
	mu     sync.Mutex
	events []bus.Event
}

func (b *mockBus) Publish(_ context.Context, e bus.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
	return nil
}

func (b *mockBus) Subscribe(string, bus.Handler) bus.Unsubscribe { return func() {} }
func (b *mockBus) SubscribeAsync(string, bus.AsyncHandler) bus.Unsubscribe {
	return func() {}
}
func (b *mockBus) Close(context.Context) error { return nil }

func (b *mockBus) lastEvent() *bus.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) == 0 {
		return nil
	}
	return &b.events[len(b.events)-1]
}

func (b *mockBus) eventCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

// mockRunStore records CreateAgentRun / UpdateAgentRun calls.
type mockRunStore struct {
	runs map[string]*AgentRunRecord
}

func newMockRunStore() *mockRunStore {
	return &mockRunStore{runs: make(map[string]*AgentRunRecord)}
}

func (s *mockRunStore) CreateAgentRun(_ context.Context, r *AgentRunRecord) error {
	s.runs[r.ID] = r
	return nil
}

func (s *mockRunStore) UpdateAgentRun(_ context.Context, r *AgentRunRecord) error {
	s.runs[r.ID] = r
	return nil
}

func setupStateUpdater(t *testing.T) (*StateUpdater, *MockRepository, *mockBus, *mockRunStore) {
	t.Helper()
	repo := NewMockRepository()
	logRepo := &MockLogRepository{}
	svc := NewTaskService(repo, logRepo)
	b := &mockBus{}
	store := newMockRunStore()
	su := NewStateUpdater(svc, b).WithRunStore(store)
	return su, repo, b, store
}

func TestStateUpdater_Succeeded(t *testing.T) {
	su, repo, _, _ := setupStateUpdater(t)
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &Task{
		ID:     "T-0042",
		Title:  "Fix bug",
		Status: StatusInProgress,
	})

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusSucceeded,
		Summary: "Fixed the bug",
	}

	if err := su.Update(ctx, result, "task", "T-0042", UpdateOpts{}); err != nil {
		t.Fatal(err)
	}

	task, _ := repo.GetTask(ctx, "T-0042")
	if task.Status != StatusDone {
		t.Errorf("status = %q, want DONE", task.Status)
	}
}

func TestStateUpdater_Failed(t *testing.T) {
	su, repo, _, _ := setupStateUpdater(t)
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &Task{
		ID:          "T-0042",
		Title:       "Fix bug",
		Description: "Original description",
		Status:      StatusInProgress,
	})

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusFailed,
		Summary: "Could not reproduce",
	}

	if err := su.Update(ctx, result, "task", "T-0042", UpdateOpts{}); err != nil {
		t.Fatal(err)
	}

	task, _ := repo.GetTask(ctx, "T-0042")
	if task.Status != StatusInProgress {
		t.Errorf("status = %q, want IN_PROGRESS", task.Status)
	}
	if !strings.Contains(task.Description, "[agent:failed]") {
		t.Errorf("description = %q, should contain failure note", task.Description)
	}
}

func TestStateUpdater_Partial(t *testing.T) {
	su, repo, _, _ := setupStateUpdater(t)
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &Task{
		ID:     "T-0042",
		Title:  "Multi-step",
		Status: StatusInProgress,
	})

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusPartial,
		Summary: "Completed 2 of 3 steps",
	}

	if err := su.Update(ctx, result, "task", "T-0042", UpdateOpts{}); err != nil {
		t.Fatal(err)
	}

	task, _ := repo.GetTask(ctx, "T-0042")
	if task.Status != StatusInProgress {
		t.Errorf("status = %q, want IN_PROGRESS", task.Status)
	}
	if !strings.Contains(task.Description, "[agent:partial]") {
		t.Errorf("description = %q", task.Description)
	}
}

func TestStateUpdater_Timeout(t *testing.T) {
	su, repo, _, _ := setupStateUpdater(t)
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &Task{
		ID:     "T-0042",
		Title:  "Slow task",
		Status: StatusInProgress,
	})

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusTimeout,
		Summary: "timed out",
	}

	if err := su.Update(ctx, result, "task", "T-0042", UpdateOpts{}); err != nil {
		t.Fatal(err)
	}

	task, _ := repo.GetTask(ctx, "T-0042")
	if task.Status != StatusInProgress {
		t.Errorf("status = %q, want IN_PROGRESS", task.Status)
	}
	if !strings.Contains(task.Description, "[agent:timeout]") {
		t.Errorf("description = %q", task.Description)
	}
}

func TestStateUpdater_NoStateUpdate(t *testing.T) {
	su, repo, _, _ := setupStateUpdater(t)
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &Task{
		ID:     "T-0042",
		Title:  "Test",
		Status: StatusInProgress,
	})

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusSucceeded,
		Summary: "Done",
	}

	err := su.Update(ctx, result, "task", "T-0042", UpdateOpts{
		NoStateUpdate: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Task should remain IN_PROGRESS.
	task, _ := repo.GetTask(ctx, "T-0042")
	if task.Status != StatusInProgress {
		t.Errorf("status = %q, want IN_PROGRESS (no state update)", task.Status)
	}
}

func TestStateUpdater_CreateRun(t *testing.T) {
	su, _, b, store := setupStateUpdater(t)
	ctx := context.Background()

	record := &AgentRunRecord{
		ID:         "run-001",
		Agent:      "claude",
		TargetType: "task",
		TargetID:   "T-0042",
		Status:     "running",
		StartedAt:  time.Now().UTC(),
	}

	if err := su.CreateRun(ctx, record); err != nil {
		t.Fatal(err)
	}

	// Check store.
	if _, ok := store.runs["run-001"]; !ok {
		t.Error("run not persisted to store")
	}

	// Check bus event.
	if b.eventCount() != 1 {
		t.Fatalf("event count = %d, want 1", b.eventCount())
	}
	evt := b.lastEvent()
	if string(evt.Topic) != "tlc.agent.started" {
		t.Errorf("topic = %q, want tlc.agent.started", evt.Topic)
	}
}

func TestStateUpdater_UpdateRun_Succeeded(t *testing.T) {
	su, _, b, store := setupStateUpdater(t)
	ctx := context.Background()

	record := &AgentRunRecord{
		ID:         "run-001",
		Agent:      "claude",
		TargetType: "task",
		TargetID:   "T-0042",
		Status:     "running",
		StartedAt:  time.Now().Add(-5 * time.Minute).UTC(),
	}
	store.runs["run-001"] = record

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusSucceeded,
		Summary: "All done",
	}

	if err := su.UpdateRun(ctx, "run-001", result, record); err != nil {
		t.Fatal(err)
	}

	// Verify record updated.
	r := store.runs["run-001"]
	if r.Status != "succeeded" {
		t.Errorf("run status = %q", r.Status)
	}
	if r.EndedAt == nil {
		t.Error("ended_at not set")
	}

	// Check bus event.
	evt := b.lastEvent()
	if string(evt.Topic) != "tlc.agent.completed" {
		t.Errorf("topic = %q, want tlc.agent.completed", evt.Topic)
	}
}

func TestStateUpdater_UpdateRun_Failed(t *testing.T) {
	su, _, b, store := setupStateUpdater(t)
	ctx := context.Background()

	record := &AgentRunRecord{
		ID:         "run-002",
		Agent:      "claude",
		TargetType: "task",
		TargetID:   "T-0042",
		Status:     "running",
		StartedAt:  time.Now().UTC(),
	}
	store.runs["run-002"] = record

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusFailed,
		Summary: "Crashed",
	}

	if err := su.UpdateRun(ctx, "run-002", result, record); err != nil {
		t.Fatal(err)
	}

	r := store.runs["run-002"]
	if r.Error != "Crashed" {
		t.Errorf("error = %q, want Crashed", r.Error)
	}

	evt := b.lastEvent()
	if string(evt.Topic) != "tlc.agent.failed" {
		t.Errorf("topic = %q, want tlc.agent.failed", evt.Topic)
	}
}

func TestStateUpdater_NilBus(t *testing.T) {
	repo := NewMockRepository()
	logRepo := &MockLogRepository{}
	svc := NewTaskService(repo, logRepo)
	// nil bus — should not panic.
	su := NewStateUpdater(svc, nil)
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &Task{
		ID:     "T-0001",
		Title:  "Test",
		Status: StatusInProgress,
	})

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusSucceeded,
		Summary: "Done",
	}

	if err := su.Update(ctx, result, "task", "T-0001", UpdateOpts{}); err != nil {
		t.Fatal(err)
	}

	task, _ := repo.GetTask(ctx, "T-0001")
	if task.Status != StatusDone {
		t.Errorf("status = %q, want DONE", task.Status)
	}
}
