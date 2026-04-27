package storage

import (
	"context"
	"testing"
	"time"

	"hop.top/kit/go/runtime/domain"
	"hop.top/tlc/internal/core"
)

func TestFlowRunDomainRepo_CreateGet(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	repo := NewFlowRunDomainRepo(s)
	ctx := context.Background()

	run := &core.FlowRun{
		ID:        "run-001",
		FlowID:    "flow-abc",
		Status:    core.FlowStatusQueued,
		StartedAt: time.Now().UTC().Truncate(time.Second),
	}

	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, "run-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.FlowID != "flow-abc" {
		t.Errorf("FlowID = %q, want %q", got.FlowID, "flow-abc")
	}
	if got.Status != core.FlowStatusQueued {
		t.Errorf("Status = %q, want %q", got.Status, core.FlowStatusQueued)
	}
}

func TestFlowRunDomainRepo_Update(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	repo := NewFlowRunDomainRepo(s)
	ctx := context.Background()

	run := &core.FlowRun{
		ID:        "run-002",
		FlowID:    "flow-xyz",
		Status:    core.FlowStatusQueued,
		StartedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	run.Status = core.FlowStatusRunning
	if err := repo.Update(ctx, run); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.Get(ctx, "run-002")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != core.FlowStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, core.FlowStatusRunning)
	}
}

func TestFlowRunDomainRepo_Delete(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	repo := NewFlowRunDomainRepo(s)
	ctx := context.Background()

	run := &core.FlowRun{
		ID:        "run-003",
		FlowID:    "flow-del",
		Status:    core.FlowStatusFailed,
		StartedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, "run-003"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := repo.Get(ctx, "run-003")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestFlowRunDomainRepo_List(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	repo := NewFlowRunDomainRepo(s)
	ctx := context.Background()

	for _, id := range []string{"run-a", "run-b", "run-c"} {
		run := &core.FlowRun{
			ID:        id,
			FlowID:    "flow-list",
			Status:    core.FlowStatusQueued,
			StartedAt: time.Now().UTC().Truncate(time.Second),
		}
		if err := repo.Create(ctx, run); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	runs, err := repo.List(ctx, domain.Query{Limit: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("List returned %d runs, want 2", len(runs))
	}
}

func TestBindFlowRunColumnCount(t *testing.T) {
	run := core.FlowRun{
		ID:        "run-bind",
		FlowID:    "flow-bind",
		Status:    core.FlowStatusSucceeded,
		StartedAt: time.Now().UTC().Truncate(time.Second),
	}

	cols, vals := BindFlowRun(run)
	if len(cols) != 6 {
		t.Errorf("BindFlowRun returned %d columns, want 6", len(cols))
	}
	if len(vals) != 6 {
		t.Errorf("BindFlowRun returned %d values, want 6", len(vals))
	}
}
