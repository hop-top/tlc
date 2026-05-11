package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

func TestTaskCreate_Due(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "create", "Ship v2", "--due", "tomorrow",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create --due failed: %v", err)
	}

	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(context.Background(), core.Query{})
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].DueAt == nil {
		t.Fatal("DueAt should be set")
	}
	diff := tasks[0].DueAt.Sub(time.Now())
	if diff < 23*time.Hour || diff > 25*time.Hour {
		t.Errorf("DueAt should be ~24h from now, got %v", diff)
	}
}

func TestTaskCreate_DueISO(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "create", "Release", "--due", "2025-05-01",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create --due ISO failed: %v", err)
	}

	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(context.Background(), core.Query{})
	if tasks[0].DueAt == nil {
		t.Fatal("DueAt should be set")
	}
	expected := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	if tasks[0].DueAt.Unix() != expected.Unix() {
		t.Errorf("expected %v, got %v", expected, *tasks[0].DueAt)
	}
}

func TestTaskCreate_RRule(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "create", "Check CI",
		"--rrule", "FREQ=HOURLY;INTERVAL=1",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create --rrule failed: %v", err)
	}

	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(context.Background(), core.Query{})
	if tasks[0].RRule == "" {
		t.Fatal("RRule should be set")
	}
	if tasks[0].RRule != "FREQ=HOURLY;INTERVAL=1" {
		t.Errorf("expected FREQ=HOURLY;INTERVAL=1, got %v", tasks[0].RRule)
	}
}

func TestTaskCreate_NoAutoRemind(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "create", "Review",
		"--due", "2025-05-01", "--no-auto-remind",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed: %v", err)
	}

	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(context.Background(), core.Query{})
	if !tasks[0].NoAutoRemind {
		t.Error("NoAutoRemind should be true")
	}
	if tasks[0].DueAt == nil {
		t.Error("DueAt should be set")
	}
}

func TestTaskRemind_CheckOverdue(t *testing.T) {
	dbPath := resetTestDB(t)
	viper.Set("storage.db_path", dbPath)

	// Create a task with past due date via storage directly
	s, _ := getStorageRaw()
	past := time.Now().Add(-24 * time.Hour)
	s.CreateTask(context.Background(), &core.Task{
		ID:        "T-0001",
		Title:     "Overdue task",
		Status:    core.StatusTodo,
		DueAt:     &past,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	s.Close()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "remind", "--check"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("remind --check should fail with overdue tasks")
	}
	if !strings.Contains(err.Error(), "overdue") {
		t.Errorf("error should mention overdue: %v", err)
	}
}

func TestTaskRemind_CheckClean(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))

	// Create task with future due
	s, _ := getStorageRaw()
	future := time.Now().Add(48 * time.Hour)
	s.CreateTask(context.Background(), &core.Task{
		ID:        "T-0001",
		Title:     "Future task",
		Status:    core.StatusTodo,
		DueAt:     &future,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	s.Close()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "remind", "--check"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remind --check should pass: %v", err)
	}
}

func TestTaskCreate_AutoDueFromConfig(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))
	viper.Set("task.scheduling.by_priority", map[string]interface{}{
		"P0": map[string]interface{}{
			"due":   "24h",
			"rrule": "FREQ=HOURLY;INTERVAL=2",
		},
	})
	defer viper.Set("task.scheduling.by_priority", nil)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "create", "Critical fix", "--priority", "P0",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed: %v", err)
	}

	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(context.Background(), core.Query{})
	if tasks[0].DueAt == nil {
		t.Fatal("auto-due should be set from config")
	}
	diff := tasks[0].DueAt.Sub(time.Now())
	if diff < 23*time.Hour || diff > 25*time.Hour {
		t.Errorf("auto-due should be ~24h, got %v", diff)
	}
	if tasks[0].RRule == "" {
		t.Fatal("auto-rrule should be set from config")
	}
}

