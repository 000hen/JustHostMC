package scripting

import (
	"context"
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// ModMeta is the uniform metadata a parser script extracts from a mod/plugin
// jar, regardless of the descriptor format it was read from.
type ModMeta struct {
	Loader      string // "fabric" | "quilt" | "forge" | "neoforge" | "forge-legacy" | "liteloader" | "bukkit" | "paper"
	GameVersion string // declared Minecraft version/range; empty when the descriptor does not say
	ModID       string
	Name        string
	Version     string
	Authors     []string
	Description string
	Website     string
	Icon        []byte // raw image bytes (png/jpg), optional
}

// LuaParser adapts one sandboxed Lua parser script (with a global parse(ctx)
// function) to the engine. It is the parser analog of LuaProvider.
type LuaParser struct {
	meta     Meta
	source   string
	host     *Host
	builtin  bool
	grantsFn func() GrantSet
}

// newLuaParser compiles source in a throwaway sandbox, validates its meta and
// the presence of a parse() function, and returns the adapter.
func newLuaParser(host *Host, source string, builtin bool) (*LuaParser, error) {
	inv := &invocation{ctx: context.Background(), host: host}
	L, err := inv.prepare(source)
	if err != nil {
		return nil, err
	}
	defer L.Close()
	meta, err := parseMeta(L)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScriptInvalid, err)
	}
	if L.GetGlobal("parse").Type() != lua.LTFunction {
		return nil, fmt.Errorf("%w: script does not define parse(ctx)", ErrScriptInvalid)
	}
	return &LuaParser{meta: meta, source: source, host: host, builtin: builtin}, nil
}

// Meta returns the parser's declared metadata.
func (p *LuaParser) Meta() Meta { return p.meta }

// Builtin reports whether this is a first-party parser.
func (p *LuaParser) Builtin() bool { return p.builtin }

func (p *LuaParser) grants() GrantSet {
	if p.grantsFn != nil {
		return p.grantsFn()
	}
	return nil
}

// Parse runs the script's parse(ctx) against one jar (path relative to
// serverDir). matched=false means the parser did not recognize the jar
// (returned nil); an error means the parser recognized-and-failed or is broken.
func (p *LuaParser) Parse(ctx context.Context, serverDir, jarRel string) (meta ModMeta, matched bool, err error) {
	inv := &invocation{ctx: ctx, host: p.host, granted: p.grants(), baseDir: serverDir}
	return inv.parseMod(p.source, jarRel)
}

// parseMod loads src and calls its global parse(ctx) with ctx.jar set. A nil
// (or non-table) return means "no match"; a table return is read into ModMeta.
func (inv *invocation) parseMod(src, jarRel string) (ModMeta, bool, error) {
	L, err := inv.prepare(src)
	if err != nil {
		return ModMeta{}, false, err
	}
	defer L.Close()

	fn := L.GetGlobal("parse")
	if fn.Type() != lua.LTFunction {
		return ModMeta{}, false, fmt.Errorf("%w: script does not define parse(ctx)", ErrScriptInvalid)
	}
	ctxTbl := L.NewTable()
	ctxTbl.RawSetString("jar", lua.LString(jarRel))
	if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, ctxTbl); err != nil {
		return ModMeta{}, false, inv.mapErr(err)
	}
	ret := L.Get(-1)
	L.Pop(1)
	tbl, ok := ret.(*lua.LTable)
	if !ok {
		return ModMeta{}, false, nil
	}

	m := ModMeta{
		Loader:      strField(tbl, "loader"),
		GameVersion: strField(tbl, "game_version"),
		ModID:       strField(tbl, "mod_id"),
		Name:        strField(tbl, "name"),
		Version:     strField(tbl, "version"),
		Description: strField(tbl, "description"),
		Website:     strField(tbl, "website"),
	}
	switch authors := tbl.RawGetString("authors").(type) {
	case *lua.LTable:
		authors.ForEach(func(_, v lua.LValue) {
			if s, ok := v.(lua.LString); ok && string(s) != "" {
				m.Authors = append(m.Authors, string(s))
			}
		})
	case lua.LString:
		if authors != "" {
			m.Authors = append(m.Authors, string(authors))
		}
	}
	// Icon is raw bytes: read it directly (strField would be fine, but be
	// explicit that this is binary, not display text).
	if icon, ok := tbl.RawGetString("icon").(lua.LString); ok && len(icon) > 0 {
		m.Icon = []byte(icon)
	}
	return m, true, nil
}
