package scripting

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// builtin_lib holds shared Lua helper modules (e.g. mplib) replayed into every
// sandbox by prepare(), so provider/shop scripts can build on them instead of
// copying the code. It is a separate embed from builtin/ (which holds runnable
// provider scripts) so LoadBuiltins never picks these up as providers.
//
//go:embed builtin_lib/*.lua
var builtinLibFS embed.FS

type builtinLib struct {
	name string
	src  string
}

// builtinLibSources is the ordered set of shared-library sources, read once at
// init. Loading them must be side-effect-free (define functions/constants only —
// no network or filesystem access), because they run on every script prepare().
var builtinLibSources = mustLoadBuiltinLibs()

func mustLoadBuiltinLibs() []builtinLib {
	entries, err := builtinLibFS.ReadDir("builtin_lib")
	if err != nil {
		panic(fmt.Sprintf("read builtin_lib: %v", err))
	}
	var libs []builtinLib
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		src, err := builtinLibFS.ReadFile("builtin_lib/" + e.Name())
		if err != nil {
			panic(fmt.Sprintf("read builtin_lib/%s: %v", e.Name(), err))
		}
		libs = append(libs, builtinLib{name: e.Name(), src: string(src)})
	}
	sort.Slice(libs, func(i, j int) bool { return libs[i].name < libs[j].name })
	return libs
}

// loadBuiltinLibs replays every shared-library source into L. Called after the
// jhmc table is installed and before the script's own source runs, so scripts
// see the library globals (the libs reference jhmc only at call time).
func loadBuiltinLibs(L *lua.LState) error {
	for _, lib := range builtinLibSources {
		if err := L.DoString(lib.src); err != nil {
			return fmt.Errorf("load lib %s: %w", lib.name, err)
		}
	}
	return nil
}
