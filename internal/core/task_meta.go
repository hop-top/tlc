package core

import (
	"errors"
	"fmt"
	"strings"
)

// ErrReservedMetaKey marks a user write (set or unset) that names a meta
// key tlc manages itself.
var ErrReservedMetaKey = errors.New("reserved meta key")

// MetaBlockedFrom records the status a task was in before `task block`,
// so `task unblock` can return it there.
const MetaBlockedFrom = "blocked_from"

// reservedMetaKeys are the task-meta keys tlc itself writes: dependency
// edges and their plan/recipe bookkeeping, eva annotations, priority
// provenance, sync state, recipe provenance, and the foreign-UID and
// extension carriers the vtodo codec round-trips. A user write to one of
// them would corrupt state the engine derives from, so they only change
// through their own fields and commands.
var reservedMetaKeys = map[string]struct{}{
	"blocked_by":        {},
	metaKeyPlanOwned:    {},
	metaKeyRecipeOwned:  {},
	metaKeyUnresolved:   {},
	metaKeyCrossProject: {},
	"eva":               {},
	MetaPrioritySource:  {},
	MetaPriorityRule:    {},
	MetaLastSyncHash:    {},
	MetaBlockedFrom:     {},
	"needs_push":        {},
	"external_uid":      {},
	"x_tlc":             {},
	metaRecipe:          {},
	metaRecipeVersion:   {},
	metaRecipeHash:      {},
	metaSubject:         {},
}

// reservedMetaPrefixes reserve whole families: origin_* is the sync
// layer's link to the foreign system (origin_system, origin_id,
// origin_key, origin_url, ...).
var reservedMetaPrefixes = []string{"origin_"}

// IsReservedMetaKey reports whether key is one tlc manages itself.
func IsReservedMetaKey(key string) bool {
	if _, ok := reservedMetaKeys[key]; ok {
		return true
	}
	for _, p := range reservedMetaPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// ValidateUserMetaWrite checks the top-level keys of a user meta write:
// none empty, none reserved, none both set and unset. It is the single
// gate every entry point (CLI create/update, REST POST/PATCH) shares, run
// before anything is applied so a rejected write changes nothing.
func ValidateUserMetaWrite(set map[string]any, unset []string) error {
	for k := range set {
		if err := checkUserMetaKey(k); err != nil {
			return err
		}
	}
	for _, k := range unset {
		if err := checkUserMetaKey(k); err != nil {
			return err
		}
		if _, dup := set[k]; dup {
			return fmt.Errorf("meta key %q is both set and unset in one write", k)
		}
	}
	return nil
}

func checkUserMetaKey(k string) error {
	if k == "" {
		return errors.New("meta key must not be empty")
	}
	if IsReservedMetaKey(k) {
		return fmt.Errorf("%w %q: managed by tlc, cannot be set or unset by hand", ErrReservedMetaKey, k)
	}
	return nil
}

// MergeUserMeta applies a user meta write to meta and returns the result.
// It merges, never replaces: only the keys the write names change, and
// every other key, user or internal, survives.
//
// set is a JSON Merge Patch (RFC 7396) document over meta: a key sets
// its value, a nil value removes the key, and an object value merges
// recursively into an existing object. unset removes top-level keys;
// removing an absent key is a no-op. The two never overlap (see
// ValidateUserMetaWrite).
//
// On error meta is untouched and the returned map is nil. A nil meta
// stays nil when the write changes nothing.
func MergeUserMeta(meta, set map[string]any, unset []string) (map[string]any, error) {
	if err := ValidateUserMetaWrite(set, unset); err != nil {
		return nil, err
	}
	if len(set) == 0 && len(unset) == 0 {
		return meta, nil
	}
	if meta == nil {
		meta = make(map[string]any, len(set))
	}
	for k, v := range set {
		if v == nil {
			delete(meta, k)
			continue
		}
		meta[k] = mergePatchValue(meta[k], v)
	}
	for _, k := range unset {
		delete(meta, k)
	}
	return meta, nil
}

// mergePatchValue is RFC 7396's MergePatch for one member: an object
// patch merges into the target when that is an object too (else into an
// empty one), with nil members removing; any other patch replaces.
func mergePatchValue(target, patch any) any {
	p, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	t, ok := target.(map[string]any)
	if !ok {
		t = make(map[string]any, len(p))
	}
	for k, v := range p {
		if v == nil {
			delete(t, k)
			continue
		}
		t[k] = mergePatchValue(t[k], v)
	}
	return t
}
