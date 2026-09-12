package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	"slug":     "Slug",
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
	"id", "slug", "title", "type", "status", "state", "progress", "assignee",
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

// resolveEffectiveColumns resolves the table header list for a `list`
// command from the config ladder (<domain>.list.columns ->
// defaults.list.columns -> defaults.columns), then the --cols flag
// (viper key "cols"), lowercase-normalizing throughout. `transform`, if
// non-nil, mutates the key list after normalization (used for track's
// --all-projects project-column injection). When statusProvided, the
// "status" column is pruned. Returns nil when nothing customized the
// default set (so the caller keeps the styled TTY path); otherwise returns
// resolved table headers, warning on unknown keys to cmd stderr.
func resolveEffectiveColumns(
	cmd *cobra.Command,
	defaults []string,
	registry map[string]string,
	statusProvided bool,
	transform func(keys []string) (out []string, customized bool),
) []string {
	customized := false
	keys := defaults

	if key, ok := resolveFlagDefaultKey(cmd, "columns"); ok {
		if v := viper.GetStringSlice(key); len(v) > 0 {
			keys = v
			customized = true
		}
	}
	if c := viper.GetStringSlice("cols"); len(c) > 0 {
		keys = c
		customized = true
	}

	norm := make([]string, len(keys))
	for i, k := range keys {
		norm[i] = strings.ToLower(strings.TrimSpace(k))
	}
	keys = norm

	if transform != nil {
		var tCustomized bool
		keys, tCustomized = transform(keys)
		customized = customized || tCustomized
	}

	if statusProvided {
		keys = dropKey(keys, "status")
		customized = true
	}

	if !customized {
		return nil
	}

	headers, unknown := resolveColumnHeaders(keys, registry)
	for _, u := range unknown {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: unknown column %q (skipped)\n", u)
	}
	return headers
}

// dropKey returns a new slice with all occurrences of drop removed.
func dropKey(keys []string, drop string) []string {
	out := keys[:0:0]
	for _, k := range keys {
		if k != drop {
			out = append(out, k)
		}
	}
	return out
}

// containsKey reports whether k appears in keys.
func containsKey(keys []string, k string) bool {
	for _, x := range keys {
		if x == k {
			return true
		}
	}
	return false
}

// injectAfter returns a new slice with ins inserted immediately after the
// first occurrence of anchor. When anchor is absent, ins is prepended.
func injectAfter(keys []string, anchor, ins string) []string {
	out := make([]string, 0, len(keys)+1)
	inserted := false
	for _, k := range keys {
		out = append(out, k)
		if k == anchor && !inserted {
			out = append(out, ins)
			inserted = true
		}
	}
	if !inserted {
		out = append([]string{ins}, out...)
	}
	return out
}
