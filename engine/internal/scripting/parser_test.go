package scripting

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// newBuiltinParserSet loads the embedded parsers for tests.
func newBuiltinParserSet(t *testing.T) *ParserSet {
	t.Helper()
	ps := NewParserSet(NewHost(nil, nil, nil), nil)
	if err := LoadBuiltinParsers(ps); err != nil {
		t.Fatalf("LoadBuiltinParsers: %v", err)
	}
	return ps
}

const fakeIcon = "\x89PNG\r\n fake icon bytes"

// parseFixture writes a jar with the given entries into a temp server dir and
// runs ParseJar over it.
func parseFixture(t *testing.T, ps *ParserSet, entries map[string]string) (ModMeta, string, bool) {
	t.Helper()
	dir := t.TempDir()
	writeTestJar(t, filepath.Join(dir, "mods", "test.jar"), entries)
	return ps.ParseJar(context.Background(), dir, "mods/test.jar")
}

func TestParseFabric(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, parserID, ok := parseFixture(t, ps, map[string]string{
		"fabric.mod.json": `{
			"id": "examplemod", "name": "Example Mod", "version": "2.0.1",
			"authors": ["Alice", {"name": "Bob"}],
			"description": "Does example things.",
			"contact": {"homepage": "https://example.com"},
			"icon": "assets/examplemod/icon.png"
		}`,
		"assets/examplemod/icon.png": fakeIcon,
	})
	if !ok {
		t.Fatal("fabric jar not matched")
	}
	if parserID != "parser-fabric" {
		t.Errorf("parserID = %q", parserID)
	}
	want := ModMeta{
		Loader: "fabric", ModID: "examplemod", Name: "Example Mod", Version: "2.0.1",
		Authors: []string{"Alice", "Bob"}, Description: "Does example things.",
		Website: "https://example.com", Icon: []byte(fakeIcon),
	}
	if !reflect.DeepEqual(meta, want) {
		t.Errorf("meta = %+v, want %+v", meta, want)
	}
}

func TestParseFabricIconSizeMap(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, _, ok := parseFixture(t, ps, map[string]string{
		"fabric.mod.json": `{"id":"m","version":"1","icon":{"16":"small.png","128":"big.png"}}`,
		"small.png":       "small",
		"big.png":         "BIGICON",
	})
	if !ok {
		t.Fatal("not matched")
	}
	if string(meta.Icon) != "BIGICON" {
		t.Errorf("icon = %q, want largest size variant", meta.Icon)
	}
}

func TestParseQuilt(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, parserID, ok := parseFixture(t, ps, map[string]string{
		"quilt.mod.json": `{
			"quilt_loader": {
				"id": "qmod", "version": "3.1",
				"metadata": {
					"name": "Quilt Mod",
					"description": "Quilted.",
					"contributors": {"Carol": "Owner"},
					"contact": {"homepage": "https://quilt.example"},
					"icon": "icon.png"
				}
			}
		}`,
		"icon.png": fakeIcon,
	})
	if !ok {
		t.Fatal("quilt jar not matched")
	}
	if parserID != "parser-quilt" {
		t.Errorf("parserID = %q", parserID)
	}
	if meta.Loader != "quilt" || meta.ModID != "qmod" || meta.Name != "Quilt Mod" ||
		meta.Version != "3.1" || meta.Website != "https://quilt.example" ||
		len(meta.Authors) != 1 || meta.Authors[0] != "Carol" || len(meta.Icon) == 0 {
		t.Errorf("meta = %+v", meta)
	}
}

func TestParseForge(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, parserID, ok := parseFixture(t, ps, map[string]string{
		"META-INF/mods.toml": `
modLoader = "javafml"
loaderVersion = "[47,)"
license = "MIT"
[[mods]]
modId = "forgemod"
version = "4.5.6"
displayName = "Forge Mod"
description = "Forged."
authors = "Dave, Erin"
displayURL = "https://forge.example"
logoFile = "logo.png"
`,
		"logo.png": fakeIcon,
	})
	if !ok {
		t.Fatal("forge jar not matched")
	}
	if parserID != "parser-forge" {
		t.Errorf("parserID = %q", parserID)
	}
	if meta.Loader != "forge" || meta.ModID != "forgemod" || meta.Name != "Forge Mod" ||
		meta.Version != "4.5.6" || !reflect.DeepEqual(meta.Authors, []string{"Dave", "Erin"}) ||
		meta.Website != "https://forge.example" || len(meta.Icon) == 0 {
		t.Errorf("meta = %+v", meta)
	}
}

func TestParseForgeUnexpandedVersionDropped(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, _, ok := parseFixture(t, ps, map[string]string{
		"META-INF/mods.toml": `
[[mods]]
modId = "m"
version = "${file.jarVersion}"
`,
	})
	if !ok {
		t.Fatal("not matched")
	}
	if meta.Version != "" {
		t.Errorf("version = %q, want empty for unexpanded placeholder", meta.Version)
	}
}

func TestParseNeoForge(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, parserID, ok := parseFixture(t, ps, map[string]string{
		"META-INF/neoforge.mods.toml": `
[[mods]]
modId = "neomod"
version = "1.0.0"
displayName = "Neo Mod"
authors = "Frank"
`,
	})
	if !ok {
		t.Fatal("neoforge jar not matched")
	}
	if parserID != "parser-neoforge" || meta.Loader != "neoforge" || meta.ModID != "neomod" ||
		meta.Name != "Neo Mod" || !reflect.DeepEqual(meta.Authors, []string{"Frank"}) {
		t.Errorf("parserID=%q meta = %+v", parserID, meta)
	}
}

