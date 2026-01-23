package events

import (
	"sync"
)

type Event struct {
	ID        string
	Type      string
	DataJSON  []byte
	CreatedAt string
}

type Broker struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[chan Event]struct{})}
}

func (b *Broker) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		delete(b.subs, ch)
		close(ch)
		b.mu.Unlock()
	}

	return ch, cancel
}

func (b *Broker) Publish(evt Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- evt:
		default:
			// Drop if subscriber is too slow.
		}
	}
}
