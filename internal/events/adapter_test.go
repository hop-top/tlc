package events

import (
	"context"
	"testing"

	"hop.top/kit/bus"
)

func TestBusPublisher_Publish(t *testing.T) {
	b := bus.New()
	defer b.Close(context.Background())

	var received bus.Event
	b.Subscribe("tlc.task.created", func(_ context.Context, e bus.Event) error {
		received = e
		return nil
	})

	pub := NewBusPublisher(b)
	payload := &EntityEvent{ID: "T-0001", Actor: "test"}
	err := pub.Publish(context.Background(), "tlc.task.created", "test-source", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Topic != "tlc.task.created" {
		t.Fatalf("expected topic tlc.task.created, got %s", received.Topic)
	}
	if received.Source != "test-source" {
		t.Fatalf("expected source test-source, got %s", received.Source)
	}
	ee, ok := received.Payload.(*EntityEvent)
	if !ok {
		t.Fatal("expected *EntityEvent payload")
	}
	if ee.ID != "T-0001" {
		t.Fatalf("expected ID T-0001, got %s", ee.ID)
	}
}
