package scripting

import (
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// TestNeoForgeBuiltin verifies the embedded neoforge.lua parses and declares the
// expected metadata. (Built-in scripts target live upstream APIs and cannot be
// network-stubbed, so install/versions are covered by gated e2e tests, not here.)
func TestNeoForgeBuiltin(t *testing.T) {
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	if err := LoadBuiltins(r); err != nil {
		t.Fatalf("LoadBuiltins: %v", err)
	}
	e, ok := r.Get("neoforge")
	if !ok {
		t.Fatal("neoforge provider not registered")
	}
	if e.Meta.ModLayout != "mods" {
		t.Errorf("mod_layout = %q, want mods", e.Meta.ModLayout)
	}
	hasNetwork := false
	for _, p := range e.Meta.Permissions {
		if p.Kind == mcmanagerv1.PermissionKind_PERMISSION_NETWORK {
			hasNetwork = true
		}
	}
	if !hasNetwork {
		t.Error("neoforge should declare the network permission")
	}
}
