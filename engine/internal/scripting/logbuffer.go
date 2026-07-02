package scripting

import (
	"sync"
	"time"
)

// LogLine is one captured line of automation output, tagged with the script that
// produced it.
type LogLine struct {
	ScriptID  string
	Line      string
	Timestamp time.Time
}

const (
	defaultLogRing = 500
	logSubBuffer   = 128
)

// LogBuffer is the engine-wide ring buffer of automation script output. It
// replays recent lines to new subscribers and fans out live lines, mirroring the
// console hub's pattern but for the single engine-wide automation log.
type LogBuffer struct {
	mu       sync.Mutex
	ring     []LogLine
	ringSize int
	subs     map[chan LogLine]struct{}
}

// NewLogBuffer creates a ring buffer holding the last size lines (size <= 0 uses
// a default).
func NewLogBuffer(size int) *LogBuffer {
	if size <= 0 {
		size = defaultLogRing
	}
	return &LogBuffer{ringSize: size, subs: map[chan LogLine]struct{}{}}
}

// Append records a line for a script and fans it out to live subscribers.
func (b *LogBuffer) Append(scriptID, line string) {
	ll := LogLine{ScriptID: scriptID, Line: line, Timestamp: time.Now().UTC()}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ring = append(b.ring, ll)
	if len(b.ring) > b.ringSize {
		b.ring = append([]LogLine(nil), b.ring[len(b.ring)-b.ringSize:]...)
	}
	for ch := range b.subs {
		select {
		case ch <- ll:
		default:
			// Slow subscriber: drop rather than stall script execution.
		}
	}
}

// Subscribe returns the buffered history plus a live channel of subsequent
// lines; cancel unsubscribes (closing the live channel).
func (b *LogBuffer) Subscribe() (history []LogLine, live <-chan LogLine, cancel func()) {
	b.mu.Lock()
	history = append([]LogLine(nil), b.ring...)
	ch := make(chan LogLine, logSubBuffer)
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	cancel = func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
	return history, ch, cancel
}
