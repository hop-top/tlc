package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

func TestResolveTaskReference(t *testing.T) {
	projectID := "myorg/myrepo"
	hopTopID := "hop-top/tlc"

	t.Run("AbsoluteRefUnchanged", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "tlc://myorg/myrepo/T-0001"}
		got := resolveTaskReference(task)
		if got != "tlc://myorg/myrepo/T-0001" {
			t.Errorf("got %q, want tlc://myorg/myrepo/T-0001", got)
		}
	})

	t.Run("RelativeExpandedWithTaskProjectID", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "tlc:///T-0001", ProjectID: &projectID}
		got := resolveTaskReference(task)
		want := "tlc://myorg/myrepo/T-0001"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("LegacyDoubleSlashRelativeExpanded", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "tlc://T-0001", ProjectID: &projectID}
		got := resolveTaskReference(task)
		want := "tlc://myorg/myrepo/T-0001"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("LegacyTaskSchemeExpanded", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "task://T-0001", ProjectID: &projectID}
		got := resolveTaskReference(task)
		want := "tlc://myorg/myrepo/T-0001"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("NonTaskRefUnchanged", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "https://github.com/issues/42"}
		got := resolveTaskReference(task)
		if got != "https://github.com/issues/42" {
			t.Errorf("got %q, want https://github.com/issues/42", got)
		}
	})

	t.Run("EmptyRefFallsBackToLocal", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: ""}
		got := resolveTaskReference(task)
		if got == "" {
			t.Error("expected non-empty reference")
		}
	})

	t.Run("EmptyRefDefaultsToTLCScheme", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "", ProjectID: &hopTopID}
		got := resolveTaskReference(task)
		want := "tlc://hop-top/tlc/T-0001"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("ResolvedRefMustNotUseTaskScheme", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "", ProjectID: &hopTopID}
		got := resolveTaskReference(task)
		if strings.HasPrefix(got, "task://") {
			t.Errorf("resolved reference %q uses task:// scheme; must use tlc://", got)
		}
	})
}

// TestIsInternalTaskRef covers the predicate that drives Reference-line
// suppression in default human output (T-1313 display fix).
func TestIsInternalTaskRef(t *testing.T) {
	projectID := "hop-top/tlc"
	otherProj := "myorg/myrepo"

	cases := []struct {
		name string
		task *core.Task
		ref  string
		want bool
	}{
		{
			name: "EmptyRefIsInternal",
			task: &core.Task{ID: "task_01abc"},
			ref:  "",
			want: true,
		},
		{
			name: "AbsoluteProjectScopedTypeIDIsInternal",
			task: &core.Task{ID: "task_01abc", ProjectID: &projectID},
			ref:  "tlc://hop-top/tlc/task_01abc",
			want: true,
		},
		{
			name: "LocalDefaultFormIsInternal",
			task: &core.Task{ID: "T-0001"},
			ref:  "tlc:///T-0001",
			want: true,
		},
		{
			name: "ExternalGithubRefIsExternal",
			task: &core.Task{ID: "task_01abc", ProjectID: &projectID},
			ref:  "github:issues/123",
			want: false,
		},
		{
			name: "ExternalDocsRefIsExternal",
			task: &core.Task{ID: "task_01abc", ProjectID: &projectID},
			ref:  "docs/foo.md",
			want: false,
		},
		{
			name: "ExternalHTTPSRefIsExternal",
			task: &core.Task{ID: "task_01abc", ProjectID: &projectID},
			ref:  "https://example.com/issue/42",
			want: false,
		},
		{
			name: "DifferentProjectScopedRefIsExternal",
			task: &core.Task{ID: "task_01abc", ProjectID: &otherProj},
			ref:  "tlc://some-other/repo/task_01abc",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isInternalTaskRef(tc.ref, tc.task)
			if got != tc.want {
				t.Errorf("isInternalTaskRef(%q) = %v; want %v", tc.ref, got, tc.want)
			}
		})
	}
}

// TestRenderTaskDetail_ReferenceVisibility asserts the render-layer policy
// for the "Reference:" line in default vs verbose human output (T-1313).
func TestRenderTaskDetail_ReferenceVisibility(t *testing.T) {
	projectID := "hop-top/tlc"
	typeid := "task_01kqwymh2qeh7a9nqrr5gv8ywt"

	autoTask := &core.Task{
		ID:        typeid,
		Seq:       1313,
		Title:     "Auto-referenced task",
		Status:    core.StatusTodo,
		Reference: "tlc://hop-top/tlc/" + typeid,
		ProjectID: &projectID,
	}
	externalTask := &core.Task{
		ID:        typeid + "x",
		Seq:       1314,
		Title:     "Externally referenced task",
		Status:    core.StatusTodo,
		Reference: "github:issues/123",
		ProjectID: &projectID,
	}

	t.Run("DefaultHidesAutoTypeID", func(t *testing.T) {
		viper.Set("output.verbose", false)
		defer viper.Set("output.verbose", false)
		var buf bytes.Buffer
		renderTaskDetail(&buf, autoTask, nil)
		out := buf.String()
		if strings.Contains(out, "Reference:") {
			t.Errorf("default output should not show auto-generated Reference line; got:\n%s", out)
		}
		if strings.Contains(out, "task_01") {
			t.Errorf("default output must not leak typeid prefix task_01; got:\n%s", out)
		}
	})

	t.Run("VerboseShowsFullURI", func(t *testing.T) {
		viper.Set("output.verbose", true)
		defer viper.Set("output.verbose", false)
		var buf bytes.Buffer
		renderTaskDetail(&buf, autoTask, nil)
		out := buf.String()
		if !strings.Contains(out, "Reference:") {
			t.Errorf("verbose output must show Reference line; got:\n%s", out)
		}
		if !strings.Contains(out, "tlc://hop-top/tlc/"+typeid) {
			t.Errorf("verbose output must contain full URI; got:\n%s", out)
		}
	})

	t.Run("DefaultShowsExternalRef", func(t *testing.T) {
		viper.Set("output.verbose", false)
		defer viper.Set("output.verbose", false)
		var buf bytes.Buffer
		renderTaskDetail(&buf, externalTask, nil)
		out := buf.String()
		if !strings.Contains(out, "Reference:") {
			t.Errorf("default output must show Reference line for external ref; got:\n%s", out)
		}
		if !strings.Contains(out, "github:issues/123") {
			t.Errorf("default output must contain external ref value; got:\n%s", out)
		}
	})
}

// TestRenderTable_NoTypeIDLeak asserts the default `task list` table never
// renders a typeid in any column (T-1321 regression guard).
func TestRenderTable_NoTypeIDLeak(t *testing.T) {
	tasks := []*core.Task{
		{ID: "task_01kqwymh2qeh7a9nqrr5gv8ywt", Seq: 1313, Title: "auto"},
		{ID: "task_01kqwymh2qeh7a9nqrr5gv8ywx", Seq: 1314, Title: "another"},
	}
	var buf bytes.Buffer
	renderTable(&buf, tasks)
	out := buf.String()
	if strings.Contains(out, "task_01") {
		t.Errorf("default task list must not leak typeid; got:\n%s", out)
	}
	// Sanity: aliases must appear.
	if !strings.Contains(out, "T-1313") || !strings.Contains(out, "T-1314") {
		t.Errorf("default task list must show T-NNNN aliases; got:\n%s", out)
	}
}
