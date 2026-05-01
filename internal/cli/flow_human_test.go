// Tests for `tlc flow approve|reject|cancel` CLI subcommands.
// Author: jadb
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

func resetHumanFlags() {
	flowApproveStep = ""
	flowApproveBy = ""
	flowApproveNote = ""
	flowRejectStep = ""
	flowRejectBy = ""
	flowRejectReason = ""
	flowCancelBy = ""
}

// seedAwaiting opens an awaiting record at the given store.
func seedAwaiting(t *testing.T, store *core.FileApprovalStore, runID, stepID string) {
	t.Helper()
	if err := store.Open(context.Background(), runID, stepID, "Title"); err != nil {
		t.Fatal(err)
	}
}

func TestFlowApprove_ResumesAwaitingRun(t *testing.T) {
	defer resetHumanFlags()
	dir := t.TempDir()
	store := core.NewFileApprovalStore(dir)
	seedAwaiting(t, store, "run-1", "step-A")

	resetHumanFlags()
	flowApprovalsRoot = dir
	flowApproveStep = "step-A"
	flowApproveBy = "user:jad"

	var out bytes.Buffer
	FlowApproveCmd.SetOut(&out)
	FlowApproveCmd.SetErr(&out)
	if err := FlowApproveCmd.RunE(FlowApproveCmd, []string{"run-1"}); err != nil {
		t.Fatalf("approve: %v", err)
	}

	rec, err := store.Read(context.Background(), "run-1", "step-A")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != core.ApprovalStateApproved {
		t.Errorf("status = %s, want approved", rec.Status)
	}
	if rec.By != "user:jad" {
		t.Errorf("by = %s, want user:jad", rec.By)
	}
	if !strings.Contains(out.String(), "Approved") {
		t.Errorf("output missing 'Approved': %s", out.String())
	}
}

func TestFlowApprove_RequiresStep(t *testing.T) {
	defer resetHumanFlags()
	resetHumanFlags()
	flowApprovalsRoot = t.TempDir()
	// No --step set.

	var out bytes.Buffer
	FlowApproveCmd.SetOut(&out)
	FlowApproveCmd.SetErr(&out)
	err := FlowApproveCmd.RunE(FlowApproveCmd, []string{"run-1"})
	if err == nil {
		t.Fatal("want error when --step missing")
	}
	if !strings.Contains(err.Error(), "step") {
		t.Errorf("err = %q, want 'step'", err.Error())
	}
}

func TestFlowApprove_NonexistentRun(t *testing.T) {
	defer resetHumanFlags()
	resetHumanFlags()
	flowApprovalsRoot = t.TempDir()
	flowApproveStep = "step-A"

	var out bytes.Buffer
	FlowApproveCmd.SetOut(&out)
	FlowApproveCmd.SetErr(&out)
	err := FlowApproveCmd.RunE(FlowApproveCmd, []string{"ghost-run"})
	if err == nil {
		t.Fatal("want error for nonexistent run")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %q, want 'not found'", err.Error())
	}
}

func TestFlowReject_CancelsRun(t *testing.T) {
	defer resetHumanFlags()
	dir := t.TempDir()
	store := core.NewFileApprovalStore(dir)
	seedAwaiting(t, store, "run-1", "step-A")

	resetHumanFlags()
	flowApprovalsRoot = dir
	flowRejectStep = "step-A"
	flowRejectReason = "wrong recipient"

	var out bytes.Buffer
	FlowRejectCmd.SetOut(&out)
	FlowRejectCmd.SetErr(&out)
	if err := FlowRejectCmd.RunE(FlowRejectCmd, []string{"run-1"}); err != nil {
		t.Fatalf("reject: %v", err)
	}

	rec, _ := store.Read(context.Background(), "run-1", "step-A")
	if rec.Status != core.ApprovalStateRejected {
		t.Errorf("status = %s, want rejected", rec.Status)
	}
	if rec.Reason != "wrong recipient" {
		t.Errorf("reason = %s", rec.Reason)
	}
}

func TestFlowReject_RequiresReason(t *testing.T) {
	defer resetHumanFlags()
	dir := t.TempDir()
	store := core.NewFileApprovalStore(dir)
	seedAwaiting(t, store, "run-1", "step-A")

	resetHumanFlags()
	flowApprovalsRoot = dir
	flowRejectStep = "step-A"
	// No --reason.

	var out bytes.Buffer
	FlowRejectCmd.SetOut(&out)
	FlowRejectCmd.SetErr(&out)
	err := FlowRejectCmd.RunE(FlowRejectCmd, []string{"run-1"})
	if err == nil {
		t.Fatal("want error when reason missing")
	}
}

func TestFlowCancel_StopsAllAwaiting(t *testing.T) {
	defer resetHumanFlags()
	dir := t.TempDir()
	store := core.NewFileApprovalStore(dir)
	seedAwaiting(t, store, "run-1", "step-A")
	seedAwaiting(t, store, "run-1", "step-B")

	resetHumanFlags()
	flowApprovalsRoot = dir

	var out bytes.Buffer
	FlowCancelCmd.SetOut(&out)
	FlowCancelCmd.SetErr(&out)
	if err := FlowCancelCmd.RunE(FlowCancelCmd, []string{"run-1"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	for _, sid := range []string{"step-A", "step-B"} {
		rec, _ := store.Read(context.Background(), "run-1", sid)
		if rec.Status != core.ApprovalStateCanceled {
			t.Errorf("step %s status = %s, want canceled", sid, rec.Status)
		}
	}
}

func TestFlowCancel_NonexistentRun(t *testing.T) {
	defer resetHumanFlags()
	resetHumanFlags()
	flowApprovalsRoot = t.TempDir()

	var out bytes.Buffer
	FlowCancelCmd.SetOut(&out)
	FlowCancelCmd.SetErr(&out)
	err := FlowCancelCmd.RunE(FlowCancelCmd, []string{"ghost"})
	if err == nil {
		t.Fatal("want error for nonexistent run")
	}
}

func TestFlowApprove_DoubleResolve(t *testing.T) {
	defer resetHumanFlags()
	dir := t.TempDir()
	store := core.NewFileApprovalStore(dir)
	seedAwaiting(t, store, "run-1", "step-A")

	resetHumanFlags()
	flowApprovalsRoot = dir
	flowApproveStep = "step-A"

	var out bytes.Buffer
	FlowApproveCmd.SetOut(&out)
	FlowApproveCmd.SetErr(&out)

	// First approve succeeds.
	if err := FlowApproveCmd.RunE(FlowApproveCmd, []string{"run-1"}); err != nil {
		t.Fatalf("first approve: %v", err)
	}

	// Second approve must fail.
	err := FlowApproveCmd.RunE(FlowApproveCmd, []string{"run-1"})
	if err == nil {
		t.Fatal("want error on double-resolve")
	}
	if !strings.Contains(err.Error(), "already resolved") {
		t.Errorf("err = %q, want 'already resolved'", err.Error())
	}
}
