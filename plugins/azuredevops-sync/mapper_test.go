package main

import (
	"testing"
)

func TestMapWorkItemToTask(t *testing.T) {
	const org = "myorg"
	const project = "myproject"

	baseFields := func(state string, priority float64) WorkItemFields {
		return WorkItemFields{
			"System.Title":                    "Test Item",
			"System.Description":              "desc",
			"System.State":                    state,
			"System.CreatedDate":              "2026-01-01T00:00:00Z",
			"System.ChangedDate":              "2026-01-02T00:00:00Z",
			"System.IterationPath":            "Sprint 1",
			"System.AreaPath":                 "Area",
			"Microsoft.VSTS.Common.Priority":  priority,
		}
	}

	tests := []struct {
		name          string
		wi            *WorkItem
		wantStatus    string
		wantPriority  string
		wantEffort    string
		wantBlocked   string
		wantTags      []string
		wantBlockedBy []string
		wantBlocks    []string
	}{
		{
			name:       "New state maps to TODO",
			wi:         &WorkItem{ID: 1, Fields: baseFields("New", 3)},
			wantStatus: "TODO", wantPriority: "P2",
		},
		{
			name:       "Active state maps to IN_PROGRESS",
			wi:         &WorkItem{ID: 2, Fields: baseFields("Active", 2)},
			wantStatus: "IN_PROGRESS", wantPriority: "P1",
		},
		{
			name: "Active with status:blocked tag maps to TODO + BlockedReason",
			wi: &WorkItem{ID: 3, Fields: func() WorkItemFields {
				f := baseFields("Active", 2)
				f["System.Tags"] = "status:blocked"
				return f
			}()},
			wantStatus:  "TODO",
			wantBlocked: "blocked via ADO tag",
			wantPriority: "P1",
		},
		{
			name:       "Resolved maps to DONE",
			wi:         &WorkItem{ID: 4, Fields: baseFields("Resolved", 1)},
			wantStatus: "DONE", wantPriority: "P0",
		},
		{
			name:       "Closed maps to DONE",
			wi:         &WorkItem{ID: 5, Fields: baseFields("Closed", 1)},
			wantStatus: "DONE", wantPriority: "P0",
		},
		{
			name:       "Removed maps to SKIPPED",
			wi:         &WorkItem{ID: 6, Fields: baseFields("Removed", 4)},
			wantStatus: "SKIPPED", wantPriority: "P3",
		},
		{
			name:         "ADO priority 1 maps to P0",
			wi:           &WorkItem{ID: 10, Fields: baseFields("New", 1)},
			wantStatus:   "TODO",
			wantPriority: "P0",
		},
		{
			name:         "ADO priority 2 maps to P1",
			wi:           &WorkItem{ID: 11, Fields: baseFields("New", 2)},
			wantStatus:   "TODO",
			wantPriority: "P1",
		},
		{
			name:         "ADO priority 3 maps to P2",
			wi:           &WorkItem{ID: 12, Fields: baseFields("New", 3)},
			wantStatus:   "TODO",
			wantPriority: "P2",
		},
		{
			name:         "ADO priority 4 maps to P3",
			wi:           &WorkItem{ID: 13, Fields: baseFields("New", 4)},
			wantStatus:   "TODO",
			wantPriority: "P3",
		},
		{
			name: "priority:high tag overrides native priority to P1",
			wi: &WorkItem{ID: 14, Fields: func() WorkItemFields {
				f := baseFields("New", 4)
				f["System.Tags"] = "priority:high"
				return f
			}()},
			wantStatus:   "TODO",
			wantPriority: "HIGH",
		},
		{
			name: "effort:m tag maps to effort M",
			wi: &WorkItem{ID: 15, Fields: func() WorkItemFields {
				f := baseFields("New", 3)
				f["System.Tags"] = "effort:m"
				return f
			}()},
			wantStatus:   "TODO",
			wantPriority: "P2",
			wantEffort:   "M",
		},
		{
			name: "flat tags are ignored",
			wi: &WorkItem{ID: 16, Fields: func() WorkItemFields {
				f := baseFields("New", 3)
				f["System.Tags"] = "my-flat-tag; another-tag"
				return f
			}()},
			wantStatus:   "TODO",
			wantPriority: "P2",
			wantTags:     nil,
		},
		{
			name: "dimension:scope tags become TLC tags",
			wi: &WorkItem{ID: 17, Fields: func() WorkItemFields {
				f := baseFields("New", 3)
				f["System.Tags"] = "dimension:scope:backend"
				return f
			}()},
			wantStatus:   "TODO",
			wantPriority: "P2",
			wantTags:     []string{"dimension:scope:backend"},
		},
		{
			name: "Dependency-Reverse links populate blocked_by",
			wi: &WorkItem{
				ID:     20,
				Fields: baseFields("New", 3),
				Relations: []WorkItemRelation{
					{
						Rel: "System.LinkTypes.Dependency-Reverse",
						URL: "https://dev.azure.com/myorg/myproject/_apis/wit/workItems/99",
					},
				},
			},
			wantStatus:    "TODO",
			wantPriority:  "P2",
			wantBlockedBy: []string{"AZ-99"},
		},
		{
			name: "Dependency-Forward links populate blocks",
			wi: &WorkItem{
				ID:     21,
				Fields: baseFields("New", 3),
				Relations: []WorkItemRelation{
					{
						Rel: "System.LinkTypes.Dependency-Forward",
						URL: "https://dev.azure.com/myorg/myproject/_apis/wit/workItems/55",
					},
				},
			},
			wantStatus:   "TODO",
			wantPriority: "P2",
			wantBlocks:   []string{"AZ-55"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := MapWorkItemToTask(tt.wi, org, project)

			if task.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", task.Status, tt.wantStatus)
			}
			if task.Priority != tt.wantPriority {
				t.Errorf("Priority = %q, want %q", task.Priority, tt.wantPriority)
			}
			if task.Effort != tt.wantEffort {
				t.Errorf("Effort = %q, want %q", task.Effort, tt.wantEffort)
			}
			if task.BlockedReason != tt.wantBlocked {
				t.Errorf("BlockedReason = %q, want %q",
					task.BlockedReason, tt.wantBlocked)
			}
			assertStringSlice(t, "Tags", task.Tags, tt.wantTags)

			if tt.wantBlockedBy != nil {
				raw, ok := task.Meta["blocked_by"]
				if !ok {
					t.Fatal("Meta[blocked_by] missing")
				}
				assertStringSlice(t, "blocked_by",
					raw.([]string), tt.wantBlockedBy)
			}
			if tt.wantBlocks != nil {
				raw, ok := task.Meta["blocks"]
				if !ok {
					t.Fatal("Meta[blocks] missing")
				}
				assertStringSlice(t, "blocks",
					raw.([]string), tt.wantBlocks)
			}
		})
	}
}

