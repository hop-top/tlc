// Package vtodo encodes and decodes tlc Task/Track/LogEntry models to and
// from RFC 5545 (iCalendar) + RFC 9253 (RELATED-TO RELTYPE) wire format.
//
// The encoder produces a VCALENDAR containing one VTODO per Task and per
// Track, with PARENT/CHILD/DEPENDS-ON relationships expressed via
// RELATED-TO. Optional VJOURNAL components carry log entries.
//
// The decoder reverses the mapping: VTODO/VJOURNAL → Task/LogEntry. UIDs
// minted by tlc (`<typeid>@<domain>`) round-trip; foreign UIDs cause a
// fresh TypeID to be minted with the original UID stashed in
// Task.Meta["external_uid"].
package vtodo

import (
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
	// priorities is the priority vocabulary in rank order, most urgent
	// first. Never empty after resolve — defaultOptions seeds it from
	// the project config and WithPriorityVocabulary ignores empty input.
	priorities []config.PriorityDefinition
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

// WithProductID overrides the VCALENDAR PRODID property. The default is
// DefaultProductID.
func WithProductID(id string) Option {
	return func(o *options) {
		if id != "" {
			o.productID = id
		}
	}
}

// resolve applies a slice of Options on top of the defaults and returns
// the resolved configuration.
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
