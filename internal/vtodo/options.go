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
	statusDefs  []config.StatusDefinition
}

func defaultOptions() options {
	return options{
		uidDomain:   DefaultUIDDomain,
		productID:   DefaultProductID,
		includeLogs: false,
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
	return o
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
