package scripting

import (
	"context"
	"fmt"
	"sync"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	lua "github.com/yuin/gopher-lua"
)

// Console is the subset of the console hub the automation host needs. A running
// server's hub (*console.Hub) satisfies it. It is injected to avoid an import
// cycle between scripting and the console/grpc packages.
type Console interface {
	// Subscribe returns the buffered history plus a live channel of subsequent
	// lines; cancel unsubscribes (closing the live channel).
	Subscribe(id string) (history []string, live <-chan string, cancel func())
	// Send writes a command line to the server's stdin.
	Send(id, command string) error
}

// ServerControl is the subset of the server service the automation host needs to
// start/stop servers. *grpcsvc.ServerService satisfies it; injecting an
// interface keeps scripting free of a grpc import.
type ServerControl interface {
	Start(ctx context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.Empty, error)
	Stop(ctx context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.Empty, error)
}

// Automation is a compiled automation script: its declared metadata plus the
// source it runs. Enabling it spins up a long-lived sandboxed LState that
// registers handlers; disabling tears that down.
type Automation struct {
	meta    Meta
	source  string
	builtin bool
}

// Meta returns the script's declared metadata.
func (a *Automation) Meta() Meta { return a.meta }

// Builtin reports whether this is a first-party script (declared permissions
// granted by default; cannot be removed).
func (a *Automation) Builtin() bool { return a.builtin }

// newAutomation parses a script's meta in a throwaway sandbox. Automation
// scripts register hooks at top level, so the sandbox installs no-op stubs for
// the automation API (server.*/on_*/schedule/log/print): they let the source
// load and `meta` be read without actually wiring anything up.
func newAutomation(host *Host, source string, builtin bool) (*Automation, error) {
	inv := &invocation{ctx: context.Background(), host: host}
	L := newSandbox(inv.ctx)
	L.SetGlobal("jhmc", inv.newJHMC(L))
	installAutoStubs(L)
	if err := L.DoString(source); err != nil {
		L.Close()
		return nil, fmt.Errorf("load script: %w", err)
	}
	defer L.Close()
	meta, err := parseMeta(L)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScriptInvalid, err)
	}
	return &Automation{meta: meta, source: source, builtin: builtin}, nil
}

// installAutoStubs registers no-op versions of the automation globals so a
// script's top-level hook registrations don't fail during the meta-parse load.
func installAutoStubs(L *lua.LState) {
	noop := L.NewFunction(func(*lua.LState) int { return 0 })
	srv := L.NewTable()
	for _, name := range []string{"send", "start", "stop", "restart"} {
		srv.RawSetString(name, noop)
	}
	srv.RawSetString("logs", L.NewFunction(func(L *lua.LState) int {
		t := L.NewTable()
		t.RawSetString("lines", L.NewTable())
		L.Push(t)
		return 1
	}))
	L.SetGlobal("server", srv)
	for _, name := range []string{"on_log", "on_start", "on_stop", "schedule", "log", "print"} {
		L.SetGlobal(name, noop)
	}
}

// runner holds the live state of one enabled automation: its dedicated LState
// (gopher-lua is not concurrent-safe, so every callback is serialized through
// jobs), the cancel func that tears down all its goroutines, and the registered
// hooks discovered when the script first ran.
type runner struct {
	cancel     context.CancelFunc
	jobs       chan func(*lua.LState)
	wg         sync.WaitGroup
	logHooks   map[string][]*lua.LFunction // server id -> on_log callbacks
	startHooks map[string][]*lua.LFunction
	stopHooks  map[string][]*lua.LFunction
}

// Manager owns the set of automation scripts and the goroutines backing the
// enabled ones. It is safe for concurrent use.
type Manager struct {
	host    *Host
	grants  Grants
	console Console
	control ServerControl
	logs    *LogBuffer

	mu      sync.Mutex
	order   []string
	byID    map[string]*Automation
	running map[string]*runner
}

// NewManager builds an automation manager. console and control may be nil in
// tests that exercise only registration. grants supplies persisted user
// permission decisions (nil means built-ins get their declared permissions).
func NewManager(host *Host, grants Grants, console Console, control ServerControl, logs *LogBuffer) *Manager {
	if logs == nil {
		logs = NewLogBuffer(0)
	}
	return &Manager{
		host:    host,
		grants:  grants,
		console: console,
		control: control,
		logs:    logs,
		byID:    map[string]*Automation{},
		running: map[string]*runner{},
	}
}

