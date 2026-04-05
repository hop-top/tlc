package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestTaskPromptE2E_ClassifyAndComplete creates a task via CLI, classifies
// "complete T-0001" through ClassifyPrompt, then executes the resolved command
// and verifies the task reaches DONE status.
func TestTaskPromptE2E_ClassifyAndComplete(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		_, cleanup := setupTestDir(t)
		defer cleanup()

		// Create a task then claim it (TODO → IN_PROGRESS).
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Implement login flow"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v", err)
		}

		resetTaskFlags()
		claimCmd := newTestCmd()
		claimCmd.AddCommand(TaskCmd)
		claimCmd.SetOut(buf)
		claimCmd.SetErr(buf)
		claimCmd.SetArgs([]string{"task", "claim", "T-0001"})
		if err := claimCmd.Execute(); err != nil {
			t.Fatalf("task claim failed: %v", err)
		}

		// Classify natural language.
		cmds := ClassifyPrompt("complete T-0001")
		if len(cmds) == 0 {
			t.Fatal("ClassifyPrompt returned no commands for 'complete T-0001'")
		}
		rc := cmds[0]
		if rc.Cmd != "task" {
			t.Errorf("expected Cmd 'task', got %q", rc.Cmd)
		}
		if len(rc.Args) < 2 || rc.Args[0] != "complete" || rc.Args[1] != "T-0001" {
			t.Errorf("unexpected Args: %v", rc.Args)
		}
		if rc.Confidence != 1.0 {
			t.Errorf("expected confidence 1.0, got %f", rc.Confidence)
		}

		// Replace runCommandFn with a stub that uses newTestCmd (matching
		// the pattern other E2E tests use) so global flag state is clean.
		origRun := runCommandFn
		runCommandFn = func(_ context.Context, rc ResolvedCommand) error {
			resetTaskFlags()
			c := newTestCmd()
			c.AddCommand(TaskCmd)
			c.SetArgs(append([]string{"task"}, rc.Args...))
			return c.Execute()
		}
		defer func() { runCommandFn = origRun }()

		ctx := context.Background()
		result := executeCommands(ctx, cmds, ExecuteOptions{
			NoPrompt: true,
			Writer:   buf,
		})
		if result.Error != nil {
			t.Fatalf("executeCommands failed: %v", result.Error)
		}

		// Verify the task is DONE.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task, err := s.GetTask(ctx, "T-0001")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if task == nil {
			t.Fatal("task not found after complete")
		}
		if task.Status != core.StatusDone {
			t.Errorf("expected status DONE, got %s", task.Status)
		}
	})
}

// TestTaskPromptE2E_ClassifyListMine verifies that "list my tasks" is
// classified with the --mine flag.
func TestTaskPromptE2E_ClassifyListMine(t *testing.T) {
	resetTaskFlags()

	cmds := ClassifyPrompt("list my tasks")
	if len(cmds) == 0 {
		t.Fatal("ClassifyPrompt returned no commands for 'list my tasks'")
	}
	rc := cmds[0]
	if rc.Cmd != "task" {
		t.Errorf("expected Cmd 'task', got %q", rc.Cmd)
	}

	foundMine := false
	for _, arg := range rc.Args {
		if arg == "--mine" {
			foundMine = true
			break
		}
	}
	if !foundMine {
		t.Errorf("expected --mine in args, got %v", rc.Args)
	}
}

// TestTaskPromptE2E_ContextDumpMarkdown creates a task with metadata and
// verifies the markdown context dump contains the expected fields.
func TestTaskPromptE2E_ContextDumpMarkdown(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		_, cleanup := setupTestDir(t)
		defer cleanup()

		// Create a task with description, tags, and priority.
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "create", "Design auth module",
			"--description", "Implement OAuth2 with PKCE",
			"--tag", "security",
			"--priority", "P1",
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v", err)
		}

		// Run task prompt for the created task.
		resetTaskFlags()
		taskPromptJSON = false
		cmd2 := newTestCmd()
		cmd2.AddCommand(TaskCmd)
		buf2 := new(bytes.Buffer)
		cmd2.SetOut(buf2)
		cmd2.SetErr(buf2)
		cmd2.SetArgs([]string{"task", "prompt", "T-0001"})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("task prompt failed: %v", err)
		}

		output := buf2.String()

		// Verify markdown contains expected fields.
		for _, want := range []string{
			"T-0001",
			"Design auth module",
			"Status: TODO",
			"Implement OAuth2 with PKCE",
			"Priority: P1",
			"security",
		} {
			if !contains(output, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, output)
			}
		}
	})
}

