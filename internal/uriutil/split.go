// Package uriutil provides shared URI helpers that both
// internal/core and internal/uri need. It exists to break the
// circular dependency between those two packages.
package uriutil

import "strings"

// SplitProjectTask derives (projectID, taskID) from the Space
// and ID fields returned by hop.top/cite.Parse.
//
// The task ID is always the last slash-delimited segment of the
// combined "space/id" path. Everything before it is the project
// ID.
//
//	space="hop-top"  id="tlc/T-0001"  → ("hop-top/tlc", "T-0001")
//	space="tlc"      id="T-0001"      → ("tlc",          "T-0001")
//	space=""         id="T-0001"      → ("",              "T-0001")
func SplitProjectTask(space, id string) (projectID, taskID string) {
	combined := id
	if space != "" {
		combined = space + "/" + id
	}
	if idx := strings.LastIndex(combined, "/"); idx >= 0 {
		return combined[:idx], combined[idx+1:]
	}
	return "", combined
}
