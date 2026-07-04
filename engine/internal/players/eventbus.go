package players

import "sync"

// EventType distinguishes join from leave.
type EventType int

const (
	EventJoin EventType = iota
	EventLeave
)

// PlayerEvent is a structured event emitted when a player joins or leaves.
type PlayerEvent struct {
	Type     EventType
	ServerID string
	Name     string
}

const eventSubBuffer = 128

// EventBus wraps per-server Rosters and emits structured PlayerEvents. The
// console hub calls Feed for each console line; subscribers receive events
// derived from Roster state diffs — never from their own regex matching — so
// the join/leave parsing logic lives in exactly one place (Roster.Apply).
type EventBus struct {
	mu      sync.Mutex
	rosters map[string]*Roster
	subs    map[chan PlayerEvent]struct{}
}

// NewEventBus returns an empty bus.
func NewEventBus() *EventBus {
	return &EventBus{
		rosters: map[string]*Roster{},
		subs:    map[chan PlayerEvent]struct{}{},
	}
}

// Feed parses a console line for the given server, updates its roster, and
// emits join/leave events for exactly the players whose presence changed. It
// must not block: slow subscribers drop events rather than stall the console.
func (eb *EventBus) Feed(serverID, line string) {
	eb.mu.Lock()
	roster, ok := eb.rosters[serverID]
	if !ok {
		roster = NewRoster()
		eb.rosters[serverID] = roster
	}
	eb.mu.Unlock()

	before := roster.Names()
	if !roster.Apply(line) {
		return
	}
	after := roster.Names()

	var events []PlayerEvent
	for _, name := range diff(after, before) {
		events = append(events, PlayerEvent{Type: EventJoin, ServerID: serverID, Name: name})
	}
	for _, name := range diff(before, after) {
		events = append(events, PlayerEvent{Type: EventLeave, ServerID: serverID, Name: name})
	}
	eb.publish(events)
}

// Subscribe returns a live channel of player events. Call cancel to
// unsubscribe (it closes the channel).
func (eb *EventBus) Subscribe() (live <-chan PlayerEvent, cancel func()) {
	ch := make(chan PlayerEvent, eventSubBuffer)
	eb.mu.Lock()
	eb.subs[ch] = struct{}{}
	eb.mu.Unlock()

	cancel = func() {
		eb.mu.Lock()
		if _, ok := eb.subs[ch]; ok {
			delete(eb.subs, ch)
			close(ch)
		}
		eb.mu.Unlock()
	}
	return ch, cancel
}

// OnlinePlayers returns the current roster for a server (sorted).
func (eb *EventBus) OnlinePlayers(serverID string) []string {
	eb.mu.Lock()
	roster, ok := eb.rosters[serverID]
	eb.mu.Unlock()
	if !ok {
		return nil
	}
	return roster.Names()
}

// Reset clears the roster for a server (call when the server stops) and emits
// leave events for everyone who was online.
func (eb *EventBus) Reset(serverID string) {
	eb.mu.Lock()
	roster, ok := eb.rosters[serverID]
	if ok {
		delete(eb.rosters, serverID)
	}
	eb.mu.Unlock()
	if !ok {
		return
	}
	var events []PlayerEvent
	for _, name := range roster.Names() {
		events = append(events, PlayerEvent{Type: EventLeave, ServerID: serverID, Name: name})
	}
	eb.publish(events)
}

func (eb *EventBus) publish(events []PlayerEvent) {
	if len(events) == 0 {
		return
	}
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for _, ev := range events {
		for ch := range eb.subs {
			select {
			case ch <- ev:
			default:
				// Slow subscriber: drop rather than stall the console pump.
			}
		}
	}
}

// diff returns the names in a that are not in b (both sorted or not — a map is
// used, order of the result follows a).
func diff(a, b []string) []string {
	inB := make(map[string]struct{}, len(b))
	for _, n := range b {
		inB[n] = struct{}{}
	}
	var out []string
	for _, n := range a {
		if _, ok := inB[n]; !ok {
			out = append(out, n)
		}
	}
	return out
}
