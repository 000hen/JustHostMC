package scripting

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// ParserSet holds the installed mod/plugin metadata parsers (built-in and
// user-imported) in registration order. It is the parser analog of Registry
// and is safe for concurrent use.
type ParserSet struct {
	host   *Host
	grants Grants

	mu    sync.RWMutex
	order []string
	byID  map[string]*LuaParser
}

// NewParserSet builds an empty set. grants supplies persisted user permission
// decisions (nil means built-ins get their declared permissions).
func NewParserSet(host *Host, grants Grants) *ParserSet {
	return &ParserSet{host: host, grants: grants, byID: map[string]*LuaParser{}}
}

// AddSource compiles a parser script and registers it. A user parser (builtin
// false) may not take over a built-in's id.
func (ps *ParserSet) AddSource(source string, builtin bool) (*LuaParser, error) {
	p, err := newLuaParser(ps.host, source, builtin)
	if err != nil {
		return nil, err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if existing, ok := ps.byID[p.meta.ID]; ok && existing.builtin && !builtin {
		return nil, fmt.Errorf("%w: %s", ErrProviderIDConflict, p.meta.ID)
	}
	if _, ok := ps.byID[p.meta.ID]; !ok {
		ps.order = append(ps.order, p.meta.ID)
	}
	p.grantsFn = func() GrantSet { return ps.EffectiveGrants(p.meta.ID) }
	ps.byID[p.meta.ID] = p
	return p, nil
}

// Get returns the parser registered under id.
func (ps *ParserSet) Get(id string) (*LuaParser, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	p, ok := ps.byID[id]
	return p, ok
}

// Remove forgets the parser id (built-ins are guarded at the service layer).
func (ps *ParserSet) Remove(id string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if _, ok := ps.byID[id]; !ok {
		return
	}
	delete(ps.byID, id)
	for i, x := range ps.order {
		if x == id {
			ps.order = append(ps.order[:i], ps.order[i+1:]...)
			break
		}
	}
}

// List returns all registered parsers in registration order (built-ins first,
// as they are loaded first).
func (ps *ParserSet) List() []*LuaParser {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	out := make([]*LuaParser, 0, len(ps.order))
	for _, id := range ps.order {
		out = append(out, ps.byID[id])
	}
	return out
}

// EffectiveGrants resolves the permissions parser id may use right now: a
// persisted user decision wins; otherwise built-ins get their declared
// permissions and user parsers get nothing.
func (ps *ParserSet) EffectiveGrants(id string) GrantSet {
	ps.mu.RLock()
	p, ok := ps.byID[id]
	ps.mu.RUnlock()
	if !ok {
		return nil
	}
	if ps.grants != nil {
		if g, decided := ps.grants.Granted(id); decided {
			return g
		}
	}
	if p.builtin {
		return grantSetFromList(p.meta.DeclaredKinds())
	}
	return nil
}

// ParseJarCandidates runs every installed parser against one jar in
// registration order and returns all successful matches. A single broken parser
// is logged and skipped so it can never break mod listing. Errors from built-in
// parsers are returned only when no parser recognizes the jar, allowing the
// caller to show a failed row without losing the rest of the list. Errors from
// optional user parsers remain log-only because a broken extension must not
// condemn an otherwise valid, unrecognized archive.
func (ps *ParserSet) ParseJarCandidates(ctx context.Context, serverDir, jarRel string) ([]ModParseCandidate, error) {
	var firstBuiltinErr error
	var candidates []ModParseCandidate
	for _, p := range ps.List() {
		meta, matched, err := p.Parse(ctx, serverDir, jarRel)
		if err != nil {
			log.Printf("[WARN] mod parser %q: %s: %v", p.meta.ID, jarRel, err)
			if p.builtin && firstBuiltinErr == nil {
				firstBuiltinErr = fmt.Errorf("%s: %w", p.meta.ID, err)
			}
			continue
		}
		if matched {
			candidates = append(candidates, ModParseCandidate{Meta: meta, ParserID: p.meta.ID})
		}
	}
	if len(candidates) > 0 {
		return candidates, nil
	}
	return nil, firstBuiltinErr
}

// ParseJar preserves the legacy "first match wins" parser contract for callers
// that do not have server context. Prefer ParseJarCandidates when the caller can
// rank candidates against a concrete server provider/version.
func (ps *ParserSet) ParseJar(ctx context.Context, serverDir, jarRel string) (ModMeta, string, bool, error) {
	candidates, err := ps.ParseJarCandidates(ctx, serverDir, jarRel)
	if err != nil {
		return ModMeta{}, "", false, err
	}
	if len(candidates) == 0 {
		return ModMeta{}, "", false, nil
	}
	first := candidates[0]
	return first.Meta, first.ParserID, true, nil
}
