-- Shared modpack helpers exposed as the global `mplib`. Providers that install a
-- modpack (FTB, CurseForge, Modrinth, local import) build on these instead of
-- copying the loader-install steps and manifest bookkeeping. Loaded into every
-- sandbox by prepare(); side-effect-free at load time (defines functions only,
-- no network or filesystem access).

local M = {}

local FABRIC_META = "https://meta.fabricmc.net/v2"
local FORGE_MAVEN = "https://maven.minecraftforge.net/net/minecraftforge/forge"
local NEOFORGE_MAVEN = "https://maven.neoforged.net/releases/net/neoforged/neoforge"

-- Where the normalized modpack manifest is persisted inside a server dir. Export
-- and update read it back so they work without re-fetching the upstream source.
local MANIFEST_DIR = ".jhmc"
local MANIFEST_PATH = ".jhmc/modpack.json"

M.MANIFEST_PATH = MANIFEST_PATH

-- join_path turns a { path = "./mods/", name = "x.jar" } pair into a clean
-- relative destination, rejecting any parent-directory segment. Host fs calls
-- also confine writes to the server dir, so this is defense in depth.
local function join_path(path, name)
  local p = (path or ""):gsub("\\", "/")
  p = p:gsub("^%./", ""):gsub("^/+", "")
  if p ~= "" and not p:match("/$") then
    p = p .. "/"
  end
  local dest = p .. (name or "")
  for seg in dest:gmatch("[^/]+") do
    if seg == ".." then
      error("unsafe file path in modpack: " .. dest)
    end
  end
  return dest
end
M.join_path = join_path

-- safe_path validates an already-joined relative path (e.g. a Modrinth file
-- "path" field) the same way join_path validates its output, returning it
-- cleaned.
local function safe_path(p)
  p = (p or ""):gsub("\\", "/"):gsub("^%./", ""):gsub("^/+", "")
  for seg in p:gmatch("[^/]+") do
    if seg == ".." then
      error("unsafe file path in modpack: " .. p)
    end
  end
  return p
end
M.safe_path = safe_path

-- cf_resolve_item builds a download_all entry for a CurseForge-hosted file whose
-- real URL is resolved inside the batch via the download-url endpoint (403 there
-- means the author disallows third-party downloads).
local function cf_resolve_item(dest, sha1, project, file, key)
  return {
    dest = dest,
    sha1 = sha1,
    resolve = {
      url = "https://api.curseforge.com/v1/mods/" .. project ..
        "/files/" .. file .. "/download-url",
      headers = { ["x-api-key"] = key },
    },
  }
end
M.cf_resolve_item = cf_resolve_item

-- find_args_file walks libraries/ for a generated win_args.txt. jhmc.fs.glob is
-- backed by Go's filepath.Glob (no recursive "**"), so probe nesting depths.
local function find_args_file()
  local prefix = "libraries"
  for _ = 1, 8 do
    prefix = prefix .. "/*"
    local hits = jhmc.fs.glob(prefix .. "/win_args.txt")
    if hits[1] then
      return hits[1]
    end
  end
  return nil
end
M.find_args_file = find_args_file

-- detect_launch prefers a generated win_args.txt arg file under libraries/, else
-- a runnable non-installer jar matching one of jar_patterns.
local function detect_launch(jar_patterns)
  local args_file = find_args_file()
  if args_file then
    return { "@" .. args_file, "nogui" }
  end
  for _, pat in ipairs(jar_patterns) do
    for _, jar in ipairs(jhmc.fs.glob(pat)) do
      if not jar:lower():find("installer") then
        return { "-jar", jar, "nogui" }
      end
    end
  end
  error("no win_args.txt or server jar after install")
end
M.detect_launch = detect_launch

-- fabric_installer returns the newest stable Fabric installer version (the loader
-- version itself is pinned by the pack).
local function fabric_installer()
  local entries = jhmc.http_json(FABRIC_META .. "/versions/installer")
  for _, e in ipairs(entries) do
    if e.stable and (e.version or "") ~= "" then return e.version end
  end
  for _, e in ipairs(entries) do
    if (e.version or "") ~= "" then return e.version end
  end
  error("version not found: fabric installer")
end

local function install_fabric(ctx, mc, loader_version)
  local installer = fabric_installer()
  local url = FABRIC_META .. "/versions/loader/" .. mc .. "/" .. loader_version ..
    "/" .. installer .. "/server/jar"
  ctx.step("install.progress.downloading_server", 0)
  ctx.log("server.jar")
  jhmc.download(url, { dest = "server.jar" })
  return { "-jar", "server.jar", "nogui" }
end
M.install_fabric = install_fabric

