package core

// MetaBlockedFrom records the status a task was in before `task block`,
// so `task unblock` can return it there.
const MetaBlockedFrom = "blocked_from"

// taskReservedMeta are the task-meta keys tlc itself writes: dependency
// edges and their plan/recipe bookkeeping, eva annotations, priority
// provenance, sync state, recipe provenance, and the foreign-UID and
// extension carriers the vtodo codec round-trips. A user write to one of
// them would corrupt state the engine derives from, so they only change
// through their own fields and commands. origin_* reserves the sync
// layer's whole link to the foreign system (origin_system, origin_id,
// origin_key, origin_url, ...).
var taskReservedMeta = newReservedMeta([]string{
	"blocked_by",
	metaKeyPlanOwned,
	metaKeyRecipeOwned,
	metaKeyUnresolved,
	metaKeyCrossProject,
	"eva",
	MetaPrioritySource,
	MetaPriorityRule,
	MetaLastSyncHash,
	MetaBlockedFrom,
	"needs_push",
	"external_uid",
	"x_tlc",
	metaRecipe,
	metaRecipeVersion,
	metaRecipeHash,
	metaSubject,
}, "origin_")

// IsReservedMetaKey reports whether key is a task-meta key tlc manages
// itself.
func IsReservedMetaKey(key string) bool { return taskReservedMeta.isReserved(key) }

// ValidateUserMetaWrite checks the top-level keys of a user task-meta
// write: none empty, none reserved, none both set and unset. It is the
// single gate every task entry point (CLI create/update, REST
// POST/PATCH) shares, run before anything is applied so a rejected write
// changes nothing.
func ValidateUserMetaWrite(set map[string]any, unset []string) error {
	return taskReservedMeta.validate(set, unset)
}

// MergeUserMeta applies a user write to a task's meta; see
// reservedMeta.merge for the semantics (merge never replace, RFC 7396).
func MergeUserMeta(meta, set map[string]any, unset []string) (map[string]any, error) {
	return taskReservedMeta.merge(meta, set, unset)
}
