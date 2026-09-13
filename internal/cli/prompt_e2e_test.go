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

		task := getTaskByAlias(t, ctx, "T-0001")
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

		// Run prompt task for the created task.
		resetTaskFlags()
		taskNLJSON = false
		cmd2 := newTestCmd()
		cmd2.AddCommand(PromptCmd)
		buf2 := new(bytes.Buffer)
		cmd2.SetOut(buf2)
		cmd2.SetErr(buf2)
		cmd2.SetArgs([]string{"prompt", "task", "T-0001"})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("prompt task failed: %v", err)
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

		// Run prompt task with --json.
		resetTaskFlags()
		taskNLJSON = false // reset; the flag will be set by cobra
		cmd2 := newTestCmd()
		cmd2.AddCommand(PromptCmd)
		buf2 := new(bytes.Buffer)
		cmd2.SetOut(buf2)
		cmd2.SetErr(buf2)
		cmd2.SetArgs([]string{"prompt", "task", "T-0001", "--json"})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("prompt task --json failed: %v", err)
		}

		// Verify valid JSON.
		var parsed promptJSONOutput
		if err := json.Unmarshal(buf2.Bytes(), &parsed); err != nil {
			t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf2.String())
		}

		// JSON `id` is the durable TypeID; user-facing alias lives in
		// the `alias` field so scripts can pick whichever they need.
		if !core.IsTaskID(parsed.ID) {
			t.Errorf("expected TypeID-shaped id, got %q", parsed.ID)
		}
		if parsed.Alias != "T-0001" {
			t.Errorf("expected alias T-0001, got %q", parsed.Alias)
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

		taskNLJSON = false
		cmd := newTestCmd()
		cmd.AddCommand(PromptCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"prompt", "task", "T-9999"})

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

		task := getTaskByAlias(t, ctx, "T-0001")
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
		task := getTaskByAlias(t, ctx, "T-0001")
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

// TestCrossDomainE2E_TrackQueryNoLLM verifies that "active tracks" resolves via
// the full NL pipeline (Stage1→Stage2→Stage3) with no LLM call.
// Prompt uses "active tracks" (no leading subcommand word) so cobra routes
// to RootCmd.RunE (runNLPrompt), exercising Stage2 cross-domain classifier.
// The panic in noLLMRoutePromptFn proves Stage3 (LLM) was not reached.
func TestCrossDomainE2E_TrackQueryNoLLM(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		_, cleanup := setupTestDir(t)
		defer cleanup()

		origRoute := routePromptFn
		routePromptFn = noLLMRoutePromptFn
		defer func() { routePromptFn = origRoute }()

		cmd := newTestNLRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		// "active tracks" → modifier+noun → Stage2 injects implicit "list" verb
		// → track list --status active. NL args go directly to root.
		cmd.SetArgs([]string{"--dry-run", "active", "tracks"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if !contains(output, "track") {
			t.Errorf("expected 'track' in dry-run output, got:\n%s", output)
		}
		if !contains(output, "list") {
			t.Errorf("expected 'list' in dry-run output, got:\n%s", output)
		}
		if !contains(output, "--status") || !contains(output, "active") {
			t.Errorf("expected '--status active' in dry-run output, got:\n%s", output)
		}
	})
}

// TestCrossDomainE2E_FuzzyTypoNoLLM verifies that "trakcs" (typo) resolves via
// fuzzy match through the full NL pipeline, no LLM call.
// "trakcs"→"tracks" is Levenshtein distance 2 → confidence 0.8.
// The panic in noLLMRoutePromptFn proves LLM was not reached.
func TestCrossDomainE2E_FuzzyTypoNoLLM(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		_, cleanup := setupTestDir(t)
		defer cleanup()

		origRoute := routePromptFn
		routePromptFn = noLLMRoutePromptFn
		defer func() { routePromptFn = origRoute }()

		cmd := newTestNLRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		// "find trakcs": "find" is a verb (VerbQuery), "trakcs" is a typo of "tracks";
		// Stage2 fuzzy recovery resolves noun → track list. NL args go to root.
		cmd.SetArgs([]string{"--dry-run", "find", "trakcs"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if !contains(output, "track") {
			t.Errorf("expected 'track' in dry-run output (fuzzy match), got:\n%s", output)
		}
		if !contains(output, "list") {
			t.Errorf("expected 'list' in dry-run output, got:\n%s", output)
		}
	})
}

// TestCrossDomainE2E_CountActiveTracksNoLLM verifies that "count active tracks"
// resolves to track list --status active through the full NL pipeline, no LLM call.
// Track domain does not support --counters; it is dropped.
// The panic in noLLMRoutePromptFn proves LLM was not reached.
func TestCrossDomainE2E_CountActiveTracksNoLLM(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		_, cleanup := setupTestDir(t)
		defer cleanup()

		origRoute := routePromptFn
		routePromptFn = noLLMRoutePromptFn
		defer func() { routePromptFn = origRoute }()

		cmd := newTestNLRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		// "count active tracks" → Stage2 cross-domain → track list --status active.
		// NL args go directly to root.
		cmd.SetArgs([]string{"--dry-run", "count", "active", "tracks"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if !contains(output, "track") {
			t.Errorf("expected 'track' in dry-run output, got:\n%s", output)
		}
		if !contains(output, "--status") || !contains(output, "active") {
			t.Errorf("expected '--status active' in dry-run output, got:\n%s", output)
		}
		// Track domain: --counters must NOT appear (task-only flag).
		if contains(output, "--counters") {
			t.Errorf("--counters must not appear in track domain output, got:\n%s", output)
		}
	})
}
