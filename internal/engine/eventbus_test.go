package engine

import "testing"

func TestEventBusPublishSubscribe(t *testing.T) {
	bus := NewEventBus()
	sub := bus.Subscribe(1)
	defer bus.Close()

	event := Event{Type: EventWorkflowSubmitted}
	if err := bus.Publish(event); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	got := <-sub.C
	if got.Type != EventWorkflowSubmitted {
		t.Fatalf("unexpected event type %q", got.Type)
	}

	snapshot := bus.Snapshot()
	if snapshot.Published != 1 {
		t.Fatalf("expected 1 published event, got %d", snapshot.Published)
	}
}

func TestEventBusDropAccounting(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	_ = bus.Subscribe(1)
	if err := bus.Publish(Event{Type: EventWorkflowSubmitted}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if err := bus.Publish(Event{Type: EventWorkflowSubmitted}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	snapshot := bus.Snapshot()
	if snapshot.Dropped[EventWorkflowSubmitted] == 0 {
		t.Fatalf("expected dropped event count to be recorded")
	}
}
