package core

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockRunner records calls and returns canned responses.
type mockRunner struct {
	calls   []mockCall
	results []mockResult
	idx     int
}

type mockCall struct {
	Name string
	Args []string
}

type mockResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func (m *mockRunner) Run(
	_ context.Context, name string, args ...string,
) (string, string, int, error) {
	m.calls = append(m.calls, mockCall{Name: name, Args: args})
	if m.idx >= len(m.results) {
		return "", "", 1, fmt.Errorf("no more canned results")
	}
	r := m.results[m.idx]
	m.idx++
	return r.Stdout, r.Stderr, r.ExitCode, r.Err
}

func TestPodShell_CheckAvailable(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{Stdout: "pod v0.5.0", ExitCode: 0},
		}}
		ps := NewPodShell(r)
		if err := ps.CheckAvailable(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{Err: fmt.Errorf("exec: not found")},
		}}
		ps := NewPodShell(r)
		err := ps.CheckAvailable(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "pod not found") {
			t.Errorf("error = %q, want 'pod not found'", err)
		}
	})

	t.Run("exit non-zero", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{Stderr: "bad version", ExitCode: 1},
		}}
		ps := NewPodShell(r)
		err := ps.CheckAvailable(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "exit 1") {
			t.Errorf("error = %q", err)
		}
	})

	t.Run("empty output", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{Stdout: "", ExitCode: 0},
		}}
		ps := NewPodShell(r)
		err := ps.CheckAvailable(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "empty output") {
			t.Errorf("error = %q", err)
		}
	})
}

func TestPodShell_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{Stdout: `{"name":"tlc-abc","id":"abc123","status":"running","provider":"docker"}`, ExitCode: 0},
		}}
		ps := NewPodShell(r)
		info, err := ps.Create(context.Background(), PodCreateOpts{
			Image:    "ghcr.io/hop-top/pod-claude:latest",
			Provider: "docker",
			Network:  "none",
			Labels:   map[string]string{"tlc-managed": "true"},
			EnvVars:  map[string]string{"KEY": "val"},
			Mounts: []MountSpec{
				{Source: "/repo", Target: "/workspace", Mode: "ro"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Name != "tlc-abc" {
			t.Errorf("name = %q", info.Name)
		}
		if info.ID != "abc123" {
			t.Errorf("id = %q", info.ID)
		}

		// Verify the command args include expected flags.
		call := r.calls[0]
		joined := strings.Join(call.Args, " ")
		for _, want := range []string{
			"--format json",
			"--provider docker",
			"--image ghcr.io/hop-top/pod-claude:latest",
			"--network none",
			"--mount /repo:/workspace:ro",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("args missing %q: %s", want, joined)
			}
		}
	})

	t.Run("OOM exit 137", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{ExitCode: 137, Stderr: "killed"},
		}}
		ps := NewPodShell(r)
		_, err := ps.Create(context.Background(), PodCreateOpts{
			Image: "test",
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "OOM") {
			t.Errorf("error = %q, want OOM mention", err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{Stdout: "not-json", ExitCode: 0},
		}}
		ps := NewPodShell(r)
		_, err := ps.Create(context.Background(), PodCreateOpts{Image: "test"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "parse JSON") {
			t.Errorf("error = %q", err)
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{Stdout: `{"name":"x","id":"","status":""}`, ExitCode: 0},
		}}
		ps := NewPodShell(r)
		_, err := ps.Create(context.Background(), PodCreateOpts{Image: "test"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "expected fields") {
			t.Errorf("error = %q", err)
		}
	})
}

func TestPodShell_CopyTo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{{ExitCode: 0}}}
		ps := NewPodShell(r)
		err := ps.CopyTo(
			context.Background(), "pod-1", "/tmp/ctx.json",
			"/workspace/.tlc/context.json",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		call := r.calls[0]
		if call.Args[0] != "cp" {
			t.Errorf("expected cp, got %q", call.Args[0])
		}
	})

	t.Run("failure", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{ExitCode: 1, Stderr: "no space"},
		}}
		ps := NewPodShell(r)
		err := ps.CopyTo(context.Background(), "pod-1", "/a", "/b")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestPodShell_VerifyFile(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{{ExitCode: 0}}}
		ps := NewPodShell(r)
		if err := ps.VerifyFile(
			context.Background(), "pod-1", "/workspace/.tlc/context.json",
		); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{{ExitCode: 1}}}
		ps := NewPodShell(r)
		err := ps.VerifyFile(
			context.Background(), "pod-1", "/workspace/.tlc/context.json",
		)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "file not found in container") {
			t.Errorf("error = %q", err)
		}
	})
}

func TestPodShell_Exec(t *testing.T) {
	r := &mockRunner{results: []mockResult{
		{Stdout: `{"status":"succeeded"}`, ExitCode: 0},
	}}
	ps := NewPodShell(r)
	stdout, _, code, err := ps.Exec(
		context.Background(), "pod-1", "claude",
		"--context", "/workspace/.tlc/context.json",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d", code)
	}
	if !strings.Contains(stdout, "succeeded") {
		t.Errorf("stdout = %q", stdout)
	}
	// Verify args: pod exec pod-1 -- claude --context ...
	call := r.calls[0]
	if call.Args[0] != "exec" || call.Args[1] != "pod-1" || call.Args[2] != "--" {
		t.Errorf("unexpected args: %v", call.Args)
	}
}

func TestPodShell_CopyFrom(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{{ExitCode: 0}}}
		ps := NewPodShell(r)
		err := ps.CopyFrom(
			context.Background(), "pod-1",
			"/workspace/.tlc/results.json", "/tmp/results.json",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{ExitCode: 1, Stderr: "not found"},
		}}
		ps := NewPodShell(r)
		err := ps.CopyFrom(context.Background(), "pod-1", "/a", "/b")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestPodShell_Destroy(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{{ExitCode: 0}}}
		ps := NewPodShell(r)
		if err := ps.Destroy(context.Background(), "pod-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ps.Destroyed() {
			t.Error("expected destroyed=true")
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{{ExitCode: 0}}}
		ps := NewPodShell(r)
		_ = ps.Destroy(context.Background(), "pod-1")
		// Second call should be no-op.
		if err := ps.Destroy(context.Background(), "pod-1"); err != nil {
			t.Fatalf("second destroy should be no-op: %v", err)
		}
		if len(r.calls) != 1 {
			t.Errorf("expected 1 call, got %d", len(r.calls))
		}
	})

	t.Run("failure", func(t *testing.T) {
		r := &mockRunner{results: []mockResult{
			{ExitCode: 1, Stderr: "cannot destroy"},
		}}
		ps := NewPodShell(r)
		err := ps.Destroy(context.Background(), "pod-1")
		if err == nil {
			t.Fatal("expected error")
		}
		if ps.Destroyed() {
			t.Error("should not be marked destroyed on failure")
		}
	})
}
