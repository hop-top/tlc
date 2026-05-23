package main

import (
	"strings"
	"testing"
)

func TestMapBitbucketIssueToTask(t *testing.T) {
	tests := []struct {
		name         string
		issue        *BitbucketIssue
		components   []string
		wantStatus   string
		wantPriority string
		wantEffort   string
		wantBlocked  string
		wantTags     []string
		wantMetaKey  string
		wantMetaVal  string
	}{
		{
			name:       "new state maps to TODO",
			issue:      &BitbucketIssue{ID: 1, Title: "t", State: "new"},
			wantStatus: "TODO",
		},
		{
			name:       "open state maps to TODO",
			issue:      &BitbucketIssue{ID: 2, Title: "t", State: "open"},
			wantStatus: "TODO",
		},
		{
			name:       "open + status:in-progress maps to IN_PROGRESS",
			issue:      &BitbucketIssue{ID: 3, Title: "t", State: "open"},
			components: []string{"status:in-progress"},
			wantStatus: "IN_PROGRESS",
		},
		{
			name:        "open + status:blocked maps to TODO with BlockedReason",
			issue:       &BitbucketIssue{ID: 4, Title: "t", State: "open"},
			components:  []string{"status:blocked"},
			wantStatus:  "TODO",
			wantBlocked: "blocked (from Bitbucket status:blocked label)",
		},
		{
			name:       "resolved maps to DONE",
			issue:      &BitbucketIssue{ID: 5, Title: "t", State: "resolved"},
			wantStatus: "DONE",
		},
		{
			name:       "closed maps to DONE",
			issue:      &BitbucketIssue{ID: 6, Title: "t", State: "closed"},
			wantStatus: "DONE",
		},
		{
			name:       "wontfix maps to SKIPPED",
			issue:      &BitbucketIssue{ID: 7, Title: "t", State: "wontfix"},
			wantStatus: "SKIPPED",
		},
		{
			name:       "invalid maps to SKIPPED",
			issue:      &BitbucketIssue{ID: 8, Title: "t", State: "invalid"},
			wantStatus: "SKIPPED",
		},
		{
			name:         "native priority blocker maps to P0",
			issue:        &BitbucketIssue{ID: 10, Title: "t", State: "new", Priority: "blocker"},
			wantStatus:   "TODO",
			wantPriority: "P0",
		},
		{
			name:         "native priority major maps to P1",
			issue:        &BitbucketIssue{ID: 11, Title: "t", State: "new", Priority: "major"},
			wantStatus:   "TODO",
			wantPriority: "P1",
		},
		{
			name:         "native priority minor maps to P2",
			issue:        &BitbucketIssue{ID: 12, Title: "t", State: "new", Priority: "minor"},
			wantStatus:   "TODO",
			wantPriority: "P2",
		},
		{
			name:         "native priority trivial maps to P3",
			issue:        &BitbucketIssue{ID: 13, Title: "t", State: "new", Priority: "trivial"},
			wantStatus:   "TODO",
			wantPriority: "P3",
		},
		{
			name:         "priority:high label overrides native priority",
			issue:        &BitbucketIssue{ID: 14, Title: "t", State: "new", Priority: "trivial"},
			components:   []string{"priority:high"},
			wantStatus:   "TODO",
			wantPriority: "P1",
		},
		{
			name:       "effort:m maps to M",
			issue:      &BitbucketIssue{ID: 15, Title: "t", State: "new"},
			components: []string{"effort:m"},
			wantStatus: "TODO",
			wantEffort: "M",
		},
		{
			name:       "dimension:foo goes into Tags",
			issue:      &BitbucketIssue{ID: 16, Title: "t", State: "new"},
			components: []string{"dimension:foo"},
			wantStatus: "TODO",
			wantTags:   []string{"dimension:foo"},
		},
		{
			name:       "flat labels are ignored",
			issue:      &BitbucketIssue{ID: 17, Title: "t", State: "new"},
			components: []string{"backend"},
			wantStatus: "TODO",
		},
		{
			name: "body blocked by #3 sets meta blocked_by",
			issue: &BitbucketIssue{
				ID: 18, Title: "t", State: "new",
				Content: &BitbucketContent{Raw: "blocked by #3"},
			},
			wantStatus:  "TODO",
			wantMetaKey: "blocked_by",
			wantMetaVal: "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := MapBitbucketIssueToTask(tt.issue, tt.components)

			if task.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", task.Status, tt.wantStatus)
			}
			if tt.wantPriority != "" && task.Priority != tt.wantPriority {
				t.Errorf("Priority = %q, want %q", task.Priority, tt.wantPriority)
			}
			if tt.wantEffort != "" && task.Effort != tt.wantEffort {
				t.Errorf("Effort = %q, want %q", task.Effort, tt.wantEffort)
			}
			if tt.wantBlocked != "" && task.BlockedReason != tt.wantBlocked {
				t.Errorf("BlockedReason = %q, want %q",
					task.BlockedReason, tt.wantBlocked)
			}
			if len(tt.wantTags) > 0 {
				if len(task.Tags) != len(tt.wantTags) {
					t.Fatalf("Tags len = %d, want %d",
						len(task.Tags), len(tt.wantTags))
				}
				for i, tag := range tt.wantTags {
					if task.Tags[i] != tag {
						t.Errorf("Tags[%d] = %q, want %q",
							i, task.Tags[i], tag)
					}
				}
			}
			if tt.wantMetaKey != "" {
				v, ok := task.Meta[tt.wantMetaKey].(string)
				if !ok {
					t.Fatalf("Meta[%q] missing", tt.wantMetaKey)
				}
				if v != tt.wantMetaVal {
					t.Errorf("Meta[%q] = %q, want %q",
						tt.wantMetaKey, v, tt.wantMetaVal)
				}
			}
		})
	}
}

