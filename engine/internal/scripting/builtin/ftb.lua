-- FTB modpack provider: installs a Feed The Beast modpack as a self-contained
-- server. It is hidden from the create-server UI — the "ftb" shop drives it,
-- passing an opaque "<packId>/<versionId>" as the version. install() fetches the
-- pack's version manifest, downloads its server files (resolving CurseForge-hosted
-- ones with the configured key), then installs the pinned mod loader by
-- replicating the exact-version steps of the fabric/forge/neoforge providers.

meta = {
  id = "ftb",
  name = "FTB Modpack",
  website = "https://www.feed-the-beast.com",
  description = "Installs a Feed The Beast modpack as a self-contained server.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "mods",
  hidden = true, -- driven by the FTB shop, not offered in the create-server list
  permissions = {
    { kind = "network", reason = "Download the modpack manifest, its files, and the loader installer." },
    { kind = "install", reason = "Resolve a JRE and run the Forge/NeoForge installer's --installServer." },
    { kind = "fs_server", reason = "Write the pack's mods/configs and detect the launch command." },
  },
  config = {
    { key = "curseforge_api_key", type = "secret", name = "CurseForge API key",
      description = "Used for pack files hosted on CurseForge", required = false },
  },
}

local FTB_API = "https://api.feed-the-beast.com/v1/modpacks/public"
local FABRIC_META = "https://meta.fabricmc.net/v2"
local FORGE_MAVEN = "https://maven.minecraftforge.net/net/minecraftforge/forge"
local NEOFORGE_MAVEN = "https://maven.neoforged.net/releases/net/neoforged/neoforge"

-- versions is required by the provider contract but never a real picker: a
-- modpack installs from an opaque "packId/versionId" chosen in the shop, so this
-- provider is hidden and Create never validates against this list.
function versions()
  return {}
end

-- curseforge_key returns the configured CurseForge API key ("" when unset). It is
-- seeded from the shared shop key by cmd/engine wiring when the user leaves it
-- blank.
local function curseforge_key(ctx)
  local cfg = ctx.config or {}
  return cfg.curseforge_api_key or ""
end

-- join_path turns an FTB file's { path = "./mods/", name = "x.jar" } into a clean
-- relative destination, rejecting any parent-directory segment. jhmc.download
-- also confines writes to the server dir, so this is defense in depth.
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

-- install_files downloads every server-side file in the manifest through one
-- parallel jhmc.download_all batch. Directly-hosted files carry a url;
-- CurseForge-hosted ones resolve their real URL inside the batch workers via
-- the download-url endpoint (403 = the author disallows third-party
-- downloads). The missing-key check runs up front, before anything downloads.
local function install_files(ctx, files)
  local key = curseforge_key(ctx)
  local items = {}
  for _, f in ipairs(files) do
    if type(f) == "table" and not f.clientonly and (f.name or "") ~= "" then
      local dest = join_path(f.path, f.name)
      if (f.url or "") ~= "" then
        items[#items + 1] = { dest = dest, sha1 = f.sha1, url = f.url }
      else
        local cf = f.curseforge
        if type(cf) == "table" and cf.project and cf.file then
          if key == "" then
            error("install failed: file " .. (f.name or dest) ..
              " is hosted on CurseForge and needs a CurseForge API key (set it in the FTB provider settings)")
          end
          items[#items + 1] = { dest = dest, sha1 = f.sha1, resolve = {
            url = "https://api.curseforge.com/v1/mods/" .. cf.project ..
              "/files/" .. cf.file .. "/download-url",
            headers = { ["x-api-key"] = key },
          } }
        else
          error("install failed: file " .. (f.name or dest) .. " has no download source")
        end
      end
    end
  end
  if #items == 0 then return end
  ctx.step("shop.install.downloading", 0)
  jhmc.download_all(items)
end

-- find_args_file walks libraries/ for a generated win_args.txt. jhmc.fs.glob is
-- backed by Go's filepath.Glob (no recursive "**"), so we probe nesting depths
-- (copied from the forge/neoforge providers).
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

-- detect_launch prefers a generated win_args.txt arg file under libraries/, else
-- a runnable non-installer jar matching one of jar_patterns (copied from the
-- forge/neoforge providers).
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

