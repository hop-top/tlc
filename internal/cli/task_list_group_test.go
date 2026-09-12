package cli

import (
	"fmt"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// ptr is local to this file's fixtures: Task.AssignedTo, TrackID and
// ProjectID are all *string, and a grouping test needs "set" and "unset"
// to be different facts.
func strptr(s string) *string { return &s }

func groupNames(gs []TaskGroup) []string {
	out := make([]string, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.Name)
	}
	return out
}

func groupIDs(gs []TaskGroup, name string) []string {
	for _, g := range gs {
		if g.Name != name {
			continue
		}
		out := make([]string, 0, len(g.Tasks))
		for _, t := range g.Tasks {
			out = append(out, t.ID)
		}
		return out
	}
	return nil
}

// TestGroupByKeysAreClosed pins the closed key set and its validator:
// every declared key resolves, anything else is rejected naming the set.
func TestGroupByKeysAreClosed(t *testing.T) {
	want := []string{"track", "assignee", "tag", "status", "priority", "project"}
	if got := strings.Join(GroupByKeys(), ","); got != strings.Join(want, ",") {
		t.Errorf("GroupByKeys() = %q, want %q", got, strings.Join(want, ","))
	}
	for _, k := range want {
		if !ValidGroupByKey(k) {
			t.Errorf("ValidGroupByKey(%q) = false, want true", k)
		}
	}
	if ValidGroupByKey("assinee") {
		t.Error("ValidGroupByKey(\"assinee\") = true, want false")
	}
	if ValidGroupByKey("") {
		t.Error("ValidGroupByKey(\"\") = true, want false")
	}
	err := unknownGroupByError("bogus")
	if err == nil {
		t.Fatal("unknownGroupByError returned nil")
	}
	if !strings.Contains(err.Error(), `"bogus"`) {
		t.Errorf("error should quote the rejected key, got: %s", err)
	}
	// The canon must appear verbatim, so the message is rendered FROM the
	// canon rather than from a literal that agrees with it today.
	if w := strings.Join(want, ", "); !strings.Contains(err.Error(), w) {
		t.Errorf("error should name the valid set %q, got: %s", w, err)
	}
}

// TestGroupByFlagRegisteredAsEnum pins the flag to the same canon, via
// the same command-scoped enum mechanism --status and --priority use, so
// help text, parse rejection and shell completion all come for free.
func TestGroupByFlagRegisteredAsEnum(t *testing.T) {
	root := kitRootInstance
	got := strings.Join(root.CommandFlagEnum("task list", "group-by"), ",")
	if want := strings.Join(GroupByKeys(), ","); got != want {
		t.Errorf("task list --group-by enum = %q, want %q", got, want)
	}
	cmd, _, err := root.Cmd.Find([]string{"task", "list"})
	if err != nil {
		t.Fatalf("find task list: %v", err)
	}
	f := cmd.Flags().Lookup("group-by")
	if f == nil {
		t.Fatal("task list has no --group-by flag")
	}
	// kit renders the values from the registration; a literal in the
	// declared usage is the duplicate that drifts.
	if strings.Contains(f.Usage, strings.Join(GroupByKeys(), ", ")) {
		t.Errorf("--group-by usage hand-writes the value set: %q", f.Usage)
	}
}

// TestGroupTasksEmptyInput: no tasks means no groups, and the zero value
// must be an empty slice rather than a one-group "everything" bucket.
func TestGroupTasksEmptyInput(t *testing.T) {
	for _, key := range GroupByKeys() {
		if got := groupTasks(nil, key); len(got) != 0 {
			t.Errorf("groupTasks(nil, %q) = %d groups, want 0", key, len(got))
		}
		if got := groupTasks([]*core.Task{}, key); len(got) != 0 {
			t.Errorf("groupTasks([], %q) = %d groups, want 0", key, len(got))
		}
	}
}

// TestGroupTasksNameOrderAscending: the free-text dimensions sort by
// group name ascending.
func TestGroupTasksNameOrdering(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", AssignedTo: strptr("zoe"), TrackID: strptr("zeta"), ProjectID: strptr("zzz")},
		{ID: "T-0002", AssignedTo: strptr("ann"), TrackID: strptr("alpha"), ProjectID: strptr("aaa")},
		{ID: "T-0003", AssignedTo: strptr("mia"), TrackID: strptr("mu"), ProjectID: strptr("mmm")},
	}
	for key, want := range map[string][]string{
		"assignee": {"ann", "mia", "zoe"},
		"track":    {"alpha", "mu", "zeta"},
		"project":  {"aaa", "mmm", "zzz"},
	} {
		got := groupNames(groupTasks(tasks, key))
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("groupTasks(_, %q) names = %v, want %v", key, got, want)
		}
	}
}

