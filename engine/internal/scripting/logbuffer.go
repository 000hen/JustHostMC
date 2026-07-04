package scripting

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/000hen/justhostmc/engine/internal/logging"
)

// LogLine is one captured line of automation output, tagged with the script that
// produced it.
type LogLine struct {
	ScriptID         string    `json:"script_id"`
	Line             string    `json:"line"`
	Timestamp        time.Time `json:"timestamp"`
	SessionID        string    `json:"session_id"`
	SessionStartedAt time.Time `json:"session_started_at"`
	CurrentSession   bool      `json:"-"`
}

const (
	defaultLogRing = 500
	logSubBuffer   = 128
)

// LogBuffer is the engine-wide ring buffer of automation script output. It
// replays recent lines to new subscribers and fans out live lines, mirroring the
// console hub's pattern but for the single engine-wide automation log.
type LogBuffer struct {
	mu               sync.Mutex
	ring             []LogLine
	ringSize         int
	subs             map[chan LogLine]struct{}
	persistentRoot   string
	sessionID        string
	sessionStartedAt time.Time
	logger           *logging.Logger
}

// NewLogBuffer creates a ring buffer holding the last size lines (size <= 0 uses
// a default).
func NewLogBuffer(size int) *LogBuffer {
	if size <= 0 {
		size = defaultLogRing
	}
	startedAt := time.Now().UTC()
	return &LogBuffer{
		ringSize:         size,
		subs:             map[chan LogLine]struct{}{},
		sessionID:        sessionID(startedAt),
		sessionStartedAt: startedAt,
	}
}

// NewPersistentLogBuffer creates an automation log buffer that restores every
// retained session under root and writes this application run to its own .log
// file. Keeping these files below the shared logs root makes the normal TTL,
// size-cap, clear-logs, and remove-all-data paths apply without special cases.
// A positive size limits the in-memory replay; zero retains all on-disk lines.
func NewPersistentLogBuffer(size int, root string) (*LogBuffer, error) {
	startedAt := time.Now().UTC()
	b := &LogBuffer{
		ringSize:         size,
		subs:             map[chan LogLine]struct{}{},
		persistentRoot:   root,
		sessionID:        sessionID(startedAt),
		sessionStartedAt: startedAt,
	}
	if err := b.loadHistory(); err != nil {
		return nil, err
	}
	return b, nil
}

// Append records a line for a script and fans it out to live subscribers.
func (b *LogBuffer) Append(scriptID, line string) {
	ll := LogLine{
		ScriptID:         scriptID,
		Line:             line,
		Timestamp:        time.Now().UTC(),
		SessionID:        b.sessionID,
		SessionStartedAt: b.sessionStartedAt,
		CurrentSession:   true,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.persistentRoot != "" {
		if encoded, err := json.Marshal(ll); err == nil {
			if b.logger == nil {
				b.logger, _ = logging.Open(b.sessionPath())
			}
			if b.logger != nil {
				_ = b.logger.WriteLine(string(encoded))
			}
		}
	}
	b.ring = append(b.ring, ll)
	if b.ringSize > 0 && len(b.ring) > b.ringSize {
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

// Close releases the current session file. Append reopens it on demand, which
// lets the retention janitor safely close files before deleting them.
func (b *LogBuffer) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.logger != nil {
		_ = b.logger.Close()
		b.logger = nil
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

func (b *LogBuffer) loadHistory() error {
	paths, err := filepath.Glob(filepath.Join(b.persistentRoot, "session-*.log"))
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := b.loadFile(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if b.ringSize > 0 && len(b.ring) > b.ringSize {
		b.ring = append([]LogLine(nil), b.ring[len(b.ring)-b.ringSize:]...)
	}
	return nil
}

func (b *LogBuffer) loadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var line LogLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue // one damaged record must not hide the rest of a session
		}
		if line.SessionID == "" || line.Timestamp.IsZero() {
			continue
		}
		line.CurrentSession = false
		b.ring = append(b.ring, line)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func (b *LogBuffer) sessionPath() string {
	return filepath.Join(b.persistentRoot, "session-"+b.sessionID+".log")
}

func sessionID(startedAt time.Time) string {
	// The fixed-width UTC representation sorts chronologically as a filename.
	return startedAt.Format("20060102T150405.000000000Z")
}
