package bus_test

import (
	"andrelas6/ai-agent-go/internal/bus"
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
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

	want := []events.Type{events.WorkflowStarted, events.WorkflowCompleted}
	if got := eventTypes(events1); !slices.Equal(got, want) {
		t.Errorf("subscriber 1 got %v, want %v", got, want)
	}
	if got := eventTypes(events2); !slices.Equal(got, want) {
		t.Errorf("subscriber 2 got %v, want %v", got, want)
	}
}

func TestLiveEventsArriveInPublishOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	_, live, unsubscribe := b.Subscribe()
	defer unsubscribe()

	want := []string{"event-1", "event-2", "event-3"}
	for _, id := range want {
		b.Publish(events.Event{Message: id})
	}

	got := make([]string, 0, len(want))
	for range want {
		select {
		case e := <-live:
			got = append(got, e.Message)
		case <-ctx.Done():
			t.Fatalf("got %v before timeout, want %v", got, want)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("live = %v, want %v", got, want)
	}
}

func TestEachSubscriberGetsHistoryAsOfItsSubscribe(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	b.Publish(events.Event{Message: "event-1"})
	historyA, _, unsubscribeA := b.Subscribe()
	defer unsubscribeA()

	b.Publish(events.Event{Message: "event-2"})
	historyB, _, unsubscribeB := b.Subscribe()
	defer unsubscribeB()

	b.Publish(events.Event{Message: "event-3"})
	historyC, _, unsubscribeC := b.Subscribe()
	defer unsubscribeC()

	cases := []struct {
		name    string
		history []events.Event
		want    []string
	}{
		{"A", historyA, []string{"event-1"}},
		{"B", historyB, []string{"event-1", "event-2"}},
		{"C", historyC, []string{"event-1", "event-2", "event-3"}},
	}
	for _, c := range cases {
		if got := eventMessages(c.history); !slices.Equal(got, c.want) {
			t.Errorf("subscriber %s history = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSubscribeWhilePublishing(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 200 {
			b.Publish(events.Event{Type: events.Log})
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			_, _, unsubscribe := b.Subscribe()
			unsubscribe()
		}
	}()
	wg.Wait()
}

func TestHistoryAndLiveHoldEachEventOnce(t *testing.T) {
	want := make([]string, 0, 20)
	for i := range 20 {
		want = append(want, fmt.Sprintf("event-%d", i))
	}

	for range 200 {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		b := bus.New(ctx)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, id := range want {
				b.Publish(events.Event{Message: id})
			}
		}()
		history, live, unsubscribe := b.Subscribe()
		wg.Wait()
		unsubscribe()

		got := eventMessages(history)
		for e := range live {
			got = append(got, e.Message)
		}
		cancel()

		if !slices.Equal(got, want) {
			t.Fatalf("history (%d events) + live = %v, want %v", len(history), got, want)
		}
	}
}

func eventTypes(list []events.Event) []events.Type {
	types := make([]events.Type, 0, len(list))
	for _, e := range list {
		types = append(types, e.Type)
	}
	return types
}

func TestPublishStampsIDAndTimestamp(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	_, live, unsubscribe := b.Subscribe()
	defer unsubscribe()

	before := time.Now().UnixMilli()
	b.Publish(events.Event{Type: events.Log, Message: "hello"})

	var fromLive events.Event
	select {
	case fromLive = <-live:
	case <-ctx.Done():
		t.Fatal("no live event before the timeout")
	}

	history, _, unsubscribeLate := b.Subscribe()
	defer unsubscribeLate()
	if len(history) != 1 {
		t.Fatalf("history has %d events, want 1", len(history))
	}
	fromHistory := history[0]

	if _, err := uuid.Parse(fromLive.ID); err != nil {
		t.Errorf("live ID %q is not a UUID: %v", fromLive.ID, err)
	}
	if fromLive.TS < before {
		t.Errorf("live TS = %d, want at least %d (milliseconds)", fromLive.TS, before)
	}
	if fromHistory.ID != fromLive.ID || fromHistory.TS != fromLive.TS {
		t.Errorf("history has ID %q and TS %d, live has ID %q and TS %d; want the same stamp", fromHistory.ID, fromHistory.TS, fromLive.ID, fromLive.TS)
	}
}

func eventMessages(history []events.Event) []string {
	messages := make([]string, 0, len(history))
	for _, e := range history {
		messages = append(messages, e.Message)
	}
	return messages
}
