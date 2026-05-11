package cli

import (
	"strings"

	"github.com/sahilm/fuzzy"
)

// statusAliases maps lowercase alias/variant → canonical uppercase value.
var statusAliases = map[string]string{
	// canonical (lowercased)
	"todo":        "TODO",
	"in_progress": "IN_PROGRESS",
	"done":        "DONE",
	"skipped":     "SKIPPED",
	// friendly aliases
	"complete":    "DONE",
	"completed":   "DONE",
	"finish":      "DONE",
	"finished":    "DONE",
	"open":        "TODO",
	"to-do":       "TODO",
	"wip":         "IN_PROGRESS",
	"inprogress":  "IN_PROGRESS",
	"in-progress": "IN_PROGRESS",
	"skip":        "SKIPPED",
}

// statusCanonical is the ordered list for fuzzy matching.
var statusCanonical = []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"}

// priorityAliases maps lowercase alias/variant → canonical uppercase value.
var priorityAliases = map[string]string{
	// canonical (lowercased)
	"p0": "P0",
	"p1": "P1",
	"p2": "P2",
	"p3": "P3",
	// numeric shorthands
	"0":        "P0",
	"1":        "P1",
	"2":        "P2",
	"3":        "P3",
	// descriptive aliases
	"critical": "P0",
	"high":     "P1",
	"medium":   "P2",
	"med":      "P2",
	"low":      "P3",
}

// priorityCanonical is the ordered list for fuzzy matching.
var priorityCanonical = []string{"P0", "P1", "P2", "P3"}

// effortAliases maps lowercase alias/variant → canonical uppercase value.
// Single-letter canonicals (S, M, L) make fuzzy matching unreliable for
// descriptive inputs like "small" or "medium", so each descriptive form
// is registered explicitly here to be resolved at step 1 before fuzzy
// runs.
var effortAliases = map[string]string{
	// canonical (lowercased)
	"xs": "XS",
	"s":  "S",
	"m":  "M",
	"l":  "L",
	"xl": "XL",
	// descriptive aliases
	"extra-small": "XS",
	"extrasmall":  "XS",
	"xsmall":      "XS",
	"tiny":        "XS",
	"small":       "S",
	"medium":      "M",
	"med":         "M",
	"large":       "L",
	"extra-large": "XL",
	"extralarge":  "XL",
	"xlarge":      "XL",
	"huge":        "XL",
}

// effortCanonical is the ordered list for fuzzy matching.
var effortCanonical = []string{"XS", "S", "M", "L", "XL"}

// NormalizeStatus resolves input to a canonical status string.
// Resolution order: exact (case-insensitive) → alias → fuzzy.
// Returns ("", false) when no match found.
func NormalizeStatus(input string) (string, bool) {
	return normalizeEnum(input, statusAliases, statusCanonical)
}

// NormalizePriority resolves input to a canonical priority string.
// Resolution order: exact (case-insensitive) → alias → fuzzy.
// Returns ("", false) when no match found.
func NormalizePriority(input string) (string, bool) {
	return normalizeEnum(input, priorityAliases, priorityCanonical)
}

// NormalizeEffort resolves input to a canonical effort string.
// Resolution order: exact (case-insensitive) → alias → fuzzy.
// Returns ("", false) when no match found.
func NormalizeEffort(input string) (string, bool) {
	return normalizeEnum(input, effortAliases, effortCanonical)
}

// normalizeEnum is the shared resolution logic for any enum field.
// Resolution order:
//  1. Exact (case-insensitive) via alias table — covers canonical forms and aliases.
//  2. Fuzzy match against lowercased canonical values.
//  3. Fuzzy match against alias keys, then resolve through alias table.
func normalizeEnum(input string, aliases map[string]string, canonical []string) (string, bool) {
	if input == "" {
		return "", false
	}

	key := strings.ToLower(strings.TrimSpace(input))

	// 1. Exact match via alias table (covers canonical + aliases).
	if v, ok := aliases[key]; ok {
		return v, true
	}

	// 2. Fuzzy match against the lowercased canonical set.
	lowerCanon := make([]string, len(canonical))
	for i, c := range canonical {
		lowerCanon[i] = strings.ToLower(c)
	}
	if results := fuzzy.Find(key, lowerCanon); len(results) > 0 {
		return canonical[results[0].Index], true
	}

	// 3. Fuzzy match against alias keys, then resolve via alias table.
	aliasKeys := make([]string, 0, len(aliases))
	for k := range aliases {
		aliasKeys = append(aliasKeys, k)
	}
	if results := fuzzy.Find(key, aliasKeys); len(results) > 0 {
		if v, ok := aliases[aliasKeys[results[0].Index]]; ok {
			return v, true
		}
	}

	return "", false
}

// unescapeMarkdown removes shell-style backslash escaping from markdown
// content. Agents and shells often escape backticks (\`) when passing
// descriptions via CLI flags, producing broken code fences on sync push.
func unescapeMarkdown(s string) string {
	return strings.ReplaceAll(s, "\\`", "`")
}
