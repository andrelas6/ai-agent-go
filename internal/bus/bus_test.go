package bus_test

import (
	"andrelas6/ai-agent-go/internal/bus"
	"andrelas6/ai-agent-go/internal/events"
	"testing"
	"time"
)

// Is this the best way to test this?
// Also, maybe I should use a timeout (with context) so that this test does not hang if there is a regression problem with it?
func TestEventBus(t *testing.T) {
	// send event, get event
	b := bus.New()

	_, readStream, unsubscribe := b.Subscribe()
	defer unsubscribe()

	b.Publish(events.Event{
		Type: events.WorkflowStarted,
	})

	select {
	case event := <-readStream:
		if event.Type != events.WorkflowStarted {
			t.Fatalf("expected %v. got %v", events.WorkflowStarted, event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no event within 1s")
	}
}

// two event bus don't get each other events
func TestEventBusTwice(t *testing.T) {
	b1 := bus.New()
	b2 := bus.New()

	_, r1, u1 := b1.Subscribe()
	_, r2, u2 := b2.Subscribe()

	b1.Publish(events.Event{Type: events.WorkflowStarted})
	b2.Publish(events.Event{Type: events.WorkflowCompleted})

	// buffered channel - close so that for does not block execution
	u1()
	u2()

	for e1 := range r1 {
		for e2 := range r2 {
			if e1.Type == e2.Type {
				t.Fatal("expected types to be different")
			}
		}

	}
}
