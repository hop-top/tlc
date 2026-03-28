package cli

import (
	"testing"

	"hop.top/tlc/internal/core"
)

func TestResolveTaskReference(t *testing.T) {
	projectID := "myorg/myrepo"

	t.Run("AbsoluteRefUnchanged", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "task://myorg/myrepo/T-0001"}
		got := resolveTaskReference(task)
		if got != "task://myorg/myrepo/T-0001" {
			t.Errorf("got %q, want task://myorg/myrepo/T-0001", got)
		}
	})

	t.Run("RelativeExpandedWithTaskProjectID", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "task://T-0001", ProjectID: &projectID}
		got := resolveTaskReference(task)
		want := "task://myorg/myrepo/T-0001"
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

	t.Run("EmptyRefFallsBackToRelative", func(t *testing.T) {
		// No project available in test env; falls back to task://T-XXXX.
		task := &core.Task{ID: "T-0001", Reference: ""}
		got := resolveTaskReference(task)
		if got == "" {
			t.Error("expected non-empty reference")
		}
	})
}
