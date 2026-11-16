package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

func defaultBoard() []core.TaskStatus {
	return []core.TaskStatus{"todo", "in_progress", "in_review", "blocked", "done"}
}

func defaultRole(s core.TaskStatus) string {
	switch s {
	case "todo":
		return config.RoleInitial
	case "in_progress":
		return config.RoleActive
	case "in_review":
		return config.RoleReview
	case "blocked":
		return config.RoleBlocked
	case "done":
		return config.RoleCompleted
	}
	return ""
}

func allPopulated() map[core.TaskStatus]int {
	return map[core.TaskStatus]int{"todo": 5, "in_progress": 1, "in_review": 2, "blocked": 1, "done": 3}
}

func TestKanbanLayout_FitShowsEverything(t *testing.T) {
	shown, hidden := kanbanLayout(defaultBoard(), allPopulated(), defaultRole, 120)
	if !reflect.DeepEqual(shown, defaultBoard()) || len(hidden) != 0 {
		t.Errorf("shown=%v hidden=%v, want everything shown", shown, hidden)
	}
	// Unknown width must not hide anything.
	shown, hidden = kanbanLayout(defaultBoard(), allPopulated(), defaultRole, 0)
	if len(shown) != 5 || len(hidden) != 0 {
		t.Errorf("zero width hid columns: shown=%v hidden=%v", shown, hidden)
	}
}

func TestKanbanLayout_EmptyColumnsGoFirst(t *testing.T) {
	counts := map[core.TaskStatus]int{"todo": 2, "in_progress": 1, "done": 1}
	// width 80 holds three columns: the two empty ones are enough.
	shown, hidden := kanbanLayout(defaultBoard(), counts, defaultRole, 80)
	want := []core.TaskStatus{"todo", "in_progress", "done"}
	if !reflect.DeepEqual(shown, want) {
		t.Errorf("shown = %v, want %v", shown, want)
	}
	if !reflect.DeepEqual(hidden, []core.TaskStatus{"in_review", "blocked"}) {
		t.Errorf("hidden = %v", hidden)
	}
}

func TestKanbanLayout_DropOrderByRole(t *testing.T) {
	cases := []struct {
		width int
		shown []core.TaskStatus
		hid   []core.TaskStatus
	}{
		{80, []core.TaskStatus{"in_progress", "in_review", "blocked"}, []core.TaskStatus{"done", "todo"}},
		{60, []core.TaskStatus{"in_progress", "in_review"}, []core.TaskStatus{"done", "todo", "blocked"}},
		{40, []core.TaskStatus{"in_progress"}, []core.TaskStatus{"done", "todo", "blocked", "in_review"}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.width), func(t *testing.T) {
			shown, hidden := kanbanLayout(defaultBoard(), allPopulated(), defaultRole, tc.width)
			if !reflect.DeepEqual(shown, tc.shown) || !reflect.DeepEqual(hidden, tc.hid) {
				t.Errorf("shown=%v hidden=%v, want %v / %v", shown, hidden, tc.shown, tc.hid)
			}
		})
	}
}

func TestKanbanLayout_NeverDropsActive(t *testing.T) {
	for _, w := range []int{1, 10, 24, 30} {
		shown, _ := kanbanLayout(defaultBoard(), allPopulated(), defaultRole, w)
		if len(shown) != 1 || shown[0] != "in_progress" {
			t.Errorf("width %d: shown = %v, want [in_progress]", w, shown)
		}
	}
}

func TestKanbanLayout_CustomVocabulary(t *testing.T) {
	cols := []core.TaskStatus{"TODO", "DOING", "QA", "SHIPPED"}
	roles := map[core.TaskStatus]string{
		"TODO": config.RoleInitial, "DOING": config.RoleActive,
		"QA": "qa", "SHIPPED": config.RoleCompleted,
	}
	counts := map[core.TaskStatus]int{"TODO": 1, "DOING": 1, "QA": 1, "SHIPPED": 1}
	roleOf := func(s core.TaskStatus) string { return roles[s] }

	// Unknown roles leave before the well-known ones.
	shown, hidden := kanbanLayout(cols, counts, roleOf, 80)
	if !reflect.DeepEqual(hidden, []core.TaskStatus{"QA"}) {
		t.Errorf("hidden = %v, want [QA]; shown=%v", hidden, shown)
	}

	// Two active-role columns: rightmost goes first once nothing else can.
	cols = []core.TaskStatus{"A", "B", "C"}
	roleOf = func(core.TaskStatus) string { return config.RoleActive }
	counts = map[core.TaskStatus]int{"A": 1, "B": 1, "C": 1}
	shown, hidden = kanbanLayout(cols, counts, roleOf, 50)
	if !reflect.DeepEqual(shown, []core.TaskStatus{"A", "B"}) || !reflect.DeepEqual(hidden, []core.TaskStatus{"C"}) {
		t.Errorf("shown=%v hidden=%v, want rightmost-first drop", shown, hidden)
	}
}

