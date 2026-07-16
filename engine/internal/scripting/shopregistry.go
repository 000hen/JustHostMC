package scripting

import (
	"context"
	"fmt"
	"sync"
)

// ShopSet holds the installed mod-shop scripts (built-in and user-imported)
// in registration order. It is the shop analog of ParserSet and is safe for
// concurrent use.
type ShopSet struct {
	host   *Host
	grants Grants
	// keyFn resolves the API key for a shop id ("" = none). Wired by the
	// service layer to settings + baked-in defaults.
	keyFn func(shopID string) string

	config *ConfigStore

	mu      sync.RWMutex
	order   []string
	byID    map[string]*LuaShop
	aliases map[string]string // old id -> canonical id (Meta.Aliases)
}

// NewShopSet builds an empty set. grants supplies persisted user permission
// decisions; keyFn may be nil (no shop keys available).
func NewShopSet(host *Host, grants Grants, keyFn func(shopID string) string) *ShopSet {
	return &ShopSet{host: host, grants: grants, keyFn: keyFn, byID: map[string]*LuaShop{}, aliases: map[string]string{}}
}

// SetConfigStore wires the typed-config store the set hands to its shop scripts.
func (ss *ShopSet) SetConfigStore(cs *ConfigStore) { ss.config = cs }

func (ss *ShopSet) configValues(id string) map[string]string {
	if ss.config == nil {
		return nil
	}
	return ss.config.Values(id)
}

// AddSource compiles a shop script and registers it. A user shop (builtin
// false) may not take over a built-in's id.
func (ss *ShopSet) AddSource(ctx context.Context, source string, builtin bool) (*LuaShop, error) {
	s, err := newLuaShop(ctx, ss.host, source, builtin)
	if err != nil {
		return nil, err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	id := s.meta.ID
	if owner, ok := ss.aliases[id]; ok {
		return nil, fmt.Errorf("%w: canonical id %q is already an alias for %q", ErrProviderIDConflict, id, owner)
	}
	existing, exists := ss.byID[id]
	if exists && existing.builtin && !builtin {
		return nil, fmt.Errorf("%w: %s", ErrProviderIDConflict, s.meta.ID)
	}
	for _, alias := range s.meta.Aliases {
		if _, ok := ss.byID[alias]; ok {
			return nil, fmt.Errorf("%w: alias %q is already a canonical id", ErrProviderIDConflict, alias)
		}
		if owner, ok := ss.aliases[alias]; ok && owner != id {
			return nil, fmt.Errorf("%w: alias %q is already assigned to %q", ErrProviderIDConflict, alias, owner)
		}
	}

	if !exists {
		ss.order = append(ss.order, id)
	} else {
		for _, alias := range existing.meta.Aliases {
			if ss.aliases[alias] == id {
				delete(ss.aliases, alias)
			}
		}
	}
	s.grantsFn = func() GrantSet { return ss.EffectiveGrants(s.meta.ID) }
	s.keyFn = func() string {
		if ss.keyFn == nil {
			return ""
		}
		return ss.keyFn(s.meta.ID)
	}
	s.configFn = func() map[string]string { return ss.configValues(s.meta.ID) }
	ss.byID[id] = s
	for _, a := range s.meta.Aliases {
		ss.aliases[a] = id
	}
	return s, nil
}

// Get returns the shop registered under id, resolving an alias to its canonical
// shop.
func (ss *ShopSet) Get(id string) (*LuaShop, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if s, ok := ss.byID[id]; ok {
		return s, true
	}
	if canonical, ok := ss.aliases[id]; ok {
		s, ok := ss.byID[canonical]
		return s, ok
	}
	return nil, false
}

// Remove forgets the shop id (built-ins are guarded at the service layer).
func (ss *ShopSet) Remove(id string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	s, ok := ss.byID[id]
	if !ok {
		return
	}
	for _, a := range s.meta.Aliases {
		if ss.aliases[a] == id {
			delete(ss.aliases, a)
		}
	}
	delete(ss.byID, id)
	for i, x := range ss.order {
		if x == id {
			ss.order = append(ss.order[:i], ss.order[i+1:]...)
			break
		}
	}
}

// List returns all registered shops in registration order (built-ins first).
func (ss *ShopSet) List() []*LuaShop {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	out := make([]*LuaShop, 0, len(ss.order))
	for _, id := range ss.order {
		out = append(out, ss.byID[id])
	}
	return out
}

// EffectiveGrants resolves the permissions shop id may use right now: a
// persisted user decision wins; otherwise built-ins get their declared
// permissions and user shops get nothing.
func (ss *ShopSet) EffectiveGrants(id string) GrantSet {
	ss.mu.RLock()
	s, ok := ss.byID[id]
	ss.mu.RUnlock()
	if !ok {
		return nil
	}
	if ss.grants != nil {
		if g, decided := ss.grants.Granted(id); decided {
			return g
		}
	}
	if s.builtin {
		return grantSetFromList(s.meta.DeclaredKinds())
	}
	return nil
}
