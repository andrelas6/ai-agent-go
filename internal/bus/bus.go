package bus

import (
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"log"
	"slices"
	"time"

	"github.com/google/uuid"
)

const SubscriberBuffer = 256
const HistoryLimit = 1000

type subscription struct {
	liveEventCh chan events.Event
	// to avoid data races in the history slice, the writing/reading of the history slice must be done in the same select
	// the reply channel is there just to make sure that works
	replyCh     chan []events.Event
	lastEventID string
}

type Bus struct {
	publishCh     chan events.Event
	subscribeCh   chan subscription
	unsubscribeCh chan chan events.Event
	clearCh       chan struct{}
	doneCh        <-chan struct{}
}

func (b *Bus) Clear() {
	select {
	case <-b.doneCh:
	case b.clearCh <- struct{}{}:
	}
}

func (b *Bus) Publish(event events.Event) {
	// don't add this to a go routine to avoid breaking ordering with Subscribe
	select {
	case <-b.doneCh:
	case b.publishCh <- event:
	}
}

func (b *Bus) Subscribe(lastEventID string) (history []events.Event, liveEventCh <-chan events.Event, unsubscribe func()) {
	eventCh := make(chan events.Event, SubscriberBuffer)
	// throwaway reply channel just to get the history back without data races (publish/subscribe concurrently) - check select below
	replyCh := make(chan []events.Event, 1)

	// order is also important, because a publisher after a subscribe should work for that subscribe (causal consistency)
	select {
	case <-b.doneCh:
		return nil, closedStream(), func() {}
	case b.subscribeCh <- subscription{liveEventCh: eventCh, replyCh: replyCh, lastEventID: lastEventID}:
		return <-replyCh,
			eventCh, func() {
				select {
				case <-b.doneCh:
				case b.unsubscribeCh <- eventCh:
				}
			}
	}
}

func closedStream() <-chan events.Event {
	ch := make(chan events.Event)
	close(ch)
	return ch
}

func New(ctx context.Context) *Bus {
	streams := make(map[chan events.Event]struct{})

	history := make([]events.Event, 0)
	b := &Bus{
		publishCh:     make(chan events.Event),
		subscribeCh:   make(chan subscription),
		unsubscribeCh: make(chan chan events.Event),
		clearCh:       make(chan struct{}),
		doneCh:        ctx.Done(),
	}
	go func() {
		for {
			select {
			case ch := <-b.unsubscribeCh:
				if _, ok := streams[ch]; ok {
					delete(streams, ch)
					close(ch)
				}
			case event := <-b.publishCh:
				event.ID = uuid.NewString()
				event.TS = time.Now().UnixMilli()
				if len(history) == HistoryLimit {
					history = history[1:]
				}
				history = append(history, event)
				for eventStream := range streams {
					select {
					case eventStream <- event:
					// if buffer is full, gets here
					default:
						close(eventStream)
						delete(streams, eventStream)
						log.Printf("bus: subscriber fell %d events behind, closing its stream so it reconnects", SubscriberBuffer)
					}
				}
			case subscription := <-b.subscribeCh:
				streams[subscription.liveEventCh] = struct{}{}
				lastEventIndex := slices.IndexFunc(history, func(e events.Event) bool { return e.ID == subscription.lastEventID })
				if lastEventIndex == -1 && subscription.lastEventID != "" {
					log.Printf("bus: last event id %q is not in the history, replaying all %d events", subscription.lastEventID, len(history))
				}
				subscription.replyCh <- slices.Clone(history[lastEventIndex+1:])
			case <-b.clearCh:
				history = make([]events.Event, 0)
			case <-ctx.Done():
				return
			}
		}
	}()

	return b
}
