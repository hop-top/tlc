package main

import "testing"

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

func labelsOf(fields map[string]interface{}) []string {
	raw, ok := fields["labels"]
	if !ok {
		return nil
	}
	out, _ := raw.([]string)
	return out
}

// TestCustomPriorityRoundTrip is the regression this change exists for:
// a configured priority must survive push -> pull in the TYPED Priority
// field rather than degrading into a tag.
func TestCustomPriorityRoundTrip(t *testing.T) {
	v := customVocab()

	fields := MapTaskToGiteaIssueWith(&Task{Status: "BACKLOG", Priority: "DROP_EVERYTHING"}, v)
	pushed := labelsOf(fields)

	var found bool
	for _, l := range pushed {
		if l == "priority:drop-everything" {
			found = true
		}
	}
	if !found {
		t.Fatalf("push dropped the configured priority: %v", pushed)
	}

	issue := &GiteaIssue{State: "open"}
	for _, l := range pushed {
		issue.Labels = append(issue.Labels, GiteaLabel{Name: l})
	}
	got := MapGiteaIssueToTaskWith(issue, nil, v)

	if got.Priority != "DROP_EVERYTHING" {
		t.Errorf("Priority = %q, want DROP_EVERYTHING", got.Priority)
	}
	for _, tag := range got.Tags {
		if tag == "priority:drop-everything" || tag == "urgent" {
			t.Errorf("priority degraded into a tag: %v", got.Tags)
		}
	}
}

// TestCustomActiveStatusRoundTrip pins the status axis under a rename.
func TestCustomActiveStatusRoundTrip(t *testing.T) {
	v := customVocab()

	pushed := labelsOf(MapTaskToGiteaIssueWith(&Task{Status: "DOING"}, v))
	var found bool
	for _, l := range pushed {
		if l == "status:doing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("push dropped the configured active status: %v", pushed)
	}

	issue := &GiteaIssue{State: "open", Labels: []GiteaLabel{{Name: "status:doing"}}}
	if got := MapGiteaIssueToTaskWith(issue, nil, v); got.Status != "DOING" {
		t.Errorf("Status = %q, want DOING", got.Status)
	}
}

// TestCustomTerminalStatusClosesIssue proves a renamed terminal status
// still closes the issue, so open/closed keeps carrying terminality.
func TestCustomTerminalStatusClosesIssue(t *testing.T) {
	v := customVocab()

	if got := MapTaskToGiteaIssueWith(&Task{Status: "SHIPPED"}, v)["state"]; got != "closed" {
		t.Errorf("SHIPPED state = %v, want closed", got)
	}
	if got := MapTaskToGiteaIssueWith(&Task{Status: "BACKLOG"}, v)["state"]; got != "open" {
		t.Errorf("BACKLOG state = %v, want open", got)
	}

	issue := &GiteaIssue{State: "closed"}
	if got := MapGiteaIssueToTaskWith(issue, nil, v); got.Status != "SHIPPED" {
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
		want := MapTaskToGiteaIssue(task)
		got := MapTaskToGiteaIssueWith(task, nil)

		if want["state"] != got["state"] {
			t.Errorf("state for %+v: want %v got %v", task, want["state"], got["state"])
		}
		wl, gl := labelsOf(want), labelsOf(got)
		if len(wl) != len(gl) {
			t.Fatalf("labels for %+v: want %v got %v", task, wl, gl)
		}
		for i := range wl {
			if wl[i] != gl[i] {
				t.Errorf("label %d for %+v: want %q got %q", i, task, wl[i], gl[i])
			}
		}
	}
}
