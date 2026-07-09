package scripting

import (
	"context"
	"embed"
	"fmt"
	"strings"
)

// Built-in parser scripts are embedded in the binary (separate dir from the
// provider builtins so neither loader picks up the other's scripts).
//
//go:embed builtin_parsers/*.lua
var builtinParsersFS embed.FS

// LoadBuiltinParsers registers every embedded parser script as builtin.
func LoadBuiltinParsers(ctx context.Context, ps *ParserSet) error {
	entries, err := builtinParsersFS.ReadDir("builtin_parsers")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		src, err := builtinParsersFS.ReadFile("builtin_parsers/" + e.Name())
		if err != nil {
			return fmt.Errorf("builtin parser %s: %w", e.Name(), err)
		}
		if _, err := ps.AddSource(ctx, string(src), true); err != nil {
			return fmt.Errorf("builtin parser %s: %w", e.Name(), err)
		}
	}
	return nil
}
