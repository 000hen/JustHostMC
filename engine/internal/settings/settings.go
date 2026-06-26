// Package settings persists user-configurable engine settings (currently the log
// retention policy) as a small JSON file under the app data directory. A missing
// file yields sensible defaults so a fresh install just works (PROMPT §15).
package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/000hen/justhostmc/engine/internal/logging"
)

// Settings holds all persisted engine settings.
type Settings struct {
	KeepLogDays      int   `json:"keep_log_days"`       // delete logs older than this many days (0 = no age limit)
	MaxLogTotalBytes int64 `json:"max_log_total_bytes"` // cap on total log bytes (0 = no size limit)
	UseDocker        bool  `json:"use_docker"`          // opt-in: run servers in Docker when available (PROMPT §10.7)
}

// Defaults returns the settings used when nothing has been saved yet: keep two
// weeks of logs and cap their total size at 256 MiB.
func Defaults() Settings {
	return Settings{KeepLogDays: 14, MaxLogTotalBytes: 256 << 20}
}

// Policy maps the persisted retention fields to a logging.Policy.
func (s Settings) Policy() logging.Policy {
	return logging.Policy{KeepDays: s.KeepLogDays, MaxTotalBytes: s.MaxLogTotalBytes}
}

// Store reads and writes Settings to a JSON file. It is safe for concurrent use.
type Store struct {
	mu   sync.Mutex
	path string
}

// NewStore returns a Store backed by path.
func NewStore(path string) *Store { return &Store{path: path} }

// Load returns the stored settings, or Defaults() if the file does not exist.
func (s *Store) Load() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), err
	}
	out := Defaults()
	if err := json.Unmarshal(b, &out); err != nil {
		return Defaults(), err
	}
	return out, nil
}

// Save writes settings atomically (write to a temp file, then rename).
func (s *Store) Save(v Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
