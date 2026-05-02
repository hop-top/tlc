package events

import (
	"context"
	"testing"

	"hop.top/kit/go/runtime/bus"
	"hop.top/tlc/internal/core"
)

func TestDomainOptions_NilPublisher(t *testing.T) {
	opts := DomainOptions[core.Task](nil, "tlc.task")
	if opts != nil {
		t.Fatal("expected nil options for nil publisher")
	}
}

func TestDomainOptions_WithPublisher(t *testing.T) {
	b := bus.New()
	defer b.Close(context.Background())
	pub := NewBusPublisher(b)

	// publisher only (no prefix) -> 1 option (publisher).
	opts := DomainOptions[core.Task](pub, "")
	if len(opts) != 1 {
		t.Fatalf("expected 1 option (publisher), got %d", len(opts))
	}
}

func TestDomainOptions_WithPublisherAndPrefix(t *testing.T) {
	b := bus.New()
	defer b.Close(context.Background())
	pub := NewBusPublisher(b)

	// publisher + prefix -> 2 options (publisher + topics).
	opts := DomainOptions[core.Task](pub, "tlc.task")
	if len(opts) != 2 {
		t.Fatalf("expected 2 options (publisher+topics), got %d", len(opts))
	}
}
