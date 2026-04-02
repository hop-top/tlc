package flowtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hop.top/tlc/internal/core"
	xrr "hop.top/xrr"
)

// TestSandboxAgentRunnerDispatchesViaResolver verifies that Run() delegates
// binary selection to the resolver rather than hardcoding "claude".
func TestSandboxAgentRunnerDispatchesViaResolver(t *testing.T) {
	// Build a minimal sandbox with a fake binary that records which binary was called.
	sb, err := NewSandbox(findRepoRoot(t))
	if err != nil {
		t.Fatalf("NewSandbox: %v", err)
	}
	defer sb.Teardown()

	// Write a fake "llm" binary that prints valid JSON and exits 0.
	fakeBin := filepath.Join(sb.BinDir, "llm")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho '{\"type\":\"result\",\"adapter\":\"llm\"}'\n"), 0o755); err != nil {
		t.Fatalf("write fake llm: %v", err)
	}

	adapters := map[string]AgentAdapter{
		"claude": NewClaudeAdapter(),
		"llm":    NewLLMAdapter(),
	}
	flow := &core.Flow{Agent: core.AgentRef{Name: "llm"}}
	resolver := NewAdapterResolver(adapters, flow, nil)
	runner := NewSandboxAgentRunner(sb, xrr.ModeReplay, nil, resolver)

	step := core.Step{ID: "test-step", Type: core.StepTypeTask, Title: "Test"}
	out, err := runner.Run(context.Background(), step, "hello")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if out["adapter"] != "llm" {
		t.Errorf("expected adapter=llm in output, got %v", out)
	}
}

// TestSandboxAgentRunnerResolverFatalPropagates ensures a resolution failure
// surfaces as an error from Run(), not a panic.
func TestSandboxAgentRunnerResolverFatalPropagates(t *testing.T) {
	sb, err := NewSandbox(findRepoRoot(t))
	if err != nil {
		t.Fatalf("NewSandbox: %v", err)
	}
	defer sb.Teardown()

	// No agent declared anywhere → resolver must fatal.
	resolver := NewAdapterResolver(map[string]AgentAdapter{
		"claude": NewClaudeAdapter(),
	}, &core.Flow{}, nil)
	runner := NewSandboxAgentRunner(sb, xrr.ModeReplay, nil, resolver)

	step := core.Step{ID: "no-agent-step", Type: core.StepTypeTask, Title: "Test"}
	_, err = runner.Run(context.Background(), step, "hello")
	if err == nil {
		t.Fatal("Run() expected error for unresolvable adapter, got nil")
	}
}

// findRepoRoot walks up from the test file to locate the module root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}
