package main

import (
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

func ptr[T any](v T) *T { return &v }

func TestMapGitLabIssueToTask(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		issue         *gitlab.Issue
		wantStatus    string
		wantPriority  string
		wantEffort    string
		wantBlocked   string
		wantTags      []string
		wantMetaKey   string
		wantMetaValue string
	}{
		{
			name: "opened maps to TODO",
			issue: &gitlab.Issue{
				IID:       1,
				Title:     "open issue",
				State:     "opened",
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			wantStatus: "TODO",
		},
		{
			name: "opened with status:in-progress maps to IN_PROGRESS",
			issue: &gitlab.Issue{
				IID:       2,
				Title:     "wip issue",
				State:     "opened",
				Labels:    gitlab.Labels{"status:in-progress"},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			wantStatus: "IN_PROGRESS",
		},
		{
			name: "opened with status:blocked maps to TODO with BlockedReason",
			issue: &gitlab.Issue{
				IID:       3,
				Title:     "blocked issue",
				State:     "opened",
				Labels:    gitlab.Labels{"status:blocked"},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			wantStatus:  "TODO",
			wantBlocked: "blocked (see linked issues)",
		},
		{
			name: "closed maps to DONE",
			issue: &gitlab.Issue{
				IID:       4,
				Title:     "done issue",
				State:     "closed",
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			wantStatus: "DONE",
		},
		{
			name: "closed with wontfix maps to SKIPPED",
			issue: &gitlab.Issue{
				IID:       5,
				Title:     "skipped issue",
				State:     "closed",
				Labels:    gitlab.Labels{"wontfix"},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			wantStatus: "SKIPPED",
		},
		{
			name: "priority:critical maps to P0",
			issue: &gitlab.Issue{
				IID:       6,
				Title:     "critical issue",
				State:     "opened",
				Labels:    gitlab.Labels{"priority:critical"},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			wantStatus:   "TODO",
			wantPriority: "P0",
		},
		{
			name: "effort:m maps to M",
			issue: &gitlab.Issue{
				IID:       7,
				Title:     "medium effort issue",
				State:     "opened",
				Labels:    gitlab.Labels{"effort:m"},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			wantStatus: "TODO",
			wantEffort: "M",
		},
		{
			name: "flat labels are ignored",
			issue: &gitlab.Issue{
				IID:       8,
				Title:     "flat label issue",
				State:     "opened",
				Labels:    gitlab.Labels{"bug", "enhancement"},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			wantStatus: "TODO",
			wantTags:   nil,
		},
		{
			name: "scoped labels become tags",
			issue: &gitlab.Issue{
				IID:       9,
				Title:     "scoped label issue",
				State:     "opened",
				Labels:    gitlab.Labels{"team:backend", "area:auth"},
				CreatedAt: &now,
				UpdatedAt: &now,
			},
			wantStatus: "TODO",
			wantTags:   []string{"team:backend", "area:auth"},
		},
		{
			name: "body blocked by #5 sets meta blocked_by",
			issue: &gitlab.Issue{
				IID:         10,
				Title:       "blocked by ref",
				State:       "opened",
				Description: "This is blocked by #5",
				CreatedAt:   &now,
				UpdatedAt:   &now,
			},
			wantStatus:    "TODO",
			wantMetaKey:   "blocked_by",
			wantMetaValue: "5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := MapGitLabIssueToTask(tt.issue)

			if task.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", task.Status, tt.wantStatus)
			}
			if tt.wantPriority != "" && task.Priority != tt.wantPriority {
				t.Errorf("Priority = %q, want %q", task.Priority, tt.wantPriority)
			}
			if tt.wantEffort != "" && task.Effort != tt.wantEffort {
				t.Errorf("Effort = %q, want %q", task.Effort, tt.wantEffort)
			}
			if task.BlockedReason != tt.wantBlocked {
				t.Errorf("BlockedReason = %q, want %q",
					task.BlockedReason, tt.wantBlocked)
			}
			if tt.wantTags != nil {
				if len(task.Tags) != len(tt.wantTags) {
					t.Errorf("Tags = %v, want %v", task.Tags, tt.wantTags)
				} else {
					for i, tag := range tt.wantTags {
						if task.Tags[i] != tag {
							t.Errorf("Tags[%d] = %q, want %q",
								i, task.Tags[i], tag)
						}
					}
				}
			} else if tt.wantTags == nil && tt.name == "flat labels are ignored" {
				if len(task.Tags) != 0 {
					t.Errorf("Tags = %v, want nil/empty", task.Tags)
				}
			}
			if tt.wantMetaKey != "" {
				v, ok := task.Meta[tt.wantMetaKey].(string)
				if !ok {
					t.Errorf("Meta[%q] missing", tt.wantMetaKey)
				} else if v != tt.wantMetaValue {
					t.Errorf("Meta[%q] = %q, want %q",
						tt.wantMetaKey, v, tt.wantMetaValue)
				}
			}
		})
	}
}

func TestMapTaskToGitLabIssueData(t *testing.T) {
	tests := []struct {
		name       string
		task       *Task
		wantState  string
		wantLabels []string
	}{
		{
			name: "TODO maps to reopen",
			task: &Task{
				Title:  "todo task",
				Status: "TODO",
				Meta:   map[string]interface{}{},
			},
			wantState:  "reopen",
			wantLabels: nil,
		},
		{
			name: "IN_PROGRESS maps to reopen with status:in-progress",
			task: &Task{
				Title:  "in progress task",
				Status: "IN_PROGRESS",
				Meta:   map[string]interface{}{},
			},
			wantState:  "reopen",
			wantLabels: []string{"status:in-progress"},
		},
		{
			name: "DONE maps to close",
			task: &Task{
				Title:  "done task",
				Status: "DONE",
				Meta:   map[string]interface{}{},
			},
			wantState:  "close",
			wantLabels: nil,
		},
		{
			name: "SKIPPED maps to close with wontfix",
			task: &Task{
				Title:  "skipped task",
				Status: "SKIPPED",
				Meta:   map[string]interface{}{},
			},
			wantState:  "close",
			wantLabels: []string{"wontfix"},
		},
		{
			name: "priority and effort become labels",
			task: &Task{
				Title:    "prioritized task",
				Status:   "TODO",
				Priority: "P1",
				Effort:   "L",
				Meta:     map[string]interface{}{},
			},
			wantState:  "reopen",
			wantLabels: []string{"priority:high", "effort:l"},
		},
		{
			name: "tags passed through as labels",
			task: &Task{
				Title:  "tagged task",
				Status: "TODO",
				Tags:   []string{"team:backend", "area:auth"},
				Meta:   map[string]interface{}{},
			},
			wantState:  "reopen",
			wantLabels: []string{"team:backend", "area:auth"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := MapTaskToGitLabIssueData(tt.task)

			if data.State != tt.wantState {
				t.Errorf("State = %q, want %q", data.State, tt.wantState)
			}

			if tt.wantLabels == nil {
				if len(data.Labels) != 0 {
					t.Errorf("Labels = %v, want empty", data.Labels)
				}
				return
			}

			if len(data.Labels) != len(tt.wantLabels) {
				t.Errorf("Labels = %v, want %v", data.Labels, tt.wantLabels)
				return
			}
			for i, want := range tt.wantLabels {
				if data.Labels[i] != want {
					t.Errorf("Labels[%d] = %q, want %q",
						i, data.Labels[i], want)
				}
			}
		})
	}
}

// --- TypeID round-trip (T-0823) ---

func TestTypeIDRoundTrip_PreservesEmbedded(t *testing.T) {
	const taskID = "task_01h455vb4pex5vsknk084sn02q"

	original := &Task{
		ID:          taskID,
		Title:       "round-trip",
		Status:      "TODO",
		Description: "body text",
	}

	data := MapTaskToGitLabIssueData(original)
	if !containsSub(data.Description, "<!-- tlc-uid: "+taskID+" -->") {
		t.Fatalf("Description missing tlc-uid footer: %q", data.Description)
	}

	now := time.Now()
	issue := &gitlab.Issue{
		IID:         42,
		Title:       "round-trip",
		Description: data.Description,
		State:       "opened",
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	got := MapGitLabIssueToTask(issue)

	if got.ID != taskID {
		t.Errorf("round-trip ID = %q, want %q", got.ID, taskID)
	}
}

func TestTypeIDRoundTrip_GeneratesWhenAbsent(t *testing.T) {
	now := time.Now()
	issue := &gitlab.Issue{
		IID:         43,
		Title:       "no footer",
		Description: "plain description",
		State:       "opened",
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	got := MapGitLabIssueToTask(issue)

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

// containsSub reports whether sub appears anywhere in s.
func containsSub(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestParseLabelDimensions(t *testing.T) {
	tests := []struct {
		name         string
		labels       []string
		wantPriority string
		wantEffort   string
		wantTags     []string
	}{
		{
			name:         "case insensitive Priority:High maps to P1",
			labels:       []string{"Priority:High"},
			wantPriority: "P1",
		},
		{
			name:         "case insensitive PRIORITY:CRITICAL maps to P0",
			labels:       []string{"PRIORITY:CRITICAL"},
			wantPriority: "P0",
		},
		{
			name:       "case insensitive Effort:XL maps to XL",
			labels:     []string{"Effort:XL"},
			wantEffort: "XL",
		},
		{
			name:         "mixed case labels parsed correctly",
			labels:       []string{"Priority:Medium", "Effort:S", "Team:Frontend"},
			wantPriority: "P2",
			wantEffort:   "S",
			wantTags:     []string{"Team:Frontend"},
		},
		{
			name:   "status labels are skipped",
			labels: []string{"status:in-progress"},
		},
		{
			name:   "flat labels are ignored",
			labels: []string{"bug", "feature"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{}
			parseLabelDimensions(tt.labels, task)

			if tt.wantPriority != "" && task.Priority != tt.wantPriority {
				t.Errorf("Priority = %q, want %q",
					task.Priority, tt.wantPriority)
			}
			if tt.wantEffort != "" && task.Effort != tt.wantEffort {
				t.Errorf("Effort = %q, want %q", task.Effort, tt.wantEffort)
			}
			if tt.wantTags != nil {
				if len(task.Tags) != len(tt.wantTags) {
					t.Errorf("Tags = %v, want %v", task.Tags, tt.wantTags)
				} else {
					for i, tag := range tt.wantTags {
						if task.Tags[i] != tag {
							t.Errorf("Tags[%d] = %q, want %q",
								i, task.Tags[i], tag)
						}
					}
				}
			}
		})
	}
}
