package core

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempPlan(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp plan: %v", err)
	}
	return path
}

func TestParsePlanFrontmatter_WithTasks(t *testing.T) {
	path := writeTempPlan(t, `---
title: Use cached vectors
tracks: [rnd-improve-performance]
tasks:
  - title: "Benchmark current vector lookups"
    description: "Measure p50/p99 latency."
    tags: [phase:1, domain:perf]
    effort: S
    priority: P1
    assigned-to: "@me"
    blocked-by: []
  - title: "Implement vector cache layer"
    effort: M
    priority: P1
    blocked-by: [0]
  - title: "Validate cache hit ratio"
    tags: [phase:2]
    effort: S
    priority: P2
    blocked-by: [1]
---

# Plan body
`)
	fm, err := ParsePlanFrontmatter(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Title != "Use cached vectors" {
		t.Errorf("title = %q, want %q", fm.Title, "Use cached vectors")
	}
	if len(fm.Tracks) != 1 || fm.Tracks[0] != "rnd-improve-performance" {
		t.Errorf("tracks = %v, want [rnd-improve-performance]", fm.Tracks)
	}
	if len(fm.Tasks) != 3 {
		t.Fatalf("tasks len = %d, want 3", len(fm.Tasks))
	}

	task0 := fm.Tasks[0]
	if task0.Title != "Benchmark current vector lookups" {
		t.Errorf("task[0].Title = %q", task0.Title)
	}
	if task0.Effort != "S" {
		t.Errorf("task[0].Effort = %q", task0.Effort)
	}
	if task0.Priority != "P1" {
		t.Errorf("task[0].Priority = %q", task0.Priority)
	}
	if task0.AssignedTo != "@me" {
		t.Errorf("task[0].AssignedTo = %q", task0.AssignedTo)
	}
	if len(task0.BlockedBy) != 0 {
		t.Errorf("task[0].BlockedBy = %v, want []", task0.BlockedBy)
	}

	task1 := fm.Tasks[1]
	if len(task1.BlockedBy) != 1 || task1.BlockedBy[0] != 0 {
		t.Errorf("task[1].BlockedBy = %v, want [0]", task1.BlockedBy)
	}

	task2 := fm.Tasks[2]
	if len(task2.BlockedBy) != 1 || task2.BlockedBy[0] != 1 {
		t.Errorf("task[2].BlockedBy = %v, want [1]", task2.BlockedBy)
	}
}

func TestParsePlanFrontmatter_WithoutTasks(t *testing.T) {
	path := writeTempPlan(t, `---
title: Simple plan
---

Content here.
`)
	fm, err := ParsePlanFrontmatter(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Title != "Simple plan" {
		t.Errorf("title = %q", fm.Title)
	}
	if len(fm.Tasks) != 0 {
		t.Errorf("tasks len = %d, want 0", len(fm.Tasks))
	}
}

func TestParsePlanFrontmatter_WithTracks(t *testing.T) {
	path := writeTempPlan(t, `---
title: Multi-track plan
tracks: [track-a, track-b]
---
`)
	fm, err := ParsePlanFrontmatter(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fm.Tracks) != 2 {
		t.Fatalf("tracks len = %d, want 2", len(fm.Tracks))
	}
	if fm.Tracks[0] != "track-a" || fm.Tracks[1] != "track-b" {
		t.Errorf("tracks = %v", fm.Tracks)
	}
}

func TestParsePlanFrontmatter_MissingFrontmatter(t *testing.T) {
	path := writeTempPlan(t, `# No frontmatter here

Just a regular markdown file.
`)
	fm, err := ParsePlanFrontmatter(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil for no frontmatter, got %+v", fm)
	}
}

func TestParsePlanFrontmatter_MalformedYAML(t *testing.T) {
	path := writeTempPlan(t, `---
title: [bad yaml
  - this is: broken {{{
---
`)
	_, err := ParsePlanFrontmatter(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestParsePlanFrontmatter_EmptyFile(t *testing.T) {
	path := writeTempPlan(t, "")
	fm, err := ParsePlanFrontmatter(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil for empty file, got %+v", fm)
	}
}

func TestParsePlanFrontmatter_FileNotFound(t *testing.T) {
	_, err := ParsePlanFrontmatter("/nonexistent/plan.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParsePlanFrontmatter_UnclosedFrontmatter(t *testing.T) {
	path := writeTempPlan(t, `---
title: Unclosed
`)
	_, err := ParsePlanFrontmatter(path)
	if err == nil {
		t.Fatal("expected error for unclosed frontmatter")
	}
}
