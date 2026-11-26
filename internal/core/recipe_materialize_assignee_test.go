package core

import "testing"

// TestMaterialize_HumanAssignee: a human step's human.assignee becomes
// the task assignee when the step carries no assignee of its own; the
// step-level assignee wins; other kinds ignore a stray human block.
func TestMaterialize_HumanAssignee(t *testing.T) {
	cases := []struct {
		name string
		step RecipeStep
		want string // "" = unassigned
	}{
		{"human.assignee applies", RecipeStep{ID: "s", Kind: TaskKindHuman, Human: &HumanSpec{Assignee: "alice"}}, "alice"},
		{"leading @ stripped", RecipeStep{ID: "s", Kind: TaskKindHuman, Human: &HumanSpec{Assignee: "@alice"}}, "alice"},
		{
			"step assignee wins",
			RecipeStep{ID: "s", Kind: TaskKindHuman, Assignee: "bob", Human: &HumanSpec{Assignee: "alice"}},
			"bob",
		},
		{"human block without assignee", RecipeStep{ID: "s", Kind: TaskKindHuman, Human: &HumanSpec{Timeout: "1h"}}, ""},
		{"step assignee alone", RecipeStep{ID: "s", Kind: TaskKindHuman, Assignee: "bob"}, "bob"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newMatFixture()
			tc.step.Ordinal = 1
			res := f.materialize(t, matInput([]RecipeStep{tc.step}))
			task := f.task(t, stepTaskIDs(res)["s"])
			switch {
			case tc.want == "" && task.AssignedTo != nil:
				t.Errorf("assigned_to = %q; want unassigned", *task.AssignedTo)
			case tc.want != "" && (task.AssignedTo == nil || *task.AssignedTo != tc.want):
				t.Errorf("assigned_to = %v; want %q", task.AssignedTo, tc.want)
			}
		})
	}
}
