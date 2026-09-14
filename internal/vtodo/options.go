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
