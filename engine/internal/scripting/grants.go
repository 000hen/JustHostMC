package scripting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// GrantStore persists per-script permission grants as a small JSON file. It
// satisfies the Grants interface the registry consults.
type GrantStore struct {
	path string
	mu   sync.RWMutex
	data map[string][]int32 // script id -> granted PermissionKind enum values
}

// NewGrantStore loads (or starts) the grant file at path.
func NewGrantStore(path string) *GrantStore {
	gs := &GrantStore{path: path, data: map[string][]int32{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &gs.data)
	}
	return gs
}

func (g *GrantStore) save() error {
	if err := os.MkdirAll(filepath.Dir(g.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(g.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(g.path, b, 0o644)
}

// Granted implements Grants: it returns the persisted grant set for an id, and
// false when the user has made no decision yet (so defaults apply).
func (g *GrantStore) Granted(id string) (GrantSet, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	vals, ok := g.data[id]
	if !ok {
		return nil, false
	}
	set := make(GrantSet, len(vals))
	for _, v := range vals {
		set[mcmanagerv1.PermissionKind(v)] = true
	}
	return set, true
}

// Set records and persists the granted kinds for a script id.
func (g *GrantStore) Set(id string, kinds []mcmanagerv1.PermissionKind) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	vals := make([]int32, 0, len(kinds))
	for _, k := range kinds {
		vals = append(vals, int32(k))
	}
	g.data[id] = vals
	return g.save()
}

// Forget removes any grant record for an id (e.g. when a provider is removed).
func (g *GrantStore) Forget(id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.data[id]; !ok {
		return nil
	}
	delete(g.data, id)
	return g.save()
}
