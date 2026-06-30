package scripting

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed builtin/*.lua
var builtinFS embed.FS

// LoadBuiltins registers every embedded built-in provider script into r. New
// built-ins are added by dropping a .lua file in builtin/ — no Go change.
func LoadBuiltins(r *Registry) error {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		src, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return err
		}
		if _, err := r.AddSource(string(src), true); err != nil {
			return fmt.Errorf("builtin %s: %w", e.Name(), err)
		}
	}
	return nil
}
