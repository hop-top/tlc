package core

import (
	"errors"
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
)

func defaultWM(t *testing.T) *WorkflowManager {
	t.Helper()
	wm, err := NewWorkflowManager(&config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return wm
}

// TestDefaultGraph_SevenNodes pins every allowed and refused edge of the
// default state machine (rules only, no guards).
func TestDefaultGraph_SevenNodes(t *testing.T) {
	wm := defaultWM(t)
	all := []TaskStatus{
		StatusBacklog, StatusTodo, StatusInProgress, StatusInReview,
		StatusBlocked, StatusDone, StatusCancelled,
	}
	allowed := map[TaskStatus][]TaskStatus{
		StatusBacklog:    {StatusTodo, StatusCancelled},
		StatusTodo:       {StatusBacklog, StatusInProgress, StatusBlocked, StatusCancelled},
		StatusInProgress: {StatusTodo, StatusInReview, StatusBlocked, StatusDone, StatusCancelled},
		StatusInReview:   {StatusInProgress, StatusDone, StatusBlocked, StatusCancelled},
		StatusBlocked:    {StatusTodo, StatusInProgress, StatusInReview, StatusCancelled},
	}
	for _, from := range all {
		for _, to := range all {
			if from == to {
				continue
			}
			ok := false
			for _, a := range allowed[from] {
				ok = ok || a == to
			}
			err := wm.ValidateTransition(from, to, false)
			if ok && err != nil {
				t.Errorf("%s -> %s: want allowed, got %v", from, to, err)
			}
			if !ok && err == nil {
				t.Errorf("%s -> %s: want refused", from, to)
			}
		}
	}
}

func TestDefaultRoles(t *testing.T) {
	wm := defaultWM(t)
	want := map[string]TaskStatus{
		config.RoleBacklog:   StatusBacklog,
		config.RoleInitial:   StatusTodo,
		config.RoleActive:    StatusInProgress,
		config.RoleReview:    StatusInReview,
		config.RoleBlocked:   StatusBlocked,
		config.RoleCompleted: StatusDone,
		config.RoleSkipped:   StatusCancelled,
	}
	for role, st := range want {
		got, err := wm.StatusForRole(role)
		if err != nil || got != st {
			t.Errorf("role %s = %q, %v; want %q", role, got, err, st)
		}
	}
}

func guardCtx(from, to TaskStatus) TransitionContext {
	return TransitionContext{From: from, To: to}
}

func TestGuards_Defaults(t *testing.T) {
	wm := defaultWM(t)
	tests := []struct {
		name  string
		tc    TransitionContext
		check string // "" = must pass
	}{
		{"block needs reason", guardCtx(StatusTodo, StatusBlocked), config.CheckReason},
		{"block with reason", TransitionContext{From: StatusTodo, To: StatusBlocked, Reason: "waiting"}, ""},
		{"block whitespace reason", TransitionContext{From: StatusTodo, To: StatusBlocked, Reason: "  "}, config.CheckReason},
		{"cancel needs note", guardCtx(StatusTodo, StatusCancelled), config.CheckNote},
		{"cancel with note", TransitionContext{From: StatusTodo, To: StatusCancelled, Note: "dup"}, ""},
		{"submit needs assignee", guardCtx(StatusInProgress, StatusInReview), config.CheckAssignee},
		{"submit with assignee", TransitionContext{From: StatusInProgress, To: StatusInReview, Assignee: "a"}, ""},
		{"done, no blockers", guardCtx(StatusInProgress, StatusDone), ""},
		{
			"done, open blocker",
			TransitionContext{
				From: StatusInProgress, To: StatusDone, BlockedBy: []string{"T-1"},
				Blockers: map[string]TaskStatus{"T-1": StatusTodo},
			},
			config.CheckBlockersClear,
		},
		{
			"done, terminal blocker",
			TransitionContext{
				From: StatusInProgress, To: StatusDone, BlockedBy: []string{"T-1"},
				Blockers: map[string]TaskStatus{"T-1": StatusCancelled},
			},
			"",
		},
		{
			"done, unknown blocker counts open",
			TransitionContext{From: StatusInReview, To: StatusDone, BlockedBy: []string{"x/T-9"}},
			config.CheckBlockersClear,
		},
		{"from-only guard does not hit other source", guardCtx(StatusBlocked, StatusInReview), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wm.ValidateTransitionContext(tt.tc)
			if tt.check == "" {
				if err != nil {
					t.Fatalf("want pass, got %v", err)
				}
				return
			}
			var ge ErrGuardRefused
			if !errors.As(err, &ge) {
				t.Fatalf("want ErrGuardRefused, got %v", err)
			}
			if ge.Check != tt.check {
				t.Errorf("check = %s, want %s", ge.Check, tt.check)
			}
			msg := err.Error()
			if !strings.Contains(msg, string(tt.tc.From)) || !strings.Contains(msg, string(tt.tc.To)) ||
				!strings.Contains(msg, tt.check) {
				t.Errorf("message must name transition and check: %s", msg)
			}
		})
	}
}

