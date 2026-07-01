package grpcsvc

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestItemIconResolverReadsModModelTexture(t *testing.T) {
	serverDir := t.TempDir()
	t.Setenv("APPDATA", t.TempDir())
	want := []byte("mod-icon-png")
	writeAssetArchive(t, filepath.Join(serverDir, "mods", "example.jar"), map[string][]byte{
		"assets/example/models/item/wand.json":        []byte(`{"parent":"example:item/tool_base","textures":{"layer0":"#tool"}}`),
		"assets/example/models/item/tool_base.json":   []byte(`{"parent":"item/generated","textures":{"tool":"example:item/tools/wand"}}`),
		"assets/example/textures/item/tools/wand.png": want,
	})

	asset := newItemAssetResolver(serverDir, "").Resolve("example:wand")
	got := asset.Textures["example:item/tools/wand"]
	if string(got) != string(want) {
		t.Fatalf("icon = %q, want %q", got, want)
	}
}

func TestItemIconResolverReadsModernItemDefinition(t *testing.T) {
	serverDir := t.TempDir()
	t.Setenv("APPDATA", t.TempDir())
	want := []byte("modern-icon-png")
	writeAssetArchive(t, filepath.Join(serverDir, "resourcepacks", "pack.zip"), map[string][]byte{
		"assets/example/items/hammer.json":              []byte(`{"model":{"type":"minecraft:model","model":"example:item/custom_hammer"}}`),
		"assets/example/models/item/custom_hammer.json": []byte(`{"textures":{"layer0":"example:item/hammer_head"}}`),
		"assets/example/textures/item/hammer_head.png":  want,
	})

	asset := newItemAssetResolver(serverDir, "").Resolve("example:hammer")
	got := asset.Textures["example:item/hammer_head"]
	if string(got) != string(want) {
		t.Fatalf("icon = %q, want %q", got, want)
	}
}

func TestItemIconResolverUsesLocalMinecraftClient(t *testing.T) {
	serverDir := t.TempDir()
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	want := []byte("vanilla-icon-png")
	client := filepath.Join(appData, ".minecraft", "versions", "1.21.7", "1.21.7.jar")
	writeAssetArchive(t, client, map[string][]byte{
		"assets/minecraft/models/item/wooden_axe.json":  []byte(`{"textures":{"layer0":"minecraft:item/wooden_axe"}}`),
		"assets/minecraft/textures/item/wooden_axe.png": want,
	})

	asset := newItemAssetResolver(serverDir, "1.21.7").Resolve("minecraft:wooden_axe")
	got := asset.Textures["minecraft:item/wooden_axe"]
	if string(got) != string(want) {
		t.Fatalf("icon = %q, want %q", got, want)
	}
}

func TestLocalMinecraftClientDoesNotUseDifferentVersion(t *testing.T) {
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	writeAssetArchive(t, filepath.Join(appData, ".minecraft", "versions", "1.21.7", "1.21.7.jar"), map[string][]byte{
		"assets/minecraft/models/item/stone.json": []byte(`{}`),
	})
	if got := localMinecraftClient("26.2"); got != "" {
		t.Fatalf("client = %q, want no cross-version fallback", got)
	}
}

func TestItemAssetResolverPreservesBlockGeometry(t *testing.T) {
	serverDir := t.TempDir()
	t.Setenv("APPDATA", t.TempDir())
	texture := []byte("spruce-texture-png")
	writeAssetArchive(t, filepath.Join(serverDir, "resourcepacks", "pack.zip"), map[string][]byte{
		"assets/minecraft/items/spruce_fence_gate.json":          []byte(`{"model":{"type":"minecraft:model","model":"minecraft:block/spruce_fence_gate"}}`),
		"assets/minecraft/models/block/spruce_fence_gate.json":   []byte(`{"parent":"minecraft:block/template_fence_gate","textures":{"texture":"minecraft:block/spruce_planks"}}`),
		"assets/minecraft/models/block/template_fence_gate.json": []byte(`{"display":{"gui":{"rotation":[30,45,0],"translation":[0,-1,0],"scale":[0.8,0.8,0.8]}},"textures":{"particle":"#texture"},"elements":[{"from":[0,5,7],"to":[2,16,9],"faces":{"north":{"uv":[0,0,2,11],"texture":"#texture"}}}]}`),
		"assets/minecraft/textures/block/spruce_planks.png":      texture,
	})

	asset := newItemAssetResolver(serverDir, "").Resolve("minecraft:spruce_fence_gate")
	if string(asset.Textures["minecraft:block/spruce_planks"]) != string(texture) {
		t.Fatal("resolved asset did not include the fence-gate texture")
	}
	var model resolvedModel
	if err := json.Unmarshal([]byte(asset.ModelJSON), &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Elements) != 1 {
		t.Fatalf("flattened element count = %d, want 1", len(model.Elements))
	}
	if model.GUI.Rotation != [3]float64{30, 45, 0} {
		t.Fatalf("GUI rotation = %v, want [30 45 0]", model.GUI.Rotation)
	}
}

func TestItemIconResolverReadsSpecialEnderChest(t *testing.T) {
	serverDir := t.TempDir()
	t.Setenv("APPDATA", t.TempDir())
	want := []byte("ender-chest-texture")
	writeAssetArchive(t, filepath.Join(serverDir, "resourcepacks", "pack.zip"), map[string][]byte{
		"assets/minecraft/items/ender_chest.json":          []byte(`{"model":{"type":"minecraft:special","base":"minecraft:item/ender_chest","model":{"type":"minecraft:chest","texture":"minecraft:ender"}}}`),
		"assets/minecraft/models/item/ender_chest.json":    []byte(`{"parent":"minecraft:item/template_chest"}`),
		"assets/minecraft/models/item/template_chest.json": []byte(`{"display":{"gui":{"rotation":[30,45,0],"scale":[0.625,0.625,0.625]}}}`),
		"assets/minecraft/textures/entity/chest/ender.png": want,
	})

	asset := newItemAssetResolver(serverDir, "").Resolve("minecraft:ender_chest")
	if string(asset.Textures["minecraft:entity/chest/ender"]) != string(want) {
		t.Fatal("special Ender Chest texture was not resolved")
	}
	var model resolvedModel
	if err := json.Unmarshal([]byte(asset.ModelJSON), &model); err != nil || model.Special != "chest" {
		t.Fatalf("special model = %+v, %v", model, err)
	}
}

func writeAssetArchive(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, data := range entries {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
