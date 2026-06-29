package scripting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// EnabledStore persists which automation scripts the user has enabled, as a
// small JSON file. The manager consults it at startup to decide which scripts to
// run, and updates it when the user toggles a script.
type EnabledStore struct {
	path string
	mu   sync.RWMutex
	data map[string]bool // script id -> enabled
}

// NewEnabledStore loads (or starts) the enabled-state file at path.
func NewEnabledStore(path string) *EnabledStore {
	es := &EnabledStore{path: path, data: map[string]bool{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &es.data)
	}
	return es
}

func (e *EnabledStore) save() error {
	if err := os.MkdirAll(filepath.Dir(e.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(e.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(e.path, b, 0o644)
}

// IsEnabled reports whether the user has enabled the automation id.
func (e *EnabledStore) IsEnabled(id string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.data[id]
}

// EnabledIDs returns every id currently marked enabled.
func (e *EnabledStore) EnabledIDs() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]string, 0, len(e.data))
	for id, on := range e.data {
		if on {
			out = append(out, id)
		}
	}
	return out
}

// Set records and persists the enabled state for an id.
func (e *EnabledStore) Set(id string, enabled bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.data[id] = enabled
	return e.save()
}

// Forget removes any enabled record for an id (e.g. when a script is removed).
func (e *EnabledStore) Forget(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.data[id]; !ok {
		return nil
	}
	delete(e.data, id)
	return e.save()
}