func TestParseForgeLegacy(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, parserID, ok := parseFixture(t, ps, map[string]string{
		"mcmod.info": `[{
			"modid": "legacymod", "name": "Legacy Mod", "version": "0.9",
			"description": "Old but gold.",
			"authorList": ["Grace"],
			"url": "https://legacy.example",
			"logoFile": "logo.png"
		}]`,
		"logo.png": fakeIcon,
	})
	if !ok {
		t.Fatal("legacy forge jar not matched")
	}
	if parserID != "parser-forge-legacy" || meta.Loader != "forge-legacy" ||
		meta.ModID != "legacymod" || meta.Name != "Legacy Mod" ||
		!reflect.DeepEqual(meta.Authors, []string{"Grace"}) || len(meta.Icon) == 0 {
		t.Errorf("parserID=%q meta = %+v", parserID, meta)
	}
}

func TestParseForgeLegacyModListWrapper(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, _, ok := parseFixture(t, ps, map[string]string{
		"mcmod.info": `{"modListVersion": 2, "modList": [{"modid": "wrapped", "version": "1"}]}`,
	})
	if !ok {
		t.Fatal("modList-wrapped mcmod.info not matched")
	}
	if meta.ModID != "wrapped" {
		t.Errorf("meta = %+v", meta)
	}
}

func TestParseBukkitPlugin(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, parserID, ok := parseFixture(t, ps, map[string]string{
		"plugin.yml": `
name: WorldEdit
version: '7.2'
main: com.example.Main
description: Edits worlds.
authors: [sk89q, wizjany]
website: https://worldedit.example
`,
	})
	if !ok {
		t.Fatal("bukkit jar not matched")
	}
	if parserID != "parser-bukkit" || meta.Loader != "bukkit" || meta.Name != "WorldEdit" ||
		meta.Version != "7.2" || !reflect.DeepEqual(meta.Authors, []string{"sk89q", "wizjany"}) ||
		meta.Website != "https://worldedit.example" {
		t.Errorf("parserID=%q meta = %+v", parserID, meta)
	}
}

func TestParsePaperPlugin(t *testing.T) {
	ps := newBuiltinParserSet(t)
	meta, _, ok := parseFixture(t, ps, map[string]string{
		"paper-plugin.yml": `
name: PaperThing
version: 1.5
author: Heidi
`,
	})
	if !ok {
		t.Fatal("paper jar not matched")
	}
	// An unquoted YAML number stringifies via Lua tostring ("1.5").
	if meta.Loader != "paper" || meta.Name != "PaperThing" || meta.Version != "1.5" ||
		!reflect.DeepEqual(meta.Authors, []string{"Heidi"}) {
		t.Errorf("meta = %+v", meta)
	}
}

func TestParseJarNoMatch(t *testing.T) {
	ps := newBuiltinParserSet(t)
	_, _, ok := parseFixture(t, ps, map[string]string{"just/a/Class.class": "bytes"})
	if ok {
		t.Fatal("jar with no descriptor should not match")
	}
}

// TestBrokenParserSkipped verifies a user parser that crashes never breaks
// ParseJar: the next parser still runs.
func TestBrokenParserSkipped(t *testing.T) {
	ps := newBuiltinParserSet(t)
	// Registered after built-ins; matches everything but explodes.
	if _, err := ps.AddSource(`
meta = { id = "broken", name = "Broken",
  permissions = { {kind = "fs_server", reason = "x"} } }
function parse(ctx)
  error("boom")
end`, false); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	// A fabric jar still parses via the fabric built-in (registered earlier).
	meta, parserID, ok := parseFixture(t, ps, map[string]string{
		"fabric.mod.json": `{"id":"m","version":"1"}`,
	})
	if !ok || parserID != "parser-fabric" || meta.ModID != "m" {
		t.Fatalf("ok=%v parserID=%q meta=%+v", ok, parserID, meta)
	}
	// A jar only the broken parser would claim degrades to no-match.
	if _, _, ok := parseFixture(t, ps, map[string]string{"x": "y"}); ok {
		t.Fatal("broken parser must not produce a match")
	}
}

// TestUserParserUngrantedCannotRead verifies a user parser without granted
// fs_server can't read the jar: its permission error is swallowed and the jar
// simply doesn't match.
func TestUserParserUngrantedCannotRead(t *testing.T) {
	ps := NewParserSet(NewHost(nil, nil, nil), nil)
	if _, err := ps.AddSource(`
meta = { id = "user-parser", name = "User Parser",
  permissions = { {kind = "fs_server", reason = "read jars"} } }
function parse(ctx)
  local raw = jhmc.zip_read(ctx.jar, "anything.txt")
  if raw == nil then return nil end
  return { name = "should not happen" }
end`, false); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	_, _, ok := parseFixture(t, ps, map[string]string{"anything.txt": "hi"})
	if ok {
		t.Fatal("ungranted user parser must not be able to parse")
	}
}

func TestParserMetaFormats(t *testing.T) {
	ps := newBuiltinParserSet(t)
	p, ok := ps.Get("parser-bukkit")
	if !ok {
		t.Fatal("parser-bukkit not registered")
	}
	if !reflect.DeepEqual(p.Meta().Formats, []string{"plugin.yml", "paper-plugin.yml"}) {
		t.Errorf("formats = %v", p.Meta().Formats)
	}
}

func TestParserRejectsMissingParseFunction(t *testing.T) {
	ps := NewParserSet(NewHost(nil, nil, nil), nil)
	_, err := ps.AddSource(`meta = { id = "nop", name = "Nop", permissions = {} }`, false)
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("err = %v, want missing-parse error", err)
	}
}
