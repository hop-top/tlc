package cli

import (
	"context"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestResolveTaskIDs_ExactSingle(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	ctx := context.Background()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})

	resolved, confirm, err := resolveTaskIDs(ctx, []string{"T-0001"}, s, core.Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 || resolved[0].Task.ID != "T-0001" {
		t.Errorf("expected T-0001, got %v", resolved)
	}
	if confirm {
		t.Error("exact ID should not require confirmation")
	}
}

func TestResolveTaskIDs_ExactMultiple(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	ctx := context.Background()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

	resolved, confirm, err := resolveTaskIDs(ctx, []string{"T-0001", "T-0002"}, s, core.Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(resolved))
	}
	if confirm {
		t.Error("exact IDs should not require confirmation")
	}
}

func TestResolveTaskIDs_RegexPattern(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	ctx := context.Background()
	s.CreateTask(ctx, &core.Task{ID: "T-0010", Title: "A", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0011", Title: "B", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0020", Title: "C", Status: core.StatusTodo})

	resolved, confirm, err := resolveTaskIDs(ctx, []string{"T-001[01]"}, s, core.Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(resolved))
	}
	if !confirm {
		t.Error("regex matching >1 task should require confirmation")
	}
}

func TestResolveTaskIDs_GlobStar(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	ctx := context.Background()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

	resolved, confirm, err := resolveTaskIDs(ctx, []string{"*"}, s, core.Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(resolved))
	}
	if !confirm {
		t.Error("glob * matching >1 task should require confirmation")
	}
}

func TestResolveTaskIDs_RegexNoMatch(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	ctx := context.Background()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})

	_, _, err := resolveTaskIDs(ctx, []string{"T-999[0-9]"}, s, core.Query{})
	if err == nil {
		t.Error("expected error for pattern matching no tasks")
	}
}

func TestResolveTaskIDs_LowercaseAlias(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	ctx := context.Background()
	s.CreateTask(ctx, &core.Task{ID: "T-0034", Title: "A", Status: core.StatusTodo})

	resolved, confirm, err := resolveTaskIDs(ctx, []string{"t-0034"}, s, core.Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 || resolved[0].Task.ID != "T-0034" {
		t.Errorf("expected T-0034, got %v", resolved)
	}
	if confirm {
		t.Error("exact lowercase ID should not require confirmation")
	}
}

func TestResolveTaskIDs_RegexSingleMatchNoConfirm(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	ctx := context.Background()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

	resolved, confirm, err := resolveTaskIDs(ctx, []string{"T-0001"}, s, core.Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 {
		t.Errorf("expected 1 task, got %d", len(resolved))
	}
	// Exact ID — no confirmation even though it looks like it could be a pattern
	if confirm {
		t.Error("single exact match should not require confirmation")
	}
}
