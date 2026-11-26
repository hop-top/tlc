package core

// Track-meta keys tlc itself writes.
const (
	// MetaTrackPlans lists the plan files linked to a track
	// (`track update --add-plan`); plan reconciliation reads it back.
	MetaTrackPlans = "plans"
	// MetaDueDateOnly marks a track's (or task's) DueAt as a calendar
	// date rather than an instant, so a calendar export writes a date
	// again. Set when a date-only due is imported.
	MetaDueDateOnly = "due_date_only"
)

// trackReservedMeta are the track-meta keys tlc itself writes: the plan
// links, the foreign-UID and extension carriers the vtodo codec
// round-trips, and the all-day due marker it derives. A user write to
// one of them would corrupt state the engine reads back, so they only
// change through their own fields and commands.
var trackReservedMeta = newReservedMeta([]string{
	MetaTrackPlans,
	"external_uid",
	"x_tlc",
	MetaDueDateOnly,
})

// IsReservedTrackMetaKey reports whether key is a track-meta key tlc
// manages itself.
func IsReservedTrackMetaKey(key string) bool { return trackReservedMeta.isReserved(key) }

// ValidateTrackMetaWrite is ValidateUserMetaWrite for tracks: the gate
// every track entry point (CLI create/update, REST POST/PATCH) shares.
func ValidateTrackMetaWrite(set map[string]any, unset []string) error {
	return trackReservedMeta.validate(set, unset)
}

// MergeTrackMeta applies a user write to a track's meta with the same
// merge semantics as MergeUserMeta, against the track's reserved set.
func MergeTrackMeta(meta, set map[string]any, unset []string) (map[string]any, error) {
	return trackReservedMeta.merge(meta, set, unset)
}
