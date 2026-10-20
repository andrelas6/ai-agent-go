package bus

import (
	"andrelas6/ai-agent-go/internal/events"

	"github.com/google/uuid"
)

type connectionId string
type Bus struct {
	history      []events.Event
	eventStreams map[connectionId]chan events.Event
}

func (e *Bus) Clear() {
	e.history = make([]events.Event, 0)
}

func (e *Bus) Publish(event events.Event) {
	for _, stream := range e.eventStreams {
		stream <- event
	}
}

func (e *Bus) Subscribe() (history []events.Event, readStream <-chan events.Event, unsubscribe func()) {
	eventStream := make(chan events.Event, 256)
	channelId := connectionId(uuid.NewString())
	e.eventStreams[channelId] = eventStream
	return e.history,
		eventStream, func() {
			close(eventStream)
			delete(e.eventStreams, channelId)
		}
}

func New() *Bus {
	streams := make(map[connectionId]chan events.Event)

	return &Bus{
		history:      make([]events.Event, 0),
		eventStreams: streams,
	}
}
