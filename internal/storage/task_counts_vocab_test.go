package storage

// The overdue predicate under a RENAMED status vocabulary.
//
// "A finished task is not late" is what the overdue predicate promises,
// and it spelled "finished" as the built-in DONE/SKIPPED constants. A
// config declaring SHIPPED/CANCELED as its terminal statuses matches
// neither, so shipped work stayed in the overdue population forever —
// in the list query and in all three count queries at once, since they
// share the fragment.

import (
	"testing"
	"time"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// withTerminalVocab installs a task config whose terminal statuses are
// SHIPPED and CANCELED, sharing no name with the built-in set. Two
// terminal statuses, because the built-in set has two: a fix that
// recognized only the first would pass against a single-terminal
// vocabulary and still be wrong.
func withTerminalVocab(t *testing.T) {
	t.Helper()
	core.SetTaskConfigProvider(func() *config.TaskConfig {
		return &config.TaskConfig{
			Statuses: []config.StatusDefinition{
				{Name: "BACKLOG", Label: "Backlog", Role: config.RoleInitial, TLSMarker: " "},
				{Name: "DOING", Label: "Doing", Role: config.RoleActive, TLSMarker: ">"},
				{Name: "SHIPPED", Label: "Shipped", IsTerminal: true, Role: config.RoleCompleted, TLSMarker: "x"},
				{Name: "CANCELED", Label: "Canceled", IsTerminal: true, Role: config.RoleSkipped, TLSMarker: "-"},
			},
			StateMachine: &config.WorkflowDefinition{
				Rules: map[string][]string{
					"BACKLOG": {"DOING", "CANCELED"},
					"DOING":   {"SHIPPED", "CANCELED", "BACKLOG"},
				},
			},
		}
	})
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})
}

// TestQueryOverdue_HonorsConfiguredTerminalStatuses is the regression.
// It asserts across all four consumers of the shared fragment, because
// the terminal set is bound into it once and a desynchronised bind arg
// would corrupt every one of them at the same time.
// terminalNames are the two statuses the vocabulary marks terminal.
// None of them may survive the overdue predicate.
var terminalNames = []string{"SHIPPED", "CANCELED"}

// seedOverdueVocab seeds one row per interesting case and returns the
// storage handle. Two open past-due rows is the whole expected answer.
func seedOverdueVocab(t *testing.T) *SQLiteStorage {
	t.Helper()
	s := newCountsStorage(t)
	ctx := t.Context()

	past := time.Now().UTC().Add(-48 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)

	for _, seed := range []struct {
		id     string
		status core.TaskStatus
		due    *time.Time
	}{
		{"T-0001", "BACKLOG", &past},   // overdue
		{"T-0002", "DOING", &past},     // overdue
		{"T-0003", "SHIPPED", &past},   // finished, so not late
		{"T-0004", "CANCELED", &past},  // finished, so not late
		{"T-0005", "BACKLOG", &future}, // not due yet
		{"T-0006", "BACKLOG", nil},     // no due date at all
	} {
		if err := s.CreateTask(ctx, &core.Task{
			ID: seed.id, Title: seed.id, Status: seed.status, DueAt: seed.due,
		}); err != nil {
			t.Fatalf("seed %s: %v", seed.id, err)
		}
	}
	return s
}

// assertOverdueCount pins the scalar count.
func assertOverdueCount(t *testing.T, s *SQLiteStorage, q core.Query) {
	t.Helper()
	count, err := s.CountTasks(t.Context(), q)
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if count != 2 {
		t.Errorf("overdue count = %d, want 2 (open and past due only)", count)
	}
}

// assertOverdueRows pins the rows the list query selects.
func assertOverdueRows(t *testing.T, s *SQLiteStorage, q core.Query) {
	t.Helper()
	tasks, err := s.ListTasks(t.Context(), q)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("ListTasks returned %d rows, want 2", len(tasks))
	}
	for _, task := range tasks {
		if isTerminalName(string(task.Status)) {
			t.Errorf("%s is %s and must not be overdue", task.ID, task.Status)
		}
	}
}

// assertOverdueByStatus pins the per-status counts.
func assertOverdueByStatus(t *testing.T, s *SQLiteStorage, q core.Query) {
	t.Helper()
	byStatus, err := s.CountTasksByStatus(t.Context(), q)
	if err != nil {
		t.Fatalf("CountTasksByStatus: %v", err)
	}
	for _, terminal := range terminalNames {
		if n, ok := byStatus[terminal]; ok {
			t.Errorf("%s present (%d) in overdue counts", terminal, n)
		}
	}
}

// assertOverdueByProject pins the per-project counts.
func assertOverdueByProject(t *testing.T, s *SQLiteStorage, q core.Query) {
	t.Helper()
	byProject, err := s.CountTasksByProjectAndStatus(t.Context(), q)
	if err != nil {
		t.Fatalf("CountTasksByProjectAndStatus: %v", err)
	}
	for _, statuses := range byProject {
		for _, terminal := range terminalNames {
			if n, ok := statuses[terminal]; ok {
				t.Errorf("%s present (%d) in per-project overdue counts", terminal, n)
			}
		}
	}
}

