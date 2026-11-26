package core

import (
	"errors"
	"fmt"
	"strings"
)

// ErrReservedMetaKey marks a user write (set or unset) that names a meta
// key tlc manages itself.
var ErrReservedMetaKey = errors.New("reserved meta key")

// reservedMeta is the set of meta keys tlc manages itself for one entity
// kind. Tasks and tracks share the write mechanism (validate, merge) and
// each declares its own set, because the keys the engine writes into a
// task differ from the ones it writes into a track.
type reservedMeta struct {
	keys     map[string]struct{}
	prefixes []string
}

func newReservedMeta(keys []string, prefixes ...string) reservedMeta {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return reservedMeta{keys: m, prefixes: prefixes}
}

func (r reservedMeta) isReserved(key string) bool {
	if _, ok := r.keys[key]; ok {
		return true
	}
	for _, p := range r.prefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// validate checks the top-level keys of a user meta write: none empty,
// none reserved, none both set and unset. Entry points run it before
// anything is applied so a rejected write changes nothing.
func (r reservedMeta) validate(set map[string]any, unset []string) error {
	for k := range set {
		if err := r.checkKey(k); err != nil {
			return err
		}
	}
	for _, k := range unset {
		if err := r.checkKey(k); err != nil {
			return err
		}
		if _, dup := set[k]; dup {
			return fmt.Errorf("meta key %q is both set and unset in one write", k)
		}
	}
	return nil
}

func (r reservedMeta) checkKey(k string) error {
	if k == "" {
		return errors.New("meta key must not be empty")
	}
	if r.isReserved(k) {
		return fmt.Errorf("%w %q: managed by tlc, cannot be set or unset by hand", ErrReservedMetaKey, k)
	}
	return nil
}

// merge applies a user meta write to meta and returns the result. It
// merges, never replaces: only the keys the write names change, and
// every other key, user or internal, survives.
//
// set is a JSON Merge Patch (RFC 7396) document over meta: a key sets
// its value, a nil value removes the key, and an object value merges
// recursively into an existing object. unset removes top-level keys;
// removing an absent key is a no-op. The two never overlap (see
// validate).
//
// On error meta is untouched and the returned map is nil. A nil meta
// stays nil when the write changes nothing.
func (r reservedMeta) merge(meta, set map[string]any, unset []string) (map[string]any, error) {
	if err := r.validate(set, unset); err != nil {
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
