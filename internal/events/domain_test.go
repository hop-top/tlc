package events

import (
	"context"
	"testing"

	"hop.top/kit/go/runtime/bus"
	"hop.top/tlc/internal/core"
)

func TestDomainOptions_NilPublisher(t *testing.T) {
	opts := DomainOptions[core.Task](nil)
	if opts != nil {
		t.Fatal("expected nil options for nil publisher")
	}
}

func TestDomainOptions_WithPublisher(t *testing.T) {
	b := bus.New()
	defer b.Close(context.Background())
	pub := NewBusPublisher(b)

	opts := DomainOptions[core.Task](pub)
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}
