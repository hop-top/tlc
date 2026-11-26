package cli

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"hop.top/kit/go/core/util"
	"hop.top/kit/go/runtime/domain"
	"hop.top/tlc/internal/core"
)

// TrackFieldChanges is the set of track fields `track update` and
// PATCH /tracks/{id} both write. A nil pointer means "leave the field
// alone", so an omitted field never clobbers stored data. It is the one
// place the two entry points share parsing, the clear sentinels and meta
// validation, so they cannot drift.
type TrackFieldChanges struct {
	Status *string
	// AssignedTo "", "-" or "null" clears; a leading "@" is stripped.
	AssignedTo *string
	// Due "" or "-" clears; anything else goes through util.ParseUntil.
	Due *string
	// Meta is a JSON Merge Patch over the track's user meta and UnsetMeta
	// removes top-level keys; see core.MergeTrackMeta.
	Meta      map[string]any
	UnsetMeta []string

	// dueLabel names the due input in errors ("--due" for the CLI flag);
	// empty reads "due", the REST field.
	dueLabel string
}

// Empty reports whether the changes write nothing.
func (c TrackFieldChanges) Empty() bool {
	return c.Status == nil && c.AssignedTo == nil && c.Due == nil &&
		len(c.Meta) == 0 && len(c.UnsetMeta) == 0
}

// parseTrackDue parses a due value; a nil time with a nil error clears.
func parseTrackDue(raw, label string) (*time.Time, error) {
	if raw == "" || raw == "-" {
		return nil, nil //nolint:nilnil // nil time means "clear the due date"
	}
	dt, err := util.ParseUntil(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: %w", label, raw, err)
	}
	return &dt, nil
}

// mutation validates everything that can refuse the changes and returns
// the function that applies them to a track. Validation runs here, before
// any write, so a refused request changes nothing; the status transition
// itself is checked by TrackService.UpdateTrack against the stored track.
func (c TrackFieldChanges) mutation() (func(*core.Track) error, error) {
	var due *time.Time
	if c.Due != nil {
		label := c.dueLabel
		if label == "" {
			label = "due"
		}
		d, err := parseTrackDue(*c.Due, label)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", domain.ErrValidation, err)
		}
		due = d
	}
	if err := core.ValidateTrackMetaWrite(c.Meta, c.UnsetMeta); err != nil {
		return nil, metaWriteError(err)
	}
	return func(t *core.Track) error {
		if c.Status != nil {
			t.Status = core.TrackStatus(*c.Status)
		}
		if c.AssignedTo != nil {
			switch a := *c.AssignedTo; a {
			case "", "-", "null":
				t.AssignedTo = nil
			default:
				a = normalizeAssignee(a)
				t.AssignedTo = &a
			}
		}
		if c.Due != nil {
			t.DueAt = due
		}
		meta, err := core.MergeTrackMeta(t.Meta, c.Meta, c.UnsetMeta)
		if err != nil {
			return metaWriteError(err)
		}
		t.Meta = meta
		return nil
	}, nil
}

// metaSummary describes the meta write for a dry-run preview, "" when it
// writes none: "set a, b; unset c".
func (c TrackFieldChanges) metaSummary() string {
	var parts []string
	if len(c.Meta) > 0 {
		keys := make([]string, 0, len(c.Meta))
		for k := range c.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts = append(parts, "set "+strings.Join(keys, ", "))
	}
	if len(c.UnsetMeta) > 0 {
		parts = append(parts, "unset "+strings.Join(c.UnsetMeta, ", "))
	}
	return strings.Join(parts, "; ")
}

// metaFilterError classes a refused meta filter as a validation error
// (HTTP 422, CLI usage failure) while keeping core's sentinel matchable.
func metaFilterError(err error) error {
	return fmt.Errorf("%w: %w", domain.ErrValidation, err)
}

// metaFiltersFromQuery collects the meta.<key>=<value> parameters of a
// list request. Every key is validated. Parameters are ANDed, so one key
// given two different values can match nothing and is refused rather
// than answered with an empty list.
func metaFiltersFromQuery(q url.Values) (map[string]string, error) {
	var out map[string]string
	for param, vals := range q {
		key, ok := strings.CutPrefix(param, "meta.")
		if !ok {
			continue
		}
		if err := core.ValidateMetaFilterKey(key); err != nil {
			return nil, metaFilterError(err)
		}
		for _, v := range vals {
			if out == nil {
				out = map[string]string{}
			}
			if prev, dup := out[key]; dup && prev != v {
				return nil, fmt.Errorf("%w: meta.%s given with conflicting values %q and %q",
					domain.ErrValidation, key, prev, v)
			}
			out[key] = v
		}
	}
	return out, nil
}

// writeTrackUpdateError maps a TrackService.UpdateTrack or
// TrackFieldChanges error to its HTTP status: an illegal status
// transition is a 409 carrying the message the CLI prints, a refused
// write is 422, anything else (a storage failure) 500.
func writeTrackUpdateError(w http.ResponseWriter, err error) {
	var tr core.ErrInvalidTrackTransition
	switch {
	case errors.As(err, &tr):
		writeAPIErrorf(w, http.StatusConflict, "invalid_transition", "%v", err)
	case errors.Is(err, domain.ErrValidation):
		writeAPIErrorf(w, http.StatusUnprocessableEntity, "validation_error", "%v", err)
	default:
		writeAPIErrorf(w, http.StatusInternalServerError, "internal_error", "%v", err)
	}
}
