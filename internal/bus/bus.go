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
	publishCh          chan events.Event
	subscriptionStream chan subscription
	unsubscribeCh      chan chan events.Event
	clearCh            chan struct{}
	done               <-chan struct{}
}

func (e *Bus) Clear() {
	select {
	case <-e.done:
	case e.clearCh <- struct{}{}:
	}
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
	eventStream := make(chan events.Event, SubscriberBuffer)
	// throaway reply channel just to get the history back without data races (publish/subscribe concurrently)
	replyCh := make(chan []events.Event, 1)

	// order is also important, because a publisher after a subscribe should work for that subscribe (causal consistency)
	select {
	case <-e.done:
		return nil, nil, func() {}
	case e.subscriptionStream <- subscription{eventStream: eventStream, reply: replyCh}:
		return <-replyCh,
			eventStream, func() {
				select {
				case <-e.done:
				case e.unsubscribeCh <- eventStream:
				}
			}
	}
}

func New(ctx context.Context) *Bus {
	streams := make(map[chan events.Event]struct{})

	history := make([]events.Event, 0)
	publishCh := make(chan events.Event)
	subscriptionCh := make(chan subscription)
	unsubscribeCh := make(chan chan events.Event)
	clearCh := make(chan struct{})
	go func() {
		for {
			select {
			case ch := <-unsubscribeCh:
				if _, ok := streams[ch]; ok {
					delete(streams, ch)
					close(ch)
				}
			case event := <-publishCh:
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
			case subscription := <-subscriptionCh:
				streams[subscription.eventStream] = struct{}{}
				subscription.reply <- slices.Clone(history)
			case <-clearCh:
				history = make([]events.Event, 0)
			case <-ctx.Done():
				return
			}
		}
	}()

	return &Bus{
		publishCh:          publishCh,
		subscriptionStream: subscriptionCh,
		unsubscribeCh:      unsubscribeCh,
		clearCh:            clearCh,
		done:               ctx.Done(),
	}
}
