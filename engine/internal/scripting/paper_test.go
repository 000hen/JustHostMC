package scripting

import (
	"context"

	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// TestPaperBuiltin verifies the embedded paper.lua parses and declares the
// expected metadata. (Built-in scripts target live upstream APIs and cannot be
// network-stubbed, so install/versions are covered by gated e2e tests, not here.)
func TestPaperBuiltin(t *testing.T) {
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	if err := LoadBuiltins(context.Background(), r); err != nil {
		t.Fatalf("LoadBuiltins: %v", err)
	}
	e, ok := r.Get("paper")
	if !ok {
		t.Fatal("paper provider not registered")
	}
	if e.Meta.ModLayout != "plugins" {
		t.Errorf("mod_layout = %q, want plugins", e.Meta.ModLayout)
	}
	hasNetwork := false
	for _, p := range e.Meta.Permissions {
		if p.Kind == mcmanagerv1.PermissionKind_PERMISSION_NETWORK {
			hasNetwork = true
		}
	}
	if !hasNetwork {
		t.Error("paper should declare the network permission")
	}
}
