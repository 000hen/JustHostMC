package modpack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// persistedFixture builds a non-FTB modpack server: a server dir carrying a
// persisted .jhmc/modpack.json plus live files, and a server that serves the one
// client-only direct-URL file. The FTB API is deliberately not wired — a correct
// export must never reach it when the manifest is present.
func persistedFixture(t *testing.T) (Options, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/blob/look.zip", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("client-only-bytes"))
	})
	mux.HandleFunc("/modpack/", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("FTB API must not be called when a persisted manifest exists")
		http.Error(w, "unexpected", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
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
	manifest := fmt.Sprintf(`{
	  "format": 1,
	  "name": "Persisted Pack",
	  "version_name": "2.0.0",
	  "mc_version": "1.21.1",
	  "loader": "neoforge",
	  "loader_version": "21.1.77",
	  "files": [
	    { "dest": "mods/server-mod.jar", "sha1": "a", "project_id": 111, "file_id": 1111 },
	    { "dest": "mods/client-mod.jar", "client_only": true, "project_id": 222, "file_id": 2222 },
	    { "dest": "resourcepacks/look.zip", "client_only": true, "url": "%s/blob/look.zip" },
	    { "dest": "config/pack.toml", "url": "IGNORED-server-file-from-disk" }
	  ]
	}`, srv.URL)
	mustWrite(".jhmc/modpack.json", manifest)
	mustWrite("config/pack.toml", "edited-on-server") // server-side → from disk
	mustWrite("mods/server-mod.jar", "cf-covered")    // covered by CF entry → excluded
	mustWrite("mods/local-extra.jar", "hand-added")   // not in manifest → included
	mustWrite("world/level.dat", "world")             // excluded

	dest := filepath.Join(t.TempDir(), "out.zip")
	return Options{
		ServerDir:   serverDir,
		DestZip:     dest,
		PackVersion: "999/1", // must be ignored in favor of the persisted manifest
		ServerName:  "Persisted Server",
		ProviderID:  "curseforge_modpacks",
		FTBAPIBase:  srv.URL,
	}, dest
}

func TestExportFromPersistedManifest(t *testing.T) {
	opts, dest := persistedFixture(t)
	if err := Export(context.Background(), http.DefaultClient, opts, nil); err != nil {
		t.Fatalf("Export: %v", err)
	}
	files := readZip(t, dest)

	var m struct {
		ManifestType string `json:"manifestType"`
		Name         string `json:"name"`
		Version      string `json:"version"`
		Overrides    string `json:"overrides"`
		Minecraft    struct {
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
	if m.ManifestType != "minecraftModpack" || m.Name != "Persisted Server" ||
		m.Version != "2.0.0" || m.Overrides != "overrides" {
		t.Errorf("header = type=%q name=%q version=%q overrides=%q",
			m.ManifestType, m.Name, m.Version, m.Overrides)
	}
	if m.Minecraft.Version != "1.21.1" || len(m.Minecraft.ModLoaders) != 1 ||
		m.Minecraft.ModLoaders[0].ID != "neoforge-21.1.77" || !m.Minecraft.ModLoaders[0].Primary {
		t.Errorf("minecraft = %+v", m.Minecraft)
	}
	// Both the server-side and the client-only CF-hosted mods are listed.
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

	// Overrides: live config + hand-added mod + downloaded client-only file;
	// the CF-covered mod must not be duplicated; excluded dirs stay out.
	if files["overrides/config/pack.toml"] != "edited-on-server" {
		t.Errorf("server config = %q", files["overrides/config/pack.toml"])
	}
	if files["overrides/mods/local-extra.jar"] != "hand-added" {
		t.Error("hand-added mod missing")
	}
	if _, ok := files["overrides/mods/server-mod.jar"]; ok {
		t.Error("CF-covered mod duplicated into overrides")
	}
	if files["overrides/resourcepacks/look.zip"] != "client-only-bytes" {
		t.Errorf("client-only direct file = %q", files["overrides/resourcepacks/look.zip"])
	}
	for name := range files {
		if name == "overrides/world/level.dat" {
			t.Errorf("banned path in zip: %s", name)
		}
	}
}