// TestGroupTasksLifecycleOrdering is the heart of the ordering rule:
// status and priority order by the DECLARED lifecycle, not the alphabet.
// Both built-in vocabularies sort differently as text than as lifecycle,
// which is what makes this test able to fail.
func TestGroupTasksLifecycleOrdering(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		// Fed in reverse lifecycle order so insertion order cannot
		// produce the expected answer by accident.
		tasks := []*core.Task{
			{ID: "T-0001", Status: core.StatusDone},
			{ID: "T-0002", Status: core.StatusInProgress},
			{ID: "T-0003", Status: core.StatusTodo},
		}
		got := groupNames(groupTasks(tasks, "status"))
		want := []string{"TODO", "IN_PROGRESS", "DONE"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("status group order = %v, want %v (lifecycle, not alphabetical)", got, want)
		}
		// Alphabetical would be DONE, IN_PROGRESS, TODO.
		alpha := []string{"DONE", "IN_PROGRESS", "TODO"}
		if strings.Join(got, ",") == strings.Join(alpha, ",") {
			t.Error("status groups came out alphabetical")
		}
		// And the order must track the configured vocabulary, not a
		// literal: every rendered name appears in declaration order.
		assertSubsequence(t, core.ConfiguredTaskStatusStrings(), got)
	})

	t.Run("priority", func(t *testing.T) {
		tasks := []*core.Task{
			{ID: "T-0001", Priority: core.PriorityP3},
			{ID: "T-0002", Priority: core.PriorityP1},
			{ID: "T-0003", Priority: core.PriorityP0},
			{ID: "T-0004", Priority: core.PriorityP2},
		}
		got := groupNames(groupTasks(tasks, "priority"))
		want := []string{"P0", "P1", "P2", "P3"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("priority group order = %v, want %v", got, want)
		}
		assertSubsequence(t, core.ConfiguredPriorityStrings(), got)
	})
}

// assertSubsequence checks that got appears in canon in the same relative
// order — i.e. the grouper read the vocabulary rather than sorting.
func assertSubsequence(t *testing.T, canon, got []string) {
	t.Helper()
	i := 0
	for _, name := range canon {
		if i < len(got) && got[i] == name {
			i++
		}
	}
	if i != len(got) {
		t.Errorf("group order %v is not a subsequence of the declared vocabulary %v", got, canon)
	}
}

// TestGroupTasksTagIsManyToMany: a task with two tags lands in BOTH
// groups. Row count exceeding task count is the intended behavior, so
// the grouper must not dedupe.
func TestGroupTasksTagManyToMany(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Tags: []string{"api", "urgent"}},
		{ID: "T-0002", Tags: []string{"urgent"}},
	}
	groups := groupTasks(tasks, "tag")
	if got, want := groupNames(groups), []string{"api", "urgent"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tag group names = %v, want %v", got, want)
	}
	if got := groupIDs(groups, "api"); strings.Join(got, ",") != "T-0001" {
		t.Errorf("api group = %v, want [T-0001]", got)
	}
	if got := groupIDs(groups, "urgent"); strings.Join(got, ",") != "T-0001,T-0002" {
		t.Errorf("urgent group = %v, want [T-0001 T-0002]", got)
	}
	rows := 0
	for _, g := range groups {
		rows += len(g.Tasks)
	}
	if rows != 3 {
		t.Errorf("total rows = %d, want 3 (2 distinct tasks, one multi-tagged)", rows)
	}
}

// TestGroupTasksPreservesIncomingOrder: within a group, tasks keep the
// order the store returned them in. Re-sorting here would silently
// override --sort-by.
func TestGroupTasksPreservesIncomingOrder(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0009", AssignedTo: strptr("ann")},
		{ID: "T-0001", AssignedTo: strptr("ann")},
		{ID: "T-0005", AssignedTo: strptr("ann")},
	}
	got := groupIDs(groupTasks(tasks, "assignee"), "ann")
	if strings.Join(got, ",") != "T-0009,T-0001,T-0005" {
		t.Errorf("within-group order = %v, want incoming order [T-0009 T-0001 T-0005]", got)
	}
}

// TestGroupTasksDeterministic: repeated runs over the same input produce
// byte-identical group order. This is why the return type is a slice and
// not a map — Go randomizes map iteration per run AND per range.
func TestGroupTasksDeterministic(t *testing.T) {
	var tasks []*core.Task
	for i := 0; i < 40; i++ {
		tasks = append(tasks, &core.Task{
			ID:         fmt.Sprintf("T-%04d", i),
			AssignedTo: strptr(fmt.Sprintf("user%02d", i%13)),
			Tags:       []string{fmt.Sprintf("tag%02d", i%7), fmt.Sprintf("tag%02d", i%5)},
			Status:     core.TaskStatus(core.ConfiguredTaskStatusStrings()[i%len(core.ConfiguredTaskStatusStrings())]),
			Priority:   core.Priority(core.ConfiguredPriorityStrings()[i%len(core.ConfiguredPriorityStrings())]),
		})
	}
	for _, key := range []string{"assignee", "tag", "status", "priority"} {
		first := render(groupTasks(tasks, key))
		for run := 0; run < 50; run++ {
			if got := render(groupTasks(tasks, key)); got != first {
				t.Fatalf("groupTasks(_, %q) non-deterministic:\nrun 0: %s\nrun %d: %s", key, first, run+1, got)
			}
		}
	}
}

func render(gs []TaskGroup) string {
	var b strings.Builder
	for _, g := range gs {
		b.WriteString(g.Name)
		b.WriteString(":")
		for _, task := range g.Tasks {
			b.WriteString(task.ID)
			b.WriteString(" ")
		}
		b.WriteString("|")
	}
	return b.String()
}
