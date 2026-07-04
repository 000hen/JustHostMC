package automation

import (
	"fmt"

	"github.com/000hen/justhostmc/engine/internal/scripting"
	lua "github.com/yuin/gopher-lua"
)

// Automation is a compiled automation script: its declared metadata plus the
// source it runs. Enabling it spins up a long-lived sandboxed LState that
// registers handlers; disabling tears that down.
type Automation struct {
	meta    scripting.Meta
	source  string
	builtin bool
}

// Meta returns the script's declared metadata.
func (a *Automation) Meta() scripting.Meta { return a.meta }

// Builtin reports whether this is a first-party script (declared permissions
// granted by default; cannot be removed).
func (a *Automation) Builtin() bool { return a.builtin }

// newAutomation parses a script's meta in a throwaway sandbox. Automation
// scripts register hooks at top level, so the sandbox installs no-op stubs for
// the automation API (server.*/on_*/schedule/log/print): they let the source
// load and `meta` be read without actually wiring anything up.
func newAutomation(host *scripting.Host, source string, builtin bool) (*Automation, error) {
	inv := scripting.NewInvocation(scripting.InvocationConfig{Host: host})
	L := scripting.NewSandbox(inv.Ctx())
	L.SetGlobal("jhmc", inv.NewJHMC(L))
	installAutoStubs(L)
	if err := L.DoString(source); err != nil {
		L.Close()
		return nil, fmt.Errorf("load script: %w", err)
	}
	defer L.Close()
	meta, err := scripting.ParseMeta(L)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", scripting.ErrScriptInvalid, err)
	}
	return &Automation{meta: meta, source: source, builtin: builtin}, nil
}

// installAutoStubs registers no-op versions of the automation globals so a
// script's top-level hook registrations don't fail during the meta-parse load.
func installAutoStubs(L *lua.LState) {
	noop := L.NewFunction(func(*lua.LState) int { return 0 })
	emptyList := L.NewFunction(func(L *lua.LState) int {
		L.Push(L.NewTable())
		return 1
	})
	srv := L.NewTable()
	for _, name := range []string{"send", "start", "stop", "restart", "kick", "ban", "unban"} {
		srv.RawSetString(name, noop)
	}
	for _, name := range []string{"list", "players", "bans"} {
		srv.RawSetString(name, emptyList)
	}
	srv.RawSetString("info", noop)
	srv.RawSetString("logs", L.NewFunction(func(L *lua.LState) int {
		t := L.NewTable()
		t.RawSetString("lines", L.NewTable())
		L.Push(t)
		return 1
	}))
	L.SetGlobal("server", srv)
	for _, name := range []string{
		"on_log", "on_start", "on_stop", "on_join", "on_leave",
		"schedule", "sleep", "log", "print",
	} {
		L.SetGlobal(name, noop)
	}
}
