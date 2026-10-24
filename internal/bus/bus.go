package bus

import (
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
)

const SubscriberBuffer = 256

type subscription struct {
	eventStream chan events.Event
	// to avoid data races in the history slice, the writing/reading of the history slice must be done in the same select
	// the reply channel is there just to make sure that works
	reply chan []events.Event
}

type Bus struct {
	publish     chan events.Event
	subscribe   chan subscription
	unsubscribe chan chan events.Event
	clear       chan struct{}
	done        <-chan struct{}
}

func (b *Bus) Clear() {
	select {
	case <-b.done:
	case b.clear <- struct{}{}:
	}
}

func (b *Bus) Publish(event events.Event) {
	// don't add this to a go routine to avoid breaking ordering with Subscribe
	// TODO: add test to check ordering
	select {
	case <-b.done:
	case b.publish <- event:
	}
}

func (b *Bus) Subscribe() (history []events.Event, live <-chan events.Event, unsubscribe func()) {
	stream := make(chan events.Event, SubscriberBuffer)
	// throaway reply channel just to get the history back without data races (publish/subscribe concurrently)
	reply := make(chan []events.Event, 1)

	// order is also important, because a publisher after a subscribe should work for that subscribe (causal consistency)
	select {
	case <-b.done:
		return nil, nil, func() {}
	case b.subscribe <- subscription{eventStream: stream, reply: reply}:
		return <-reply,
			stream, func() {
				select {
				case <-b.done:
				case b.unsubscribe <- stream:
				}
			}
	}
}

func New(ctx context.Context) *Bus {
	streams := make(map[chan events.Event]struct{})

	history := make([]events.Event, 0)
	b := &Bus{
		publish:     make(chan events.Event),
		subscribe:   make(chan subscription),
		unsubscribe: make(chan chan events.Event),
		clear:       make(chan struct{}),
		done:        ctx.Done(),
	}
	go func() {
		for {
			select {
			case ch := <-b.unsubscribe:
				if _, ok := streams[ch]; ok {
					delete(streams, ch)
					close(ch)
				}
			case event := <-b.publish:
				event.ID = uuid.NewString()
				event.TS = time.Now().UnixMilli()
				history = append(history, event)
				for eventStream := range streams {
					// if buffer is full, drop events
					// TODO: disconnect handler assigned to that stream so that they can reconnect
					select {
					case eventStream <- event:
					default:
					}
				}
			case subscription := <-b.subscribe:
				streams[subscription.eventStream] = struct{}{}
				subscription.reply <- slices.Clone(history)
			case <-b.clear:
				history = make([]events.Event, 0)
			case <-ctx.Done():
				return
			}
		}
	}()

	return b
}