-- strip_mc_prefix removes a leading "<mc>-" from a loader version, so a pack that
-- pins forge as either "47.2.0" or "1.20.1-47.2.0" yields the same coordinate.
local function strip_mc_prefix(version, mc)
  local pfx = mc .. "-"
  if version:sub(1, #pfx) == pfx then
    return version:sub(#pfx + 1)
  end
  return version
end
M.strip_mc_prefix = strip_mc_prefix

local function install_forge(ctx, mc, forge_version, java_major)
  local full = mc .. "-" .. strip_mc_prefix(forge_version, mc)
  local url = FORGE_MAVEN .. "/" .. full .. "/forge-" .. full .. "-installer.jar"
  ctx.step("install.progress.downloading_installer", 0)
  ctx.log("forge-" .. full .. "-installer.jar")
  jhmc.download(url, { dest = "installer.jar" })
  ctx.step("install.progress.running_installer", -1)
  ctx.log("java -jar installer.jar --installServer")
  jhmc.run_jar({ java_major = java_major, args = { "-jar", "installer.jar", "--installServer" }, dir = "." })
  return detect_launch({ "forge*.jar", "server.jar" })
end
M.install_forge = install_forge

local function install_neoforge(ctx, neoforge_version, java_major)
  local url = NEOFORGE_MAVEN .. "/" .. neoforge_version ..
    "/neoforge-" .. neoforge_version .. "-installer.jar"
  ctx.step("install.progress.downloading_installer", 0)
  ctx.log("neoforge-" .. neoforge_version .. "-installer.jar")
  jhmc.download(url, { dest = "installer.jar" })
  ctx.step("install.progress.running_installer", -1)
  ctx.log("java -jar installer.jar --installServer")
  jhmc.run_jar({ java_major = java_major, args = { "-jar", "installer.jar", "--installServer" }, dir = "." })
  return detect_launch({ "neoforge*.jar", "forge*.jar", "server.jar" })
end
M.install_neoforge = install_neoforge

-- install_loader installs the pinned mod loader and returns the launch args.
local function install_loader(ctx, name, version, mc, java_major)
  if name == "fabric" then
    return install_fabric(ctx, mc, version)
  elseif name == "forge" then
    return install_forge(ctx, mc, version, java_major)
  elseif name == "neoforge" then
    return install_neoforge(ctx, version, java_major)
  end
  error("unsupported modloader: " .. name)
end
M.install_loader = install_loader

-- to_download_item turns one normalized file entry into a jhmc.download_all item.
-- Directly-hosted files carry a url; CurseForge-hosted ones (project_id +
-- file_id, no url) resolve their real URL in the batch and need an API key.
local function to_download_item(f, key)
  if (f.url or "") ~= "" then
    return { dest = f.dest, sha1 = f.sha1, url = f.url }
  end
  if f.project_id and f.file_id then
    if (key or "") == "" then
      error("install failed: file " .. tostring(f.dest) ..
        " is hosted on CurseForge and needs a CurseForge API key (set it in the provider settings)")
    end
    return cf_resolve_item(f.dest, f.sha1, f.project_id, f.file_id, key)
  end
  error("install failed: file " .. tostring(f.dest) .. " has no download source")
end
M.to_download_item = to_download_item

-- download_files downloads the server-side (non-client_only) files of a
-- normalized manifest in one parallel batch. The missing-key check runs up
-- front, before anything downloads.
local function download_files(ctx, files, key)
  local items = {}
  for _, f in ipairs(files or {}) do
    if type(f) == "table" and not f.client_only and (f.dest or "") ~= "" then
      items[#items + 1] = to_download_item(f, key)
    end
  end
  if #items == 0 then return end
  ctx.step("shop.install.downloading", 0)
  jhmc.download_all(items)
end
M.download_files = download_files

-- index_by_dest turns a normalized files array into a dest -> entry map.
local function index_by_dest(files)
  local by_dest = {}
  for _, f in ipairs(files or {}) do
    if type(f) == "table" and (f.dest or "") ~= "" then
      by_dest[f.dest] = f
    end
  end
  return by_dest
end
M.index_by_dest = index_by_dest

-- server_index is index_by_dest restricted to server-side (non-client_only)
-- files — the set that actually lives on disk, and the correct input for an
-- update diff so a file that was client-only in the old version is treated as
-- new rather than assumed present.
local function server_index(files)
  local by_dest = {}
  for _, f in ipairs(files or {}) do
    if type(f) == "table" and not f.client_only and (f.dest or "") ~= "" then
      by_dest[f.dest] = f
    end
  end
  return by_dest
end
M.server_index = server_index

-- diff_apply reconciles an installed set of files (old_files: dest -> normalized
-- entry) with a target set (new_files). Files the target dropped are deleted;
-- new or changed server files are downloaded in one batch. opts.changed(old,
-- new) decides whether an existing dest must be re-downloaded (default: sha1
-- differs); opts.to_item(entry) maps a target entry to a download_all item.
local function diff_apply(ctx, old_files, new_files, opts)
  local changed = opts.changed or function(old, new)
    return (old.sha1 or "") ~= (new.sha1 or "")
  end
  local to_item = opts.to_item

  -- Delete files the new version dropped (guarded by existence: client-only
  -- entries were never written to the server dir).
  for dest in pairs(old_files) do
    if not new_files[dest] and jhmc.fs.exists(dest) then
      ctx.log("- " .. dest)
      jhmc.fs.remove(dest)
    end
  end

  -- Download new files and files whose pack version changed.
  local items = {}
  for dest, f in pairs(new_files) do
    if not f.client_only then
      local old = old_files[dest]
      if not old or changed(old, f) then
        items[#items + 1] = to_item(f)
      end
    end
  end
  if #items == 0 then return end
  ctx.step("shop.install.downloading", 0)
  jhmc.download_all(items)
end
M.diff_apply = diff_apply

-- write_manifest persists the normalized modpack manifest into the server dir.
local function write_manifest(tbl)
  jhmc.fs.mkdir(MANIFEST_DIR)
  jhmc.fs.write(MANIFEST_PATH, jhmc.json_encode(tbl))
end
M.write_manifest = write_manifest

-- read_manifest loads the normalized manifest, or returns nil when absent.
local function read_manifest()
  if not jhmc.fs.exists(MANIFEST_PATH) then
    return nil
  end
  return jhmc.json_decode(jhmc.fs.read(MANIFEST_PATH))
end
M.read_manifest = read_manifest

mplib = M
