package tui

// The TUI under a RENAMED status vocabulary.
//
// Every literal the TUI spelled as core.StatusTodo / StatusInProgress /
// StatusDone / StatusSkipped was a WHOLE-VOCABULARY assumption, not one
// constant standing in for a role: the rotate ring, the create default,
// the kanban columns, the sort rank and the glyph table each enumerated
// the built-in four and had no answer for anything else. A config
// declaring BACKLOG/DOING/SHIPPED/DROPPED shares no name with that set,
// so the ring targeted a status the config never declared (and the
// service rejected the transition), created tasks landed in an
// undeclared TODO, and custom statuses got no column, no rank and no
// glyph.

import (
	"context"
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
)

// customVocab is the vocabulary under test. It shares no status name
// with the built-in set, so any surviving built-in literal misses it
// entirely rather than accidentally matching.
//
// Four statuses, two of them terminal, because the built-in set has
// that shape: a fix that handled only the first terminal status would
// pass against a single-terminal vocabulary and still be wrong.
func customVocab() *config.TaskConfig {
	return &config.TaskConfig{
		Statuses: []config.StatusDefinition{
			{
				Name: "BACKLOG", Label: "Backlog",
				Role: config.RoleInitial, TLSMarker: " ", Color: "#6c6c6c",
			},
			{
				Name: "DOING", Label: "Doing",
				Role: config.RoleActive, TLSMarker: ">", Color: "#00afff",
			},
			{
				Name: "SHIPPED", Label: "Shipped", IsTerminal: true,
				Role: config.RoleCompleted, TLSMarker: "x", Color: "#00af00",
			},
			{
				Name: "DROPPED", Label: "Dropped", IsTerminal: true,
				Role: config.RoleSkipped, TLSMarker: "-",
			},
		},
		StateMachine: &config.WorkflowDefinition{
			Rules: map[string][]string{
				"BACKLOG": {"DOING", "DROPPED"},
				"DOING":   {"SHIPPED", "DROPPED", "BACKLOG"},
			},
		},
	}
}

// withCustomVocab installs customVocab as the process workflow for the
// duration of one test.
func withCustomVocab(t *testing.T) {
	t.Helper()
	core.SetTaskConfigProvider(customVocab)
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})
}

func newTestModel(t *testing.T) (Model, *core.MockRepository) {
	t.Helper()
	repo := core.NewMockRepository()
	logRepo := core.NewMockLogRepository()
	svc := core.NewTaskService(repo, logRepo)
	return NewModel(svc, styles.DefaultKitTheme()), repo
}

// TestRotateStatus_OffersOnlyTransitionsTheWorkflowAccepts is the
// property that was actually broken. The ring hardcoded
// TODO->IN_PROGRESS->DONE->TODO with `default: TODO`, so under a custom
// vocabulary EVERY status fell to the default and the key targeted
// BACKLOG-equivalent-of-nothing: a status name the config never
// declared. The service validates against DefaultWorkflow, so the
// transition was rejected and the keypress silently did nothing.
//
// Asserting "next == X" for each status would only pin today's answer.
// The invariant is stronger and is what the user actually needs: every
// target the ring offers must be one the workflow accepts.
func TestRotateStatus_OffersOnlyTransitionsTheWorkflowAccepts(t *testing.T) {
	withCustomVocab(t)

	wm := core.DefaultWorkflow()
	for _, name := range wm.GetAllStatuses() {
		status := core.TaskStatus(name)
		task := &core.Task{ID: "task_x", Status: status}

		next, ok := nextRotationStatus(task)
		if !ok {
			// No offer is only legitimate when the workflow genuinely
			// allows no move out of this status.
			for _, cand := range wm.GetAllStatuses() {
				if core.TaskStatus(cand) == status {
					continue
				}
				if err := wm.ValidateTransition(status, core.TaskStatus(cand), false); err == nil {
					t.Errorf("status %s: ring offered nothing, but %s -> %s is allowed",
						name, name, cand)
				}
			}
			continue
		}

		if next == status {
			t.Errorf("status %s: ring offered the same status; that is not a rotation", name)
		}
		if err := wm.ValidateTransition(status, next, false); err != nil {
			t.Errorf("status %s: ring offered %s, which the workflow REJECTS: %v",
				name, next, err)
		}
	}
}

