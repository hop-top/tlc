package main

import (
	"testing"

	"github.com/google/go-github/v69/github"
)

// --- helpers ---

func strPtr(s string) *string { return &s }

func makeLabel(name string) *github.Label {
	return &github.Label{Name: &name}
}

func makeIssue(opts ...func(*github.Issue)) *github.Issue {
	state := "open"
	iss := &github.Issue{
		Number: github.Ptr(1),
		Title:  strPtr("test issue"),
		State:  &state,
		Body:   strPtr(""),
	}
	for _, o := range opts {
		o(iss)
	}
	return iss
}

func withState(s string) func(*github.Issue) {
	return func(i *github.Issue) { i.State = &s }
}

func withStateReason(r string) func(*github.Issue) {
	return func(i *github.Issue) { i.StateReason = &r }
}

func withBody(b string) func(*github.Issue) {
	return func(i *github.Issue) { i.Body = &b }
}

func withLabels(names ...string) func(*github.Issue) {
	return func(i *github.Issue) {
		for _, n := range names {
			i.Labels = append(i.Labels, makeLabel(n))
		}
	}
}

// --- MapGitHubIssueToTask ---

func TestMapGitHubIssueToTask(t *testing.T) {
	tests := []struct {
		name          string
		issue         *github.Issue
		wantStatus    string
		wantPriority  string
		wantEffort    string
		wantBlocked   *string
		wantTags      []string
		wantBlockedBy []string // Meta["blocked_by"]
	}{
		{
			name:       "open issue → TODO",
			issue:      makeIssue(),
			wantStatus: "TODO",
		},
		{
			name:       "open + status:in-progress → IN_PROGRESS",
			issue:      makeIssue(withLabels("status:in-progress")),
			wantStatus: "IN_PROGRESS",
		},
		{
			name:        "open + status:blocked → TODO + BlockedReason",
			issue:       makeIssue(withLabels("status:blocked")),
			wantStatus:  "TODO",
			wantBlocked: strPtr("blocked"),
		},
		{
			name:       "closed completed → DONE",
			issue:      makeIssue(withState("closed"), withStateReason("completed")),
			wantStatus: "DONE",
		},
		{
			name:       "closed not_planned → SKIPPED",
			issue:      makeIssue(withState("closed"), withStateReason("not_planned")),
			wantStatus: "SKIPPED",
		},
		{
			name:         "priority:critical → P0",
			issue:        makeIssue(withLabels("priority:critical")),
			wantStatus:   "TODO",
			wantPriority: "P0",
		},
		{
			name:         "priority:low → P3",
			issue:        makeIssue(withLabels("priority:low")),
			wantStatus:   "TODO",
			wantPriority: "P3",
		},
		{
			name:       "effort:xl → XL",
			issue:      makeIssue(withLabels("effort:xl")),
			wantStatus: "TODO",
			wantEffort: "XL",
		},
		{
			name:       "flat labels (bug, enhancement) ignored",
			issue:      makeIssue(withLabels("bug", "enhancement")),
			wantStatus: "TODO",
			wantTags:   nil,
		},
		{
			name:       "dimension:scope labels → Tags",
			issue:      makeIssue(withLabels("area:frontend", "team:platform")),
			wantStatus: "TODO",
			wantTags:   []string{"area:frontend", "team:platform"},
		},
		{
			name:          "body with blocked by #42 → Meta[blocked_by]",
			issue:         makeIssue(withBody("Blocked by #42")),
			wantStatus:    "TODO",
			wantBlockedBy: []string{"42"},
		},
		{
			name: "BlockedReason derived from refs after parseBlockedBy",
			issue: makeIssue(
				withLabels("status:blocked"),
				withBody("blocked by #10\nDepends on #20"),
			),
			wantStatus:    "TODO",
			wantBlocked:   strPtr("blocked by #10, #20"),
			wantBlockedBy: []string{"10", "20"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := MapGitHubIssueToTask(tt.issue)

			if task.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", task.Status, tt.wantStatus)
			}
			if task.Priority != tt.wantPriority {
				t.Errorf("Priority = %q, want %q", task.Priority, tt.wantPriority)
			}
			if task.Effort != tt.wantEffort {
				t.Errorf("Effort = %q, want %q", task.Effort, tt.wantEffort)
			}

			// BlockedReason
			switch {
			case tt.wantBlocked == nil && task.BlockedReason != nil:
				t.Errorf("BlockedReason = %q, want nil", *task.BlockedReason)
			case tt.wantBlocked != nil && task.BlockedReason == nil:
				t.Errorf("BlockedReason = nil, want %q", *tt.wantBlocked)
			case tt.wantBlocked != nil && *task.BlockedReason != *tt.wantBlocked:
				t.Errorf("BlockedReason = %q, want %q",
					*task.BlockedReason, *tt.wantBlocked)
			}

			// Tags
			if !strSliceEqual(task.Tags, tt.wantTags) {
				t.Errorf("Tags = %v, want %v", task.Tags, tt.wantTags)
			}

			// blocked_by meta
			if len(tt.wantBlockedBy) > 0 {
				raw, ok := task.Meta["blocked_by"]
				if !ok {
					t.Fatal("Meta[blocked_by] missing")
				}
				refs, ok := raw.([]string)
				if !ok {
					t.Fatalf("Meta[blocked_by] type = %T, want []string", raw)
				}
				if !strSliceEqual(refs, tt.wantBlockedBy) {
					t.Errorf("Meta[blocked_by] = %v, want %v",
						refs, tt.wantBlockedBy)
				}
			}
		})
	}
}

