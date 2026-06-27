//go:build windows

package e2e

import (
	"testing"

	"github.com/000hen/justhostmc/engine/internal/provider"
	"github.com/000hen/justhostmc/engine/internal/scripting"
)

// vanillaProvider returns the built-in vanilla Lua provider for integration
// tests (the Go provider.NewVanilla() was replaced by the embedded script).
func vanillaProvider(t *testing.T) provider.Provider {
	t.Helper()
	reg := scripting.NewRegistry(scripting.NewHost(nil, nil, nil), nil)
	if err := scripting.LoadBuiltins(reg); err != nil {
		t.Fatalf("load builtin providers: %v", err)
	}
	e, ok := reg.Get("vanilla")
	if !ok {
		t.Fatal("vanilla provider not registered")
	}
	return e.Provider
}
