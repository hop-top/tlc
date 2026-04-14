package cli

import (
	"strings"
	"testing"

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
