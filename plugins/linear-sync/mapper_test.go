package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	body := MapTaskToLinearDescription(original)
	if !strings.Contains(body, "<!-- tlc-uid: "+taskID+" -->") {
		t.Fatalf("push body missing tlc-uid footer: %q", body)
	}

	// Pull back as if Linear stored that body.
	issue := baseIssue()
	issue["description"] = body
	got := MapLinearIssueToTask(issue)

	if got.ID != taskID {
		t.Errorf("round-trip ID = %q, want %q", got.ID, taskID)
	}
}

func TestTypeIDRoundTrip_GeneratesWhenAbsent(t *testing.T) {
	issue := baseIssue()
	issue["description"] = "plain description, no footer"
	got := MapLinearIssueToTask(issue)

	if !isTaskTypeID(got.ID) {
		t.Errorf("ID = %q, want fresh task_<26char> typeid", got.ID)
	}
}

// isTaskTypeID is a local shape check — see internal/core/typeid.go.
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

func baseIssue() map[string]interface{} {
	return map[string]interface{}{
		"id":         "issue-uuid-1",
		"identifier": "ENG-42",
		"title":      "Fix login bug",
		"description": "Users can't login",
		"url":        "https://linear.app/team/issue/ENG-42",
		"createdAt":  "2025-01-15T10:00:00Z",
		"updatedAt":  "2025-01-16T12:00:00Z",
		"state": map[string]interface{}{
			"name": "In Progress",
		},
	}
}

func TestMapLinearIssueToTask_WithDueDate(t *testing.T) {
	issue := baseIssue()
	issue["dueDate"] = "2025-05-01"

	task := MapLinearIssueToTask(issue)

	require.NotNil(t, task.DueAt)
	expected := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, *task.DueAt)
}

func TestMapLinearIssueToTask_WithoutDueDate(t *testing.T) {
	issue := baseIssue()

	task := MapLinearIssueToTask(issue)

	assert.Nil(t, task.DueAt)
}

func TestMapLinearIssueToTask_EmptyDueDate(t *testing.T) {
	issue := baseIssue()
	issue["dueDate"] = ""

	task := MapLinearIssueToTask(issue)

	assert.Nil(t, task.DueAt)
}