func TestKanbanLayout_AllEmptyStillShowsABoard(t *testing.T) {
	shown, _ := kanbanLayout(defaultBoard(), map[core.TaskStatus]int{}, defaultRole, 80)
	if len(shown) == 0 {
		t.Error("an all-empty board rendered no columns")
	}
}

func TestKanbanHint(t *testing.T) {
	counts := allPopulated()
	if got := kanbanHint(nil, counts); got != "" {
		t.Errorf("hint with nothing hidden = %q", got)
	}
	got := kanbanHint([]core.TaskStatus{"done", "todo"}, counts)
	want := "hidden: done (3), todo (5); widen terminal to show"
	if got != want {
		t.Errorf("hint = %q, want %q", got, want)
	}
}

// populatedBoard is a board with work in every default column.
func populatedBoard(t *testing.T, width int) Model {
	t.Helper()
	m := goldenModel(t)
	mk := func(seq int, title string, st core.TaskStatus) *core.Task {
		return &core.Task{
			ID:     fmt.Sprintf("task_01h455vb4pex5vsknk084sn%03d", 100+seq),
			Seq:    int64(seq),
			Title:  title,
			Status: st,
		}
	}
	m.tasks = []*core.Task{
		mk(1, "Draft spec", "todo"), mk(2, "Write docs", "todo"),
		mk(3, "Build parser", "in_progress"),
		mk(4, "Review parser", "in_review"),
		mk(5, "Waiting on vendor", "blocked"),
		mk(6, "Ship v1", "done"), mk(7, "Ship v2", "done"), mk(8, "Ship v3", "done"),
	}
	m.width = width
	m.view = viewKanban
	return m
}

func TestGoldenKanbanPopulated(t *testing.T) {
	for _, w := range []int{60, 80, 120} {
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			m := populatedBoard(t, w)
			assertGolden(t, fmt.Sprintf("kanban_populated_%d", w), m.View().Content)
		})
	}
}

func TestKanbanView_HintOnlyWhenHidden(t *testing.T) {
	wide := populatedBoard(t, 120).kanbanView()
	if strings.Contains(wide, "hidden:") {
		t.Errorf("board that fits shows a hint:\n%s", wide)
	}
	narrow := populatedBoard(t, 80).kanbanView()
	if !strings.Contains(narrow, "hidden: done (3), todo (2); widen terminal to show") {
		t.Errorf("narrow board missing the hint:\n%s", narrow)
	}
}

func TestMoveTask_TargetsVisibleColumns(t *testing.T) {
	m := populatedBoard(t, 80)
	shown, _ := m.kanbanColumns()
	for _, s := range shown {
		if s == "done" || s == "todo" {
			t.Errorf("hidden column %s offered as a move target", s)
		}
	}
}

// TestMoveTask_DoesNotStepOntoHiddenColumn pins the behavior, not just
// the column list: at 80 columns blocked is the last visible column, so
// moving right from it has nowhere to go and must do nothing, rather than
// transitioning the task to the hidden done column.
func TestMoveTask_DoesNotStepOntoHiddenColumn(t *testing.T) {
	m := populatedBoard(t, 80)
	var blocked *core.Task
	for _, task := range m.tasks {
		if task.Status == "blocked" {
			blocked = task
		}
	}
	if blocked == nil {
		t.Fatal("fixture has no blocked task")
	}
	if msg := m.moveTask(blocked, 1)(); msg != nil {
		t.Errorf("moveTask past the last visible column returned %v, want nil", msg)
	}
}
