package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func newTestTask(id string) *core.Task {
	assigned := "alice"
	track := "track-1"
	return &core.Task{
		ID:         id,
		Title:      "Test Task",
		Status:     core.StatusTodo,
		Priority:   core.PriorityP1,
		Effort:     core.EffortM,
		Tags:       []string{"feat", "backend"},
		AssignedTo: &assigned,
		TrackID:    &track,
		CreatedAt:  time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
	}
}

func TestProjectorCanonicalFile(t *testing.T) {
	dir := t.TempDir()
	p := NewFilesystemProjector(FilesystemProjectorConfig{
		BaseDir: dir,
		GroupBy: []string{"status"},
		SortBy:  []string{"id"},
	})

	task := newTestTask("T-0001")
	if err := p.ProjectTask(task); err != nil {
		t.Fatalf("ProjectTask: %v", err)
	}

	canonical := filepath.Join(dir, "all", "T-0001.json")
	data, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}

	var got core.Task
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "T-0001" {
		t.Errorf("canonical ID = %q, want T-0001", got.ID)
	}
	if got.Title != "Test Task" {
		t.Errorf("canonical Title = %q, want 'Test Task'", got.Title)
	}
	if got.Status != core.StatusTodo {
		t.Errorf("canonical Status = %q, want TODO", got.Status)
	}
}

func TestProjectorSymlinksResolve(t *testing.T) {
	dir := t.TempDir()
	p := NewFilesystemProjector(FilesystemProjectorConfig{
		BaseDir: dir,
		GroupBy: []string{"status"},
		SortBy:  []string{"id"},
	})

	task := newTestTask("T-0002")
	if err := p.ProjectTask(task); err != nil {
		t.Fatalf("ProjectTask: %v", err)
	}

	linkDir := filepath.Join(dir, "by-status", "todo")
	entries, err := os.ReadDir(linkDir)
	if err != nil {
		t.Fatalf("readdir by-status/todo: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 symlink, got %d", len(entries))
	}

	linkPath := filepath.Join(linkDir, entries[0].Name())
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}

	// Symlink should resolve to the canonical file
	resolved := filepath.Join(linkDir, target)
	data, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatalf("read via symlink: %v", err)
	}

	var got core.Task
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "T-0002" {
		t.Errorf("symlink-resolved ID = %q, want T-0002", got.ID)
	}
}

func TestProjectorMultiTagFanOut(t *testing.T) {
	dir := t.TempDir()
	p := NewFilesystemProjector(FilesystemProjectorConfig{
		BaseDir: dir,
		GroupBy: []string{"tag"},
		SortBy:  []string{"id"},
	})

	task := newTestTask("T-0003")
	task.Tags = []string{"feat", "backend", "urgent"}
	if err := p.ProjectTask(task); err != nil {
		t.Fatalf("ProjectTask: %v", err)
	}

	for _, tag := range task.Tags {
		tagDir := filepath.Join(dir, "by-tag", Slug(tag))
		entries, err := os.ReadDir(tagDir)
		if err != nil {
			t.Errorf("readdir by-tag/%s: %v", tag, err)
			continue
		}
		if len(entries) != 1 {
			t.Errorf("by-tag/%s: expected 1 symlink, got %d", tag, len(entries))
		}
	}
}