func isTerminalName(status string) bool {
	for _, terminal := range terminalNames {
		if status == terminal {
			return true
		}
	}
	return false
}

func TestQueryOverdue_HonorsConfiguredTerminalStatuses(t *testing.T) {
	withTerminalVocab(t)
	s := seedOverdueVocab(t)

	now := time.Now().UTC()
	q := core.Query{AllProjects: true, Overdue: &now}

	// One subtest per consumer of the shared WHERE fragment. The
	// terminal set is bound into that fragment once, so a regression
	// reaches all four at the same time -- and each is asserted
	// separately so the failure names which one the user would see.
	for name, assert := range map[string]func(*testing.T, *SQLiteStorage, core.Query){
		"CountTasks":                   assertOverdueCount,
		"ListTasks":                    assertOverdueRows,
		"CountTasksByStatus":           assertOverdueByStatus,
		"CountTasksByProjectAndStatus": assertOverdueByProject,
	} {
		t.Run(name, func(t *testing.T) { assert(t, s, q) })
	}
}

// TestQueryOverdue_TerminalSetDoesNotDesyncBindArgs guards the hazard
// the fix introduces rather than the one it removes. The terminal set is
// now variable-length, and its placeholders sit AFTER the due_at bound
// inside a single clause, so a mis-ordered append silently binds a
// timestamp as a status name. A one-terminal vocabulary changes the
// placeholder count against the built-in two; if the args tracked the
// clause, the predicate still selects exactly the open past-due rows.
func TestQueryOverdue_TerminalSetDoesNotDesyncBindArgs(t *testing.T) {
	core.SetTaskConfigProvider(func() *config.TaskConfig {
		return &config.TaskConfig{
			Statuses: []config.StatusDefinition{
				{Name: "BACKLOG", Label: "Backlog", Role: config.RoleInitial, TLSMarker: " "},
				{Name: "DOING", Label: "Doing", Role: config.RoleActive, TLSMarker: ">"},
				{Name: "SHIPPED", Label: "Shipped", IsTerminal: true, Role: config.RoleCompleted, TLSMarker: "x"},
			},
		}
	})
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})

	s := newCountsStorage(t)
	ctx := t.Context()

	past := time.Now().UTC().Add(-48 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)

	// A due_at bound leaking into the status list, or a status name
	// leaking into the due_at compare, changes which of these six rows
	// come back — the assertion below pins the exact set.
	for _, seed := range []struct {
		id     string
		status core.TaskStatus
		due    *time.Time
	}{
		{"T-0001", "BACKLOG", &past},
		{"T-0002", "DOING", &past},
		{"T-0003", "SHIPPED", &past},
		{"T-0004", "BACKLOG", &future},
		{"T-0005", "DOING", &future},
		{"T-0006", "SHIPPED", nil},
	} {
		if err := s.CreateTask(ctx, &core.Task{
			ID: seed.id, Title: seed.id, Status: seed.status, DueAt: seed.due,
		}); err != nil {
			t.Fatalf("seed %s: %v", seed.id, err)
		}
	}

	now := time.Now().UTC()
	tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true, Overdue: &now})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	got := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		got[task.ID] = true
	}
	want := map[string]bool{"T-0001": true, "T-0002": true}
	if len(got) != len(want) {
		t.Fatalf("overdue rows = %v, want %v", got, want)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("expected %s in overdue rows, got %v", id, got)
		}
	}
}

// TestQueryOverdue_BuiltinVocabularyUnchanged keeps the default set on
// exactly the rows it selected before, so the config-driven terminal set
// is a generalization rather than a change of meaning.
func TestQueryOverdue_BuiltinVocabularyUnchanged(t *testing.T) {
	core.SetTaskConfigProvider(nil)
	core.ResetDefaultWorkflow()
	t.Cleanup(core.ResetDefaultWorkflow)

	s := newCountsStorage(t)
	ctx := t.Context()

	past := time.Now().UTC().Add(-48 * time.Hour)
	for _, seed := range []struct {
		id     string
		status core.TaskStatus
	}{
		{"T-0001", core.StatusTodo},
		{"T-0002", core.StatusInProgress},
		{"T-0003", core.StatusDone},
		{"T-0004", core.StatusSkipped},
	} {
		due := past
		if err := s.CreateTask(ctx, &core.Task{
			ID: seed.id, Title: seed.id, Status: seed.status, DueAt: &due,
		}); err != nil {
			t.Fatalf("seed %s: %v", seed.id, err)
		}
	}

	now := time.Now().UTC()
	count, err := s.CountTasks(ctx, core.Query{AllProjects: true, Overdue: &now})
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if count != 2 {
		t.Errorf("overdue count = %d, want 2 (TODO and IN_PROGRESS)", count)
	}
}
