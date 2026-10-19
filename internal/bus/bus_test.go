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

	readStream, unsubscribe := b.Subscribe()
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