func TestProjectorSortPrefix(t *testing.T) {
	dir := t.TempDir()
	p := NewFilesystemProjector(FilesystemProjectorConfig{
		BaseDir: dir,
		GroupBy: []string{"status"},
		SortBy:  []string{"priority", "id"},
	})

	task := newTestTask("T-0004")
	task.Priority = core.PriorityP0
	if err := p.ProjectTask(task); err != nil {
		t.Fatalf("ProjectTask: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "by-status", "todo"))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	name := entries[0].Name()
	want := "P0-T-0004.json"
	if name != want {
		t.Errorf("symlink name = %q, want %q", name, want)
	}
}

func TestProjectorUnsetDir(t *testing.T) {
	dir := t.TempDir()
	p := NewFilesystemProjector(FilesystemProjectorConfig{
		BaseDir: dir,
		GroupBy: []string{"priority"},
		SortBy:  []string{"id"},
	})

	task := newTestTask("T-0005")
	task.Priority = "" // unset
	if err := p.ProjectTask(task); err != nil {
		t.Fatalf("ProjectTask: %v", err)
	}

	unsetDir := filepath.Join(dir, "by-priority", "_unset")
	entries, err := os.ReadDir(unsetDir)
	if err != nil {
		t.Fatalf("readdir _unset: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 symlink in _unset, got %d", len(entries))
	}
}

func TestProjectorRebuildAllIdempotent(t *testing.T) {
	dir := t.TempDir()
	p := NewFilesystemProjector(FilesystemProjectorConfig{
		BaseDir: dir,
		GroupBy: []string{"status", "tag"},
		SortBy:  []string{"id"},
	})

	tasks := []*core.Task{
		newTestTask("T-0010"),
		newTestTask("T-0011"),
	}
	tasks[1].Status = core.StatusInProgress

	// Build twice — should be identical
	if err := p.RebuildAll(tasks); err != nil {
		t.Fatalf("RebuildAll #1: %v", err)
	}
	if err := p.RebuildAll(tasks); err != nil {
		t.Fatalf("RebuildAll #2: %v", err)
	}

	// Verify canonical files
	for _, task := range tasks {
		path := filepath.Join(dir, "all", task.ID+".json")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing canonical for %s: %v", task.ID, err)
		}
	}

	// Verify status dirs
	todoEntries, _ := os.ReadDir(filepath.Join(dir, "by-status", "todo"))
	if len(todoEntries) != 1 {
		t.Errorf("by-status/todo: expected 1, got %d", len(todoEntries))
	}
	ipEntries, _ := os.ReadDir(filepath.Join(dir, "by-status", "in-progress"))
	if len(ipEntries) != 1 {
		t.Errorf("by-status/in-progress: expected 1, got %d", len(ipEntries))
	}
}

func TestProjectorRemoveTask(t *testing.T) {
	dir := t.TempDir()
	p := NewFilesystemProjector(FilesystemProjectorConfig{
		BaseDir: dir,
		GroupBy: []string{"status", "tag"},
		SortBy:  []string{"id"},
	})

	task := newTestTask("T-0020")
	if err := p.ProjectTask(task); err != nil {
		t.Fatalf("ProjectTask: %v", err)
	}

	// Verify files exist before removal
	canonical := filepath.Join(dir, "all", "T-0020.json")
	if _, err := os.Stat(canonical); err != nil {
		t.Fatalf("canonical should exist: %v", err)
	}

	if err := p.RemoveTask("T-0020"); err != nil {
		t.Fatalf("RemoveTask: %v", err)
	}

	// Canonical should be gone
	if _, err := os.Stat(canonical); !os.IsNotExist(err) {
		t.Errorf("canonical should be removed after RemoveTask")
	}

	// No symlinks should remain pointing to T-0020.json
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(path)
			if filepath.Base(target) == "T-0020.json" {
				t.Errorf("stale symlink found: %s -> %s", path, target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walkdir: %v", err)
	}
}

func TestProjectorMultipleGroups(t *testing.T) {
	dir := t.TempDir()
	p := NewFilesystemProjector(FilesystemProjectorConfig{
		BaseDir: dir,
		GroupBy: []string{"status", "assigned_to", "track"},
		SortBy:  []string{"id"},
	})

	task := newTestTask("T-0030")
	if err := p.ProjectTask(task); err != nil {
		t.Fatalf("ProjectTask: %v", err)
	}

	// Check all three group dirs
	for _, check := range []struct{ group, val string }{
		{"by-status", "todo"},
		{"by-assigned_to", "alice"},
		{"by-track", "track-1"},
	} {
		d := filepath.Join(dir, check.group, check.val)
		entries, err := os.ReadDir(d)
		if err != nil {
			t.Errorf("readdir %s/%s: %v", check.group, check.val, err)
			continue
		}
		if len(entries) != 1 {
			t.Errorf("%s/%s: expected 1 symlink, got %d", check.group, check.val, len(entries))
		}
	}
}

func TestSlug(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"TODO", "todo"},
		{"IN_PROGRESS", "in-progress"},
		{"Hello World!", "hello-world"},
		{"  spaces  ", "spaces"},
		{"feat/backend", "feat-backend"},
	}
	for _, tt := range tests {
		got := Slug(tt.input)
		if got != tt.want {
			t.Errorf("Slug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
