package store

import "sync"

// ChangeKind identifies the single registry item affected by a store mutation.
type ChangeKind uint8

const (
	ChangeUpsert ChangeKind = iota + 1
	ChangeDeleted
)

// Change is an immutable snapshot of one successful store mutation.
type Change struct {
	Kind     ChangeKind
	Server   *Server
	ServerID string
}

// Subscription is a bounded stream of server registry changes.
type Subscription struct {
	Events <-chan Change
	cancel func()
}

// Cancel removes and closes the subscription. It is safe to call repeatedly.
func (s Subscription) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// ChangeSource creates subscriptions to successful store mutations.
type ChangeSource interface {
	Subscribe() Subscription
}

// Observable decorates a Store with bounded, non-blocking change fan-out.
type Observable struct {
	Store
	broker *changeBroker
}

// NewObservable wraps base and gives each subscriber the requested capacity.
func NewObservable(base Store, capacity int) *Observable {
	if capacity < 1 {
		capacity = 1
	}
	return &Observable{
		Store: base,
		broker: &changeBroker{
			capacity: capacity,
			subs:     make(map[uint64]chan Change),
		},
	}
}

// Put persists server and publishes one immutable upsert after success.
func (s *Observable) Put(server *Server) error {
	if err := s.Store.Put(server); err != nil {
		return err
	}
	s.broker.publish(Change{Kind: ChangeUpsert, Server: cloneServer(server)})
	return nil
}

// Delete removes id and publishes one deletion after success.
func (s *Observable) Delete(id string) error {
	if err := s.Store.Delete(id); err != nil {
		return err
	}
	s.broker.publish(Change{Kind: ChangeDeleted, ServerID: id})
	return nil
}

// Subscribe registers a bounded change subscriber.
func (s *Observable) Subscribe() Subscription {
	return s.broker.subscribe()
}

func cloneServer(server *Server) *Server {
	clone := *server
	clone.LaunchArgs = append([]string(nil), server.LaunchArgs...)
	return &clone
}

type changeBroker struct {
	mu       sync.Mutex
	capacity int
	nextID   uint64
	subs     map[uint64]chan Change
}

func (b *changeBroker) subscribe() Subscription {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	events := make(chan Change, b.capacity)
	b.subs[id] = events
	b.mu.Unlock()

	return Subscription{
		Events: events,
		cancel: func() { b.unsubscribe(id) },
	}
}

func (b *changeBroker) unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	events, ok := b.subs[id]
	if !ok {
		return
	}
	delete(b.subs, id)
	close(events)
}

func (b *changeBroker) publish(change Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, events := range b.subs {
		select {
		case events <- change:
		default:
			delete(b.subs, id)
			close(events)
		}
	}
}
