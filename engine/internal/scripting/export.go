package scripting

// This file is the deliberate surface the scripting/automation subpackage
// builds on: it re-exports the sandbox, the per-call invocation state, and the
// meta/grant helpers that are otherwise package-private. Everything here is
// engine-internal (under internal/), not a public API.

import (
	"context"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	lua "github.com/yuin/gopher-lua"
)

// Invocation is the exported handle to the per-call state host functions close
// over. Subpackages use it to install the jhmc API into their own sandboxes
// with the same permission gating and error mapping as provider scripts.
type Invocation struct {
	inv *invocation
}

// InvocationConfig configures a long-lived invocation for a script runtime.
type InvocationConfig struct {
	Ctx     context.Context
	Host    *Host
	Granted GrantSet
	// KV + ScriptID bind jhmc.store; leave zero for scripts without storage.
	KV       KV
	ScriptID string
	// BaseDir confines jhmc.fs; empty disables filesystem access.
	BaseDir string
	// Config supplies the script's typed config values (surfaced as jhmc.config);
	// nil when the script declares no config or none is stored.
	Config map[string]string
}

// NewInvocation builds an invocation from cfg.
func NewInvocation(cfg InvocationConfig) *Invocation {
	ctx := cfg.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return &Invocation{inv: &invocation{
		ctx:      ctx,
		host:     cfg.Host,
		granted:  cfg.Granted,
		kv:       cfg.KV,
		scriptID: cfg.ScriptID,
		baseDir:  cfg.BaseDir,
		config:   cfg.Config,
	}}
}

// Ctx returns the invocation's context (cancelled when the runtime stops).
func (i *Invocation) Ctx() context.Context { return i.inv.ctx }

// NewJHMC builds the `jhmc` table bound to this invocation.
func (i *Invocation) NewJHMC(L *lua.LState) *lua.LTable { return i.inv.newJHMC(L) }

// Require raises a Lua error (caught by PCall) when kind is not granted.
func (i *Invocation) Require(L *lua.LState, kind mcmanagerv1.PermissionKind) {
	i.inv.require(L, kind)
}

// Fail stashes a structured error and raises it as a Lua error.
func (i *Invocation) Fail(L *lua.LState, err error) { i.inv.fail(L, err) }

// LastErr returns the structured error stashed by the most recent Fail/Require,
// for mapping after a PCall returns.
func (i *Invocation) LastErr() error { return i.inv.lastErr }

// NewSandbox creates an LState with only the safe standard libraries and the
// file/loader escape hatches removed (see newSandbox).
func NewSandbox(ctx context.Context) *lua.LState { return newSandbox(ctx) }

// ParseMeta reads and validates the global `meta` table of a loaded script.
func ParseMeta(L *lua.LState) (Meta, error) { return parseMeta(L) }

// GrantSetFromList builds a GrantSet from a slice of kinds.
func GrantSetFromList(kinds []mcmanagerv1.PermissionKind) GrantSet {
	return grantSetFromList(kinds)
}