func TestTaskCreate_ExplicitDueOverridesConfig(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))
	viper.Set("task.scheduling.by_priority", map[string]interface{}{
		"P0": map[string]interface{}{"due": "24h"},
	})
	defer viper.Set("task.scheduling.by_priority", nil)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "create", "Critical fix",
		"--priority", "P0", "--due", "in 3 days",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed: %v", err)
	}

	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(context.Background(), core.Query{})
	diff := tasks[0].DueAt.Sub(time.Now())
	// Should be ~3 days, not ~24h
	if diff < 71*time.Hour || diff > 73*time.Hour {
		t.Errorf("explicit --due should override config, got %v", diff)
	}
}

func TestTaskRemind_AgeNudge(t *testing.T) {
	// Test CollectAgeNudges directly — avoids viper/initConfig
	// isolation issues that plague cmd.Execute() in batch mode.
	now := time.Now()
	staleTask := &core.Task{
		ID:        "T-0001",
		Title:     "Stale WIP",
		Status:    core.StatusInProgress,
		CreatedAt: now.Add(-72 * time.Hour),
		UpdatedAt: now.Add(-72 * time.Hour),
	}
	freshTask := &core.Task{
		ID:        "T-0002",
		Title:     "Fresh",
		Status:    core.StatusInProgress,
		CreatedAt: now.Add(-1 * time.Hour),
		UpdatedAt: now.Add(-1 * time.Hour),
	}
	doneTask := &core.Task{
		ID:        "T-0003",
		Title:     "Done",
		Status:    core.StatusDone,
		UpdatedAt: now.Add(-72 * time.Hour),
	}

	viper.Set("task.scheduling.age_nudges", []interface{}{
		map[string]interface{}{
			"status":    "IN_PROGRESS",
			"threshold": "48h",
			"action":    "remind",
		},
	})
	t.Cleanup(func() {
		viper.Set("task.scheduling.age_nudges", nil)
	})

	nudges := CollectAgeNudges([]*core.Task{staleTask, freshTask, doneTask})
	if len(nudges) != 1 {
		t.Fatalf("expected 1 nudge, got %d", len(nudges))
	}
	if nudges[0].TaskID != "T-0001" {
		t.Errorf("expected T-0001, got %s", nudges[0].TaskID)
	}
	if nudges[0].Action != "remind" {
		t.Errorf("expected remind, got %s", nudges[0].Action)
	}
}

func TestTaskUpdate_ClearDue(t *testing.T) {
	dbPath := resetTestDB(t)
	viper.Set("storage.db_path", dbPath)

	s, _ := getStorageRaw()
	due := time.Now().Add(24 * time.Hour)
	s.CreateTask(context.Background(), &core.Task{
		ID:        "T-0001",
		Title:     "Has due",
		Status:    core.StatusTodo,
		DueAt:     &due,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	s.Close()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "update", "T-0001", "--due", "-",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("update --due - failed: %v", err)
	}

	s2, _ := getStorageRaw()
	defer s2.Close()
	task, _ := s2.GetTask(context.Background(), "T-0001")
	if task.DueAt != nil {
		t.Error("DueAt should be nil after --due -")
	}
}

func TestTaskList_DueColumn(t *testing.T) {
	dbPath := resetTestDB(t)
	viper.Set("storage.db_path", dbPath)

	s, _ := getStorageRaw()
	due := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	s.CreateTask(context.Background(), &core.Task{
		ID:        "T-0001",
		Title:     "Task with due",
		Status:    core.StatusTodo,
		DueAt:     &due,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	s.Close()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}

	output := buf.String()
	// T-1384: table format renders DueAt humanised ("Nd ago" /
	// "in Nd"). The 2025-05-01 fixture is in the past relative to
	// any plausible test runtime, so the marker shows "! " plus a
	// "ago" suffix. Assert the structural shape rather than an
	// exact number that drifts with wall-clock time.
	if !strings.Contains(output, "ago") {
		t.Errorf("list should show humanised past due ('ago'), got: %s", output)
	}
	if !strings.Contains(output, "! ") {
		t.Errorf("list should show overdue marker '! ', got: %s", output)
	}
	if strings.Contains(output, "2025-05-01") {
		t.Errorf("table column should not leak absolute date 2025-05-01 (humanise per T-1384), got: %s", output)
	}
}
