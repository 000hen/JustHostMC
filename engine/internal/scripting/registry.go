package scripting

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/000hen/justhostmc/engine/internal/provider"
)

// Grants is the persistence the registry consults for user permission grants.
// A5's permission store satisfies it; nil means "use defaults".
type Grants interface {
	// Granted returns the set of permission kinds granted to the script id, or
	// nil if the user has made no decision yet.
	Granted(id string) (GrantSet, bool)
}

// Entry is one installed provider: its metadata + the provider.Provider that
// installs it. Most entries wrap a LuaProvider; tests may inject their own.
type Entry struct {
	Meta     Meta
	Provider provider.Provider
	Builtin  bool
}

// ConfigReader is the read side of the typed-config store the registry injects
// into provider scripts. *ConfigStore implements it directly; wrap it with
// NewFallbackConfigReader to layer a lazily-computed default onto one key.
type ConfigReader interface {
	// Values returns the stored config overrides for a script id (a possibly
	// empty, freely-mutable map).
	Values(id string) map[string]string
}

// Registry holds the installed providers keyed by id, in insertion order, and
// resolves each provider's effective grants (persisted grant, or — for trusted
// built-ins — all declared permissions by default).
type Registry struct {
	mu      sync.RWMutex
	host    *Host
	grants  Grants
	config  ConfigReader
	order   []string
	byID    map[string]*Entry
	aliases map[string]string // old id -> canonical id (Meta.Aliases)
}

// NewRegistry builds an empty registry. host runs the scripts; grants (optional)
// supplies persisted user permission decisions.
func NewRegistry(host *Host, grants Grants) *Registry {
	return &Registry{host: host, grants: grants, byID: map[string]*Entry{}, aliases: map[string]string{}}
}

// SetConfigStore wires the typed-config reader the registry hands to its
// scripts. A plain *ConfigStore is the common case; a wrapper may layer defaults.
func (r *Registry) SetConfigStore(cs ConfigReader) { r.config = cs }

// configValues returns the stored config overrides for id (nil when no store).
func (r *Registry) configValues(id string) map[string]string {
	if r.config == nil {
		return nil
	}
	return r.config.Values(id)
}

// AddSource compiles a Lua provider script (with no bundled assets) and
// registers it. builtin marks first-party scripts, whose declared permissions
// are granted by default.
func (r *Registry) AddSource(ctx context.Context, source string, builtin bool) (*Entry, error) {
	return r.addWithConfig(ctx, source, builtin, "", nil)
}

// AddSourceWithConfig registers a provider script whose typed config is read
// from cfg instead of the registry's own store. It is used for a merged source
// script's hidden provider role, whose config lives in the shared shop store so
// both roles read and write one entry.
func (r *Registry) AddSourceWithConfig(ctx context.Context, source string, builtin bool, cfg ConfigReader) (*Entry, error) {
	return r.addWithConfig(ctx, source, builtin, "", cfg)
}

// AddProviderDir registers the provider whose script is dir/provider.lua, with
// dir as its asset directory (so the script can use a bundled custom jar).
func (r *Registry) AddProviderDir(ctx context.Context, dir string, builtin bool) (*Entry, error) {
	src, err := os.ReadFile(filepath.Join(dir, "provider.lua"))
	if err != nil {
		return nil, err
	}
	return r.addWithConfig(ctx, string(src), builtin, dir, nil)
}

func (r *Registry) addWithConfig(ctx context.Context, source string, builtin bool, assetDir string, cfg ConfigReader) (*Entry, error) {
	lp, err := newLuaProvider(ctx, r.host, source, builtin, assetDir)
	if err != nil {
		return nil, err
	}
	id := lp.meta.ID
	lp.grantsFn = func() GrantSet { return r.effectiveGrants(id, builtin, lp.meta) }
	if cfg != nil {
		lp.configFn = func() map[string]string { return cfg.Values(id) }
	} else {
		lp.configFn = func() map[string]string { return r.configValues(id) }
	}
	e := &Entry{Meta: lp.meta, Provider: lp, Builtin: builtin}
	if err := r.put(id, e); err != nil {
		return nil, err
	}
	return e, nil
}

// AddEntry registers a pre-built entry (used by tests and non-Lua providers).
func (r *Registry) AddEntry(e *Entry) error { return r.put(e.Meta.ID, e) }

func (r *Registry) put(id string, e *Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if owner, ok := r.aliases[id]; ok {
		return fmt.Errorf("%w: canonical id %q is already an alias for %q", ErrProviderIDConflict, id, owner)
	}
	existing, exists := r.byID[id]
	if exists && existing.Builtin && !e.Builtin {
		return fmt.Errorf("%w: %q", ErrProviderIDConflict, id)
	}
	for _, alias := range e.Meta.Aliases {
		if _, ok := r.byID[alias]; ok {
			return fmt.Errorf("%w: alias %q is already a canonical id", ErrProviderIDConflict, alias)
		}
		if owner, ok := r.aliases[alias]; ok && owner != id {
			return fmt.Errorf("%w: alias %q is already assigned to %q", ErrProviderIDConflict, alias, owner)
		}
	}

	if !exists {
		r.order = append(r.order, id)
	} else {
		for _, alias := range existing.Meta.Aliases {
			if r.aliases[alias] == id {
				delete(r.aliases, alias)
			}
		}
	}
	r.byID[id] = e
	for _, a := range e.Meta.Aliases {
		r.aliases[a] = id
	}
	return nil
}

// Remove deletes a provider by id.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok {
		return
	}
	for _, a := range e.Meta.Aliases {
		if r.aliases[a] == id {
			delete(r.aliases, a)
		}
	}
	delete(r.byID, id)
	for i, x := range r.order {
		if x == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// Get returns the entry for an id, resolving an alias to its canonical entry.
func (r *Registry) Get(id string) (*Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.byID[id]; ok {
		return e, true
	}
	if canonical, ok := r.aliases[id]; ok {
		e, ok := r.byID[canonical]
		return e, ok
	}
	return nil, false
}

// Provider returns just the installer for an id (convenience for the gRPC layer).
func (r *Registry) Provider(id string) (provider.Provider, bool) {
	if e, ok := r.Get(id); ok {
		return e.Provider, true
	}
	return nil, false
}

// List returns all entries in insertion order.
func (r *Registry) List() []*Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Entry, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.byID[id])
	}
	return out
}

// effectiveGrants resolves the permissions a script may use right now.
func (r *Registry) effectiveGrants(id string, builtin bool, meta Meta) GrantSet {
	if r.grants != nil {
		if g, decided := r.grants.Granted(id); decided {
			return g
		}
	}
	if builtin {
		// First-party scripts are trusted: grant their declared permissions
		// until the user explicitly revokes one.
		return grantSetFromList(meta.DeclaredKinds())
	}
	return nil
}

// EffectiveGrants returns the permissions the provider id may use right now
// (persisted grant, or built-in defaults). Empty for an unknown id.
func (r *Registry) EffectiveGrants(id string) GrantSet {
	e, ok := r.Get(id)
	if !ok {
		return nil
	}
	return r.effectiveGrants(id, e.Builtin, e.Meta)
}

// MustAddSource is a test/bootstrap helper that panics on a bad script.
func (r *Registry) MustAddSource(ctx context.Context, source string, builtin bool) *Entry {
	e, err := r.AddSource(ctx, source, builtin)
	if err != nil {
		panic(fmt.Sprintf("scripting: bad builtin script: %v", err))
	}
	return e
}
