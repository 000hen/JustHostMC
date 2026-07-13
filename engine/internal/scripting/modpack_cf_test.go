package scripting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// builtinProvider loads the embedded providers and returns one by id.
func builtinProvider(t *testing.T, id string) *Entry {
	t.Helper()
	r := NewRegistry(NewHost(http.DefaultClient, nil, nil), nil)
	if err := LoadBuiltins(context.Background(), r); err != nil {
		t.Fatalf("LoadBuiltins: %v", err)
	}
	e, ok := r.Get(id)
	if !ok {
		t.Fatalf("%s provider not registered", id)
	}
	return e
}

func TestCurseForgeModpacksProviderHidden(t *testing.T) {
	e := builtinProvider(t, "curseforge_modpacks")
	if !e.Meta.Hidden {
		t.Error("curseforge_modpacks provider should be hidden")
	}
	if e.Meta.ModLayout != "mods" {
		t.Errorf("mod_layout = %q, want mods", e.Meta.ModLayout)
	}
	var hasKey bool
	for _, c := range e.Meta.Config {
		if c.Key == "curseforge_api_key" && c.Type == mcmanagerv1.ConfigOptionType_CONFIG_OPTION_SECRET {
			hasKey = true
		}
	}
	if !hasKey {
		t.Errorf("should declare a curseforge_api_key secret, config=%+v", e.Meta.Config)
	}
}

func TestCurseForgeModpacksShopIsModpackKind(t *testing.T) {
	ss := newBuiltinShops(t, http.NotFoundHandler())
	sh, ok := ss.Get("curseforge_modpacks")
	if !ok {
		t.Fatal("curseforge_modpacks shop not registered")
	}
	if !sh.Meta().NeedsKey {
		t.Error("curseforge_modpacks shop should need a key")
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

// TestCFPackMeta covers parsing a CurseForge client pack's manifest.json and
// resolving its file names/hashes/urls through the batch files endpoint.
func TestCFPackMeta(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/mods/files" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": 100, "modId": 10, "fileName": "cool-1.0.jar",
						"downloadUrl": "http://edge/cool-1.0.jar",
						"hashes":      []map[string]any{{"algo": 1, "value": "aaa"}}},
					{"id": 200, "modId": 20, "fileName": "neat-2.0.jar",
						"downloadUrl": nil, // author-restricted → resolved later via download-url
						"hashes":      []map[string]any{{"algo": 1, "value": "bbb"}}},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: rewriteTransport{target: u}}

	dir := t.TempDir()
	manifest := `{
	  "minecraft": { "version": "1.20.1", "modLoaders": [{"id":"forge-47.2.0","primary":true}] },
	  "manifestType": "minecraftModpack", "name": "Test Pack", "version": "1.0.0",
	  "files": [
	    {"projectID":10,"fileID":100,"required":true},
	    {"projectID":20,"fileID":200,"required":true}
	  ],
	  "overrides": "overrides"
	}`
	writeTestJar(t, filepath.Join(dir, ".jhmc", "pack.zip"), map[string]string{"manifest.json": manifest})

	inv := &invocation{
		ctx:     context.Background(),
		host:    NewHost(client, nil, nil),
		baseDir: dir,
		granted: GrantSet{
			mcmanagerv1.PermissionKind_PERMISSION_NETWORK:   true,
			mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER: true,
		},
	}
	src := `
function check()
  local m = mplib.cf_pack_meta(".jhmc/pack.zip", "KEY")
  assert(m.mc == "1.20.1", "mc " .. tostring(m.mc))
  assert(m.loader_name == "forge" and m.loader_version == "47.2.0", "loader")
  assert(m.name == "Test Pack" and m.version == "1.0.0", "name/version")
  assert(#m.files == 2, "files " .. #m.files)
  local by = {}
  for _, f in ipairs(m.files) do by[f.dest] = f end
  local a = by["mods/cool-1.0.jar"]
  assert(a and a.url == "http://edge/cool-1.0.jar", "cool url")
  assert(tostring(a.project_id) == "10" and tostring(a.file_id) == "100", "cool ids")
  assert(a.sha1 == "aaa", "cool sha1")
  local b = by["mods/neat-2.0.jar"]
  assert(b ~= nil, "neat present")
  assert(b.url == nil or b.url == "", "neat has no direct url")
  assert(b.sha1 == "bbb", "neat sha1")
  assert(tostring(b.project_id) == "20", "neat project")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

// TestCFPackMetaRequiresKey verifies the manifest lookup fails clearly without a
// CurseForge API key.
func TestCFPackMetaRequiresKey(t *testing.T) {
	dir := t.TempDir()
	manifest := `{
	  "minecraft": { "version": "1.20.1", "modLoaders": [{"id":"forge-47.2.0","primary":true}] },
	  "name": "P", "version": "1", "files": [{"projectID":10,"fileID":100}]
	}`
	writeTestJar(t, filepath.Join(dir, ".jhmc", "pack.zip"), map[string]string{"manifest.json": manifest})
	inv := &invocation{
		ctx:     context.Background(),
		host:    NewHost(http.DefaultClient, nil, nil),
		baseDir: dir,
		granted: GrantSet{
			mcmanagerv1.PermissionKind_PERMISSION_NETWORK:   true,
			mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER: true,
		},
	}
	err := runInv(t, inv, `function check() mplib.cf_pack_meta(".jhmc/pack.zip", "") end`)
	if err == nil {
		t.Fatal("expected an error without an API key")
	}
}
