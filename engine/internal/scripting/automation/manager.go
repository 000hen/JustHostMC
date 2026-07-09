package automation

import (
	"context"
	"fmt"
	"sync"

	"github.com/000hen/justhostmc/engine/internal/players"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/scriptlog"
	lua "github.com/yuin/gopher-lua"
)

// ManagerConfig wires a Manager to its collaborators. Console, Control, Query,
// Players, Events and KV may all be nil (e.g. in tests): the corresponding
// script APIs then fail with a clear error instead of crashing.
type ManagerConfig struct {
	Host    *scripting.Host
	Grants  scripting.Grants
	Console Console
	Control ServerControl
	Logs    *scriptlog.LogBuffer
	Query   ServerQuery
	Players PlayerManager
	Events  PlayerEvents
	KV      scripting.KV
}

// Manager owns the set of automation scripts and the goroutines backing the
// enabled ones. It is safe for concurrent use.
type Manager struct {
	cfg  ManagerConfig
	logs *scriptlog.LogBuffer

	mu      sync.Mutex
	order   []string
	byID    map[string]*Automation
	running map[string]*runner
}

// NewManager builds an automation manager from cfg.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.Logs == nil {
		cfg.Logs = scriptlog.NewLogBuffer(0)
	}
	return &Manager{
		cfg:     cfg,
		logs:    cfg.Logs,
		byID:    map[string]*Automation{},
		running: map[string]*runner{},
	}
}

// Logs returns the engine-wide automation log ring buffer (for StreamLog).
func (m *Manager) Logs() *scriptlog.LogBuffer { return m.logs }

// AddSource compiles an automation script and registers it (disabled). builtin
// marks first-party scripts, whose declared permissions are granted by default.
func (m *Manager) AddSource(ctx context.Context, source string, builtin bool) (*Automation, error) {
	a, err := newAutomation(ctx, m.cfg.Host, source, builtin)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[a.meta.ID]; !ok {
		m.order = append(m.order, a.meta.ID)
	}
	m.byID[a.meta.ID] = a
	return a, nil
}

// Get returns the automation registered under id.
func (m *Manager) Get(id string) (*Automation, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.byID[id]
	return a, ok
}

// List returns all registered automations in insertion order.
func (m *Manager) List() []*Automation {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Automation, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.byID[id])
	}
	return out
}

// Enabled reports whether the automation id is currently running.
func (m *Manager) Enabled(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.running[id]
	return ok
}

// Remove disables (if running) and forgets the automation id.
func (m *Manager) Remove(id string) {
	m.Disable(id)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[id]; !ok {
		return
	}
	delete(m.byID, id)
	for i, x := range m.order {
		if x == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

// effectiveGrants resolves the permissions an automation may use right now.
func (m *Manager) effectiveGrants(a *Automation) scripting.GrantSet {
	if m.cfg.Grants != nil {
		if g, decided := m.cfg.Grants.Granted(a.meta.ID); decided {
			return g
		}
	}
	if a.builtin {
		return scripting.GrantSetFromList(a.meta.DeclaredKinds())
	}
	return nil
}

// EffectiveGrants returns the permissions automation id may use right now.
func (m *Manager) EffectiveGrants(id string) scripting.GrantSet {
	a, ok := m.Get(id)
	if !ok {
		return nil
	}
	return m.effectiveGrants(a)
}

// Enable starts the automation id: it runs the script once (registering its
// hooks) in a dedicated goroutine-serialized LState, then wires up the live
// console/server-event subscriptions that drive those hooks. Re-enabling an
// already-running script is a no-op.
func (m *Manager) Enable(id string) error {
	m.mu.Lock()
	if _, ok := m.running[id]; ok {
		m.mu.Unlock()
		return nil
	}
	a, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("automation %q not found", id)
	}
	granted := m.effectiveGrants(a)
	m.mu.Unlock()

	r, err := m.start(a, granted)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.running[id] = r
	m.mu.Unlock()
	return nil
}

// Disable stops the automation id, cancelling all its goroutines.
func (m *Manager) Disable(id string) {
	m.mu.Lock()
	r, ok := m.running[id]
	if ok {
		delete(m.running, id)
	}
	m.mu.Unlock()
	if ok {
		r.stop()
	}
}

// Shutdown disables every running automation.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	runners := make([]*runner, 0, len(m.running))
	for id, r := range m.running {
		runners = append(runners, r)
		delete(m.running, id)
	}
	m.mu.Unlock()
	for _, r := range runners {
		r.stop()
	}
}