func TestMapTaskToBitbucketIssue(t *testing.T) {
	tests := []struct {
		name          string
		task          *Task
		wantState     string
		wantPriority  string
		wantComponent string
		wantBody      string
	}{
		{
			name:      "TODO maps to open",
			task:      &Task{Title: "t", Status: "TODO"},
			wantState: "open",
		},
		{
			name:          "IN_PROGRESS maps to open with status:in-progress",
			task:          &Task{Title: "t", Status: "IN_PROGRESS"},
			wantState:     "open",
			wantComponent: "status:in-progress",
		},
		{
			name:      "DONE maps to resolved",
			task:      &Task{Title: "t", Status: "DONE"},
			wantState: "resolved",
		},
		{
			name:      "SKIPPED maps to invalid",
			task:      &Task{Title: "t", Status: "SKIPPED"},
			wantState: "invalid",
		},
		{
			name:          "Priority P0 produces priority:critical component",
			task:          &Task{Title: "t", Status: "TODO", Priority: "P0"},
			wantState:     "open",
			wantPriority:  "critical",
			wantComponent: "priority:p0",
		},
		{
			name:          "Effort L produces effort:l component",
			task:          &Task{Title: "t", Status: "TODO", Effort: "L"},
			wantState:     "open",
			wantComponent: "effort:l",
		},
		{
			name: "Tags passed through to component",
			task: &Task{
				Title: "t", Status: "TODO",
				Tags: []string{"scope:api"},
			},
			wantState:     "open",
			wantComponent: "scope:api",
		},
		{
			name: "BlockedBy in meta appends body cross-ref",
			task: &Task{
				Title:  "t",
				Status: "TODO",
				Meta:   map[string]interface{}{"blocked_by": "5"},
			},
			wantState: "open",
			wantBody:  "Blocked by #5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := MapTaskToBitbucketIssue(tt.task)

			if req.State != tt.wantState {
				t.Errorf("State = %q, want %q", req.State, tt.wantState)
			}
			if tt.wantPriority != "" && req.Priority != tt.wantPriority {
				t.Errorf("Priority = %q, want %q",
					req.Priority, tt.wantPriority)
			}
			if tt.wantComponent != "" {
				if req.Component == nil {
					t.Fatalf("Component nil, want containing %q",
						tt.wantComponent)
				}
				parts := strings.Split(req.Component.Name, ",")
				if !containsComponent(parts, tt.wantComponent) {
					t.Errorf("Component = %q, want containing %q",
						req.Component.Name, tt.wantComponent)
				}
			}
			if tt.wantBody != "" {
				if req.Content == nil {
					t.Fatalf("Content nil, want body %q", tt.wantBody)
				}
				if req.Content.Raw != tt.wantBody {
					t.Errorf("Body = %q, want %q",
						req.Content.Raw, tt.wantBody)
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

	req := MapTaskToBitbucketIssue(original)
	if req.Content == nil || !strings.Contains(req.Content.Raw, "<!-- tlc-uid: "+taskID+" -->") {
		raw := ""
		if req.Content != nil {
			raw = req.Content.Raw
		}
		t.Fatalf("push body missing tlc-uid footer: %q", raw)
	}

	// Pull back: build a BB issue with that body.
	issue := &BitbucketIssue{
		ID:    77,
		Title: "round-trip",
		State: "new",
		Content: &BitbucketContent{
			Raw: req.Content.Raw,
		},
	}
	got := MapBitbucketIssueToTask(issue, nil)

	if got.ID != taskID {
		t.Errorf("round-trip ID = %q, want %q", got.ID, taskID)
	}
}

func TestTypeIDRoundTrip_GeneratesWhenAbsent(t *testing.T) {
	issue := &BitbucketIssue{
		ID:      78,
		Title:   "no footer",
		State:   "new",
		Content: &BitbucketContent{Raw: "plain description"},
	}
	got := MapBitbucketIssueToTask(issue, nil)

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

func TestMapTLCPriorityToBitbucket(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"P0", "critical"},
		{"P1", "major"},
		{"P2", "minor"},
		{"P3", "trivial"},
		{"", ""},
		{"unknown", ""},
	}

	for _, tt := range tests {
		name := tt.input
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			got := mapTLCPriorityToBitbucket(tt.input)
			if got != tt.want {
				t.Errorf("mapTLCPriorityToBitbucket(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}
