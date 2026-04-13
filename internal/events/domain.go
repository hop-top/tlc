package events

import "hop.top/kit/domain"

// DomainOptions returns domain.Option slice with the bus publisher wired.
// Returns nil if the publisher is nil, allowing callers to safely spread
// the result without nil checks.
func DomainOptions[T domain.Entity](pub domain.EventPublisher) []domain.Option[T] {
	if pub == nil {
		return nil
	}
	return []domain.Option[T]{domain.WithPublisher[T](pub)}
}