func TestMapTaskToWorkItem(t *testing.T) {
	tests := []struct {
		name      string
		task      *Task
		wantOps   map[string]interface{} // path → value
		wantTag   string                 // substring in System.Tags value
	}{
		{
			name: "Status TODO maps to state New",
			task: &Task{
				Title:  "T1",
				Status: "TODO",
			},
			wantOps: map[string]interface{}{
				"/fields/System.State": "New",
			},
		},
		{
			name: "Status IN_PROGRESS maps to state Active",
			task: &Task{
				Title:  "T2",
				Status: "IN_PROGRESS",
			},
			wantOps: map[string]interface{}{
				"/fields/System.State": "Active",
			},
			wantTag: "status:in-progress",
		},
		{
			name: "Status DONE maps to state Closed",
			task: &Task{
				Title:  "T3",
				Status: "DONE",
			},
			wantOps: map[string]interface{}{
				"/fields/System.State": "Closed",
			},
		},
		{
			name: "Status SKIPPED maps to state Removed",
			task: &Task{
				Title:  "T4",
				Status: "SKIPPED",
			},
			wantOps: map[string]interface{}{
				"/fields/System.State": "Removed",
			},
		},
		{
			name: "Priority P0 sets ADO priority 1 and tag",
			task: &Task{
				Title:    "T5",
				Status:   "TODO",
				Priority: "P0",
			},
			wantOps: map[string]interface{}{
				"/fields/Microsoft.VSTS.Common.Priority": 1,
			},
			wantTag: "priority:p0",
		},
		{
			name: "Priority P2 sets ADO priority 3",
			task: &Task{
				Title:    "T6",
				Status:   "TODO",
				Priority: "P2",
			},
			wantOps: map[string]interface{}{
				"/fields/Microsoft.VSTS.Common.Priority": 3,
			},
			wantTag: "priority:p2",
		},
		{
			name: "Effort populates effort tag",
			task: &Task{
				Title:  "T7",
				Status: "TODO",
				Effort: "M",
			},
			wantTag: "effort:m",
		},
		{
			name: "Tags passed through to System.Tags",
			task: &Task{
				Title:  "T8",
				Status: "TODO",
				Tags:   []string{"dimension:scope:backend"},
			},
			wantTag: "dimension:scope:backend",
		},
		{
			name: "BlockedReason adds status:blocked tag",
			task: &Task{
				Title:         "T9",
				Status:        "TODO",
				BlockedReason: "dependency stall",
			},
			wantTag: "status:blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := MapTaskToWorkItem(tt.task)
			opMap := patchMap(ops)

			for path, want := range tt.wantOps {
				got, ok := opMap[path]
				if !ok {
					t.Errorf("missing patch for %s", path)
					continue
				}
				// ADO priority comes as int from map but
				// PatchOperation stores interface{}.
				if !valEqual(got, want) {
					t.Errorf("patch %s = %v (%T), want %v (%T)",
						path, got, got, want, want)
				}
			}

			if tt.wantTag != "" {
				tagVal, ok := opMap["/fields/System.Tags"]
				if !ok {
					t.Fatalf("System.Tags patch missing")
				}
				s, _ := tagVal.(string)
				if !containsSubstr(s, tt.wantTag) {
					t.Errorf("System.Tags = %q, want substring %q",
						s, tt.wantTag)
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

	ops := MapTaskToWorkItem(original)
	opMap := patchMap(ops)

	desc, _ := opMap["/fields/System.Description"].(string)
	if !contains(desc, "<!-- tlc-uid: "+taskID+" -->") {
		t.Fatalf("System.Description missing tlc-uid footer: %q", desc)
	}

	// Pull back: reconstruct a WorkItem with the patched description.
	wi := &WorkItem{
		ID: 99,
		Fields: WorkItemFields{
			"System.Title":       "round-trip",
			"System.Description": desc,
			"System.State":       "New",
			"System.CreatedDate": "2026-01-01T00:00:00Z",
			"System.ChangedDate": "2026-01-02T00:00:00Z",
		},
	}
	got := MapWorkItemToTask(wi, "org", "proj")

	if got.ID != taskID {
		t.Errorf("round-trip ID = %q, want %q", got.ID, taskID)
	}
}

func TestTypeIDRoundTrip_GeneratesWhenAbsent(t *testing.T) {
	wi := &WorkItem{
		ID: 100,
		Fields: WorkItemFields{
			"System.Title":       "no footer",
			"System.Description": "plain description",
			"System.State":       "New",
			"System.CreatedDate": "2026-01-01T00:00:00Z",
			"System.ChangedDate": "2026-01-02T00:00:00Z",
		},
	}
	got := MapWorkItemToTask(wi, "org", "proj")

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

func contains(s, sub string) bool { return findSubstr(s, sub) }

func TestMapBlockedByToLinkPatches(t *testing.T) {
	ops := MapBlockedByToLinkPatches(
		[]string{"AZ-10", "AZ-20"}, "org1", "proj1",
	)
	if len(ops) != 2 {
		t.Fatalf("got %d ops, want 2", len(ops))
	}
	for i, op := range ops {
		if op.Op != "add" {
			t.Errorf("op[%d].Op = %q, want add", i, op.Op)
		}
		if op.Path != "/relations/-" {
			t.Errorf("op[%d].Path = %q, want /relations/-", i, op.Path)
		}
		val, ok := op.Value.(map[string]interface{})
		if !ok {
			t.Fatalf("op[%d].Value not map", i)
		}
		if val["rel"] != "System.LinkTypes.Dependency-Reverse" {
			t.Errorf("op[%d] rel = %v", i, val["rel"])
		}
	}
}

// --- helpers ---

func assertStringSlice(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s len = %d, want %d (%v vs %v)",
			label, len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", label, i, got[i], want[i])
		}
	}
}

func patchMap(ops []PatchOperation) map[string]interface{} {
	m := make(map[string]interface{}, len(ops))
	for _, op := range ops {
		m[op.Path] = op.Value
	}
	return m
}

func valEqual(a, b interface{}) bool {
	// Handle int vs float64 comparison (JSON number ambiguity).
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			return av == bv
		}
	case float64:
		if bv, ok := b.(int); ok {
			return int(av) == bv
		}
	}
	return a == b
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		len(sub) == 0 ||
		findSubstr(s, sub))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
