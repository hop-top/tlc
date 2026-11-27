package storage

import (
	"errors"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestTaskFieldNames_MatchesAdvertisedSet(t *testing.T) {
	if got := TaskFieldNames(); !sameStrings(got, sortableFields) {
		t.Errorf("TaskFieldNames = %v, want %v", got, sortableFields)
	}
	// The slice is a copy: mutating it must not widen the allowlist.
	names := TaskFieldNames()
	names[0] = "(SELECT 1)"
	if _, ok := resolveTaskField("(SELECT 1)"); ok {
		t.Error("mutating the returned slice widened the allowlist")
	}
}

func TestValidateTaskSort(t *testing.T) {
	for _, tc := range []struct {
		by, dir string
		err     error
	}{
		{"", "", nil},
		{"title", "asc", nil},
		{"status,created_at", "desc", nil},
		{"title; SELECT 1", "asc", core.ErrInvalidSortField},
		{"title", "up", core.ErrInvalidSortDirection},
		{"", "up", core.ErrInvalidSortDirection},
	} {
		if err := ValidateTaskSort(tc.by, tc.dir); !errors.Is(err, tc.err) && (tc.err != nil || err != nil) {
			t.Errorf("ValidateTaskSort(%q, %q) = %v, want %v", tc.by, tc.dir, err, tc.err)
		}
	}
}
