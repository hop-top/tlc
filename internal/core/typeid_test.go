package core

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func TestNewTaskID(t *testing.T) {
	id := NewTaskID()
	if !strings.HasPrefix(id, "task_") {
		t.Fatalf("NewTaskID() = %q; want prefix \"task_\"", id)
	}
	if !IsTaskID(id) {
		t.Fatalf("IsTaskID(%q) = false; want true", id)
	}
}

func TestNewTrackID(t *testing.T) {
	id := NewTrackID()
	if !strings.HasPrefix(id, "track_") {
		t.Fatalf("NewTrackID() = %q; want prefix \"track_\"", id)
	}
	if !IsTrackID(id) {
		t.Fatalf("IsTrackID(%q) = false; want true", id)
	}
}

func TestIsTaskID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"task_01h455vb4pex5vsknk084sn02q", true},
		{"track_01h455vb4pex5vsknk084sn02q", false},
		{"T-0042", false},
		{"task_short", false},
		{"task_01H455VB4PEX5VSKNK084SN02Q", false}, // uppercase rejected
		{"", false},
		{"task_", false},
	}
	for _, c := range cases {
		if got := IsTaskID(c.in); got != c.want {
			t.Errorf("IsTaskID(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestIsTrackID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"track_01h455vb4pex5vsknk084sn02q", true},
		{"task_01h455vb4pex5vsknk084sn02q", false},
		{"auth-rewrite", false},
		{"track_short", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsTrackID(c.in); got != c.want {
			t.Errorf("IsTrackID(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestTaskIDsAreSortableByCreationTime(t *testing.T) {
	const n = 100
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = NewTaskID()
		time.Sleep(time.Millisecond) // ensure uuidv7 timestamps differ
	}

	sorted := make([]string, n)
	copy(sorted, ids)
	sort.Strings(sorted)

	for i := range ids {
		if ids[i] != sorted[i] {
			t.Fatalf("TypeIDs not lexicographically time-sorted at index %d: created %q, sort position %q",
				i, ids[i], sorted[i])
		}
	}
}

func TestNewTaskIDIsUnique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := NewTaskID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate TypeID generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewRecipeRunID(t *testing.T) {
	id := NewRecipeRunID()
	if !strings.HasPrefix(id, "run_") {
		t.Fatalf("NewRecipeRunID() = %q; want prefix \"run_\"", id)
	}
	if !IsRecipeRunID(id) {
		t.Fatalf("IsRecipeRunID(%q) = false; want true", id)
	}
	if IsTaskID(id) || IsTrackID(id) {
		t.Fatalf("run id %q reads as a task or track id", id)
	}
}

func TestIsRecipeRunID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"run_01h455vb4pex5vsknk084sn02q", true},
		{"task_01h455vb4pex5vsknk084sn02q", false},
		{"run-001", false},
		{"run_short", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsRecipeRunID(c.in); got != c.want {
			t.Errorf("IsRecipeRunID(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}
