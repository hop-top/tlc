package core

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrInvalidMetaFilterKey marks a meta filter whose key is not a plain
// identifier.
var ErrInvalidMetaFilterKey = errors.New("invalid meta filter key")

// metaFilterKeyRe is the whitelist for a key used in a meta filter: a
// letter, digit or underscore, then letters, digits, underscores or
// hyphens, at most 64 characters. Dots are excluded (no nested paths),
// so a key is always exactly one top-level member of the meta object.
// The storage layer still binds the key as a parameter; this whitelist
// is the second wall.
var metaFilterKeyRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,63}$`)

// ValidateMetaFilterKey reports whether key may appear in a meta filter.
func ValidateMetaFilterKey(key string) error {
	if !metaFilterKeyRe.MatchString(key) {
		return fmt.Errorf(
			"%w %q: use letters, digits, '_' or '-' (max 64, no leading '-')",
			ErrInvalidMetaFilterKey, key,
		)
	}
	return nil
}

// ValidateMetaFilters validates every key of a meta filter set.
func ValidateMetaFilters(filters map[string]string) error {
	for k := range filters {
		if err := ValidateMetaFilterKey(k); err != nil {
			return err
		}
	}
	return nil
}
