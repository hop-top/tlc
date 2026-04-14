package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// T-0587: CopyTo returns an error on runner failure.
// Verifies the PodShell.CopyTo contract: a non-zero exit from the
// runner surfaces as an error to the caller. The orchestration layer
// (which decides whether to call Exec) is not exercised here.
func TestPodShell_CopyToFailure_ReturnsError(t *testing.T) {
	// Mock runner: create succeeds, cp fails.
	r := &mockRunner{results: []mockResult{
		// Create → success
		{
			Stdout:   `{"name":"pod-x","id":"x1","status":"running","provider":"docker"}`,
			ExitCode: 0,
		},
		// CopyTo → failure (simulates disk full / network error)
		{
			Stderr:   "no space left on device",
			ExitCode: 1,
		},
	}}

	ps := NewPodShell(r)
	ctx := context.Background()

	// Step 1: Create pod.
	podInfo, err := ps.Create(ctx, PodCreateOpts{Image: "test"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Step 2: CopyTo should fail.
	err = ps.CopyTo(ctx, podInfo.Name, "/tmp/ctx.json", "/workspace/.tlc/context.json")
	if err == nil {
		t.Fatal("expected CopyTo to fail")
	}

	// The error from CopyTo should mention "pod cp failed".
	if !strings.Contains(err.Error(), "pod cp failed") {
		t.Errorf("unexpected error: %q", err)
	}

	// Verify exactly 2 runner calls were made (create, cp).
	if len(r.calls) != 2 {
		t.Errorf("expected 2 calls (create, cp), got %d", len(r.calls))
		for i, c := range r.calls {
			t.Logf("  call[%d]: %s %v", i, c.Name, c.Args)
		}
	}

	// Verify the second call was "cp".
	if len(r.calls) >= 2 {
		if r.calls[1].Args[0] != "cp" {
			t.Errorf("second call should be cp, got %q", r.calls[1].Args[0])
		}
	}
}

// T-0587: VerifyFile returns a "context upload failed" error when the
// file check exits non-zero. Tests the VerifyFile contract directly.
func TestPodShell_VerifyFileFails_ReturnsUploadError(t *testing.T) {
	r := &mockRunner{results: []mockResult{
		// CopyTo → success
		{ExitCode: 0},
		// VerifyFile → failure (file not found in container)
		{ExitCode: 1, Stderr: ""},
	}}

	ps := NewPodShell(r)
	ctx := context.Background()

	// CopyTo succeeds.
	if err := ps.CopyTo(ctx, "pod-x", "/tmp/ctx.json", "/workspace/.tlc/context.json"); err != nil {
		t.Fatalf("CopyTo should succeed: %v", err)
	}

	// VerifyFile fails — this is the "context upload failed" path.
	err := ps.VerifyFile(ctx, "pod-x", "/workspace/.tlc/context.json")
	if err == nil {
		t.Fatal("expected VerifyFile to fail")
	}
	if !strings.Contains(err.Error(), "context upload failed") {
		t.Errorf("error should mention 'context upload failed': %q", err)
	}

	// Exec was never called: only 2 calls (cp, exec-test).
	if len(r.calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(r.calls))
	}
}

// T-0587: CopyTo wraps runner exec errors with "exec failed".
func TestPodShell_CopyToExecError_ReturnsExecError(t *testing.T) {
	r := &mockRunner{results: []mockResult{
		// CopyTo → exec error (binary not found)
		{Err: fmt.Errorf("exec: pod binary not found")},
	}}

	ps := NewPodShell(r)
	ctx := context.Background()

	err := ps.CopyTo(ctx, "pod-y", "/tmp/ctx.json", "/workspace/.tlc/context.json")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "exec failed") {
		t.Errorf("error should mention exec failure: %q", err)
	}

	// Only 1 call made (failed cp).
	if len(r.calls) != 1 {
		t.Errorf("expected 1 call, got %d", len(r.calls))
	}
}
