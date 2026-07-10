package scripting

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/httpcache"
	"github.com/000hen/justhostmc/engine/internal/provider"
	lua "github.com/yuin/gopher-lua"
)

// scriptUserAgent identifies JustHostMC to upstream APIs for script HTTP calls.
// (Mirrors the unexported provider.userAgent.)
const scriptUserAgent = "JustHostMC (+https://github.com/000hen/justhostmc)"

// Host runs sandboxed Lua scripts. It carries the shared dependencies the host
// API needs (an HTTP client and a Java resolver); per-call state lives in an
// invocation. A Host is safe for concurrent use: every call gets a fresh LState.
type Host struct {
	client http.Client
	jre    provider.JavaResolver // runtime-only (java)
	jdk    provider.JavaResolver // full JDK (java + javac), e.g. for Spigot BuildTools
	cache  *httpcache.Cache      // jhmc.http_cache backend; unwired = network-only
}

// SetHTTPCache wires the disk-backed ETag cache behind jhmc.http_cache.
// Without it the function still works but every call hits the network.
func (h *Host) SetHTTPCache(c *httpcache.Cache) { h.cache = c }

// NewHost builds a Host. jre/jdk resolve (downloading if needed) a java.exe for a
// Java major version; either may be nil for scripts that never resolve Java.
func NewHost(client *http.Client, jre, jdk provider.JavaResolver) *Host {
	h := &Host{jre: jre, jdk: jdk}
	if client != nil {
		h.client = *client
	} else {
		h.client = *http.DefaultClient
	}
	return h
}

// KV is the per-script persistent key-value store the jhmc.store API binds to.
// Implemented by scriptdata.KVStore; scriptID scopes every call to the owning
// script so scripts cannot see each other's data.
type KV interface {
	Get(scriptID, key string) (string, bool)
	Set(scriptID, key, value string) error
	Delete(scriptID, key string) error
	Keys(scriptID string) []string
}

// invocation is the per-call state the host functions close over.
type invocation struct {
	ctx      context.Context
	host     *Host
	baseDir  string   // server dir; downloads + fs are confined here ("" disables fs)
	assetDir string   // dir holding files bundled with the script (e.g. a custom jar)
	granted  GrantSet // permissions the user granted this script
	report   func(provider.Progress)
	lastErr  error // structured error stashed before a Lua RaiseError, for mapping

	kv       KV                // jhmc.store backend (nil where no store is wired, e.g. providers)
	scriptID string            // id scoping jhmc.store access
	config   map[string]string // typed config values surfaced as ctx.config / jhmc.config (nil = none)
}

func (inv *invocation) emit(p provider.Progress) {
	if inv.report != nil {
		inv.report(p)
	}
}

// configTable builds a Lua table of the invocation's typed config values for a
// script's ctx.config (empty table when the invocation carries no config).
func (inv *invocation) configTable(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	for k, v := range inv.config {
		t.RawSetString(k, lua.LString(v))
	}
	return t
}

// require raises a Lua error (caught by PCall) when kind is not granted.
func (inv *invocation) require(L *lua.LState, kind mcmanagerv1.PermissionKind) {
	if !inv.granted.Has(kind) {
		inv.fail(L, fmt.Errorf("%w: %s", ErrPermissionDenied, PermName(kind)))
	}
}

// fail stashes a structured error so the adapter can recover it after PCall,
// then raises it as a Lua error.
func (inv *invocation) fail(L *lua.LState, err error) {
	inv.lastErr = err
	L.RaiseError("%s", err.Error())
}

// prepare builds a sandboxed state with the jhmc table installed and the script
// source loaded (which defines meta + the versions/install functions).
func (inv *invocation) prepare(src string) (*lua.LState, error) {
	L := newSandbox(inv.ctx)
	L.SetGlobal("jhmc", inv.newJHMC(L))
	if err := L.DoString(src); err != nil {
		L.Close()
		return nil, fmt.Errorf("load script: %w", err)
	}
	return L, nil
}

