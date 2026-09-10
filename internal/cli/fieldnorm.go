package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"

	"hop.top/tlc/internal/core"
)

// builtinStatusAliases maps lowercase alias/variant → canonical uppercase
// value for the BUILT-IN statuses. These are hand-written conveniences
// ("wip", "complete") that no rule could derive, so they stay declared.
//
// They apply only to the status they name: an entry survives into the
// effective alias table only when its target is in the effective
// vocabulary. A config that keeps the built-in four therefore keeps every
// alias exactly as before, while a config that drops DONE does not leave
// "complete" resolving to a status the user no longer has.
var builtinStatusAliases = map[string]string{
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

// statusCanonical returns the effective status vocabulary: the user's
// `task.statuses` when declared, else the built-in set.
//
// A function, not the package-level var it replaced. The var was
// initialised at package-init time — long before any config file is
// read — so it could only ever hold the built-ins. Resolution has to
// happen per call, once config is loaded.
func statusCanonical() []string {
	return core.ConfiguredTaskStatusStrings()
}

// statusAliases returns the effective alias table for the current
// vocabulary: the hand-written built-in aliases whose target is still a
// declared status, plus trivial spelling variants derived for EVERY
// declared status.
//
// Derivation exists so a user-declared status is not a second-class
// citizen: IN_REVIEW gets "in_review", "in-review" and "inreview" for the
// same reason IN_PROGRESS has them, without the user restating the
// obvious in config. Only mechanical variants are derived — lowercase,
// and underscore/hyphen/removed separators. Nothing semantic like
// "review" is invented, because guessing a meaning the user did not
// declare is how a typo silently resolves to the wrong status.
//
// Built-in aliases are applied last and win, so an unchanged vocabulary
// resolves precisely as it did before this became config-driven.
func statusAliases() map[string]string {
	canon := statusCanonical()
	aliases := make(map[string]string, len(canon)*4+len(builtinStatusAliases))

	// Derived variants for every declared status.
	for _, name := range canon {
		lower := strings.ToLower(name)
		aliases[lower] = name
		aliases[strings.ReplaceAll(lower, "_", "-")] = name
		aliases[strings.ReplaceAll(lower, "-", "_")] = name
		aliases[strings.NewReplacer("_", "", "-", "").Replace(lower)] = name
	}

	// Hand-written built-ins win, but only for statuses still declared.
	declared := make(map[string]bool, len(canon))
	for _, name := range canon {
		declared[name] = true
	}
	for alias, target := range builtinStatusAliases {
		if declared[target] {
			aliases[alias] = target
		}
	}
	return aliases
}

// priorityAliases maps lowercase alias/variant → canonical uppercase value.
var priorityAliases = map[string]string{
	// canonical (lowercased)
	"p0": "P0",
	"p1": "P1",
	"p2": "P2",
	"p3": "P3",
	// numeric shorthands
	"0": "P0",
	"1": "P1",
	"2": "P2",
	"3": "P3",
	// descriptive aliases
	"critical": "P0",
	"high":     "P1",
	"medium":   "P2",
	"med":      "P2",
	"low":      "P3",
}

// priorityCanonical is the ordered list for fuzzy matching, read from the
// domain canon (see statusCanonical).
var priorityCanonical = core.PriorityStrings()

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

// effortCanonical is the ordered list for fuzzy matching, read from the
// domain canon (see statusCanonical).
var effortCanonical = core.EffortStrings()

// unknownStatusError renders the rejection for a status the normaliser
// could not resolve, naming the legal set. Every caller — task list, task
// create, task update, the HTTP surface — formats through this one helper,
// so the message and the flag-enum registration cannot drift apart.
func unknownStatusError(input string) error {
	return fmt.Errorf("unknown status %q; valid values: %s", input, enumList(statusCanonical()))
}

// unknownPriorityError is unknownStatusError for --priority.
func unknownPriorityError(input string) error {
	return fmt.Errorf("unknown priority %q; valid values: %s", input, enumList(priorityCanonical))
}

// invalidPriorityError is the set-on-write rejection for --priority. The
// wording differs from unknownPriorityError ("invalid" + "must be one of"
// rather than "unknown" + "valid values") because the filter path and the
// write path have always phrased it differently; both now read the same
// canonical set, so only the prose differs.
func invalidPriorityError(input string) error {
	return fmt.Errorf("invalid priority %q: must be one of %s", input, enumList(priorityCanonical))
}

// unknownEffortError is unknownStatusError for --effort.
func unknownEffortError(input string) error {
	return fmt.Errorf("invalid effort %q: must be one of %s", input, enumList(effortCanonical))
}

// enumList renders a canonical set the way the error messages and the
// flag-enum help suffix both spell it: comma-separated, declaration order.
func enumList(values []string) string {
	return strings.Join(values, ", ")
}

// NormalizeStatus resolves input to a canonical status string.
// Resolution order: exact (case-insensitive) → alias → fuzzy.
// Returns ("", false) when no match found.
func NormalizeStatus(input string) (string, bool) {
	return normalizeEnum(input, statusAliases(), statusCanonical())
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
	// Sort keys so the fuzzy fallback is deterministic when multiple
	// aliases tie on score (fuzzy.Find preserves input order for ties,
	// and Go map iteration order is randomised per range).
	aliasKeys := make([]string, 0, len(aliases))
	for k := range aliases {
		aliasKeys = append(aliasKeys, k)
	}
	sort.Strings(aliasKeys)
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