func TestGuards_BlockersMessageNamesTask(t *testing.T) {
	wm := defaultWM(t)
	err := wm.ValidateTransitionContext(TransitionContext{
		From: StatusInProgress, To: StatusDone, BlockedBy: []string{"T-0012"},
		Blockers: map[string]TaskStatus{"T-0012": StatusInProgress},
	})
	if err == nil || !strings.Contains(err.Error(), "T-0012 is still open") {
		t.Fatalf("got %v", err)
	}
}

func TestGuards_BlockerLabelShownInMessage(t *testing.T) {
	wm := defaultWM(t)
	err := wm.ValidateTransitionContext(TransitionContext{
		From: StatusInProgress, To: StatusDone, BlockedBy: []string{"task_01abc"},
		Blockers:      map[string]TaskStatus{"task_01abc": StatusInProgress},
		BlockerLabels: map[string]string{"task_01abc": "T-0012"},
	})
	if err == nil || !strings.Contains(err.Error(), "T-0012 is still open") || strings.Contains(err.Error(), "task_01abc") {
		t.Fatalf("got %v", err)
	}
}

func TestGuards_RulesBeforeGuards(t *testing.T) {
	wm := defaultWM(t)
	err := wm.ValidateTransitionContext(guardCtx(StatusBacklog, StatusDone))
	var ge ErrGuardRefused
	if err == nil || errors.As(err, &ge) {
		t.Fatalf("rule refusal expected, got %v", err)
	}
}

func TestGuards_ForceBypassRecorded(t *testing.T) {
	wm := defaultWM(t)
	tk := &Task{ID: "T-1", Status: StatusInProgress}
	tc := TransitionContext{To: StatusBlocked, Force: true}
	entry, err := tk.TransitionWithContext(tc, "u", wm)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusBlocked {
		t.Fatalf("status = %s", tk.Status)
	}
	if entry.Meta["forced"] != true || entry.Meta["bypassed"] == "" {
		t.Errorf("forced not recorded: %v", entry.Meta)
	}

	// A forced move that needed no bypass is not marked forced.
	tk2 := &Task{ID: "T-2", Status: StatusTodo}
	entry, err = tk2.TransitionWithContext(TransitionContext{To: StatusInProgress, Force: true}, "u", wm)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := entry.Meta["forced"]; ok {
		t.Errorf("unneeded force recorded: %v", entry.Meta)
	}
}

func TestGuards_RefusalLeavesTaskUntouched(t *testing.T) {
	wm := defaultWM(t)
	tk := &Task{ID: "T-1", Status: StatusTodo}
	if _, err := tk.TransitionWithContext(TransitionContext{To: StatusCancelled}, "u", wm); err == nil {
		t.Fatal("want refusal")
	}
	if tk.Status != StatusTodo {
		t.Errorf("status mutated: %s", tk.Status)
	}
}

func TestGuards_PerTagOverrideReplaces(t *testing.T) {
	cfg := &config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
		Workflows: map[string]config.WorkflowOverride{
			"hotfix": {StateMachine: &config.WorkflowDefinition{
				Guards: []config.GuardDefinition{{To: "done", Require: []string{config.CheckNote}}},
			}},
			"inherit": {StateMachine: &config.WorkflowDefinition{}},
		},
	}
	base, err := NewWorkflowManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	hot, _ := base.GetWorkflowForTags([]string{"hotfix"})
	// base blocked-guard is replaced away; the override's note guard applies.
	if err := hot.ValidateTransitionContext(guardCtx(StatusTodo, StatusBlocked)); err != nil {
		t.Errorf("replaced guard list must not keep default reason guard: %v", err)
	}
	if err := hot.ValidateTransitionContext(guardCtx(StatusInProgress, StatusDone)); err == nil {
		t.Error("override note guard must apply")
	}
	inh, _ := base.GetWorkflowForTags([]string{"inherit"})
	if err := inh.ValidateTransitionContext(guardCtx(StatusTodo, StatusBlocked)); err == nil {
		t.Error("override without guards inherits base guards")
	}
}

func TestUnfinishedRoles(t *testing.T) {
	for role, want := range map[string]bool{
		config.RoleInitial: true, config.RoleActive: true, config.RoleReview: true,
		config.RoleBlocked: true, config.RoleBacklog: false,
		config.RoleCompleted: false, config.RoleSkipped: false,
	} {
		if isUnfinishedRole(role) != want {
			t.Errorf("isUnfinishedRole(%s) != %v", role, want)
		}
	}
}