// newSandbox creates an LState with only the safe standard libraries and with
// the file/loader escape hatches removed. Raw os/io/package/debug are never
// opened, so scripts cannot touch the filesystem or spawn processes except
// through the permission-gated jhmc API.
func newSandbox(ctx context.Context) *lua.LState {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	if ctx != nil {
		L.SetContext(ctx)
	}
	for _, lib := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(lib.fn))
		L.Push(lua.LString(lib.name))
		L.Call(1, 0)
	}
	// Strip the base-lib functions that would read files or load arbitrary code.
	for _, name := range []string{
		"dofile", "loadfile", "load", "loadstring",
		"require", "module", "collectgarbage", "newproxy",
	} {
		L.SetGlobal(name, lua.LNil)
	}
	return L
}

// versions loads src and calls its global versions() function, returning the
// list of installable version strings.
func (inv *invocation) versions(src string) ([]string, error) {
	L, err := inv.prepare(src)
	if err != nil {
		return nil, err
	}
	defer L.Close()

	fn := L.GetGlobal("versions")
	if fn.Type() != lua.LTFunction {
		return nil, fmt.Errorf("script defines no versions() function")
	}
	if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}); err != nil {
		return nil, inv.mapErr(err)
	}
	ret := L.Get(-1)
	L.Pop(1)
	tbl, ok := ret.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("versions() must return a table of strings")
	}
	out := make([]string, 0, tbl.Len())
	tbl.ForEach(func(_, v lua.LValue) {
		if s, ok := v.(lua.LString); ok {
			out = append(out, string(s))
		}
	})
	return out, nil
}

// install loads src and calls install(ctx), returning the launch spec. dir is
// the server directory; downloads and fs access are confined to it.
func (inv *invocation) install(src, dir, version string) (provider.LaunchSpec, error) {
	inv.baseDir = dir
	L, err := inv.prepare(src)
	if err != nil {
		return provider.LaunchSpec{}, err
	}
	defer L.Close()

	fn := L.GetGlobal("install")
	if fn.Type() != lua.LTFunction {
		return provider.LaunchSpec{}, fmt.Errorf("script defines no install() function")
	}

	ictx := L.NewTable()
	ictx.RawSetString("dir", lua.LString(dir))
	ictx.RawSetString("version", lua.LString(version))
	ictx.RawSetString("step", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		frac := float64(L.OptNumber(2, -1))
		inv.emit(provider.Progress{Step: key, Fraction: frac})
		return 0
	}))
	ictx.RawSetString("log", L.NewFunction(func(L *lua.LState) int {
		inv.emit(provider.Progress{LogLine: L.CheckString(1)})
		return 0
	}))
	ictx.RawSetString("config", inv.configTable(L))

	if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, ictx); err != nil {
		return provider.LaunchSpec{}, inv.mapErr(err)
	}
	ret := L.Get(-1)
	L.Pop(1)
	spec, ok := ret.(*lua.LTable)
	if !ok {
		return provider.LaunchSpec{}, fmt.Errorf("install() must return a launch spec table")
	}

	major := 0
	if n, ok := spec.RawGetString("java_major").(lua.LNumber); ok {
		major = int(n)
	}
	var args []string
	if at, ok := spec.RawGetString("args").(*lua.LTable); ok {
		at.ForEach(func(_, v lua.LValue) {
			args = append(args, lua.LVAsString(v))
		})
	}
	return provider.LaunchSpec{
		JavaMajor: major,
		Args:      args,
		McVersion: strField(spec, "mc_version"),
		Loader:    strField(spec, "loader"),
	}, nil
}

// mapErr recovers the structured error stashed by fail(), falling back to the
// raw Lua error text. As a convenience it bridges the common script idiom
// error("version not found: …") to the provider sentinel the gRPC layer maps.
func (inv *invocation) mapErr(err error) error {
	if inv.lastErr != nil {
		return inv.lastErr
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "version not found") {
		return provider.ErrVersionNotFound
	}
	return err
}
