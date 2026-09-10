package core

// The tag policy: whether a tag has to be in a vocabulary before it may
// be written to a task.
//
// Tags were free-form end to end — `--add-tag` dropped its argument into
// a map and assigned it, the TLS parser appended any `#token` verbatim,
// and the HTTP route took arbitrary strings. That is the right default
// and it stays the default (`open`). What it could not express is a
// project that has DECIDED on a vocabulary, for which every typo'd tag
// is a silent new category that only shows up much later as a filter
// that returns nothing.
//
// The gate lives here, in core, rather than in internal/cli, for two
// reasons. It has to be reachable from every write path, and those are
// not all in the CLI: the plan importer and the inbox processor build
// tasks in core and internal/inbox with no CLI in the picture. And core
// is where the other configured vocabularies already resolve, through
// the same non-caching accessor, so the composition below reads the
// user's effective config rather than a snapshot of it.

import (
	"fmt"
	"sort"
	"strings"

	"hop.top/tlc/internal/config"
)

// dimensionAxes names the four `dimension:value` axes, in the order
// internal/labels emits them.
//
// One declaration of WHICH axes exist, separate from what each one
// CONTAINS. The contents are already derived — three of the four from
// the user's config — but the set of axis names was retyped at every
// consumer, and the TLS parser is where that cost showed up: its
// prefix list named `effort:` and `prio:` and had never learned
// `type:`, `status:` or `priority:`, so a `type:feat` token written by
// tlc's own `label init` vocabulary landed in the task title instead of
// on the task. A consumer that derives the set from here cannot go deaf
// to a fifth axis the way that list did.
var dimensionAxes = []string{"type", "priority", "effort", "status"}

// DimensionAxes returns the axis names, without the trailing colon.
// The returned slice is a copy.
func DimensionAxes() []string {
	return append([]string(nil), dimensionAxes...)
}

// wildcardSuffix is the only wildcard shape `task.tags.allowed` accepts:
// a dimension prefix followed by `:*`.
//
// Deliberately not a general glob and not a regex. `domain:*` widens one
// namespace and cannot widen past its colon, so a config that opens the
// domain axis has still closed everything else — the guarantee survives
// in a readable form. A bare `*` or a pattern language would let one
// entry silently reinstate `open` under a config that says `closed`,
// which is the one outcome a policy must not permit.
const wildcardSuffix = ":*"

// commitTypeTags is the `type:*` axis: one tag per Conventional Commits
// type, plus `type:breaking` for the `!` marker.
//
// Duplicated from internal/labels rather than imported because the
// dependency runs the other way — labels imports core — and inverting it
// to share eleven string literals would be the wrong trade. The axis
// mirrors a published spec, not a tlc config surface, so it does not
// drift with the user's config the way status, priority and effort do;
// labels/templates.go carries the same list with its colors and is
// where the spec is read against.
var commitTypeTags = []string{
	"type:feat",
	"type:fix",
	"type:refactor",
	"type:docs",
	"type:test",
	"type:chore",
	"type:perf",
	"type:build",
	"type:ci",
	"type:style",
	"type:breaking",
}

// builtinPriorityTagAlias is the rank-indexed alias the built-in
// priorities carry on the `priority:*` axis. Mirrors
// labels.builtinPriorityLabelValue and every sync plugin's
// priorityToLabel; see the call site for why membership admits both
// spellings where emission picks one.
var builtinPriorityTagAlias = map[string]string{
	"P0": "critical",
	"P1": "high",
	"P2": "medium",
	"P3": "low",
}

