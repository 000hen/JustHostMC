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

	mu    sync.RWMutex
	order []string
	byID  map[string]*LuaShop
}

// NewShopSet builds an empty set. grants supplies persisted user permission
// decisions; keyFn may be nil (no shop keys available).
func NewShopSet(host *Host, grants Grants, keyFn func(shopID string) string) *ShopSet {
	return &ShopSet{host: host, grants: grants, keyFn: keyFn, byID: map[string]*LuaShop{}}
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
	if existing, ok := ss.byID[s.meta.ID]; ok && existing.builtin && !builtin {
		return nil, fmt.Errorf("%w: %s", ErrProviderIDConflict, s.meta.ID)
	}
	if _, ok := ss.byID[s.meta.ID]; !ok {
		ss.order = append(ss.order, s.meta.ID)
	}
	s.grantsFn = func() GrantSet { return ss.EffectiveGrants(s.meta.ID) }
	s.keyFn = func() string {
		if ss.keyFn == nil {
			return ""
		}
		return ss.keyFn(s.meta.ID)
	}
	ss.byID[s.meta.ID] = s
	return s, nil
}

// Get returns the shop registered under id.
func (ss *ShopSet) Get(id string) (*LuaShop, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	s, ok := ss.byID[id]
	return s, ok
}

// Remove forgets the shop id (built-ins are guarded at the service layer).
func (ss *ShopSet) Remove(id string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if _, ok := ss.byID[id]; !ok {
		return
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
