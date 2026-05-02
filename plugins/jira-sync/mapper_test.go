package main

import (
	"strings"
	"testing"

	"github.com/andygrunwald/go-jira"
)

// --- TypeID round-trip (T-0823) ---

func TestTypeIDRoundTrip_PreservesEmbedded(t *testing.T) {
	const taskID = "task_01h455vb4pex5vsknk084sn02q"

	original := &Task{
		ID:          taskID,
		Title:       "round-trip",
		Status:      "TODO",
		Description: "body text",
	}

	body := MapTaskToJiraDescription(original)

	// Footer must be present on push body.
	if !strings.Contains(body, "<!-- tlc-uid: "+taskID+" -->") {
		t.Fatalf("push body missing tlc-uid footer: %q", body)
	}

	// Pull back: build a Jira issue with that body and verify ID recovered.
	issue := &jira.Issue{
		Key: "JIRA-7",
		Fields: &jira.IssueFields{
			Summary:     "round-trip",
			Description: body,
			Status:      &jira.Status{Name: "Open"},
		},
	}
	got := MapJiraIssueToTask(issue)

	if got.ID != taskID {
		t.Errorf("round-trip ID = %q, want %q", got.ID, taskID)
	}
}

func TestTypeIDRoundTrip_GeneratesWhenAbsent(t *testing.T) {
	issue := &jira.Issue{
		Key: "JIRA-8",
		Fields: &jira.IssueFields{
			Summary:     "no footer",
			Description: "plain description",
			Status:      &jira.Status{Name: "Open"},
		},
	}
	got := MapJiraIssueToTask(issue)

	if !isTaskTypeID(got.ID) {
		t.Errorf("ID = %q, want fresh task_<26char> typeid", got.ID)
	}
}

// isTaskTypeID is a local shape check — see internal/core/typeid.go for the
// canonical pattern.
func isTaskTypeID(s string) bool {
	if len(s) != len("task_")+26 {
		return false
	}
	if s[:5] != "task_" {
		return false
	}
	for _, r := range s[5:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}
