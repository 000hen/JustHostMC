// Package logging persists server and automation logs to disk and enforces a
// shared retention policy over them: delete logs older than a number of days and
// keep their total size under a cap (PROMPT §15). Persisting logs makes failures
// and automation output findable after the fact.
package logging

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// logExt is the suffix Purge treats as a log file.
const logExt = ".log"

// Policy describes log retention. A zero value imposes no limits.
type Policy struct {
	KeepDays      int   // delete logs older than this many days (0 = no age limit)
	MaxTotalBytes int64 // cap on total bytes across all logs (0 = no size limit)
	ForceAll      bool  // immediately purge all log files
}

// Logger appends lines to a single log file. It is safe for concurrent use.
type Logger struct {
	mu sync.Mutex
	f  *os.File
}

// Open opens (creating parent dirs) path for appending and returns a Logger.
func Open(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{f: f}, nil
}

// WriteLine appends a single line (a trailing newline is added).
func (l *Logger) WriteLine(s string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return errors.New("logger is closed")
	}
	_, err := l.f.WriteString(s + "\n")
	return err
}

// Close closes the underlying file. Further writes return an error.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

type logFile struct {
	path string
	size int64
	mod  time.Time
}

// Purge enforces policy over every "*.log" file under root: first it removes
// files older than KeepDays, then (if still over MaxTotalBytes) it removes the
// oldest remaining files until the total is within the cap. It returns how many
// files were removed and how many bytes were freed. A missing root is not an
// error.
func Purge(root string, policy Policy, now time.Time) (removed int, freed int64, err error) {
	var files []logFile
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			if errors.Is(e, fs.ErrNotExist) {
				return nil
			}
			return e
		}
		if d.IsDir() || filepath.Ext(d.Name()) != logExt {
			return nil
		}
		info, ie := d.Info()
		if ie != nil {
			return nil // skip files we can't stat
		}
		files = append(files, logFile{path: path, size: info.Size(), mod: info.ModTime()})
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return removed, freed, walkErr
	}

	del := func(f logFile) {
		if rmErr := os.Remove(f.path); rmErr == nil {
			removed++
			freed += f.size
		}
	}

	// Phase 1: age.
	kept := files[:0]
	if policy.ForceAll {
		for _, f := range files {
			del(f)
		}
	} else if policy.KeepDays > 0 {
		cutoff := now.AddDate(0, 0, -policy.KeepDays)
		for _, f := range files {
			if f.mod.Before(cutoff) {
				del(f)
			} else {
				kept = append(kept, f)
			}
		}
	} else {
		kept = append(kept, files...)
	}

	// Phase 2: total size cap, oldest first.
	if policy.MaxTotalBytes > 0 {
		var total int64
		for _, f := range kept {
			total += f.size
		}
		if total > policy.MaxTotalBytes {
			sort.Slice(kept, func(i, j int) bool { return kept[i].mod.Before(kept[j].mod) })
			for _, f := range kept {
				if total <= policy.MaxTotalBytes {
					break
				}
				del(f)
				total -= f.size
			}
		}
	}

	return removed, freed, err
}
