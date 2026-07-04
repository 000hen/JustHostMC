package scripting

import (
	"fmt"
	"reflect"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// goToLua converts a decoded Go value (from encoding/json, BurntSushi/toml or
// yaml.v3) to a Lua value. Maps become tables keyed by string; slices/arrays
// become 1-based tables. It is reflection-based so decoder-specific concrete
// types (int64 from TOML, []map[string]any for TOML array-of-tables,
// time.Time for TOML datetimes) all convert; anything unconvertible maps to nil.
func goToLua(L *lua.LState, v any) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch x := v.(type) {
	case bool:
		return lua.LBool(x)
	case float64:
		return lua.LNumber(x)
	case string:
		return lua.LString(x)
	case time.Time:
		return lua.LString(x.Format(time.RFC3339))
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		return lua.LBool(rv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return lua.LNumber(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return lua.LNumber(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return lua.LNumber(rv.Float())
	case reflect.String:
		return lua.LString(rv.String())
	case reflect.Slice, reflect.Array:
		t := L.NewTable()
		for i := 0; i < rv.Len(); i++ {
			t.Append(goToLua(L, rv.Index(i).Interface()))
		}
		return t
	case reflect.Map:
		t := L.NewTable()
		iter := rv.MapRange()
		for iter.Next() {
			key := iter.Key()
			var ks string
			if key.Kind() == reflect.String {
				ks = key.String()
			} else if key.Kind() == reflect.Interface && key.Elem().Kind() == reflect.String {
				ks = key.Elem().String()
			} else {
				ks = fmt.Sprint(key.Interface())
			}
			t.RawSetString(ks, goToLua(L, iter.Value().Interface()))
		}
		return t
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return lua.LNil
		}
		return goToLua(L, rv.Elem().Interface())
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
