package cli

import (
	"strings"
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

// TestResolveTaskReference_UsesTLCScheme verifies that resolved references
// use the tlc:// URI scheme, not the incorrect task:// scheme.
// See GH-2: references must be globally resolvable via the project's scheme.
func TestResolveTaskReference_UsesTLCScheme(t *testing.T) {
	projectID := "hop-top/tlc"

	t.Run("AbsoluteRefMustUseTLCScheme", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "tlc://hop-top/tlc/T-0001"}
		got := resolveTaskReference(task)
		want := "tlc://hop-top/tlc/T-0001"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("RelativeRefExpandsToTLCScheme", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "tlc://T-0001", ProjectID: &projectID}
		got := resolveTaskReference(task)
		want := "tlc://hop-top/tlc/T-0001"
		if got != want {
			t.Errorf("got %q, want %q — resolver must expand to tlc:// scheme", got, want)
		}
	})

	t.Run("EmptyRefDefaultsToTLCScheme", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "", ProjectID: &projectID}
		got := resolveTaskReference(task)
		want := "tlc://hop-top/tlc/T-0001"
		if got != want {
			t.Errorf("got %q, want %q — empty ref must resolve to tlc:// with project ID", got, want)
		}
	})

	t.Run("ResolvedRefMustNotUseTaskScheme", func(t *testing.T) {
		task := &core.Task{ID: "T-0001", Reference: "", ProjectID: &projectID}
		got := resolveTaskReference(task)
		if strings.HasPrefix(got, "task://") {
			t.Errorf("resolved reference %q uses task:// scheme; must use tlc:// — see GH-2", got)
		}
	})
}
