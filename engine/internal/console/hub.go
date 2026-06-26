// Package console multiplexes a server's single output stream to many gRPC
// subscribers. Each server gets a bounded ring buffer (replayed to new
// subscribers) plus live fan-out, and a way to write console commands to stdin.
package console

import (
	"fmt"
	"sync"

	"github.com/000hen/justhostmc/engine/internal/isolation"
)

const (
	defaultRingSize = 1000
	subBuffer       = 256
)

// Hub owns one console per server id.
type Hub struct {
	mu       sync.Mutex
	consoles map[string]*serverConsole
	ringSize int

	// observer, if set, receives every output line for side effects such as
	// persisting console logs to disk. It must not block.
	observer func(id, line string)
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{consoles: make(map[string]*serverConsole), ringSize: defaultRingSize}
}

// SetLineObserver registers a callback invoked for every console line (in
// addition to ring-buffering and live fan-out). Set it once before any Register.
func (h *Hub) SetLineObserver(fn func(id, line string)) {
	h.mu.Lock()
	h.observer = fn
	h.mu.Unlock()
}

type serverConsole struct {
	mu       sync.Mutex
	ring     []string
	ringSize int
	subs     map[chan string]struct{}
	inst     isolation.Instance
}

func (h *Hub) getOrCreate(id string) *serverConsole {
	h.mu.Lock()
	defer h.mu.Unlock()
	sc, ok := h.consoles[id]
	if !ok {
		sc = &serverConsole{subs: make(map[chan string]struct{}), ringSize: h.ringSize}
		h.consoles[id] = sc
	}
	return sc
}

// Register attaches a running instance and starts pumping its output into the
// ring buffer and live subscribers. Safe to call again on restart.
func (h *Hub) Register(id string, inst isolation.Instance) {
	sc := h.getOrCreate(id)
	sc.mu.Lock()
	sc.inst = inst
	sc.mu.Unlock()

	h.mu.Lock()
	observer := h.observer
	h.mu.Unlock()

	go func() {
		for line := range inst.Output() {
			if observer != nil {
				observer(id, line)
			}
			sc.broadcast(line)
		}
	}()
}

func (sc *serverConsole) broadcast(line string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.ring = append(sc.ring, line)
	if len(sc.ring) > sc.ringSize {
		sc.ring = append([]string(nil), sc.ring[len(sc.ring)-sc.ringSize:]...)
	}
	for ch := range sc.subs {
		select {
		case ch <- line:
		default:
			// Subscriber is too slow; drop rather than stall the whole console.
		}
	}
}

// Subscribe returns the buffered history plus a live channel of subsequent lines.
// Call cancel to unsubscribe (it closes the live channel).
func (h *Hub) Subscribe(id string) (history []string, live <-chan string, cancel func()) {
	sc := h.getOrCreate(id)
	sc.mu.Lock()
	history = append([]string(nil), sc.ring...)
	ch := make(chan string, subBuffer)
	sc.subs[ch] = struct{}{}
	sc.mu.Unlock()

	cancel = func() {
		sc.mu.Lock()
		if _, ok := sc.subs[ch]; ok {
			delete(sc.subs, ch)
			close(ch)
		}
		sc.mu.Unlock()
	}
	return history, ch, cancel
}

// Send writes a command line to the server's stdin.
func (h *Hub) Send(id, command string) error {
	h.mu.Lock()
	sc, ok := h.consoles[id]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("no console for server %q", id)
	}
	sc.mu.Lock()
	inst := sc.inst
	sc.mu.Unlock()
	if inst == nil || !inst.Running() {
		return fmt.Errorf("server %q is not running", id)
	}
	return inst.WriteStdin(command)
}

// Unregister drops a server's console and disconnects its subscribers (on delete).
func (h *Hub) Unregister(id string) {
	h.mu.Lock()
	sc, ok := h.consoles[id]
	if ok {
		delete(h.consoles, id)
	}
	h.mu.Unlock()
	if !ok {
		return
	}
	sc.mu.Lock()
	for ch := range sc.subs {
		delete(sc.subs, ch)
		close(ch)
	}
	sc.mu.Unlock()
}
