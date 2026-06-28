package scripting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// TestBuiltinSpigotMeta loads the embedded spigot script and checks its declared
// metadata + permissions parse correctly (no network).
func TestBuiltinSpigotMeta(t *testing.T) {
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	if err := LoadBuiltins(r); err != nil {
		t.Fatalf("LoadBuiltins: %v", err)
	}
	e, ok := r.Get("spigot")
	if !ok {
		t.Fatal("spigot not registered")
	}
	if e.Meta.Name != "Spigot" {
		t.Errorf("name = %q, want Spigot", e.Meta.Name)
	}
	if e.Meta.ModLayout != "plugins" {
		t.Errorf("mod_layout = %q, want plugins", e.Meta.ModLayout)
	}
	want := map[mcmanagerv1.PermissionKind]bool{
		mcmanagerv1.PermissionKind_PERMISSION_NETWORK: true,
		mcmanagerv1.PermissionKind_PERMISSION_INSTALL: true,
	}
	if len(e.Meta.Permissions) != len(want) {
		t.Fatalf("permissions = %+v, want network+install", e.Meta.Permissions)
	}
	for _, p := range e.Meta.Permissions {
		if !want[p.Kind] {
			t.Errorf("unexpected permission %v", p.Kind)
		}
	}
}

// spigotManifestStub serves a Mojang-shaped manifest mixing release and snapshot
// versions, so the script's release filter can be exercised without a network.
func spigotManifestStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"versions":[
			{"id":"26.2","type":"release"},
			{"id":"26w25a","type":"snapshot"},
			{"id":"1.21.1","type":"release"},
			{"id":"24w01a","type":"snapshot"},
			{"id":"1.20.4","type":"release"}
		]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// spigotScript is the embedded spigot.lua with its manifest URL pointed at the
// test stub, so versions() can be exercised offline. install() runs BuildTools
// (compiles from source) so it is intentionally not covered by a unit test.
func spigotScript(url string) string {
	return `
meta = {
  id = "spigot", name = "Spigot", mod_layout = "plugins",
  permissions = {
    { kind = "network", reason = "test" },
    { kind = "install", reason = "test" },
  },
}

local MANIFEST = "` + url + `/manifest"

function versions()
  local m = jhmc.http_json(MANIFEST)
  local out = {}
  for _, e in ipairs(m.versions) do
    if e.type == "release" then out[#out + 1] = e.id end
  end
  return out
end
`
}

// TestSpigotVersionsReleasesOnly verifies versions() returns only release
// versions, newest first (Mojang's order), mirroring the deleted Go provider.
func TestSpigotVersionsReleasesOnly(t *testing.T) {
	srv := spigotManifestStub(t)
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	e, err := r.AddSource(spigotScript(srv.URL), true) // builtin → network auto-granted
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	vers, err := e.Provider.Versions(context.Background())
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	want := []string{"26.2", "1.21.1", "1.20.4"}
	if len(vers) != len(want) {
		t.Fatalf("Versions = %v, want %v", vers, want)
	}
	for i := range want {
		if vers[i] != want[i] {
			t.Fatalf("Versions[%d] = %q, want %q", i, vers[i], want[i])
		}
	}
}