// TagVocabulary is the set of tags a closed policy admits, resolved from
// the effective config.
//
// The generated axes are members by CONSTRUCTION, not by the user
// restating them. `type:*`, `status:*`, `priority:*` and `effort:*` are
// already vocabularies this config declares — three of the four are
// literally generated from it by internal/labels — so requiring the user
// to list `priority:p0` in `allowed` before they could tag with it would
// be asking them to say the same thing twice, in two places that would
// then be free to disagree the first time they renamed a priority.
// Composition is what makes the closed policy about the project's OWN
// tags, which is the only part config can actually know.
type TagVocabulary struct {
	// literals are the exact tags admitted, lower-cased for matching.
	literals map[string]struct{}
	// prefixes are the `dimension:` strings a `dimension:*` entry opened.
	prefixes []string
	// display is the vocabulary as the user should see it in an error:
	// generated axes first, then the declared entries in declared order.
	display []string
}

// TagPolicyFor returns the effective tag policy and the vocabulary it
// enforces.
//
// Reads the non-caching accessor, like every other Configured* here: the
// memoising DefaultWorkflow* singleton freezes config at first touch, so
// a caller reached before argv is parsed would pin the process to the
// pre-override config and discard every `-c key=value` for every command
// in it.
func TagPolicyFor() (config.TagPolicy, TagVocabulary) {
	cfg := resolveTaskConfig()
	if cfg == nil {
		return config.TagPolicyOpen, TagVocabulary{}
	}
	policy := cfg.Tags.Effective()
	if policy != config.TagPolicyClosed {
		// Building the vocabulary under an open policy would be work
		// nothing reads, on the hot path of every write.
		return policy, TagVocabulary{}
	}
	return policy, buildTagVocabulary(cfg)
}

// buildTagVocabulary composes the generated axes with the declared list.
func buildTagVocabulary(cfg *config.TaskConfig) TagVocabulary {
	v := TagVocabulary{literals: make(map[string]struct{}, 32)}

	generated := make([]string, 0, 32)
	generated = append(generated, commitTypeTags...)
	for _, p := range ConfiguredPriorityStrings() {
		generated = append(generated, "priority:"+tagAxisValue(p))
		// The built-in priorities travel on the wire under a rank-indexed
		// ALIAS, not under their own names: every sync plugin's
		// priorityToLabel maps P0..P3 to priority:critical..priority:low,
		// and internal/labels seeds the same. A vocabulary admitting only
		// `priority:p0` would reject the tag tlc's own `label init` and
		// `sync pull` produce, so both spellings are admitted.
		//
		// Applied per-name rather than only when the whole vocabulary is
		// the built-in one — which is what labels.priorityAxis does. The
		// two answers differ for a CUSTOM vocabulary that reuses the name
		// P0: labels declines the alias there, correctly, because it must
		// pick ONE label to emit. This is a membership test, not an
		// emission choice, and the cost of the two answers is asymmetric —
		// admitting a tag the user did not quite mean is a smaller harm
		// than rejecting one their own sync plugin writes.
		if alias, ok := builtinPriorityTagAlias[p]; ok {
			generated = append(generated, "priority:"+alias)
		}
	}
	for _, e := range ConfiguredEffortStrings() {
		generated = append(generated, "effort:"+tagAxisValue(e))
	}
	for _, s := range ConfiguredTaskStatusStrings() {
		generated = append(generated, "status:"+tagAxisValue(s))
	}
	// `status:blocked` is not a status — tlc models blocked as
	// Task.BlockedReason — but it IS on the wire: all four sync plugins
	// push and pull it, so a closed policy that rejected it would reject
	// a tag tlc's own sync writes.
	generated = append(generated, "status:blocked")

	for _, g := range generated {
		v.add(g)
	}

	for _, a := range cfg.Tags.Allowed {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		v.add(a)
	}
	return v
}

// add files one vocabulary entry as either a prefix opener or a literal,
// and records it for the error message.
func (v *TagVocabulary) add(entry string) {
	v.display = append(v.display, entry)
	if strings.HasSuffix(entry, wildcardSuffix) {
		v.prefixes = append(v.prefixes, strings.ToLower(
			strings.TrimSuffix(entry, wildcardSuffix)+":",
		))
		return
	}
	v.literals[strings.ToLower(entry)] = struct{}{}
}

