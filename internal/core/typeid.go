package core

import (
	"regexp"

	"go.jetify.com/typeid"
)

const (
	taskIDPrefix  = "task"
	trackIDPrefix = "track"
)

var (
	taskTypeIDPattern  = regexp.MustCompile(`^task_[0-9a-z]{26}$`)
	trackTypeIDPattern = regexp.MustCompile(`^track_[0-9a-z]{26}$`)
)

// NewTaskID returns a fresh TypeID-shaped task identifier
// (e.g. "task_01h455vb4pex5vsknk084sn02q"). Backed by uuidv7,
// so lexicographic order matches creation order.
func NewTaskID() string {
	id, err := typeid.WithPrefix(taskIDPrefix)
	if err != nil {
		// jetify's WithPrefix only errors on invalid prefixes; ours is constant
		// and known-valid. Surface as panic so the bug is loud if reached.
		panic("typeid: NewTaskID: " + err.Error())
	}
	return id.String()
}

// NewTrackID returns a fresh TypeID-shaped track identifier
// (e.g. "track_01h455vbqkfsn02nk084ksn02q").
func NewTrackID() string {
	id, err := typeid.WithPrefix(trackIDPrefix)
	if err != nil {
		panic("typeid: NewTrackID: " + err.Error())
	}
	return id.String()
}

// IsTaskID reports whether s is a syntactically valid task TypeID.
func IsTaskID(s string) bool { return taskTypeIDPattern.MatchString(s) }

// IsTrackID reports whether s is a syntactically valid track TypeID.
func IsTrackID(s string) bool { return trackTypeIDPattern.MatchString(s) }

// IsInternalTaskRef reports whether ref is an auto-generated internal task
// reference that adds no information beyond the task's display alias.
//
// Returns true for:
//   - empty refs
//   - the local default forms: tlc:///<id>, tlc://<id>, task://<id>
//   - the absolute project-scoped form `tlc://<projectID>/<task.ID>`
//     derived from the task's own ProjectID or the locally-detected project
//
// External user-supplied refs (e.g. github:issues/42, https://...,
// docs/foo.md) return false. Used by render-layer code to suppress noisy
// `Reference:` lines in default human output. Single source of truth —
// shared by internal/cli and internal/tui.
func IsInternalTaskRef(ref string, t *Task) bool {
	if ref == "" {
		return true
	}
	if t == nil || t.ID == "" {
		return false
	}
	switch ref {
	case "tlc:///" + t.ID, "tlc://" + t.ID, "task://" + t.ID:
		return true
	}
	if t.ProjectID != nil && *t.ProjectID != "" {
		if ref == "tlc://"+*t.ProjectID+"/"+t.ID {
			return true
		}
	}
	if proj := DetectProject(); proj != nil && proj.ProjectID != "" {
		if ref == "tlc://"+proj.ProjectID+"/"+t.ID {
			return true
		}
	}
	return false
}
