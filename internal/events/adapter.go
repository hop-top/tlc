package events

import (
	"context"

	"hop.top/kit/go/runtime/bus"
)

// BusPublisher adapts kit/bus.Bus to domain.EventPublisher.
// It maps domain Publish(ctx, topic, source, payload) calls to
// bus.NewEvent + bus.Publish.
type BusPublisher struct {
	b bus.Bus
}

// NewBusPublisher returns an adapter satisfying domain.EventPublisher.
func NewBusPublisher(b bus.Bus) *BusPublisher {
	return &BusPublisher{b: b}
}

// Publish creates a bus.Event from the domain arguments and publishes it.
func (a *BusPublisher) Publish(ctx context.Context, topic, source string, payload any) error {
	return a.b.Publish(ctx, bus.NewEvent(bus.Topic(topic), source, payload)) //nolint:wrapcheck // thin bus adapter; kit typed errors surface verbatim
}