// TestTaskPromptE2E_ContextDumpJSON creates a task and verifies the JSON
// context dump is valid and contains the expected fields.
func TestTaskPromptE2E_ContextDumpJSON(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		_, cleanup := setupTestDir(t)
		defer cleanup()

		// Create a task.
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "create", "Build API gateway",
			"--description", "Route requests to microservices",
			"--priority", "P2",
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v", err)
		}

		// Run task prompt with --json.
		resetTaskFlags()
		taskPromptJSON = false // reset; the flag will be set by cobra
		cmd2 := newTestCmd()
		cmd2.AddCommand(TaskCmd)
		buf2 := new(bytes.Buffer)
		cmd2.SetOut(buf2)
		cmd2.SetErr(buf2)
		cmd2.SetArgs([]string{"task", "prompt", "T-0001", "--json"})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("task prompt --json failed: %v", err)
		}

		// Verify valid JSON.
		var parsed promptJSONOutput
		if err := json.Unmarshal(buf2.Bytes(), &parsed); err != nil {
			t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf2.String())
		}

		if parsed.ID != "T-0001" {
			t.Errorf("expected id T-0001, got %q", parsed.ID)
		}
		if parsed.Title != "Build API gateway" {
			t.Errorf("expected title 'Build API gateway', got %q", parsed.Title)
		}
		if parsed.Status != "TODO" {
			t.Errorf("expected status TODO, got %q", parsed.Status)
		}
		if parsed.Description != "Route requests to microservices" {
			t.Errorf("expected description 'Route requests to microservices', got %q", parsed.Description)
		}
		if parsed.Priority != "P2" {
			t.Errorf("expected priority P2, got %q", parsed.Priority)
		}
	})
}

// TestTaskPromptE2E_ContextDumpNotFound verifies that running task prompt with
// a non-existent ID returns an appropriate error.
func TestTaskPromptE2E_ContextDumpNotFound(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		_, cleanup := setupTestDir(t)
		defer cleanup()

		taskPromptJSON = false
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "prompt", "T-9999"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for non-existent task")
		}
		if !contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got: %v", err)
		}
	})
}

// TestTaskPromptE2E_DryRun verifies that dry-run mode prints resolved commands
// without executing them.
func TestTaskPromptE2E_DryRun(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		_, cleanup := setupTestDir(t)
		defer cleanup()

		// Create a task so the complete target exists (though dry-run should
		// not touch it).
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Dry run target", "--status", "IN_PROGRESS"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v", err)
		}

		// Classify and execute with DryRun.
		cmds := ClassifyPrompt("complete T-0001")
		if len(cmds) == 0 {
			t.Fatal("ClassifyPrompt returned no commands")
		}

		dryBuf := new(bytes.Buffer)
		ctx := context.Background()
		result := executeCommands(ctx, cmds, ExecuteOptions{
			DryRun: true,
			Writer: dryBuf,
		})

		output := dryBuf.String()
		if !contains(output, "dry-run:") {
			t.Errorf("expected 'dry-run:' prefix in output, got:\n%s", output)
		}
		if !contains(output, "complete") {
			t.Errorf("expected 'complete' in dry-run output, got:\n%s", output)
		}

		// Verify no actual execution happened (Remaining should contain our
		// commands, no Executed).
		if len(result.Executed) != 0 {
			t.Errorf("expected no executed commands in dry-run, got %d", len(result.Executed))
		}

		// Verify task is still IN_PROGRESS.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task, _ := s.GetTask(ctx, "T-0001")
		if task == nil {
			t.Fatal("task not found")
		}
		if task.Status != core.StatusInProgress {
			t.Errorf("expected status IN_PROGRESS (unchanged), got %s", task.Status)
		}
	})
}

// TestTaskPromptE2E_DestructiveGuardReject verifies that a destructive command
// (delete) is rejected when the confirmation function returns false.
func TestTaskPromptE2E_DestructiveGuardReject(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		// Create a task.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Task to maybe delete",
			Status: core.StatusTodo,
		})

		// Mock readConfirmFn to reject.
		origConfirm := readConfirmFn
		readConfirmFn = func(_ io.Reader) bool { return false }
		defer func() { readConfirmFn = origConfirm }()

		// Classify "delete T-0001".
		cmds := ClassifyPrompt("delete T-0001")
		if len(cmds) == 0 {
			t.Fatal("ClassifyPrompt returned no commands for 'delete T-0001'")
		}

		buf := new(bytes.Buffer)
		result := executeCommands(ctx, cmds, ExecuteOptions{
			NoPrompt: false, // let the destructive guard fire
			Execute:  true,  // skip confidence check, but destructive guard still applies
			Writer:   buf,
		})

		// Should have been rejected.
		if result.Error == nil {
			t.Fatal("expected error from rejected destructive command")
		}
		if !contains(result.Error.Error(), "rejected") {
			t.Errorf("expected 'rejected' in error, got: %v", result.Error)
		}

		// Verify the task still exists.
		task, _ := s.GetTask(ctx, "T-0001")
		if task == nil {
			t.Error("task should still exist after rejected delete")
		}
	})
}

// noLLMRoutePromptFn is a routePromptFn replacement that panics if called,
// ensuring the cross-domain classifier handled the prompt without LLM.
func noLLMRoutePromptFn(_ context.Context, prompt string, _ []byte) ([]ResolvedCommand, string, error) {
	panic("routePromptFn called unexpectedly for prompt: " + prompt)
}