// Logs returns the engine-wide automation log ring buffer (for StreamLog).
func (m *Manager) Logs() *LogBuffer { return m.logs }

// AddSource compiles an automation script and registers it (disabled). builtin
// marks first-party scripts, whose declared permissions are granted by default.
func (m *Manager) AddSource(source string, builtin bool) (*Automation, error) {
	a, err := newAutomation(m.host, source, builtin)
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
func (m *Manager) effectiveGrants(a *Automation) GrantSet {
	if m.grants != nil {
		if g, decided := m.grants.Granted(a.meta.ID); decided {
			return g
		}
	}
	if a.builtin {
		return grantSetFromList(a.meta.DeclaredKinds())
	}
	return nil
}

// EffectiveGrants returns the permissions automation id may use right now.
func (m *Manager) EffectiveGrants(id string) GrantSet {
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
// and subscribes to console/server events for each hooked server.
func (m *Manager) start(a *Automation, granted GrantSet) (*runner, error) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &runner{
		cancel:     cancel,
		jobs:       make(chan func(*lua.LState), 64),
		logHooks:   map[string][]*lua.LFunction{},
		startHooks: map[string][]*lua.LFunction{},
		stopHooks:  map[string][]*lua.LFunction{},
	}

	inv := &invocation{ctx: ctx, host: m.host, granted: granted}
	L := newSandbox(ctx)
	L.SetGlobal("jhmc", inv.newJHMC(L))
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
	return r, nil
}

// wireEvents subscribes to each hooked server's console (for on_log/on_start/
// on_stop) once the script has registered its hooks.
func (m *Manager) wireEvents(ctx context.Context, r *runner) {
	if m.console == nil {
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

// watchServer streams a server's console lines to the runner, firing on_log per
// line and on_start/on_stop when the live channel opens/closes. The hook slices
// are snapshotted here (on start()'s goroutine, before any callback can run) so
// the watcher goroutine never reads the runner's maps concurrently.
func (m *Manager) watchServer(ctx context.Context, r *runner, id string) {
	_, live, cancel := m.console.Subscribe(id)
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

// stop cancels the runner and waits for all its goroutines to drain.
func (r *runner) stop() {
	r.cancel()
	r.wg.Wait()
}

// fire enqueues a call to each callback with the given arg onto the job pump.
func (r *runner) fire(ctx context.Context, fns []*lua.LFunction, arg lua.LValue) {
	for _, fn := range fns {
		fn := fn
		select {
		case <-ctx.Done():
			return
		case r.jobs <- func(L *lua.LState) {
			if err := L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, arg); err != nil {
				_ = err // errors surfaced via print/log are captured; ignore the raw raise here
			}
		}:
		}
	}
}

// autoAPI binds the permission-gated server.* / on_* / schedule API to one
// running automation.
type autoAPI struct {
	mgr    *Manager
	runner *runner
	inv    *invocation
	id     string
}

// serverTable builds the `server` table: send/logs/start/stop/restart.
func (a *autoAPI) serverTable(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("send", L.NewFunction(a.send))
	t.RawSetString("logs", L.NewFunction(a.logs))
	t.RawSetString("start", L.NewFunction(a.start))
	t.RawSetString("stop", L.NewFunction(a.stop))
	t.RawSetString("restart", L.NewFunction(a.restart))
	return t
}

// installGlobals registers on_log/on_start/on_stop/schedule plus a print/log
// that captures output into the engine-wide automation log.
func (a *autoAPI) installGlobals(L *lua.LState) {
	L.SetGlobal("on_log", L.NewFunction(a.onLog))
	L.SetGlobal("on_start", L.NewFunction(a.onStart))
	L.SetGlobal("on_stop", L.NewFunction(a.onStop))
	L.SetGlobal("schedule", L.NewFunction(a.schedule))
	logFn := L.NewFunction(a.log)
	L.SetGlobal("log", logFn)
	L.SetGlobal("print", logFn)
}

func (a *autoAPI) send(L *lua.LState) int {
	a.inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_WRITE)
	id := L.CheckString(1)
	cmd := L.CheckString(2)
	if a.mgr.console == nil {
		a.inv.fail(L, fmt.Errorf("server.send: no console available"))
		return 0
	}
	if err := a.mgr.console.Send(id, cmd); err != nil {
		a.inv.fail(L, err)
		return 0
	}
	return 0
}

func (a *autoAPI) logs(L *lua.LState) int {
	a.inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_READ)
	id := L.CheckString(1)
	if a.mgr.console == nil {
		a.inv.fail(L, fmt.Errorf("server.logs: no console available"))
		return 0
	}
	history, _, cancel := a.mgr.console.Subscribe(id)
	cancel()
	out := L.NewTable()
	lines := L.NewTable()
	for _, line := range history {
		lines.Append(lua.LString(line))
	}
	out.RawSetString("lines", lines)
	L.Push(out)
	return 1
}

func (a *autoAPI) start(L *lua.LState) int { return a.control(L, "start") }
func (a *autoAPI) stop(L *lua.LState) int  { return a.control(L, "stop") }

func (a *autoAPI) restart(L *lua.LState) int {
	a.inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_SERVER_CONTROL)
	id := L.CheckString(1)
	if a.mgr.control == nil {
		a.inv.fail(L, fmt.Errorf("server.restart: no server control available"))
		return 0
	}
	if _, err := a.mgr.control.Stop(a.inv.ctx, &mcmanagerv1.ServerId{Id: id}); err != nil {
		a.inv.fail(L, err)
		return 0
	}
	if _, err := a.mgr.control.Start(a.inv.ctx, &mcmanagerv1.ServerId{Id: id}); err != nil {
		a.inv.fail(L, err)
		return 0
	}
	return 0
}

