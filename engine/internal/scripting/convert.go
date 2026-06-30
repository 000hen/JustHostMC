package scripting

import lua "github.com/yuin/gopher-lua"

// goToLua converts a decoded JSON value (from encoding/json into any) to a Lua
// value. Objects become tables keyed by string; arrays become 1-based tables.
func goToLua(L *lua.LState, v any) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(x)
	case float64:
		return lua.LNumber(x)
	case string:
		return lua.LString(x)
	case []any:
		t := L.NewTable()
		for _, e := range x {
			t.Append(goToLua(L, e))
		}
		return t
	case map[string]any:
		t := L.NewTable()
		for k, e := range x {
			t.RawSetString(k, goToLua(L, e))
		}
		return t
	default:
		return lua.LNil
	}
}

// luaToGo converts a Lua value into a Go value suitable for json.Marshal. A
// table with consecutive integer keys 1..n becomes a slice; otherwise a map.
// maxLuaDepth bounds luaToGo recursion so a cyclic or pathologically deep Lua
// table (e.g. one a script passes to json_encode) cannot overflow the Go stack,
// which would crash the whole engine — a Go stack overflow is not recoverable.
const maxLuaDepth = 64

func luaToGo(v lua.LValue) any { return luaToGoDepth(v, 0) }

func luaToGoDepth(v lua.LValue, depth int) any {
	if depth > maxLuaDepth {
		return nil
	}
	switch x := v.(type) {
	case lua.LBool:
		return bool(x)
	case lua.LNumber:
		return float64(x)
	case lua.LString:
		return string(x)
	case *lua.LTable:
		n := x.Len()
		if n > 0 {
			arr := make([]any, 0, n)
			for i := 1; i <= n; i++ {
				arr = append(arr, luaToGoDepth(x.RawGetInt(i), depth+1))
			}
			return arr
		}
		m := map[string]any{}
		x.ForEach(func(k, val lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				m[string(ks)] = luaToGoDepth(val, depth+1)
			}
		})
		return m
	default:
		return nil
	}
}