// --- MapTaskToGitHubIssueRequest ---

func TestMapTaskToGitHubIssueRequest(t *testing.T) {
	tests := []struct {
		name            string
		task            *Task
		wantState       string
		wantStateReason string
		wantLabels      []string
		wantBodyPrefix  string
	}{
		{
			name:       "TODO → open, no status labels",
			task:       &Task{Title: "t", Status: "TODO"},
			wantState:  "open",
			wantLabels: nil,
		},
		{
			name:       "IN_PROGRESS → open + status:in-progress",
			task:       &Task{Title: "t", Status: "IN_PROGRESS"},
			wantState:  "open",
			wantLabels: []string{"status:in-progress"},
		},
		{
			name: "BlockedReason → open + status:blocked",
			task: &Task{
				Title:         "t",
				Status:        "TODO",
				BlockedReason: strPtr("blocked by #5"),
			},
			wantState:  "open",
			wantLabels: []string{"status:blocked"},
		},
		{
			name:            "DONE → closed + completed",
			task:            &Task{Title: "t", Status: "DONE"},
			wantState:       "closed",
			wantStateReason: "completed",
		},
		{
			name:            "SKIPPED → closed + not_planned",
			task:            &Task{Title: "t", Status: "SKIPPED"},
			wantState:       "closed",
			wantStateReason: "not_planned",
		},
		{
			name:       "Priority P0 → priority:critical label",
			task:       &Task{Title: "t", Status: "TODO", Priority: "P0"},
			wantState:  "open",
			wantLabels: []string{"priority:critical"},
		},
		{
			name:       "Effort M → effort:m label",
			task:       &Task{Title: "t", Status: "TODO", Effort: "M"},
			wantState:  "open",
			wantLabels: []string{"effort:m"},
		},
		{
			name: "Tags passed through",
			task: &Task{
				Title:  "t",
				Status: "TODO",
				Tags:   []string{"area:backend", "team:core"},
			},
			wantState:  "open",
			wantLabels: []string{"area:backend", "team:core"},
		},
		{
			name: "Body with blocked-by refs prepended",
			task: &Task{
				Title:       "t",
				Status:      "TODO",
				Description: "some desc",
				Meta: map[string]interface{}{
					"blocked_by": []string{"42", "99"},
				},
			},
			wantState:      "open",
			wantBodyPrefix: "Blocked by #42\nBlocked by #99\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := MapTaskToGitHubIssueRequest(tt.task)

			if got := req.GetState(); got != tt.wantState {
				t.Errorf("State = %q, want %q", got, tt.wantState)
			}

			if tt.wantStateReason != "" {
				if got := req.GetStateReason(); got != tt.wantStateReason {
					t.Errorf("StateReason = %q, want %q",
						got, tt.wantStateReason)
				}
			}

			gotLabels := derefLabels(req.Labels)
			if !strSliceEqual(gotLabels, tt.wantLabels) {
				t.Errorf("Labels = %v, want %v", gotLabels, tt.wantLabels)
			}

			if tt.wantBodyPrefix != "" {
				body := req.GetBody()
				if len(body) < len(tt.wantBodyPrefix) ||
					body[:len(tt.wantBodyPrefix)] != tt.wantBodyPrefix {
					t.Errorf("Body prefix = %q, want prefix %q",
						body, tt.wantBodyPrefix)
				}
			}
		})
	}
}

// --- buildPushBody ---

func TestBuildPushBody(t *testing.T) {
	tests := []struct {
		name string
		task *Task
		want string
	}{
		{
			name: "no refs → body unchanged",
			task: &Task{Description: "hello world"},
			want: "hello world",
		},
		{
			name: "refs prepended to body",
			task: &Task{
				Description: "original",
				Meta: map[string]interface{}{
					"blocked_by": []string{"7"},
				},
			},
			want: "Blocked by #7\n\noriginal",
		},
		{
			name: "existing blocked-by lines stripped before prepend (dedup)",
			task: &Task{
				Description: "Blocked by #7\n\noriginal text",
				Meta: map[string]interface{}{
					"blocked_by": []string{"7"},
				},
			},
			want: "Blocked by #7\n\noriginal text",
		},
		{
			name: "multiple existing lines stripped",
			task: &Task{
				Description: "Blocked by #7\nDepends on #8\n\nbody here",
				Meta: map[string]interface{}{
					"blocked_by": []string{"7", "8"},
				},
			},
			want: "Blocked by #7\nBlocked by #8\n\nbody here",
		},
		{
			name: "backtick unescape",
			task: &Task{Description: "use \\`code\\` here"},
			want: "use `code` here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPushBody(tt.task)
			if got != tt.want {
				t.Errorf("buildPushBody:\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}

// --- test utilities ---

func strSliceEqual(a, b []string) bool {
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

func derefLabels(labels *[]string) []string {
	if labels == nil {
		return nil
	}
	return *labels
}