// TestRotateStatus_TransitionIsAccepted drives the real command through
// the real service, so a ring that satisfies ValidateTransition in
// isolation but still trips the service path is caught.
func TestRotateStatus_TransitionIsAccepted(t *testing.T) {
	withCustomVocab(t)

	m, repo := newTestModel(t)
	task := &core.Task{
		ID:     "task_01h455vb4pex5vsknk084sn001",
		Title:  "Rotate me",
		Status: core.TaskStatus("BACKLOG"),
	}
	repo.Tasks[task.ID] = task

	msg := m.rotateStatus(task)()
	if err, ok := msg.(error); ok {
		t.Fatalf("rotateStatus returned error under custom vocabulary: %v", err)
	}

	got, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status == core.TaskStatus("BACKLOG") {
		t.Fatalf("status unchanged after rotate: the keypress silently did nothing")
	}
	if got.Status != core.TaskStatus("DOING") {
		t.Errorf("rotate from BACKLOG landed on %s; want DOING "+
			"(first allowed target in declared order)", got.Status)
	}
}

// TestRotateStatus_TerminalStatusOffersNothing pins the documented
// rule for a status with no allowed target. Terminal statuses are
// immutable (reopen is the sanctioned exit), so the key must do nothing
// visible rather than surface a rejection.
func TestRotateStatus_TerminalStatusOffersNothing(t *testing.T) {
	withCustomVocab(t)

	for _, name := range []string{"SHIPPED", "DROPPED"} {
		task := &core.Task{ID: "task_x", Status: core.TaskStatus(name)}
		if next, ok := nextRotationStatus(task); ok {
			t.Errorf("terminal status %s offered %s; want no offer", name, next)
		}
	}
}

// TestSaveTask_UsesConfiguredInitialStatus is the third instance of a
// defect already fixed twice on this track (CLI create, HTTP route).
// The TUI create path hardcoded core.StatusTodo, so a task created
// under a custom vocabulary landed in a status the config never
// declared — and could then never be transitioned out of.
func TestSaveTask_UsesConfiguredInitialStatus(t *testing.T) {
	withCustomVocab(t)

	m, repo := newTestModel(t)
	if msg := m.saveTask("Created via TUI", "")(); msg != nil {
		if err, ok := msg.(error); ok {
			t.Fatalf("saveTask returned error: %v", err)
		}
	}

	if len(repo.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(repo.Tasks))
	}
	for _, task := range repo.Tasks {
		if task.Status != core.TaskStatus("BACKLOG") {
			t.Errorf("created task status = %q; want BACKLOG "+
				"(the role \"initial\" status of the configured vocabulary)",
				task.Status)
		}
	}
}

// TestStatusOrder_IsConfiguredVocabulary covers the dashboard grouping
// order and the sort rank at once: both were built from the built-in
// four, so a custom status sorted as rank 0 alongside the initial
// status and its dashboard group never rendered.
func TestStatusOrder_IsConfiguredVocabulary(t *testing.T) {
	withCustomVocab(t)

	want := []core.TaskStatus{"BACKLOG", "DOING", "SHIPPED", "DROPPED"}
	got := configuredStatusOrder()

	if len(got) != len(want) {
		t.Fatalf("configuredStatusOrder() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("configuredStatusOrder() = %v; want %v "+
				"(declaration order IS rank order)", got, want)
		}
	}
}

