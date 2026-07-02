package scripting

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/provider"
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
			{"id":"1.20.4","type":"release"},
			{"id":"1.7.10","type":"release"}
		]}`))
	})
	mux.HandleFunc("/versions/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
			<a href="26.2.json">26.2.json</a>
			<a href="1.21.1.json">1.21.1.json</a>
			<a href="1.20.4-pre1.json">1.20.4-pre1.json</a>
			<a href="4440.json">4440.json</a>
		</body></html>`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// spigotScript is the embedded script with its two version sources pointed at
// the test stub so its production parsing logic can be exercised offline.
func spigotScript(url string) string {
	src, err := builtinFS.ReadFile("builtin/spigot.lua")
	if err != nil {
		panic(err)
	}
	script := strings.ReplaceAll(string(src),
		"https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", url+"/manifest")
	return strings.ReplaceAll(script, "https://hub.spigotmc.org/versions/", url+"/versions/")
}

// TestSpigotVersionsSupportedReleasesOnly verifies that Mojang releases without
// a corresponding Spigot descriptor (such as 1.7.10) are not advertised.
func TestSpigotVersionsSupportedReleasesOnly(t *testing.T) {
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
	want := []string{"26.2", "1.21.1"}
	if len(vers) != len(want) {
		t.Fatalf("Versions = %v, want %v", vers, want)
	}
	for i := range want {
		if vers[i] != want[i] {
			t.Fatalf("Versions[%d] = %q, want %q", i, vers[i], want[i])
		}
	}
}

func TestSpigotInstallRejectsUnsupportedVersion(t *testing.T) {
	srv := spigotManifestStub(t)
	r := NewRegistry(NewHost(nil, nil, nil), nil)
	e, err := r.AddSource(spigotScript(srv.URL), true)
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	_, err = e.Provider.Install(context.Background(), t.TempDir(), "1.7.10", nil)
	if !errors.Is(err, provider.ErrVersionNotFound) {
		t.Fatalf("Install err = %v, want ErrVersionNotFound", err)
	}
}