// tagAxisValue renders a vocabulary NAME as the value half of a tag,
// matching the convention internal/labels applies to the same names:
// config names are shouty and underscored (IN_PROGRESS), the axis value
// is lowercase and hyphenated (status:in-progress).
//
// Kept in step with labels.labelValue by intent, not by import: a tag a
// user writes and a label a forge carries have to spell the same axis
// value the same way, or `sync push` sends a label the closed policy
// would have rejected locally.
func tagAxisValue(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// Admits reports whether the vocabulary accepts tag.
//
// Matching is case-insensitive, matching the axis convention above and
// the enum normalisers: a user who types `Type:Feat` meant the tag that
// exists, and rejecting it would be a distinction the rest of the tool
// does not draw.
func (v TagVocabulary) Admits(tag string) bool {
	lowered := strings.ToLower(strings.TrimSpace(tag))
	if lowered == "" {
		return true
	}
	if _, ok := v.literals[lowered]; ok {
		return true
	}
	for _, p := range v.prefixes {
		if strings.HasPrefix(lowered, p) && len(lowered) > len(p) {
			return true
		}
	}
	return false
}

// Display renders the vocabulary for an error message: sorted, so the
// same config always produces the same message, and de-duplicated so a
// user who did restate a generated axis does not see it twice.
func (v TagVocabulary) Display() []string {
	seen := make(map[string]struct{}, len(v.display))
	out := make([]string, 0, len(v.display))
	for _, d := range v.display {
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// ValidateTags is the gate every write path calls.
//
// A no-op under the default open policy, which is what keeps this
// callable from the hot path of every task write without changing
// behavior for a project that has configured nothing.
//
// The error names the offending tag AND the whole allowed set. Naming
// only the tag would be unusable: the vocabulary lives in a config file
// the user may not have written and cannot see from the error, so a bare
// "tag not allowed" leaves them with no way to find out what is. Every
// rejected tag is listed, not just the first, so one run tells them
// everything they have to fix.
func ValidateTags(tags []string) error {
	policy, vocab := TagPolicyFor()
	if policy != config.TagPolicyClosed {
		return nil
	}
	var rejected []string
	for _, t := range tags {
		if !vocab.Admits(t) {
			rejected = append(rejected, t)
		}
	}
	if len(rejected) == 0 {
		return nil
	}
	noun := "tag"
	if len(rejected) > 1 {
		noun = "tags"
	}
	return fmt.Errorf(
		"%s %s not allowed under tag policy %q: allowed tags are %s",
		noun, quoteList(rejected), config.TagPolicyClosed,
		strings.Join(vocab.Display(), ", "),
	)
}

// quoteList renders the rejected tags quoted and comma-separated, so a
// tag containing a space or a comma is still legible in the message.
func quoteList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf("%q", v))
	}
	return strings.Join(quoted, ", ")
}

// SuggestedTags returns the tags a chooser should OFFER, in a stable
// order, for any policy.
//
// TagPolicyFor deliberately returns an empty vocabulary under `open`,
// because nothing needs to enforce membership there and composing it
// would be work on the hot path of every write. A chooser is the one
// caller that wants the set anyway: under `open` there is no wrong
// answer, but the composed axes are the tags tlc's own `label init` and
// `sync` emit, so suggesting them is what keeps a hand-typed `type:fix`
// spelled the same as the one a forge round-trips.
//
// Wildcard OPENERS are excluded. A `domain:*` entry widens a namespace;
// it is not itself a taggable value, and offering the literal string
// would seed a tag the very policy that declared it rejects.
func SuggestedTags() []string {
	vocab := buildTagVocabulary(resolveTaskConfig())
	all := vocab.Display()
	out := make([]string, 0, len(all))
	for _, t := range all {
		if strings.HasSuffix(t, wildcardSuffix) {
			continue
		}
		out = append(out, t)
	}
	return out
}
