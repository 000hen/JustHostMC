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
func luaToGo(v lua.LValue) any {
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
				arr = append(arr, luaToGo(x.RawGetInt(i)))
			}
			return arr
		}
		m := map[string]any{}
		x.ForEach(func(k, val lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				m[string(ks)] = luaToGo(val)
			}
		})
		return m
	default:
		return nil
	}
}
