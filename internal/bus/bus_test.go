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
func TestSubscriberGetsPublishedEvent(t *testing.T) {
	// send event, get event
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	_, live, unsubscribe := b.Subscribe("")
	defer unsubscribe()

	b.Publish(events.Event{
		Type: events.WorkflowStarted,
	})

	select {
	case event := <-live:
		if event.Type != events.WorkflowStarted {
			t.Fatalf("expected %v. got %v", events.WorkflowStarted, event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("no event within 1s")
	}
}

// two event same bus get same events
func TestTwoSubscribersSameBusGetsSameEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	_, r1, u1 := b.Subscribe("")
	_, r2, u2 := b.Subscribe("")

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

	_, live, unsubscribe := b.Subscribe("")
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
	historyA, _, unsubscribeA := b.Subscribe("")
	defer unsubscribeA()

	b.Publish(events.Event{Message: "event-2"})
	historyB, _, unsubscribeB := b.Subscribe("")
	defer unsubscribeB()

	b.Publish(events.Event{Message: "event-3"})
	historyC, _, unsubscribeC := b.Subscribe("")
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
			_, _, unsubscribe := b.Subscribe("")
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
		history, live, unsubscribe := b.Subscribe("")
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

func TestSlowSubscriberDoesNotBlockOthers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*3)
	defer cancel()
	b := bus.New(ctx)
	eventCount := bus.SubscriberBuffer + 50

	_, _, unsubscribe := b.Subscribe("")
	defer unsubscribe()
	_, streamB, unsubscribe := b.Subscribe("")
	defer unsubscribe()
	missingEventStream := make(chan string, 1)
	eventsWrongOrderStream := make(chan string, 1)
	allEventsReadCh := make(chan struct{}, 1)

	go func() {
		for i := range eventCount {
			b.Publish(events.Event{Message: fmt.Sprintf("event-%d", i)})
		}
	}()

	go func() {
		counter := 0
		for {
			select {
			case event := <-streamB:
				if event.Message != fmt.Sprintf("event-%d", counter) {
					eventsWrongOrderStream <- fmt.Sprintf("got %q, want event-%d", event.Message, counter)
					return
				}
				counter++
				// counter reached eventCount - it means all events were received
				if counter == eventCount {
					allEventsReadCh <- struct{}{}
					return
				}
			case <-time.After(time.Second * 1):
				missingEventStream <- fmt.Sprintf("got %d of %d", counter, eventCount)
				return
			}
		}
	}()

	select {
	case <-allEventsReadCh:
	case reason := <-missingEventStream:
		t.Fatalf("reason: %s", reason)
	case reason := <-eventsWrongOrderStream:
		t.Fatalf("reason: %s", reason)
	}
}

func TestClearReturnsAfterShutdown(t *testing.T) {
	ctx, shutdown := context.WithCancel(t.Context())
	b := bus.New(ctx)
	shutdown()

	done := make(chan struct{})
	go func() {
		defer close(done)
		b.Clear()
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Clear did not return within 1s of the bus shutting down")
	}
}

func TestSubscribeAfterShutdownReturnsAClosedStream(t *testing.T) {
	ctx, shutdown := context.WithCancel(t.Context())
	b := bus.New(ctx)
	shutdown()

	_, stream, unsubscribe := b.Subscribe("")
	defer unsubscribe()

	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("got an event after shutdown, want a closed stream")
		}
	case <-time.After(time.Second):
		t.Fatal("stream was still open 1s after the bus shut down")
	}
}

func TestClearLeavesNewSubscriberWithNoHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	b.Publish(events.Event{Message: "event-1"})
	b.Publish(events.Event{Message: "event-2"})
	b.Clear()

	history, _, unsubscribe := b.Subscribe("")
	defer unsubscribe()

	if len(history) != 0 {
		t.Errorf("history after Clear = %v, want none", eventMessages(history))
	}
}

func TestEventsAfterClearStartANewHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	b.Publish(events.Event{Message: "before-clear"})
	b.Clear()
	b.Publish(events.Event{Message: "after-clear"})

	history, _, unsubscribe := b.Subscribe("")
	defer unsubscribe()

	if got, want := eventMessages(history), []string{"after-clear"}; !slices.Equal(got, want) {
		t.Errorf("history = %v, want %v", got, want)
	}
}

func TestPublishWithNoSubscribersDoesNotBlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 10 {
			b.Publish(events.Event{Type: events.Log})
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publishing with no subscribers did not return within 1s")
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

	_, live, unsubscribe := b.Subscribe("")
	defer unsubscribe()

	before := time.Now().UnixMilli()
	b.Publish(events.Event{Type: events.Log, Message: "hello"})

	var fromLive events.Event
	select {
	case fromLive = <-live:
	case <-ctx.Done():
		t.Fatal("no live event before the timeout")
	}

	history, _, unsubscribeLate := b.Subscribe("")
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

func TestBusResumesAfterLastID(t *testing.T) {
	// setup
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	b.Publish(events.Event{Message: "1"})
	b.Publish(events.Event{Message: "2"})
	b.Publish(events.Event{Message: "3"})
	b.Publish(events.Event{Message: "4"})
	b.Publish(events.Event{Message: "5"})

	history, _, unsubscribe := b.Subscribe("")
	unsubscribe()
	cases := []struct {
		name        string
		lastEventID string
		expected    []string
	}{
		{
			name:        "no last event id",
			lastEventID: "",
			expected:    []string{"1", "2", "3", "4", "5"},
		},
		{
			name:        "last event id",
			lastEventID: history[4].ID,
			expected:    []string{},
		},
		{
			name:        "first event id",
			lastEventID: history[0].ID,
			expected:    []string{"2", "3", "4", "5"},
		},
		{
			name:        "middle event id",
			lastEventID: history[2].ID,
			expected:    []string{"4", "5"},
		},
		{
			name:        "unknown event id",
			lastEventID: "not-an-event-id",
			expected:    []string{"1", "2", "3", "4", "5"},
		},
	}

	// execute
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			history, _, unsubscribe := b.Subscribe(c.lastEventID)
			defer unsubscribe()

			// assert
			if got, want := eventMessages(history), c.expected; !slices.Equal(got, want) {
				t.Errorf("got %v - expected %v", got, want)
			}

		})
	}

}

func TestBusResumeAfterClearGivesFullHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)

	b.Publish(events.Event{Message: "before-clear"})
	history, _, unsubscribe := b.Subscribe("")
	unsubscribe()
	seenBeforeClear := history[0].ID

	b.Clear()
	b.Publish(events.Event{Message: "after-clear-1"})
	b.Publish(events.Event{Message: "after-clear-2"})

	history, _, unsubscribe = b.Subscribe(seenBeforeClear)
	defer unsubscribe()

	if got, want := eventMessages(history), []string{"after-clear-1", "after-clear-2"}; !slices.Equal(got, want) {
		t.Errorf("history = %v, want %v", got, want)
	}
}

func TestPublishToFullChannelClosesIt(t *testing.T) {
	// setup
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*1)
	defer cancel()
	b := bus.New(ctx)

	_, stream, unsubscribe := b.Subscribe("")
	defer unsubscribe()

	streamBCh := make(chan int, 1)
	_, streamB, unsubscribe := b.Subscribe("")
	defer unsubscribe()
	go func() {
		for i := range bus.SubscriberBuffer + 1 {
			select {
			case _, ok := <-streamB:
				if !ok {
					streamBCh <- i
					return
				}
			case <-time.After(time.Second * 1):
				streamBCh <- i
				return
			}
		}

		streamBCh <- bus.SubscriberBuffer + 1
	}()

	// execute
	for range bus.SubscriberBuffer + 1 {
		b.Publish(events.Event{Type: events.WorkflowStarted})
	}

	_, _, otherUnsubscribe := b.Subscribe("")
	otherUnsubscribe()

	// read all values
	for i := range bus.SubscriberBuffer {
		select {
		case _, ok := <-stream:
			if !ok {
				t.Fatalf("stream closed after %v events, want %v", i, bus.SubscriberBuffer)
			}
		case <-time.After(time.Second * 1):
			t.Fatalf("timeout 1s after %v events", i)
		}
	}

	// reached last event of the buffer
	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("got an event past the buffer, want the channel closed")
		}
	case <-time.After(time.Second * 1):
		t.Fatal("channel was not closed within 1s")
	}

	// second subscriber
	if got := <-streamBCh; got != bus.SubscriberBuffer+1 {
		t.Fatalf("stream b got %v events, want %v", got, bus.SubscriberBuffer+1)
	}

}

func TestHistoryKeepLast1000Events(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*1)
	defer cancel()
	// setup
	b := bus.New(ctx)

	for i := range bus.HistoryLimit {
		b.Publish(events.Event{Message: fmt.Sprintf("%d", i+1)})
	}

	b.Publish(events.Event{Message: "1001"})

	history, _, unsubscribe := b.Subscribe("")
	defer unsubscribe()

	if len(history) != bus.HistoryLimit {
		t.Fatalf("history has %v events, want %v", len(history), bus.HistoryLimit)
	}
	if first, last := history[0].Message, history[len(history)-1].Message; first != "2" || last != "1001" {
		t.Errorf("history runs %q … %q, want %q … %q", first, last, "2", "1001")
	}
}

func TestHistoryReplaysFullyWhenIdOfDroppedEventIsUsed(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*1)
	defer cancel()

	b := bus.New(ctx)
	b.Publish(events.Event{})
	b.Publish(events.Event{})
	history, _, unsubscribe := b.Subscribe("")
	firstDroppedEvent := history[0]
	secondDroppedEvent := history[1]
	unsubscribe()

	for range bus.HistoryLimit {
		b.Publish(events.Event{})
	}

	replay, _, unsubscribe := b.Subscribe(firstDroppedEvent.ID)
	unsubscribe()

	full, _, unsubscribe := b.Subscribe("")
	unsubscribe()

	if len(replay) != len(full) {
		t.Fatalf("replay has %v events, want %v", len(replay), len(full))
	}

	if replay[0].ID == secondDroppedEvent.ID {
		t.Fatalf("replay starts at the second dropped event %v, want the oldest event still in history", secondDroppedEvent.ID)
	}

	for i := range len(replay) {
		if replay[i].ID != full[i].ID {
			t.Fatalf("replay[%d] = %v, want %v", i, replay[i].ID, full[i].ID)
		}
	}
}

func eventMessages(history []events.Event) []string {
	messages := make([]string, 0, len(history))
	for _, e := range history {
		messages = append(messages, e.Message)
	}
	return messages
}
