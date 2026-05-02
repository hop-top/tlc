package events

import (
	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"
)

// DomainOptions returns a domain.Option slice with the bus publisher
// wired and per-action topics overridden to the given prefix.
//
// The prefix is used to compose three lifecycle topics directly via
// domain.WithTopics:
//
//	<prefix>.created
//	<prefix>.updated
//	<prefix>.deleted
//
// We use domain.WithTopics (literal mapping) rather than
// domain.WithTopicPrefix because tlc's topic constants in topics.go
// (e.g. "tlc.task.created") are 3-segment, while bus.PrefixTopics
// requires a 3-segment prefix that yields a 4-segment topic.
// WithTopics performs no segment-count validation, so it preserves
// tlc's existing 3-segment public contract while still consuming the
// new kit "configurable topics" API shipped in kit/T-0115.
//
// Returns nil if the publisher is nil, allowing callers to safely
// spread the result without nil checks.
func DomainOptions[T domain.Entity](pub domain.EventPublisher, prefix string) []domain.Option[T] {
	if pub == nil {
		return nil
	}
	opts := []domain.Option[T]{domain.WithPublisher[T](pub)}
	if prefix != "" {
		topics := domain.Topics{
			Created: bus.Topic(prefix + ".created"),
			Updated: bus.Topic(prefix + ".updated"),
			Deleted: bus.Topic(prefix + ".deleted"),
		}
		opts = append(opts, domain.WithTopics[T](topics))
	}
	return opts
}
