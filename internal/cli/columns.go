package cli

import (
	"strings"
)

// taskColumnHeaders maps a lowercase column key to the table:"" header
// string used by the taskTableRow struct.
var taskColumnHeaders = map[string]string{
	"id":       "ID",
	"title":    "Title",
	"status":   "Status",
	"priority": "Priority",
	"assigned": "Assigned",
	"track":    "Track",
	"effort":   "Effort",
	"due":      "Due",
	"stale":    "Stale",
	"blocked":  "Blocked",
}

// taskListDefaultColumns is the built-in default column order for
// `tlc task list` (matches the columns visible today).
var taskListDefaultColumns = []string{
	"id", "title", "status", "assigned", "due", "stale", "blocked",
}

// trackColumnHeaders maps a lowercase column key to the table:"" header
// string used by the trackTableRow / trackTableRowWithProject structs.
var trackColumnHeaders = map[string]string{
	"id":       "ID",
	"project":  "Project",
	"title":    "Title",
	"type":     "Type",
	"status":   "Status",
	"state":    "State",
	"progress": "Progress",
	"assignee": "Assignee",
}

// trackListDefaultColumns is the built-in default column order for
// `tlc track list` (matches the columns visible today).
// "project" is in the registry but NOT here — it is injected only when
// --all-projects is set.
var trackListDefaultColumns = []string{
	"id", "title", "type", "status", "state", "progress", "assignee",
}

// resolveColumnHeaders maps a lowercase-normalized key list to table
// header strings via registry, case-insensitively. Unknown keys are
// skipped and returned in `unknown` (caller decides how to warn).
func resolveColumnHeaders(keys []string, registry map[string]string) (headers []string, unknown []string) {
	for _, k := range keys {
		lk := strings.ToLower(strings.TrimSpace(k))
		if h, ok := registry[lk]; ok {
			headers = append(headers, h)
		} else {
			unknown = append(unknown, k)
		}
	}
	return headers, unknown
}