// TestCrossDomainE2E_TrackQueryNoLLM verifies that "list active tracks"
// resolves via ClassifyPromptCrossDomain with confidence ≥ 0.9, no LLM call.
func TestCrossDomainE2E_TrackQueryNoLLM(t *testing.T) {
	origRoute := routePromptFn
	routePromptFn = noLLMRoutePromptFn
	defer func() { routePromptFn = origRoute }()

	cmds := ClassifyPromptCrossDomain("list active tracks")
	if len(cmds) == 0 {
		t.Fatal("ClassifyPromptCrossDomain returned no commands for 'list active tracks'")
	}
	rc := cmds[0]
	if rc.Cmd != "track" {
		t.Errorf("expected Cmd 'track', got %q", rc.Cmd)
	}
	if len(rc.Args) == 0 || rc.Args[0] != "list" {
		t.Errorf("expected args[0] 'list', got %v", rc.Args)
	}
	foundStatus := false
	for i, a := range rc.Args {
		if a == "--status" && i+1 < len(rc.Args) && rc.Args[i+1] == "active" {
			foundStatus = true
		}
	}
	if !foundStatus {
		t.Errorf("expected '--status active' in args, got %v", rc.Args)
	}
	if rc.Confidence < 0.9 {
		t.Errorf("expected confidence ≥ 0.9, got %f", rc.Confidence)
	}
}

// TestCrossDomainE2E_FuzzyTypoNoLLM verifies that "list trakcs" resolves via
// fuzzy match (noun distance 1, confidence ~0.9), no LLM call.
func TestCrossDomainE2E_FuzzyTypoNoLLM(t *testing.T) {
	origRoute := routePromptFn
	routePromptFn = noLLMRoutePromptFn
	defer func() { routePromptFn = origRoute }()

	cmds := ClassifyPromptCrossDomain("list trakcs")
	if len(cmds) == 0 {
		t.Fatal("ClassifyPromptCrossDomain returned no commands for 'list trakcs'")
	}
	rc := cmds[0]
	if rc.Cmd != "track" {
		t.Errorf("expected Cmd 'track', got %q", rc.Cmd)
	}
	if len(rc.Args) == 0 || rc.Args[0] != "list" {
		t.Errorf("expected args[0] 'list', got %v", rc.Args)
	}
	// Fuzzy noun match: "trakcs"→"tracks" is Levenshtein distance 2 (transposition
	// counts as 2 in standard Levenshtein) → conf 0.8; exact verb → 1.0; product 0.8.
	if rc.Confidence < 0.75 {
		t.Errorf("expected confidence ≥ 0.75 (fuzzy), got %f", rc.Confidence)
	}
}

// TestCrossDomainE2E_FlowRunNoLLM verifies that "run deploy flow" resolves to
// flow run deploy, no LLM call.
func TestCrossDomainE2E_FlowRunNoLLM(t *testing.T) {
	origRoute := routePromptFn
	routePromptFn = noLLMRoutePromptFn
	defer func() { routePromptFn = origRoute }()

	cmds := ClassifyPromptCrossDomain("run deploy flow")
	if len(cmds) == 0 {
		t.Fatal("ClassifyPromptCrossDomain returned no commands for 'run deploy flow'")
	}
	rc := cmds[0]
	if rc.Cmd != "flow" {
		t.Errorf("expected Cmd 'flow', got %q", rc.Cmd)
	}
	if len(rc.Args) < 2 || rc.Args[0] != "run" || rc.Args[1] != "deploy" {
		t.Errorf("expected args ['run', 'deploy', ...], got %v", rc.Args)
	}
	if rc.Confidence < 0.9 {
		t.Errorf("expected confidence ≥ 0.9, got %f", rc.Confidence)
	}
}

// TestCrossDomainE2E_CountActiveTracksNoLLM verifies that "count active tracks"
// resolves to track list --status active --counters, no LLM call.
func TestCrossDomainE2E_CountActiveTracksNoLLM(t *testing.T) {
	origRoute := routePromptFn
	routePromptFn = noLLMRoutePromptFn
	defer func() { routePromptFn = origRoute }()

	cmds := ClassifyPromptCrossDomain("count active tracks")
	if len(cmds) == 0 {
		t.Fatal("ClassifyPromptCrossDomain returned no commands for 'count active tracks'")
	}
	rc := cmds[0]
	if rc.Cmd != "track" {
		t.Errorf("expected Cmd 'track', got %q", rc.Cmd)
	}
	foundCounters := false
	foundStatus := false
	for i, a := range rc.Args {
		if a == "--counters" {
			foundCounters = true
		}
		if a == "--status" && i+1 < len(rc.Args) && rc.Args[i+1] == "active" {
			foundStatus = true
		}
	}
	if !foundCounters {
		t.Errorf("expected '--counters' in args, got %v", rc.Args)
	}
	if !foundStatus {
		t.Errorf("expected '--status active' in args, got %v", rc.Args)
	}
	if rc.Confidence < 0.9 {
		t.Errorf("expected confidence ≥ 0.9, got %f", rc.Confidence)
	}
}
