package core

import (
	"context"
	"testing"
)

// mockTaskReader is a compile-time check that TaskReader can be implemented.
type mockTaskReader struct{}

func (m *mockTaskReader) ListTasks(_ context.Context, _ Query) ([]*Task, error) {
	return nil, nil
}

func (m *mockTaskReader) GetTask(_ context.Context, _ string) (*Task, error) {
	return nil, nil
}

func (m *mockTaskReader) GetTaskLogs(_ context.Context, _ string) ([]*LogEntry, error) {
	return nil, nil
}

func (m *mockTaskReader) CountTasks(_ context.Context, _ Query) (int, error) {
	return 0, nil
}

var _ TaskReader = (*mockTaskReader)(nil)

func TestTaskReader_MockSatisfiesInterface(t *testing.T) {
	var r TaskReader = &mockTaskReader{}

	ctx := context.Background()

	tasks, err := r.ListTasks(ctx, Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tasks != nil {
		t.Fatalf("expected nil tasks, got %v", tasks)
	}

	task, err := r.GetTask(ctx, "T-0001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task != nil {
		t.Fatalf("expected nil task, got %v", task)
	}

	logs, err := r.GetTaskLogs(ctx, "T-0001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logs != nil {
		t.Fatalf("expected nil logs, got %v", logs)
	}

	count, err := r.CountTasks(ctx, Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 count, got %d", count)
	}
}
