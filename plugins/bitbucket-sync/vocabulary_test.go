package main

import (
	"strings"
	"testing"
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

// componentsOf splits the comma-joined component back into labels.
// Bitbucket has no label field, so the component IS the label list.
func componentsOf(req *BitbucketIssueRequest) []string {
	if req.Component == nil || req.Component.Name == "" {
		return nil
	}
	return strings.Split(req.Component.Name, ",")
}

// TestCustomPriorityRoundTrip is the regression this change exists for:
// a configured priority must survive push -> pull in the TYPED Priority
// field rather than degrading into a tag.
func TestCustomPriorityRoundTrip(t *testing.T) {
	v := customVocab()

	pushed := componentsOf(MapTaskToBitbucketIssueWith(&Task{Status: "BACKLOG", Priority: "DROP_EVERYTHING"}, v))

	var found bool
	for _, l := range pushed {
		if l == "priority:drop-everything" {
			found = true
		}
	}
	if !found {
		t.Fatalf("push dropped the configured priority: %v", pushed)
	}

	got := MapBitbucketIssueToTaskWith(&BitbucketIssue{State: "new"}, pushed, v)

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

	pushed := componentsOf(MapTaskToBitbucketIssueWith(&Task{Status: "DOING"}, v))
	var found bool
	for _, l := range pushed {
		if l == "status:doing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("push dropped the configured active status: %v", pushed)
	}

	got := MapBitbucketIssueToTaskWith(&BitbucketIssue{State: "open"}, []string{"status:doing"}, v)
	if got.Status != "DOING" {
		t.Errorf("Status = %q, want DOING", got.Status)
	}
}

// TestCustomTerminalStatusClosesIssue proves a renamed terminal status
// still resolves the issue.
func TestCustomTerminalStatusClosesIssue(t *testing.T) {
	v := customVocab()

	if got := MapTaskToBitbucketIssueWith(&Task{Status: "SHIPPED"}, v).State; got != "resolved" {
		t.Errorf("SHIPPED state = %q, want resolved", got)
	}
	if got := MapTaskToBitbucketIssueWith(&Task{Status: "DROPPED"}, v).State; got != "invalid" {
		t.Errorf("DROPPED state = %q, want invalid", got)
	}

	got := MapBitbucketIssueToTaskWith(&BitbucketIssue{State: "resolved"}, nil, v)
	if got.Status != "SHIPPED" {
		t.Errorf("resolved pulls back as %q, want SHIPPED", got.Status)
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
		want := MapTaskToBitbucketIssue(task)
		got := MapTaskToBitbucketIssueWith(task, nil)

		if want.State != got.State {
			t.Errorf("state for %+v: want %q got %q", task, want.State, got.State)
		}
		wl, gl := componentsOf(want), componentsOf(got)
		if len(wl) != len(gl) {
			t.Fatalf("components for %+v: want %v got %v", task, wl, gl)
		}
		for i := range wl {
			if wl[i] != gl[i] {
				t.Errorf("component %d for %+v: want %q got %q", i, task, wl[i], gl[i])
			}
		}
	}
}
