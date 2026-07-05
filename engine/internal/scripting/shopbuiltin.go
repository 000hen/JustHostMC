package scripting

import (
	"embed"
	"fmt"
	"strings"
)

// Built-in shop scripts are embedded in the binary (separate dir from the
// provider/parser builtins so no loader picks up another's scripts).
//
//go:embed builtin_shops/*.lua
var builtinShopsFS embed.FS

// LoadBuiltinShops registers every embedded shop script as builtin.
func LoadBuiltinShops(ss *ShopSet) error {
	entries, err := builtinShopsFS.ReadDir("builtin_shops")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		src, err := builtinShopsFS.ReadFile("builtin_shops/" + e.Name())
		if err != nil {
			return fmt.Errorf("builtin shop %s: %w", e.Name(), err)
		}
		if _, err := ss.AddSource(string(src), true); err != nil {
			return fmt.Errorf("builtin shop %s: %w", e.Name(), err)
		}
	}
	return nil
}
