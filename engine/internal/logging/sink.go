package logging

import (
	"path/filepath"
	"sync"
	"time"
)

// Sink persists each server's console output to a daily-rotating log file at
// <root>/<serverID>/console-YYYY-MM-DD.log. Files rotate by day so the retention
// Purge can age them out file-by-file. It is safe for concurrent use; write
// failures are swallowed so logging never disrupts the live console.
type Sink struct {
	mu      sync.Mutex
	root    string
	now     func() time.Time
	loggers map[string]*datedLogger
}

type datedLogger struct {
	day    string
	logger *Logger
}

// NewSink returns a Sink writing under root.
func NewSink(root string) *Sink {
	return &Sink{root: root, now: time.Now, loggers: make(map[string]*datedLogger)}
}

// Write appends one console line to a server's current daily log, rotating to a
// new file when the day changes.
func (s *Sink) Write(serverID, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	day := s.now().Format("2006-01-02")
	cur := s.loggers[serverID]
	if cur == nil || cur.day != day {
		if cur != nil {
			_ = cur.logger.Close()
		}
		lg, err := Open(filepath.Join(s.root, serverID, "console-"+day+".log"))
		if err != nil {
			return // best effort
		}
		cur = &datedLogger{day: day, logger: lg}
		s.loggers[serverID] = cur
	}
	_ = cur.logger.WriteLine(line)
}

// Close closes a server's current log file (e.g. when it is removed). A later
// Write reopens (and appends to) the appropriate dated file.
func (s *Sink) Close(serverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur := s.loggers[serverID]; cur != nil {
		_ = cur.logger.Close()
		delete(s.loggers, serverID)
	}
}

// CloseAll closes every open log file (on engine shutdown).
func (s *Sink) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cur := range s.loggers {
		_ = cur.logger.Close()
		delete(s.loggers, id)
	}
}
