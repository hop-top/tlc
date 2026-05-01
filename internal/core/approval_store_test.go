// Tests for the file-based ApprovalStore — cross-process persistence
// of human-step approval intents (story 025 / T-0199).
//
// Author: jadb
package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApprovalStore_OpenWritesAwaiting(t *testing.T) {
	dir := t.TempDir()
	store := NewFileApprovalStore(dir)

	if err := store.Open(context.Background(), "run-1", "step-A", "Approve send"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	state, err := store.Read(context.Background(), "run-1", "step-A")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if state.Status != ApprovalStateAwaiting {
		t.Errorf("status = %q, want awaiting", state.Status)
	}
	if state.RunID != "run-1" || state.StepID != "step-A" {
		t.Errorf("ids = %s/%s, want run-1/step-A", state.RunID, state.StepID)
	}
	if state.OpenedAt.IsZero() {
		t.Errorf("OpenedAt zero")
	}
}

func TestApprovalStore_ApproveTransitions(t *testing.T) {
	dir := t.TempDir()
	store := NewFileApprovalStore(dir)
	ctx := context.Background()
	if err := store.Open(ctx, "run-1", "step-A", "title"); err != nil {
		t.Fatal(err)
	}

	if err := store.Approve(ctx, "run-1", "step-A", "user:jad", ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	state, _ := store.Read(ctx, "run-1", "step-A")
	if state.Status != ApprovalStateApproved {
		t.Errorf("status = %q, want approved", state.Status)
	}
	if state.By != "user:jad" {
		t.Errorf("by = %q, want user:jad", state.By)
	}
	if state.ResolvedAt == nil || state.ResolvedAt.IsZero() {
		t.Errorf("ResolvedAt should be set after approve")
	}
}

func TestApprovalStore_RejectWithReason(t *testing.T) {
	dir := t.TempDir()
	store := NewFileApprovalStore(dir)
	ctx := context.Background()
	if err := store.Open(ctx, "run-1", "step-A", "title"); err != nil {
		t.Fatal(err)
	}

	if err := store.Reject(ctx, "run-1", "step-A", "user:jad", "looks risky"); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	state, _ := store.Read(ctx, "run-1", "step-A")
	if state.Status != ApprovalStateRejected {
		t.Errorf("status = %q, want rejected", state.Status)
	}
	if state.Reason != "looks risky" {
		t.Errorf("reason = %q", state.Reason)
	}
}

func TestApprovalStore_CancelEntireRun(t *testing.T) {
	dir := t.TempDir()
	store := NewFileApprovalStore(dir)
	ctx := context.Background()
	if err := store.Open(ctx, "run-1", "step-A", "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Open(ctx, "run-1", "step-B", "b"); err != nil {
		t.Fatal(err)
	}

	if err := store.CancelRun(ctx, "run-1", "user:jad"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	for _, sid := range []string{"step-A", "step-B"} {
		state, _ := store.Read(ctx, "run-1", sid)
		if state.Status != ApprovalStateCanceled {
			t.Errorf("step %s status = %q, want canceled", sid, state.Status)
		}
	}
}

func TestApprovalStore_ApproveNonexistentRun(t *testing.T) {
	dir := t.TempDir()
	store := NewFileApprovalStore(dir)

	err := store.Approve(context.Background(), "ghost", "x", "user", "")
	if err == nil {
		t.Fatal("expected error approving nonexistent run")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestApprovalStore_DoubleResolveRejected(t *testing.T) {
	dir := t.TempDir()
	store := NewFileApprovalStore(dir)
	ctx := context.Background()
	_ = store.Open(ctx, "run-1", "step-A", "t")
	_ = store.Approve(ctx, "run-1", "step-A", "user:jad", "")

	err := store.Reject(ctx, "run-1", "step-A", "user:jad", "too late")
	if err == nil {
		t.Fatal("expected error on double-resolve")
	}
	if !strings.Contains(err.Error(), "already resolved") {
		t.Errorf("error = %q, want 'already resolved'", err.Error())
	}
}

func TestApprovalStore_FilePersistedOnDisk(t *testing.T) {
	dir := t.TempDir()
	store := NewFileApprovalStore(dir)
	ctx := context.Background()
	_ = store.Open(ctx, "run-1", "step-A", "t")

	// Walk and confirm a JSON file exists.
	var found bool
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() && strings.HasSuffix(p, ".json") {
			found = true
		}
		return nil
	})
	if !found {
		t.Errorf("no JSON file written under %s", dir)
	}
}

func TestApprovalStore_WaitForResolution(t *testing.T) {
	dir := t.TempDir()
	store := NewFileApprovalStore(dir)
	ctx := context.Background()
	_ = store.Open(ctx, "run-1", "step-A", "t")

	// Resolve in a goroutine after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = store.Approve(ctx, "run-1", "step-A", "user:jad", "")
	}()

	deadline := time.Now().Add(2 * time.Second)
	state, err := store.WaitFor(ctx, "run-1", "step-A", 10*time.Millisecond, deadline)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if state.Status != ApprovalStateApproved {
		t.Errorf("status = %q, want approved", state.Status)
	}
}

func TestApprovalStore_WaitForDeadlineExceeded(t *testing.T) {
	dir := t.TempDir()
	store := NewFileApprovalStore(dir)
	ctx := context.Background()
	_ = store.Open(ctx, "run-1", "step-A", "t")

	deadline := time.Now().Add(50 * time.Millisecond)
	_, err := store.WaitFor(ctx, "run-1", "step-A", 10*time.Millisecond, deadline)
	if err == nil {
		t.Fatal("expected deadline-exceeded error")
	}
	if !strings.Contains(err.Error(), "deadline") {
		t.Errorf("error = %q, want 'deadline'", err.Error())
	}
}
