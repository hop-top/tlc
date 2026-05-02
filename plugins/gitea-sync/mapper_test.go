package main

import (
	"testing"
	"time"
)

func TestMapGiteaIssueToTask(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		issue         *GiteaIssue
		deps          []GiteaDependency
		wantStatus    string
		wantPriority  string
		wantEffort    string
		wantBlocked   string
		wantTags      []string
		wantBlockedBy []string
		wantAssignee  string
	}{
		{
			name: "open issue maps to TODO",
			issue: &GiteaIssue{
				Index: 1, Title: "task", State: "open",
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus: "TODO",
		},
		{
			name: "open with status:in-progress maps to IN_PROGRESS",
			issue: &GiteaIssue{
				Index: 2, Title: "wip", State: "open",
				Labels:    []GiteaLabel{{Name: "status:in-progress"}},
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus: "IN_PROGRESS",
		},
		{
			name: "open with status:blocked maps to TODO with BlockedReason",
			issue: &GiteaIssue{
				Index: 3, Title: "stuck", State: "open",
				Labels:    []GiteaLabel{{Name: "status:blocked"}},
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus:  "TODO",
			wantBlocked: "blocked",
		},
		{
			name: "closed issue maps to DONE",
			issue: &GiteaIssue{
				Index: 4, Title: "finished", State: "closed",
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus: "DONE",
		},
		{
			name: "closed with wontfix maps to SKIPPED",
			issue: &GiteaIssue{
				Index: 5, Title: "nope", State: "closed",
				Labels:    []GiteaLabel{{Name: "wontfix"}},
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus: "SKIPPED",
		},
		{
			name: "priority:critical maps to P0",
			issue: &GiteaIssue{
				Index: 6, Title: "urgent", State: "open",
				Labels:    []GiteaLabel{{Name: "priority:critical"}},
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus:   "TODO",
			wantPriority: "P0",
		},
		{
			name: "effort:xl maps to XL",
			issue: &GiteaIssue{
				Index: 7, Title: "big", State: "open",
				Labels:    []GiteaLabel{{Name: "effort:xl"}},
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus: "TODO",
			wantEffort: "XL",
		},
		{
			name: "flat labels are ignored",
			issue: &GiteaIssue{
				Index: 8, Title: "flat", State: "open",
				Labels:    []GiteaLabel{{Name: "bug"}, {Name: "enhancement"}},
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus: "TODO",
		},
		{
			name: "dimension:foo maps to Tags",
			issue: &GiteaIssue{
				Index: 9, Title: "tagged", State: "open",
				Labels:    []GiteaLabel{{Name: "dimension:foo"}},
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus: "TODO",
			wantTags:   []string{"foo"},
		},
		{
			name: "body blocked by #7 maps to Meta blocked_by",
			issue: &GiteaIssue{
				Index: 10, Title: "dep", State: "open",
				Body:      "This is blocked by #7",
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus:    "TODO",
			wantBlockedBy: []string{"#7"},
		},
		{
			name: "case insensitive Status:In-Progress",
			issue: &GiteaIssue{
				Index: 11, Title: "case", State: "open",
				Labels:    []GiteaLabel{{Name: "Status:In-Progress"}},
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus: "IN_PROGRESS",
		},
		{
			name: "assignee is mapped",
			issue: &GiteaIssue{
				Index: 12, Title: "assigned", State: "open",
				Assignee:  &GiteaUser{Login: "alice"},
				CreatedAt: now, UpdatedAt: now,
			},
			wantStatus:   "TODO",
			wantAssignee: "alice",
		},
		{
			name: "API deps merged with body refs",
			issue: &GiteaIssue{
				Index: 13, Title: "multi-dep", State: "open",
				Body:      "blocked by #1",
				CreatedAt: now, UpdatedAt: now,
			},
			deps:          []GiteaDependency{{Index: 1}, {Index: 2}},
			wantStatus:    "TODO",
			wantBlockedBy: []string{"#1", "#2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := MapGiteaIssueToTask(tc.issue, tc.deps)

			if task.Status != tc.wantStatus {
				t.Errorf("Status = %q; want %q", task.Status, tc.wantStatus)
			}
			if tc.wantPriority != "" && task.Priority != tc.wantPriority {
				t.Errorf("Priority = %q; want %q", task.Priority, tc.wantPriority)
			}
			if tc.wantEffort != "" && task.Effort != tc.wantEffort {
				t.Errorf("Effort = %q; want %q", task.Effort, tc.wantEffort)
			}
			if tc.wantBlocked != "" && task.BlockedReason != tc.wantBlocked {
				t.Errorf("BlockedReason = %q; want %q",
					task.BlockedReason, tc.wantBlocked)
			}
			if tc.wantAssignee != "" && task.AssignedTo != tc.wantAssignee {
				t.Errorf("AssignedTo = %q; want %q",
					task.AssignedTo, tc.wantAssignee)
			}
			if len(tc.wantTags) > 0 {
				if !stringSliceEqual(task.Tags, tc.wantTags) {
					t.Errorf("Tags = %v; want %v", task.Tags, tc.wantTags)
				}
			}
			if len(tc.wantBlockedBy) > 0 {
				got := metaBlockedBy(task)
				if !stringSliceEqual(got, tc.wantBlockedBy) {
					t.Errorf("blocked_by = %v; want %v",
						got, tc.wantBlockedBy)
				}
			}
		})
	}
}

func TestMapTaskToGiteaIssue(t *testing.T) {
	tests := []struct {
		name        string
		task        *Task
		wantState   string
		wantLabels  []string
		wantAssign  string
		wantBodySub string
	}{
		{
			name:       "TODO maps to open",
			task:       &Task{Title: "t", Status: "TODO"},
			wantState:  "open",
			wantLabels: nil,
		},
		{
			name:       "IN_PROGRESS maps to open with label",
			task:       &Task{Title: "t", Status: "IN_PROGRESS"},
			wantState:  "open",
			wantLabels: []string{"status:in-progress"},
		},
		{
			name:       "DONE maps to closed",
			task:       &Task{Title: "t", Status: "DONE"},
			wantState:  "closed",
			wantLabels: nil,
		},
		{
			name:       "SKIPPED maps to closed with wontfix",
			task:       &Task{Title: "t", Status: "SKIPPED"},
			wantState:  "closed",
			wantLabels: []string{"wontfix"},
		},
		{
			name:       "Priority P1 maps to priority:high label",
			task:       &Task{Title: "t", Status: "TODO", Priority: "P1"},
			wantState:  "open",
			wantLabels: []string{"priority:high"},
		},
		{
			name:       "Effort M maps to effort:m label",
			task:       &Task{Title: "t", Status: "TODO", Effort: "M"},
			wantState:  "open",
			wantLabels: []string{"effort:m"},
		},
		{
			name: "Tags passed through as dimension labels",
			task: &Task{
				Title: "t", Status: "TODO",
				Tags: []string{"backend", "api"},
			},
			wantState:  "open",
			wantLabels: []string{"dimension:backend", "dimension:api"},
		},
		{
			name: "AssignedTo passed through",
			task: &Task{
				Title: "t", Status: "TODO", AssignedTo: "bob",
			},
			wantState:  "open",
			wantAssign: "bob",
		},
		{
			name: "blocked_by refs appended to body",
			task: &Task{
				Title: "t", Status: "TODO",
				Description: "desc",
				Meta: map[string]interface{}{
					"blocked_by": []interface{}{"#3", "#5"},
				},
			},
			wantState:   "open",
			wantBodySub: "Blocked by #3",
		},
		{
			name: "TODO with BlockedReason gets status:blocked label",
			task: &Task{
				Title: "t", Status: "TODO",
				BlockedReason: "waiting",
			},
			wantState:  "open",
			wantLabels: []string{"status:blocked"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := MapTaskToGiteaIssue(tc.task)

			if got := result["state"].(string); got != tc.wantState {
				t.Errorf("state = %q; want %q", got, tc.wantState)
			}

			labels, _ := result["labels"].([]string)
			if len(tc.wantLabels) > 0 {
				for _, want := range tc.wantLabels {
					if !containsString(labels, want) {
						t.Errorf("labels %v missing %q", labels, want)
					}
				}
			}

			if tc.wantAssign != "" {
				if got, ok := result["assignee"].(string); !ok || got != tc.wantAssign {
					t.Errorf("assignee = %v; want %q", result["assignee"], tc.wantAssign)
				}
			}

			if tc.wantBodySub != "" {
				body := result["body"].(string)
				if !containsSubstring(body, tc.wantBodySub) {
					t.Errorf("body = %q; want substring %q", body, tc.wantBodySub)
				}
			}
		})
	}
}

func TestBlockedByIssueIndices(t *testing.T) {
	tests := []struct {
		name string
		task *Task
		want []int64
	}{
		{
			name: "nil meta returns nil",
			task: &Task{},
			want: nil,
		},
		{
			name: "no blocked_by key returns nil",
			task: &Task{Meta: map[string]interface{}{"other": "val"}},
			want: nil,
		},
		{
			name: "extracts indices from refs",
			task: &Task{
				Meta: map[string]interface{}{
					"blocked_by": []interface{}{"#3", "#15"},
				},
			},
			want: []int64{3, 15},
		},
		{
			name: "skips non-numeric refs",
			task: &Task{
				Meta: map[string]interface{}{
					"blocked_by": []interface{}{"#abc", "#7"},
				},
			},
			want: []int64{7},
		},
		{
			name: "wrong type returns nil",
			task: &Task{
				Meta: map[string]interface{}{
					"blocked_by": "not-a-slice",
				},
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BlockedByIssueIndices(tc.task)
			if !int64SliceEqual(got, tc.want) {
				t.Errorf("BlockedByIssueIndices = %v; want %v", got, tc.want)
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

	out := MapTaskToGiteaIssue(original)
	body, _ := out["body"].(string)
	if !containsSubstring(body, "<!-- tlc-uid: "+taskID+" -->") {
		t.Fatalf("push body missing tlc-uid footer: %q", body)
	}

	now := time.Now()
	issue := &GiteaIssue{
		Index:     7,
		Title:     "round-trip",
		Body:      body,
		State:     "open",
		CreatedAt: now,
		UpdatedAt: now,
	}
	got := MapGiteaIssueToTask(issue, nil)

	if got.ID != taskID {
		t.Errorf("round-trip ID = %q, want %q", got.ID, taskID)
	}
}

func TestTypeIDRoundTrip_GeneratesWhenAbsent(t *testing.T) {
	now := time.Now()
	issue := &GiteaIssue{
		Index:     8,
		Title:     "no footer",
		Body:      "plain description",
		State:     "open",
		CreatedAt: now,
		UpdatedAt: now,
	}
	got := MapGiteaIssueToTask(issue, nil)

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

// --- helpers ---

func metaBlockedBy(task *Task) []string {
	raw, ok := task.Meta["blocked_by"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		var out []string
		for _, r := range v {
			if s, ok := r.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func int64SliceEqual(a, b []int64) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		len(s) > 0 && containsSubstringSearch(s, sub))
}

func containsSubstringSearch(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
