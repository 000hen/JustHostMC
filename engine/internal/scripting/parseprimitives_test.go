package scripting

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// writeTestJar creates a zip at path with the given name->content entries.
func writeTestJar(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestTomlDecode covers the forge mods.toml shape: array-of-tables ([[mods]])
// and int64 values, which exercise the reflection-based goToLua.
func TestTomlDecode(t *testing.T) {
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	src := `
function check()
  local t = jhmc.toml_decode([==[
modLoader = "javafml"
loaderVersion = "[47,)"
issueTrackerURL = "https://example.com/issues"
count = 42
pi = 3.5
[[mods]]
modId = "examplemod"
version = "1.2.3"
displayName = "Example Mod"
authors = "Alice, Bob"
[[mods]]
modId = "second"
]==])
  assert(t.modLoader == "javafml", "modLoader")
  assert(t.count == 42, "int")
  assert(t.pi == 3.5, "float")
  assert(#t.mods == 2, "mods count " .. #t.mods)
  assert(t.mods[1].modId == "examplemod", "mods[1].modId")
  assert(t.mods[1].displayName == "Example Mod", "displayName")
  assert(t.mods[2].modId == "second", "mods[2]")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestTomlDecodeBadInput(t *testing.T) {
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	err := runInv(t, inv, `function check() jhmc.toml_decode("= not toml =") end`)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

// TestYamlDecode covers the plugin.yml shape, including a string list and ints.
func TestYamlDecode(t *testing.T) {
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	src := `
function check()
  local t = jhmc.yaml_decode([==[
name: WorldEdit
version: '7.2.15'
main: com.sk89q.worldedit.bukkit.WorldEditPlugin
authors: [sk89q, wizjany]
api-version: 1.18
depth:
  nested:
    number: 7
]==])
  assert(t.name == "WorldEdit", "name")
  assert(t.version == "7.2.15", "version")
  assert(#t.authors == 2 and t.authors[1] == "sk89q", "authors")
  assert(t.depth.nested.number == 7, "nested int")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestZipReadAndEntries(t *testing.T) {
	dir := t.TempDir()
	writeTestJar(t, filepath.Join(dir, "mods", "example.jar"), map[string]string{
		"fabric.mod.json":   `{"id":"example"}`,
		"assets/icon.png":   "\x89PNG fake bytes",
		"META-INF/MANIFEST": "x",
	})
	inv := &invocation{
		ctx:     context.Background(),
		host:    NewHost(nil, nil, nil),
		baseDir: dir,
		granted: GrantSet{mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER: true},
	}
	src := `
function check()
  local names = jhmc.zip_entries("mods/example.jar")
  assert(#names == 3, "entries " .. #names)
  local body = jhmc.zip_read("mods/example.jar", "fabric.mod.json")
  assert(body == '{"id":"example"}', "body")
  local icon = jhmc.zip_read("mods/example.jar", "assets/icon.png")
  assert(#icon == 15, "icon len " .. #icon)
  assert(jhmc.zip_read("mods/example.jar", "missing.txt") == nil, "missing entry")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestZipReadRequiresFSAndConfinement(t *testing.T) {
	dir := t.TempDir()
	// No fs_server grant → permission denied.
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil), baseDir: dir}
	err := runInv(t, inv, `function check() jhmc.zip_read("a.jar", "x") end`)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied", err)
	}
	// Escaping path → rejected.
	inv = &invocation{
		ctx:     context.Background(),
		host:    NewHost(nil, nil, nil),
		baseDir: dir,
		granted: GrantSet{mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER: true},
	}
	err = runInv(t, inv, `function check() jhmc.zip_read("../outside.jar", "x") end`)
	if !errors.Is(err, ErrPathEscape) {
		t.Fatalf("err = %v, want ErrPathEscape", err)
	}
}

// TestJSONDecodeStillWorks pins the JSON path of the generalized goToLua
// (float64 numbers, nested arrays/objects, null).
func TestJSONDecodeStillWorks(t *testing.T) {
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	src := `
function check()
  -- note: a JSON null inside an array is dropped (Lua tables cannot hold nil),
  -- matching the pre-generalization goToLua behavior.
  local t = jhmc.json_decode('{"a":1,"b":[true,null,"x"],"c":{"d":2.5}}')
  assert(t.a == 1, "a")
  assert(t.b[1] == true, "b1")
  assert(t.b[2] == "x", "b2")
  assert(t.c.d == 2.5, "cd")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}
