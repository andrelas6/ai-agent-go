package bus

import (
	"andrelas6/ai-agent-go/internal/events"
	"context"

	"github.com/google/uuid"
)

type subscriberStream struct {
	id          string
	eventStream chan events.Event
}

type Bus struct {
	history       []events.Event
	eventStreams  map[string]chan events.Event
	publishCh     chan events.Event
	subscribeCh   chan subscriberStream
	unsubscribeCh chan string
	clearCh       chan struct{}
	done          <-chan struct{}
}

func (e *Bus) Clear() {
	e.clearCh <- struct{}{}
}

func (e *Bus) Publish(event events.Event) {
	// don't add this to a go routine to avoid breaking ordering with Subscribe
	// TODO: add test to check ordering
	select {
	case <-e.done:
	case e.publishCh <- event:
	}
}

func (e *Bus) Subscribe() (history []events.Event, readStream <-chan events.Event, unsubscribe func()) {
	eventStream := make(chan events.Event, 256)
	channelId := uuid.NewString()
	// order is also important, because a publisher after a subscribe should work for that subscribe (causal consistency)
	select {
	case <-e.done:
		return e.history, nil, func() {}
	case e.subscribeCh <- subscriberStream{id: string(channelId), eventStream: eventStream}:
		return e.history,
			eventStream, func() {
				select {
				case <-e.done:
				case e.unsubscribeCh <- channelId:
				}
			}
	}
}

func New(ctx context.Context) *Bus {
	streams := make(map[string]chan events.Event)

	publishCh := make(chan events.Event)
	subscribeCh := make(chan subscriberStream)
	history := make([]events.Event, 0)
	unsubscribeCh := make(chan string)
	clearCh := make(chan struct{})
	go func() {
		for {
			select {
			case streamId := <-unsubscribeCh:
				ch, ok := streams[streamId]
				if ok {
					delete(streams, streamId)
					close(ch)
				}
			case event := <-publishCh:
				history = append(history, event)
				for _, eventStream := range streams {
					eventStream <- event
				}
			case register := <-subscribeCh:
				streams[register.id] = register.eventStream
			case <-clearCh:
				history = make([]events.Event, 0)
			case <-ctx.Done():
				return
			}
		}
	}()

	return &Bus{
		history:       history,
		eventStreams:  streams,
		publishCh:     publishCh,
		subscribeCh:   subscribeCh,
		unsubscribeCh: unsubscribeCh,
		clearCh:       clearCh,
		done:          ctx.Done(),
	}
}
