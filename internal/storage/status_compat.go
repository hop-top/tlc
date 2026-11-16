package storage

// Status-name compatibility. Rows written before the default vocabulary
// went lowercase hold SCREAMING_CASE labels (TODO, SKIPPED, ...). Those
// rows are never rewritten here; they read as their lowercase names when
// the configured vocabulary is the default one, and writes must name a
// declared status exactly.

import (
	"fmt"
	"strings"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// legacyStatusAliases maps a lowercased stored label to the default name
// it now reads as, where it is not simply the same word.
var legacyStatusAliases = map[string]string{
	"skipped": "cancelled",
}

// usesDefaultVocabulary reports whether the effective vocabulary is the
// built-in set. A custom vocabulary, even one reusing some of the same
// words, is left exactly as stored.
func usesDefaultVocabulary() bool {
	return config.UsesDefaultStatuses(core.ConfiguredTaskStatusDefinitions())
}

// canonicalDefaultStatus maps a stored or typed label to its default
// name, case-insensitively. ok is false when the label is not part of
// the default vocabulary or its legacy aliases.
func canonicalDefaultStatus(s string) (string, bool) {
	l := strings.ToLower(s)
	if a, found := legacyStatusAliases[l]; found {
		return a, true
	}
	for _, n := range core.TaskStatusStrings() {
		if l == n {
			return n, true
		}
	}
	return "", false
}

// normalizeStatus is the read-path hook: under the default vocabulary a
// legacy label maps to its lowercase name; anything else is unchanged.
func normalizeStatus(s core.TaskStatus) core.TaskStatus {
	if s == "" || !usesDefaultVocabulary() {
		return s
	}
	if c, ok := canonicalDefaultStatus(string(s)); ok {
		return core.TaskStatus(c)
	}
	return s
}

// statusSQLNames returns the stored spellings a predicate on name must
// match. Under the default vocabulary that is the name plus any legacy
// alias (compared NOCASE, see statusColumn); otherwise just name.
func statusSQLNames(name string) []string {
	if !usesDefaultVocabulary() {
		return []string{name}
	}
	c, ok := canonicalDefaultStatus(name)
	if !ok {
		return []string{name}
	}
	out := []string{c}
	for alias, target := range legacyStatusAliases {
		if target == c {
			out = append(out, alias)
		}
	}
	return out
}

// statusColumn returns the status column expression for predicates built
// from statusSQLNames: case-insensitive under the default vocabulary.
func statusColumn(col string) string {
	if usesDefaultVocabulary() {
		return col + " COLLATE NOCASE"
	}
	return col
}

// expandStatusNames flattens statusSQLNames over names, dropping repeats.
func expandStatusNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	var out []string
	for _, n := range names {
		for _, v := range statusSQLNames(n) {
			if _, dup := seen[v]; dup {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// validateStatusWrite rejects a status that is not a declared name. The
// match is exact: no case folding, no legacy aliases on write. Empty is
// left to callers that treat it as "unset".
func validateStatusWrite(s core.TaskStatus) error {
	if s == "" {
		return nil
	}
	for _, v := range core.ConfiguredTaskStatusStrings() {
		if string(s) == v {
			return nil
		}
	}
	return fmt.Errorf("invalid status %q: not a declared status (valid: %s)",
		s, strings.Join(core.ConfiguredTaskStatusStrings(), ", "))
}

// statusFilterClause renders a status filter under the default
// vocabulary. ok is false when the filter needs no rewriting.
func statusFilterClause(op core.Operator, value any) (clause string, args []any, ok bool) {
	if !usesDefaultVocabulary() || value == nil {
		return "", nil, false
	}
	var raw []string
	switch op { //nolint:exhaustive // only equality-shaped operators are rewritten
	case core.OpEq, core.OpEqual, core.OpNotEq:
		raw = []string{fmt.Sprintf("%v", value)}
	case core.OpIn:
		for _, v := range strings.Split(fmt.Sprintf("%v", value), ",") {
			raw = append(raw, strings.TrimSpace(v))
		}
	default:
		return "", nil, false
	}
	names := expandStatusNames(raw)
	ph := make([]string, len(names))
	for i, n := range names {
		ph[i] = "?"
		args = append(args, n)
	}
	kw := "IN"
	if op == core.OpNotEq {
		kw = "NOT IN"
	}
	return fmt.Sprintf("%s %s (%s)", statusColumn("status"), kw, strings.Join(ph, ",")), args, true
}

// placeholderList renders n comma-separated SQL placeholders.
func placeholderList(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// claimArgs orders the ClaimTask bind args: SET values, the identity
// predicate, the accepted from-statuses, then the actor.
func claimArgs(to, actor, stamp, id, projectID string, from []string) []any {
	out := []any{to, actor, stamp, stamp, id, projectID}
	for _, f := range from {
		out = append(out, f)
	}
	return append(out, actor)
}