// TestStatusRank_RanksEveryConfiguredStatusDistinctly proves the sort
// key separates the configured statuses. The old map returned 0 for
// every unknown status, so under a custom vocabulary all four tied and
// the sort collapsed to ID order.
func TestStatusRank_RanksEveryConfiguredStatusDistinctly(t *testing.T) {
	withCustomVocab(t)

	ranks := statusRanks()
	seen := make(map[int]core.TaskStatus, len(ranks))
	for _, name := range core.ConfiguredTaskStatusStrings() {
		status := core.TaskStatus(name)
		rank, ok := ranks[status]
		if !ok {
			t.Errorf("status %s has no sort rank", name)
			continue
		}
		if other, dup := seen[rank]; dup {
			t.Errorf("statuses %s and %s share sort rank %d", other, name, rank)
		}
		seen[rank] = status
	}
}

// TestKanbanStatusOrder_ExcludesOnlyTerminalSkip pins the column
// membership rule. Every configured status must earn a column;
// hardcoding three columns meant a custom vocabulary lost the rest.
func TestKanbanStatusOrder_ExcludesOnlyTerminalSkip(t *testing.T) {
	withCustomVocab(t)

	got := kanbanStatusOrder()
	want := []core.TaskStatus{"BACKLOG", "DOING", "SHIPPED"}

	if len(got) != len(want) {
		t.Fatalf("kanbanStatusOrder() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kanbanStatusOrder() = %v; want %v", got, want)
		}
	}
}

// TestFormatStatus_RendersConfiguredMarker covers the glyph table. It
// switched on the built-in four and fell through to the raw status
// string, so a custom vocabulary rendered bare uppercase names with no
// marker and no color.
func TestFormatStatus_RendersConfiguredMarker(t *testing.T) {
	withCustomVocab(t)

	s := styles.NewFromTheme(styles.DefaultKitTheme())
	cases := map[core.TaskStatus]string{
		"BACKLOG": "[ ]",
		"DOING":   "[>]",
		"SHIPPED": "[x]",
		"DROPPED": "[-]",
	}
	for status, want := range cases {
		got := formatStatusWithStyles(status, s)
		if !containsPlain(got, want) {
			t.Errorf("formatStatusWithStyles(%s) = %q; want it to contain %q "+
				"(the status's configured tls_marker)", status, got, want)
		}
	}
}

// TestFormatStatus_UnknownStatusFallsBack keeps the renderer total: a
// task carrying a status the workflow does not know (stale row, mid
// config edit) must still render something rather than empty.
func TestFormatStatus_UnknownStatusFallsBack(t *testing.T) {
	withCustomVocab(t)

	s := styles.NewFromTheme(styles.DefaultKitTheme())
	got := formatStatusWithStyles(core.TaskStatus("NOPE"), s)
	if !containsPlain(got, "NOPE") {
		t.Errorf("formatStatusWithStyles(NOPE) = %q; want it to name the status", got)
	}
}

// TestMoveTask_UsesConfiguredColumns proves the kanban left/right move
// walks the configured columns, not the built-in three.
func TestMoveTask_UsesConfiguredColumns(t *testing.T) {
	withCustomVocab(t)

	m, repo := newTestModel(t)
	task := &core.Task{
		ID:     "task_01h455vb4pex5vsknk084sn002",
		Title:  "Move me",
		Status: core.TaskStatus("BACKLOG"),
	}
	repo.Tasks[task.ID] = task

	if msg := m.moveTask(task, 1)(); msg != nil {
		if err, ok := msg.(error); ok {
			t.Fatalf("moveTask returned error: %v", err)
		}
	}

	got, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != core.TaskStatus("DOING") {
		t.Errorf("moveTask(+1) from BACKLOG landed on %s; want DOING", got.Status)
	}
}

// containsPlain reports whether the rendered cell contains want once
// ANSI styling is ignored. Styles are applied per status, so a
// substring check on the raw string is enough here: lipgloss wraps the
// text rather than rewriting it.
func containsPlain(got, want string) bool {
	return len(got) >= len(want) && indexOf(got, want) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
