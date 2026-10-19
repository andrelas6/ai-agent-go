package bus

import "andrelas6/ai-agent-go/internal/events"

type Bus struct {
	eventStream chan events.Event
}

// Clear implements [eventBus].
func (e *Bus) Clear() {
	panic("unimplemented")
}

// Publish implements [eventBus].
func (e *Bus) Publish(event events.Event) {
	e.eventStream <- event
}

// Subscribe implements [eventBus].
func (e *Bus) Subscribe() (readStream <-chan events.Event, unsubscribe func()) {
	readStream = e.eventStream
	return readStream, func() {
		close(e.eventStream)
	}
}

func New() *Bus {
	stream := make(chan events.Event, 256)

	return &Bus{
		eventStream: stream,
	}
}
