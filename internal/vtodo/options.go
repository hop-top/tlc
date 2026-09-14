// Package vtodo encodes and decodes tlc Task/Track/LogEntry models to and
// from RFC 5545 (iCalendar) + RFC 9253 (RELATED-TO RELTYPE) wire format.
//
// The encoder produces a VCALENDAR containing one VTODO per Task and per
// Track, with PARENT/CHILD/DEPENDS-ON relationships expressed via
// RELATED-TO. Optional VJOURNAL components carry log entries.
//
// The decoder reverses the mapping: VTODO/VJOURNAL → Task/LogEntry. UIDs
// minted by tlc (`<typeid>@<domain>`, the domain being the configured
// one) round-trip; foreign UIDs — including a typeid under a domain
// that is not ours — cause a fresh TypeID to be minted with the
// original UID stashed in Task.Meta["external_uid"].
package vtodo

import (
	"time"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// DefaultUIDDomain is the domain appended to Task TypeIDs in the iCalendar
// UID property when no override is supplied via WithUIDDomain.
const DefaultUIDDomain = "tlc.local"

// DefaultProductID is the iCalendar PRODID written to the VCALENDAR
// envelope when no override is supplied via WithProductID.
const DefaultProductID = "-//tlc//vtodo//EN"

// options holds the resolved configuration applied to a single
// BuildVCalendar / ParseVCalendar invocation.
type options struct {
	uidDomain   string
	productID   string
	includeLogs bool
	exportTime  time.Time
	statusDefs  []config.StatusDefinition
	// priorities is the priority vocabulary in rank order, most urgent
	// first. Never empty after resolve — defaultOptions seeds it from
	// the project config and WithPriorityVocabulary ignores empty input.
	priorities []config.PriorityDefinition
	// recipeRuns are the playthroughs to export as VEVENTs. Nil means
	// none were made available, not that there are none.
	recipeRuns []*core.RecipeRun
}

func defaultOptions() options {
	return options{
		uidDomain:   DefaultUIDDomain,
		productID:   DefaultProductID,
		includeLogs: false,
		priorities:  core.ConfiguredPriorityDefinitions(),
	}
}

// Option configures an encode or decode invocation.
type Option func(*options)

// WithUIDDomain overrides the domain appended to a Task TypeID in the
// VTODO UID property. The default is DefaultUIDDomain.
func WithUIDDomain(domain string) Option {
	return func(o *options) {
		if domain != "" {
			o.uidDomain = domain
		}
	}
}

// WithIncludeLogs toggles emission of VJOURNAL components for LogEntries.
// Off by default — most consumers only want todos, not the audit trail.
func WithIncludeLogs(include bool) Option {
	return func(o *options) {
		o.includeLogs = include
	}
}

// WithRecipeRuns supplies the recipe runs to export, each as a
// playthrough VEVENT (X-TLC-CONCEPT:playthrough). Runs are data rather
// than configuration, but BuildVCalendar's positional inputs are the
// three entity kinds every caller has; an option keeps that signature
// and lets a caller with no run store pass nothing. Nil entries are
// skipped.
func WithRecipeRuns(runs []*core.RecipeRun) Option {
	return func(o *options) {
		o.recipeRuns = runs
	}
}

// WithProductID overrides the VCALENDAR PRODID property. The default is
// DefaultProductID.
func WithProductID(id string) Option {
	return func(o *options) {
		if id != "" {
			o.productID = id
		}
	}
}

// WithExportTime pins the export clock. DTSTAMP is normally the
// entity's own last-modified instant (see BuildVCalendar); the export
// clock is written only for an entity that carries no timestamp at
// all, and defaults to wall-clock time at BuildVCalendar. Override it
// to keep encoder output deterministic for such entities (fixtures,
// golden tests).
//
// The zero time is ignored — it would clear DTSTAMP, which RFC 5545
// §3.6.2 requires on every VTODO.
func WithExportTime(t time.Time) Option {
	return func(o *options) {
		if !t.IsZero() {
			o.exportTime = t
		}
	}
}

// resolve applies a slice of Options on top of the defaults and returns
// the resolved configuration. The export clock is sampled once here so
// every timestamp-less entity in a single calendar shares one DTSTAMP.
func resolve(opts []Option) options {
	o := defaultOptions()
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	if len(o.priorities) == 0 {
		o.priorities = config.GetDefaultPriorities()
	}
	if o.exportTime.IsZero() {
		o.exportTime = time.Now().UTC()
	}
	return o
}

// WithPriorityVocabulary overrides the priority vocabulary used to map
// between tlc priority names and the RFC 5545 PRIORITY integer.
//
// Declaration order IS rank order, most urgent first — the same contract
// config.PriorityDefinition carries — so the slice must not be sorted.
// An empty slice is ignored and the default (the project's configured
// vocabulary) stands.
//
// The vocabulary is passed IN rather than read from internal/config here
// so the codec stays a pure function of its arguments: a caller
// round-tripping a foreign calendar can name the vocabulary that
// calendar was written against instead of whatever the ambient config
// happens to say.
func WithPriorityVocabulary(defs []config.PriorityDefinition) Option {
	return func(o *options) {
		if len(defs) > 0 {
			o.priorities = defs
		}
	}
}

// WithStatusDefinitions supplies the task status vocabulary the encoder
// and decoder map against. Each definition's Role — not its Name — picks
// the RFC 5545 STATUS wire value, so a project that renames its statuses
// still exports meaningful iCalendar.
//
// Unset, the vocabulary resolves lazily through
// core.ConfiguredTaskStatusDefinitions() at encode/decode time, which is
// what any caller wanting the ambient project config should leave it as.
// The option exists for callers holding a vocabulary that is not the
// ambient one — chiefly tests, and any future multi-project export.
func WithStatusDefinitions(defs []config.StatusDefinition) Option {
	return func(o *options) {
		if len(defs) > 0 {
			o.statusDefs = defs
		}
	}
}

// statusDefinitions returns the vocabulary this invocation maps against,
// falling back to the ambient project config when none was supplied.
//
// Resolved here rather than in defaultOptions() so the config lookup
// happens only when an encode or decode actually needs it: defaults are
// materialised on every resolve(), including paths that never touch a
// status.
func (o options) statusDefinitions() []config.StatusDefinition {
	if len(o.statusDefs) > 0 {
		return o.statusDefs
	}
	return core.ConfiguredTaskStatusDefinitions()
}
