package bus_test

import (
	"andrelas6/ai-agent-go/internal/bus"
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"testing"
	"time"
)

// Is this the best way to test this?
// Also, maybe I should use a timeout (with context) so that this test does not hang if there is a regression problem with it?
func TestEventBus(t *testing.T) {
	// send event, get event
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

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

// test concurrent access to the map
func TestConcurrentPublishSubscribe(t *testing.T) {
	b := bus.New(t.Context())

	go func() {
		for range 100 {
			b.Subscribe()
		}

	}()

	go func() {
		for range 100 {
			b.Publish(events.Event{})
		}
	}()

}

// two event same bus get same events
func TestTwoSubscribersSameBusGetsSameEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	_, r1, u1 := b.Subscribe()
	_, r2, u2 := b.Subscribe()

	b.Publish(events.Event{Type: events.WorkflowStarted})
	b.Publish(events.Event{Type: events.WorkflowCompleted})

	// buffered channel - close so that for does not block execution
	u1()
	u2()

	events1 := make([]events.Event, 0)
	events2 := make([]events.Event, 0)

	for e1 := range r1 {
		events1 = append(events1, e1)
	}

	for e2 := range r2 {
		events2 = append(events2, e2)
	}

	for i := range len(events1) {
		if events1[i].Type != events2[i].Type {
			t.Fatal("events are not the same")
		}
	}
}

func TestSubscribeReturnsEarlierEventsAsHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	b.Publish(events.Event{ID: "event-1", Type: events.WorkflowStarted})
	b.Publish(events.Event{ID: "event-2", Type: events.Log})

	history, _, unsubscribe := b.Subscribe()
	defer unsubscribe()

	if len(history) != 2 {
		t.Fatalf("history has %d events, want 2: %+v", len(history), history)
	}
	if history[0].ID != "event-1" || history[1].ID != "event-2" {
		t.Errorf("history ids = [%s %s], want [event-1 event-2]", history[0].ID, history[1].ID)
	}
}