// start builds the runner for one automation: it creates the LState, runs the
// job pump, executes the script's register()/top-level code so hooks register,
// and subscribes to console/server/player events for each hooked server.
func (m *Manager) start(a *Automation, granted scripting.GrantSet) (*runner, error) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &runner{
		cancel:     cancel,
		jobs:       make(chan func(*lua.LState), 64),
		logHooks:   map[string][]*lua.LFunction{},
		startHooks: map[string][]*lua.LFunction{},
		stopHooks:  map[string][]*lua.LFunction{},
		joinHooks:  map[string][]*lua.LFunction{},
		leaveHooks: map[string][]*lua.LFunction{},
	}

	inv := scripting.NewInvocation(scripting.InvocationConfig{
		Ctx:      ctx,
		Host:     m.cfg.Host,
		Granted:  granted,
		KV:       m.cfg.KV,
		ScriptID: a.meta.ID,
	})
	L := scripting.NewSandbox(ctx)
	L.SetGlobal("jhmc", inv.NewJHMC(L))
	api := &autoAPI{mgr: m, runner: r, inv: inv, id: a.meta.ID}
	L.SetGlobal("server", api.serverTable(L))
	api.installGlobals(L)

	if err := L.DoString(a.source); err != nil {
		L.Close()
		cancel()
		return nil, fmt.Errorf("load automation %q: %w", a.meta.ID, err)
	}
	// An optional register() entry point lets scripts set up hooks lazily.
	if fn := L.GetGlobal("register"); fn.Type() == lua.LTFunction {
		if err := L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}); err != nil {
			m.logErr(a.meta.ID, fmt.Errorf("register(): %w", err))
		}
	}

	// The job pump owns L exclusively; every callback runs here, serialized.
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer L.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-r.jobs:
				job(L)
			}
		}
	}()

	m.wireEvents(ctx, r)
	m.wirePlayerEvents(ctx, r)
	return r, nil
}

// wireEvents subscribes to each hooked server's console (for on_log/on_start/
// on_stop) once the script has registered its hooks.
func (m *Manager) wireEvents(ctx context.Context, r *runner) {
	if m.cfg.Console == nil {
		return
	}
	ids := map[string]struct{}{}
	for id := range r.logHooks {
		ids[id] = struct{}{}
	}
	for id := range r.startHooks {
		ids[id] = struct{}{}
	}
	for id := range r.stopHooks {
		ids[id] = struct{}{}
	}
	for id := range ids {
		m.watchServer(ctx, r, id)
	}
}

// wirePlayerEvents subscribes the runner to the player event bus and dispatches
// join/leave events to the script's on_join/on_leave hooks. The hook maps are
// snapshotted here (on start()'s goroutine, before any callback can run) so the
// dispatch goroutine never reads the runner's maps concurrently.
func (m *Manager) wirePlayerEvents(ctx context.Context, r *runner) {
	if m.cfg.Events == nil || (len(r.joinHooks) == 0 && len(r.leaveHooks) == 0) {
		return
	}
	joinHooks := snapshotHooks(r.joinHooks)
	leaveHooks := snapshotHooks(r.leaveHooks)
	live, cancel := m.cfg.Events.Subscribe()
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-live:
				if !ok {
					return
				}
				switch ev.Type {
				case players.EventJoin:
					if hooks := joinHooks[ev.ServerID]; len(hooks) > 0 {
						r.fire(ctx, hooks, lua.LString(ev.Name))
					}
				case players.EventLeave:
					if hooks := leaveHooks[ev.ServerID]; len(hooks) > 0 {
						r.fire(ctx, hooks, lua.LString(ev.Name))
					}
				}
			}
		}
	}()
}

func snapshotHooks(src map[string][]*lua.LFunction) map[string][]*lua.LFunction {
	out := make(map[string][]*lua.LFunction, len(src))
	for id, fns := range src {
		out[id] = append([]*lua.LFunction(nil), fns...)
	}
	return out
}

// watchServer streams a server's console lines to the runner, firing on_log per
// line and on_start/on_stop when the live channel opens/closes. The hook slices
// are snapshotted here (on start()'s goroutine, before any callback can run) so
// the watcher goroutine never reads the runner's maps concurrently.
func (m *Manager) watchServer(ctx context.Context, r *runner, id string) {
	_, live, cancel := m.cfg.Console.Subscribe(id)
	startHooks := r.startHooks[id]
	stopHooks := r.stopHooks[id]
	logHooks := r.logHooks[id]
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer cancel()
		r.fire(ctx, startHooks, lua.LString(id))
		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-live:
				if !ok {
					r.fire(ctx, stopHooks, lua.LString(id))
					return
				}
				r.fire(ctx, logHooks, lua.LString(line))
			}
		}
	}()
}

func (m *Manager) logErr(id string, err error) {
	m.logs.Append(id, "error: "+err.Error())
}
