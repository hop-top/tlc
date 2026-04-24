package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