-- strip_mc_prefix removes a leading "<mc>-" from a loader version, so a pack that
-- pins forge as either "47.2.0" or "1.20.1-47.2.0" yields the same coordinate.
local function strip_mc_prefix(version, mc)
  local pfx = mc .. "-"
  if version:sub(1, #pfx) == pfx then
    return version:sub(#pfx + 1)
  end
  return version
end

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

-- read_targets extracts the Minecraft version and mod loader (name + pinned
-- version) from the manifest's targets array.
local function read_targets(manifest)
  local mc, loader_name, loader_version
  for _, t in ipairs(manifest.targets or {}) do
    if type(t) == "table" then
      if t.type == "game" then
        mc = t.version
      elseif t.type == "modloader" then
        loader_name = string.lower(t.name or "")
        loader_version = t.version
      end
    end
  end
  return mc, loader_name, loader_version
end

function install(ctx)
  local pack, ver = tostring(ctx.version):match("^([^/]+)/([^/]+)$")
  if not pack or not ver then
    error("invalid modpack version id: " .. tostring(ctx.version))
  end

  ctx.step("install.progress.resolving_version", -1)
  local manifest = jhmc.http_json(FTB_API .. "/modpack/" .. pack .. "/" .. ver)
  if type(manifest) ~= "table" then
    error("ftb: bad manifest for " .. tostring(ctx.version))
  end

  local mc, loader_name, loader_version = read_targets(manifest)
  if not mc or mc == "" then
    error("ftb: modpack has no Minecraft version target")
  end
  if not loader_name or loader_name == "" then
    error("ftb: modpack has no modloader target")
  end

  install_files(ctx, manifest.files or {})

  local java_major = jhmc.java_major_for(mc)
  local args = install_loader(ctx, loader_name, loader_version, mc, java_major)

  ctx.step("install.progress.done", 1)
  return {
    java_major = java_major,
    args = args,
    mc_version = mc,
    loader = loader_name,
    pack_version = tostring(ctx.version),
  }
end

-- server_files indexes a manifest's server-side files by relative dest path.
local function server_files(manifest)
  local by_dest = {}
  for _, f in ipairs(manifest.files or {}) do
    if type(f) == "table" and not f.clientonly and (f.name or "") ~= "" then
      by_dest[join_path(f.path, f.name)] = f
    end
  end
  return by_dest
end

-- update moves an installed pack to another version of the same pack: files
-- only the old version listed are deleted, new/changed ones are downloaded,
-- and the loader is reinstalled only when its pinned target changed. Files the
-- pack never listed (the world, user-added configs) are untouched; a pack file
-- the user edited is overwritten by the new pack version — pack files belong
-- to the pack.
function update(ctx)
  local pack, ver = tostring(ctx.version):match("^([^/]+)/([^/]+)$")
  local opack, over = tostring(ctx.old_version):match("^([^/]+)/([^/]+)$")
  if not pack or not ver then
    error("invalid modpack version id: " .. tostring(ctx.version))
  end
  if not opack or not over then
    error("invalid modpack version id: " .. tostring(ctx.old_version))
  end
  if pack ~= opack then
    error("update must stay within the same pack (" .. opack .. " -> " .. pack .. ")")
  end

  ctx.step("install.progress.resolving_version", -1)
  local old_manifest = jhmc.http_json(FTB_API .. "/modpack/" .. opack .. "/" .. over)
  local new_manifest = jhmc.http_json(FTB_API .. "/modpack/" .. pack .. "/" .. ver)
  if type(old_manifest) ~= "table" or type(new_manifest) ~= "table" then
    error("ftb: bad manifest for update " .. tostring(ctx.old_version) ..
      " -> " .. tostring(ctx.version))
  end

  local old_files, new_files = server_files(old_manifest), server_files(new_manifest)

  -- Delete files the new version dropped.
  for dest in pairs(old_files) do
    if not new_files[dest] and jhmc.fs.exists(dest) then
      ctx.log("- " .. dest)
      jhmc.fs.remove(dest)
    end
  end

  -- Download new files and files whose pack hash changed.
  local changed = {}
  for dest, f in pairs(new_files) do
    local old = old_files[dest]
    if not old or (old.sha1 or "") ~= (f.sha1 or "") then
      changed[#changed + 1] = f
    end
  end
  install_files(ctx, changed)

  local mc, loader_name, loader_version = read_targets(new_manifest)
  if not mc or mc == "" then
    error("ftb: modpack has no Minecraft version target")
  end
  if not loader_name or loader_name == "" then
    error("ftb: modpack has no modloader target")
  end
  local omc, oloader_name, oloader_version = read_targets(old_manifest)

  local java_major = jhmc.java_major_for(mc)
  local args
  if loader_name ~= oloader_name or loader_version ~= oloader_version or mc ~= omc then
    args = install_loader(ctx, loader_name, loader_version, mc, java_major)
  end

  ctx.step("install.progress.done", 1)
  return {
    java_major = java_major,
    -- args is nil when the loader target is unchanged; the engine keeps the
    -- server's existing launch args in that case.
    args = args,
    mc_version = mc,
    loader = loader_name,
    pack_version = tostring(ctx.version),
  }
end
