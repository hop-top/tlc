package main

import (
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// customVocab shares no value name with the built-in vocabulary, so any
// assertion it satisfies is satisfied by config rather than by a
// hardcoded table that happens to agree.
func customVocab() *Vocabulary {
	return &Vocabulary{
		PriorityToLabel:  map[string]string{"DROP_EVERYTHING": "priority:drop-everything", "LATER": "priority:later"},
		EffortToLabel:    map[string]string{"TINY": "effort:tiny", "HUGE": "effort:huge"},
		ActiveStatuses:   map[string]string{"DOING": "status:doing"},
		InitialStatus:    "BACKLOG",
		TerminalStatuses: map[string]string{"SHIPPED": "completed", "DROPPED": "not_planned"},
	}
}

// TestCustomPriorityRoundTrip is the regression this change exists for:
// a configured priority must survive push -> pull in the TYPED Priority
// field rather than degrading into a tag.
func TestCustomPriorityRoundTrip(t *testing.T) {
	v := customVocab()

	pushed := MapTaskToGitLabIssueDataWith(&Task{Status: "BACKLOG", Priority: "DROP_EVERYTHING"}, v).Labels

	var found bool
	for _, l := range pushed {
		if l == "priority:drop-everything" {
			found = true
		}
	}
	if !found {
		t.Fatalf("push dropped the configured priority: %v", pushed)
	}

	issue := &gitlab.Issue{State: "opened", Labels: []string(pushed)}
	got := MapGitLabIssueToTaskWith(issue, v)

	if got.Priority != "DROP_EVERYTHING" {
		t.Errorf("Priority = %q, want DROP_EVERYTHING", got.Priority)
	}
	for _, tag := range got.Tags {
		if tag == "priority:drop-everything" {
			t.Errorf("priority degraded into a tag: %v", got.Tags)
		}
	}
}

// TestCustomActiveStatusRoundTrip pins the status axis under a rename.
func TestCustomActiveStatusRoundTrip(t *testing.T) {
	v := customVocab()

	pushed := MapTaskToGitLabIssueDataWith(&Task{Status: "DOING"}, v).Labels
	var found bool
	for _, l := range pushed {
		if l == "status:doing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("push dropped the configured active status: %v", pushed)
	}

	issue := &gitlab.Issue{State: "opened", Labels: []string{"status:doing"}}
	if got := MapGitLabIssueToTaskWith(issue, v); got.Status != "DOING" {
		t.Errorf("Status = %q, want DOING", got.Status)
	}
}

// TestCustomTerminalStatusClosesIssue proves a renamed terminal status
// still closes the issue.
func TestCustomTerminalStatusClosesIssue(t *testing.T) {
	v := customVocab()

	if got := MapTaskToGitLabIssueDataWith(&Task{Status: "SHIPPED"}, v).State; got != "close" {
		t.Errorf("SHIPPED state = %q, want close", got)
	}
	if got := MapTaskToGitLabIssueDataWith(&Task{Status: "BACKLOG"}, v).State; got != "reopen" {
		t.Errorf("BACKLOG state = %q, want reopen", got)
	}

	issue := &gitlab.Issue{State: "closed"}
	if got := MapGitLabIssueToTaskWith(issue, v); got.Status != "SHIPPED" {
		t.Errorf("closed pulls back as %q, want SHIPPED", got.Status)
	}
}

// TestNilVocabularyMatchesBuiltin is the behavior-identical guarantee.
func TestNilVocabularyMatchesBuiltin(t *testing.T) {
	cases := []*Task{
		{Status: "IN_PROGRESS", Priority: "P0", Effort: "XS"},
		{Status: "TODO", Priority: "P3", Effort: "XL", BlockedReason: "waiting"},
		{Status: "SKIPPED", Priority: "P2"},
		{Status: "DONE"},
	}
	for _, task := range cases {
		want := MapTaskToGitLabIssueData(task)
		got := MapTaskToGitLabIssueDataWith(task, nil)

		if want.State != got.State {
			t.Errorf("state for %+v: want %q got %q", task, want.State, got.State)
		}
		if len(want.Labels) != len(got.Labels) {
			t.Fatalf("labels for %+v: want %v got %v", task, want.Labels, got.Labels)
		}
		for i := range want.Labels {
			if want.Labels[i] != got.Labels[i] {
				t.Errorf("label %d for %+v: want %q got %q", i, task, want.Labels[i], got.Labels[i])
			}
		}
	}
}
