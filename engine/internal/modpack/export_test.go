package modpack

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/000hen/justhostmc/engine/internal/provider"
)

// fixtureManifest mirrors the FTB version-manifest fields Export consumes.
const fixtureManifest = `{
  "name": "1.5.0",
  "targets": [
    { "type": "game", "version": "1.21.1" },
    { "type": "modloader", "name": "neoforge", "version": "21.1.77" }
  ],
  "files": [
    { "path": "./mods/", "name": "server-mod.jar", "clientonly": false,
      "curseforge": { "project": 111, "file": 1111 } },
    { "path": "./mods/", "name": "client-mod.jar", "clientonly": true,
      "curseforge": { "project": 222, "file": 2222 } },
    { "path": "./config/", "name": "pack.toml", "clientonly": false,
      "url": "IGNORED-server-file-comes-from-disk" },
    { "path": "./resourcepacks/", "name": "look.zip", "clientonly": true,
      "url": "%s/blob/look.zip" }
  ]
}`

func exportFixture(t *testing.T) (Options, string) {
	t.Helper()
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/modpack/95/300", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, fixtureManifest, base)
	})
	mux.HandleFunc("/blob/look.zip", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("client-only-bytes"))
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)

	serverDir := t.TempDir()
	mustWrite := func(rel, content string) {
		full := filepath.Join(serverDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("config/pack.toml", "edited-on-server")
	mustWrite("config/nested/extra.json", "{}")
	mustWrite("kubejs/server_scripts/recipes.js", "// kubejs")
	mustWrite("mods/server-mod.jar", "cf-covered-jar")    // covered by CF entry → excluded
	mustWrite("mods/local-extra.jar", "hand-added-jar")   // not in manifest → included
	mustWrite("world/level.dat", "world")                 // excluded
	mustWrite("logs/latest.log", "log")                   // excluded
	mustWrite("libraries/dep.jar", "lib")                 // excluded
	mustWrite("server.properties", "server-port=25565")   // excluded
	mustWrite("eula.txt", "eula=true")                    // excluded
	mustWrite("neoforge-installer.jar", "root installer") // excluded (root jar)

	dest := filepath.Join(t.TempDir(), "out.zip")
	return Options{
		ServerDir:   serverDir,
		DestZip:     dest,
		PackVersion: "95/300",
		ServerName:  "My Pack Server",
		FTBAPIBase:  srv.URL,
	}, dest
}

func readZip(t *testing.T, dest string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestExportBuildsCurseForgeManifest(t *testing.T) {
	opts, dest := exportFixture(t)
	if err := Export(context.Background(), http.DefaultClient, opts, nil); err != nil {
		t.Fatalf("Export: %v", err)
	}
	files := readZip(t, dest)

	var m struct {
		ManifestType    string `json:"manifestType"`
		ManifestVersion int    `json:"manifestVersion"`
		Name            string `json:"name"`
		Version         string `json:"version"`
		Overrides       string `json:"overrides"`
		Minecraft       struct {
			Version    string `json:"version"`
			ModLoaders []struct {
				ID      string `json:"id"`
				Primary bool   `json:"primary"`
			} `json:"modLoaders"`
		} `json:"minecraft"`
		Files []struct {
			ProjectID int64 `json:"projectID"`
			FileID    int64 `json:"fileID"`
			Required  bool  `json:"required"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(files["manifest.json"]), &m); err != nil {
		t.Fatalf("manifest.json: %v (%q)", err, files["manifest.json"])
	}
	if m.ManifestType != "minecraftModpack" || m.ManifestVersion != 1 {
		t.Errorf("manifest type/version = %s/%d", m.ManifestType, m.ManifestVersion)
	}
	if m.Name != "My Pack Server" || m.Version != "1.5.0" || m.Overrides != "overrides" {
		t.Errorf("name/version/overrides = %q/%q/%q", m.Name, m.Version, m.Overrides)
	}
	if m.Minecraft.Version != "1.21.1" ||
		len(m.Minecraft.ModLoaders) != 1 || m.Minecraft.ModLoaders[0].ID != "neoforge-21.1.77" ||
		!m.Minecraft.ModLoaders[0].Primary {
		t.Errorf("minecraft block = %+v", m.Minecraft)
	}
	// Both the server-side AND the clientonly CF-hosted mods are listed.
	if len(m.Files) != 2 {
		t.Fatalf("files = %+v, want 2 CF entries", m.Files)
	}
	seen := map[int64]int64{}
	for _, f := range m.Files {
		if !f.Required {
			t.Errorf("file %d not required", f.ProjectID)
		}
		seen[f.ProjectID] = f.FileID
	}
	if seen[111] != 1111 || seen[222] != 2222 {
		t.Errorf("CF ids = %v", seen)
	}
}

func TestExportOverridesContents(t *testing.T) {
	opts, dest := exportFixture(t)
	if err := Export(context.Background(), http.DefaultClient, opts, nil); err != nil {
		t.Fatalf("Export: %v", err)
	}
	files := readZip(t, dest)

	if files["overrides/config/pack.toml"] != "edited-on-server" {
		t.Errorf("server-edited config missing or wrong: %q", files["overrides/config/pack.toml"])
	}
	if _, ok := files["overrides/config/nested/extra.json"]; !ok {
		t.Error("nested config missing")
	}
	if _, ok := files["overrides/kubejs/server_scripts/recipes.js"]; !ok {
		t.Error("kubejs missing")
	}
	if files["overrides/mods/local-extra.jar"] != "hand-added-jar" {
		t.Error("hand-added mod missing from overrides")
	}
	if _, ok := files["overrides/mods/server-mod.jar"]; ok {
		t.Error("CF-covered mod duplicated into overrides")
	}
	// Client-only direct-URL file downloaded at export time.
	if files["overrides/resourcepacks/look.zip"] != "client-only-bytes" {
		t.Errorf("clientonly direct file = %q", files["overrides/resourcepacks/look.zip"])
	}
	for name := range files {
		for _, banned := range []string{"overrides/world/", "overrides/logs/", "overrides/libraries/",
			"overrides/server.properties", "overrides/eula.txt", "overrides/neoforge-installer.jar"} {
			if strings.HasPrefix(name, banned) {
				t.Errorf("banned path in zip: %s", name)
			}
		}
	}
}

func TestExportEmitsProgressSteps(t *testing.T) {
	opts, dest := exportFixture(t)
	var steps []string
	err := Export(context.Background(), http.DefaultClient, opts, func(p provider.Progress) {
		if p.Step != "" {
			steps = append(steps, p.Step)
		}
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	joined := strings.Join(steps, ",")
	if !strings.Contains(joined, "shop.export.preparing") ||
		!strings.Contains(joined, "shop.export.zipping") {
		t.Errorf("steps = %v", steps)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("dest zip missing: %v", err)
	}
}