func (a *autoAPI) control(L *lua.LState, action string) int {
	a.inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_SERVER_CONTROL)
	id := L.CheckString(1)
	if a.mgr.control == nil {
		a.inv.fail(L, fmt.Errorf("server.%s: no server control available", action))
		return 0
	}
	var err error
	if action == "start" {
		_, err = a.mgr.control.Start(a.inv.ctx, &mcmanagerv1.ServerId{Id: id})
	} else {
		_, err = a.mgr.control.Stop(a.inv.ctx, &mcmanagerv1.ServerId{Id: id})
	}
	if err != nil {
		a.inv.fail(L, err)
	}
	return 0
}

func (a *autoAPI) onLog(L *lua.LState) int {
	a.inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_READ)
	id := L.CheckString(1)
	fn := L.CheckFunction(2)
	a.runner.logHooks[id] = append(a.runner.logHooks[id], fn)
	return 0
}

func (a *autoAPI) onStart(L *lua.LState) int {
	id := L.CheckString(1)
	fn := L.CheckFunction(2)
	a.runner.startHooks[id] = append(a.runner.startHooks[id], fn)
	return 0
}

func (a *autoAPI) onStop(L *lua.LState) int {
	id := L.CheckString(1)
	fn := L.CheckFunction(2)
	a.runner.stopHooks[id] = append(a.runner.stopHooks[id], fn)
	return 0
}

// schedule runs fn every `seconds` seconds until the automation is disabled.
func (a *autoAPI) schedule(L *lua.LState) int {
	a.inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_SCHEDULE)
	seconds := float64(L.CheckNumber(1))
	fn := L.CheckFunction(2)
	if seconds <= 0 {
		a.inv.fail(L, fmt.Errorf("schedule: interval must be positive"))
		return 0
	}
	r := a.runner
	ctx := a.inv.ctx
	d := time.Duration(seconds * float64(time.Second))
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		t := time.NewTicker(d)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.fire(ctx, []*lua.LFunction{fn}, lua.LNil)
			}
		}
	}()
	return 0
}

// log appends a print/log line from the script to the engine-wide automation log.
func (a *autoAPI) log(L *lua.LState) int {
	n := L.GetTop()
	parts := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		parts = append(parts, L.ToStringMeta(L.Get(i)).String())
	}
	a.mgr.logs.Append(a.id, joinSpace(parts))
	return 0
}

func joinSpace(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}
