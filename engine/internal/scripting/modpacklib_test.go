package scripting

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

// fsInv returns an invocation rooted at dir with the fs_server grant, for
// exercising mplib helpers that read and write the server directory.
func fsInv(dir string) *invocation {
	return &invocation{
		ctx:     context.Background(),
		host:    NewHost(nil, nil, nil),
		baseDir: dir,
		granted: GrantSet{mcmanagerv1.PermissionKind_PERMISSION_FS_SERVER: true},
	}
}

func TestModpackLibGlobalAndJoinPath(t *testing.T) {
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	src := `
function check()
  assert(type(mplib) == "table", "mplib global missing")
  assert(mplib.join_path("./mods/", "a.jar") == "mods/a.jar", "join")
  assert(mplib.join_path("", "a.jar") == "a.jar", "join empty path")
  assert(mplib.join_path("config\\sub", "b.toml") == "config/sub/b.toml", "join backslash")
  local ok = pcall(function() mplib.join_path("../evil", "x") end)
  assert(not ok, "traversal must be rejected")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

// TestModpackManifestPersisted validates the exact on-disk shape of
// .jhmc/modpack.json — the contract the Go export path reads back.
func TestModpackManifestPersisted(t *testing.T) {
	dir := t.TempDir()
	inv := fsInv(dir)
	src := `
function check()
  assert(mplib.read_manifest() == nil, "manifest absent before write")
  mplib.write_manifest({
    format = 1,
    name = "All the Mods 9",
    version_name = "0.2.60",
    mc_version = "1.20.1",
    loader = "forge",
    loader_version = "47.2.20",
    files = {
      { dest = "mods/direct.jar", sha1 = "aa", url = "https://example/direct.jar" },
      { dest = "mods/cf.jar", sha1 = "bb", project_id = 123, file_id = 456 },
      { dest = "mods/client.jar", client_only = true, project_id = 1, file_id = 2 },
    },
  })
  local back = mplib.read_manifest()
  assert(back ~= nil and back.name == "All the Mods 9", "round-trip name")
  assert(#back.files == 3, "round-trip file count")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}

	// Assert the on-disk JSON matches the schema the Go export consumes.
	raw, err := os.ReadFile(filepath.Join(dir, ".jhmc", "modpack.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m struct {
		Format        int    `json:"format"`
		Name          string `json:"name"`
		VersionName   string `json:"version_name"`
		MCVersion     string `json:"mc_version"`
		Loader        string `json:"loader"`
		LoaderVersion string `json:"loader_version"`
		Files         []struct {
			Dest       string `json:"dest"`
			Sha1       string `json:"sha1"`
			URL        string `json:"url"`
			ProjectID  int64  `json:"project_id"`
			FileID     int64  `json:"file_id"`
			ClientOnly bool   `json:"client_only"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if m.Format != 1 || m.MCVersion != "1.20.1" || m.Loader != "forge" || m.LoaderVersion != "47.2.20" {
		t.Fatalf("manifest header wrong: %+v", m)
	}
	if len(m.Files) != 3 {
		t.Fatalf("files = %d, want 3", len(m.Files))
	}
	byDest := map[string]int{}
	for i, f := range m.Files {
		byDest[f.Dest] = i
	}
	if f := m.Files[byDest["mods/direct.jar"]]; f.URL != "https://example/direct.jar" || f.Sha1 != "aa" {
		t.Fatalf("direct file wrong: %+v", f)
	}
	if f := m.Files[byDest["mods/cf.jar"]]; f.ProjectID != 123 || f.FileID != 456 {
		t.Fatalf("cf file wrong: %+v", f)
	}
	if f := m.Files[byDest["mods/client.jar"]]; !f.ClientOnly {
		t.Fatalf("client file should be client_only: %+v", f)
	}
}

func TestModpackToDownloadItem(t *testing.T) {
	inv := &invocation{ctx: context.Background(), host: NewHost(nil, nil, nil)}
	src := `
function check()
  local u = mplib.to_download_item({ dest = "mods/a.jar", sha1 = "abc", url = "http://x/a.jar" }, "")
  assert(u.url == "http://x/a.jar" and u.dest == "mods/a.jar" and u.sha1 == "abc", "direct item")

  local c = mplib.to_download_item({ dest = "mods/b.jar", sha1 = "d", project_id = 1, file_id = 2 }, "KEY")
  assert(c.resolve.url:find("/mods/1/files/2/download%-url"), "cf resolve url: " .. c.resolve.url)
  assert(c.resolve.headers["x-api-key"] == "KEY", "cf key header")

  local ok, err = pcall(function()
    mplib.to_download_item({ dest = "mods/c.jar", project_id = 1, file_id = 2 }, "")
  end)
  assert(not ok, "missing key must error")
  assert(tostring(err):find("CurseForge API key"), "error should mention the key: " .. tostring(err))
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

// TestModpackDiffApplyDeletes covers diff_apply's delete path with no downloads:
// files the new version dropped are removed, unchanged ones are kept.
func TestModpackDiffApplyDeletes(t *testing.T) {
	dir := t.TempDir()
	inv := fsInv(dir)
	src := `
function check()
  local ctx = { log = function() end, step = function() end }
  jhmc.fs.write("mods/keep.jar", "K")
  jhmc.fs.write("mods/drop.jar", "D")
  local old = {
    ["mods/keep.jar"] = { dest = "mods/keep.jar", sha1 = "1" },
    ["mods/drop.jar"] = { dest = "mods/drop.jar", sha1 = "1" },
  }
  local new = { ["mods/keep.jar"] = { dest = "mods/keep.jar", sha1 = "1" } }
  mplib.diff_apply(ctx, old, new, {
    to_item = function() error("no downloads expected") end,
  })
  assert(jhmc.fs.exists("mods/keep.jar"), "kept file must remain")
  assert(not jhmc.fs.exists("mods/drop.jar"), "dropped file must be deleted")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestUnzipPrefix(t *testing.T) {
	dir := t.TempDir()
	writeTestJar(t, filepath.Join(dir, "pack.zip"), map[string]string{
		"manifest.json":           `{"name":"x"}`,
		"overrides/config/a.toml": "A",
		"overrides/mods/m.jar":    "M",
	})
	inv := fsInv(dir)
	src := `
function check()
  -- Plain unzip extracts everything verbatim.
  jhmc.unzip("pack.zip", "full")
  assert(jhmc.fs.exists("full/manifest.json"), "full manifest")
  assert(jhmc.fs.exists("full/overrides/config/a.toml"), "full override")

  -- Prefixed unzip extracts only overrides/, stripping the prefix.
  jhmc.unzip("pack.zip", "srv", { prefix = "overrides/" })
  assert(jhmc.fs.exists("srv/config/a.toml"), "stripped config")
  assert(jhmc.fs.read("srv/config/a.toml") == "A", "stripped content")
  assert(jhmc.fs.exists("srv/mods/m.jar"), "stripped mod")
  assert(not jhmc.fs.exists("srv/manifest.json"), "manifest excluded by prefix")
  assert(not jhmc.fs.exists("srv/overrides/config/a.toml"), "prefix not stripped")
end`
	if err := runInv(t, inv, src); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}
