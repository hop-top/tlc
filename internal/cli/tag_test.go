package cli

import (
	"bytes"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestTagList verifies `tlc tag list` / `tlc tag ls`.
func TestTagList(t *testing.T) {
	t.Run("ListsUniqueTags", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "A", Status: core.StatusTodo,
			Tags: []string{"feat", "auth"},
		})
		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "B", Status: core.StatusTodo,
			Tags: []string{"feat", "bug"},
		})

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("tag list failed: %v", err)
		}

		out := buf.String()
		for _, tag := range []string{"feat", "auth", "bug"} {
			if !contains(out, tag) {
				t.Errorf("expected tag %q in output; got: %s", tag, out)
			}
		}
	})

	t.Run("EmptyOutput", func(t *testing.T) {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("tag list (empty) failed: %v", err)
		}

		if !contains(buf.String(), "No tags found") {
			t.Errorf("expected 'No tags found' message; got: %s", buf.String())
		}
	})

	t.Run("AliasLs", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "A", Status: core.StatusTodo,
			Tags: []string{"docs"},
		})

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "ls"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("tag ls failed: %v", err)
		}

		if !contains(buf.String(), "docs") {
			t.Errorf("expected 'docs' tag in output; got: %s", buf.String())
		}
	})
}

// TestTagAdd verifies `tlc tag <task-id> <tag…>`.
func TestTagAdd(t *testing.T) {
	t.Run("AddTagsToTask", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Task", Status: core.StatusTodo,
			Tags: []string{"existing"},
		})

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "T-0001", "feat", "auth"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("tag add failed: %v", err)
		}

		task, err := s.GetTask(ctx, "T-0001")
		if err != nil || task == nil {
			t.Fatalf("failed to get task: %v", err)
		}

		tagSet := make(map[string]bool)
		for _, tg := range task.Tags {
			tagSet[tg] = true
		}
		for _, expected := range []string{"existing", "feat", "auth"} {
			if !tagSet[expected] {
				t.Errorf("expected tag %q on task; tags: %v", expected, task.Tags)
			}
		}
	})

	t.Run("DeduplicatesTags", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Task", Status: core.StatusTodo,
			Tags: []string{"feat"},
		})

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "T-0001", "feat", "feat"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("tag add (dedup) failed: %v", err)
		}

		task, _ := s.GetTask(ctx, "T-0001")
		if len(task.Tags) != 1 {
			t.Errorf("expected 1 tag after dedup; got %d: %v", len(task.Tags), task.Tags)
		}
	})

	t.Run("ErrorOnUnknownTask", func(t *testing.T) {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "T-9999", "feat"})

		err := cmd.Execute()
		if err == nil {
			t.Error("expected error for unknown task ID")
		}
	})
}

// TestTagFilter verifies `tlc tag <tag…>` filter mode.
func TestTagFilter(t *testing.T) {
	t.Run("SingleTagFilter", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Auth task", Status: core.StatusTodo,
			Tags: []string{"auth", "feat"},
		})
		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "Docs task", Status: core.StatusTodo,
			Tags: []string{"docs"},
		})

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "auth"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("tag filter (single) failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "Auth task") {
			t.Errorf("expected 'Auth task' in output; got: %s", out)
		}
		if contains(out, "Docs task") {
			t.Errorf("unexpected 'Docs task' in output; got: %s", out)
		}
	})

	t.Run("ANDFilter_MultipleArgs", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Auth+Feat", Status: core.StatusTodo,
			Tags: []string{"auth", "feat"},
		})
		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "Auth only", Status: core.StatusTodo,
			Tags: []string{"auth"},
		})

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "auth", "feat"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("tag filter (AND) failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "Auth+Feat") {
			t.Errorf("expected 'Auth+Feat' in output; got: %s", out)
		}
		if contains(out, "Auth only") {
			t.Errorf("unexpected 'Auth only' in AND-filtered output; got: %s", out)
		}
	})

	t.Run("ORFilter_CommaSeparated", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Auth task", Status: core.StatusTodo,
			Tags: []string{"auth"},
		})
		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "Bug task", Status: core.StatusTodo,
			Tags: []string{"bug"},
		})
		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0003", Title: "Docs task", Status: core.StatusTodo,
			Tags: []string{"docs"},
		})

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "auth,bug"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("tag filter (OR) failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "Auth task") {
			t.Errorf("expected 'Auth task' in OR output; got: %s", out)
		}
		if !contains(out, "Bug task") {
			t.Errorf("expected 'Bug task' in OR output; got: %s", out)
		}
		if contains(out, "Docs task") {
			t.Errorf("unexpected 'Docs task' in OR output; got: %s", out)
		}
	})

	t.Run("ANDofOR_MixedArgs", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		// task matches: feat AND (auth OR bug) → T-0001
		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Feat+Auth", Status: core.StatusTodo,
			Tags: []string{"feat", "auth"},
		})
		// has feat but neither auth nor bug → no match
		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "Feat only", Status: core.StatusTodo,
			Tags: []string{"feat"},
		})
		// has auth but not feat → no match
		_ = s.CreateTask(ctx, &core.Task{
			ID: "T-0003", Title: "Auth only", Status: core.StatusTodo,
			Tags: []string{"auth"},
		})

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag", "feat", "auth,bug"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("tag filter (AND of OR) failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "Feat+Auth") {
			t.Errorf("expected 'Feat+Auth' in AND-of-OR output; got: %s", out)
		}
		if contains(out, "Feat only") {
			t.Errorf("unexpected 'Feat only' in output; got: %s", out)
		}
		if contains(out, "Auth only") {
			t.Errorf("unexpected 'Auth only' in output; got: %s", out)
		}
	})

	t.Run("NoArgsShowsHelp", func(t *testing.T) {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TagCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tag"})

		// help output is not an error
		_ = cmd.Execute()
		out := buf.String()
		if !contains(out, "tag") {
			t.Errorf("expected help output for 'tlc tag'; got: %s", out)
		}
	})
}

// TestLooksLikeTaskID verifies the task-ID heuristic.
func TestLooksLikeTaskID(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"T-0001", true},
		{"T-1", true},
		{"X-999", true},
		{"hop/T-0001", true},
		{"tlc://T-0001", true},
		{"feat", false},
		{"auth,bug", false},
		{"t-0001", false}, // lower-case prefix → not a task ID by convention
		{"", false},
	}
	for _, tc := range cases {
		got := looksLikeTaskID(tc.input)
		if got != tc.want {
			t.Errorf("looksLikeTaskID(%q) = %v; want %v", tc.input, got, tc.want)
		}
	}
}
