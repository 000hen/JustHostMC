package scripting

import (
	"net/http"
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

func TestModrinthModpacksProviderHidden(t *testing.T) {
	e := builtinProvider(t, "modrinth_modpacks")
	if !e.Meta.Hidden {
		t.Error("modrinth_modpacks provider should be hidden")
	}
	if e.Meta.ModLayout != "mods" {
		t.Errorf("mod_layout = %q, want mods", e.Meta.ModLayout)
	}
}

func TestModrinthModpacksShopIsModpackKind(t *testing.T) {
	ss := newBuiltinShops(t, http.NotFoundHandler())
	sh, ok := ss.Get("modrinth_modpacks")
	if !ok {
		t.Fatal("modrinth_modpacks shop not registered")
	}
	if sh.Meta().NeedsKey {
		t.Error("modrinth_modpacks shop is keyless")
	}
	var isModpack bool
	for _, k := range sh.Meta().Kinds {
		if k == "modpack" {
			isModpack = true
		}
	}
	if !isModpack {
		t.Errorf("kinds = %v, want to contain modpack", sh.Meta().Kinds)
	}
}

// TestMrpackMeta covers parsing modrinth.index.json: loader detection, direct
// download URLs, and env.server=="unsupported" files flagged client_only rather
// than installed.
func TestMrpackMeta(t *testing.T) {
	dir := t.TempDir()
	index := `{
	  "formatVersion": 1, "game": "minecraft", "versionId": "1.2.0", "name": "MR Pack",
	  "files": [
	    { "path": "mods/sodium.jar", "hashes": {"sha1":"s1","sha512":"x"},
	      "env": {"client":"required","server":"required"}, "downloads": ["http://cdn/sodium.jar"], "fileSize": 100 },
	    { "path": "mods/iris.jar", "hashes": {"sha1":"s2"},
	      "env": {"client":"required","server":"unsupported"}, "downloads": ["http://cdn/iris.jar"], "fileSize": 50 }
	  ],
	  "dependencies": { "minecraft": "1.20.1", "fabric-loader": "0.15.7" }
	}`
	writeTestJar(t, filepath.Join(dir, ".jhmc", "pack.mrpack"),
		map[string]string{"modrinth.index.json": index})

	inv := fsInv(dir)
	src := `
function check()
  local m = mplib.mrpack_meta(".jhmc/pack.mrpack")
  assert(m.mc == "1.20.1", "mc " .. tostring(m.mc))
  assert(m.loader_name == "fabric" and m.loader_version == "0.15.7", "loader")
  assert(m.name == "MR Pack" and m.version == "1.2.0", "name/version")
  assert(#m.files == 2, "files " .. #m.files)
  local by = {}
  for _, f in ipairs(m.files) do by[f.dest] = f end
  local s = by["mods/sodium.jar"]
  assert(s and s.url == "http://cdn/sodium.jar" and s.sha1 == "s1", "sodium")
  assert(not s.client_only, "sodium is a server file")
  local i = by["mods/iris.jar"]
  assert(i and i.client_only, "iris unsupported-on-server must be client_only")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestMrpackQuiltUnsupported(t *testing.T) {
	dir := t.TempDir()
	index := `{
	  "formatVersion": 1, "name": "Q", "versionId": "1",
	  "files": [], "dependencies": { "minecraft": "1.20.1", "quilt-loader": "0.20.0" }
	}`
	writeTestJar(t, filepath.Join(dir, ".jhmc", "pack.mrpack"),
		map[string]string{"modrinth.index.json": index})
	inv := fsInv(dir)
	err := runInv(t, inv, `function check() mplib.mrpack_meta(".jhmc/pack.mrpack") end`)
	if err == nil {
		t.Fatal("expected Quilt to be rejected")
	}
}

// fsInvNet is fsInv plus the network grant, for pack routines that download.
func fsInvNet(dir string) *invocation {
	inv := fsInv(dir)
	inv.granted[mcmanagerv1.PermissionKind_PERMISSION_NETWORK] = true
	return inv
}

// TestMrpackServerOverridesWin verifies overrides/ then server-overrides/ are
// both extracted, with server-overrides winning on conflict.
func TestMrpackServerOverridesWin(t *testing.T) {
	dir := t.TempDir()
	writeTestJar(t, filepath.Join(dir, ".jhmc", "pack.mrpack"), map[string]string{
		"modrinth.index.json":              `{"dependencies":{"minecraft":"1.20.1","fabric-loader":"0.15.7"}}`,
		"overrides/config/a.toml":          "client-value",
		"server-overrides/config/a.toml":   "server-value",
		"overrides/config/only-client.txt": "c",
	})
	inv := fsInvNet(dir)
	src := `
function check()
  mplib.unzip_overrides(".jhmc/pack.mrpack")
  assert(jhmc.fs.read("config/a.toml") == "server-value", "server-overrides must win")
  assert(jhmc.fs.exists("config/only-client.txt"), "plain override extracted")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}
