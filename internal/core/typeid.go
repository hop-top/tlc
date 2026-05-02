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
