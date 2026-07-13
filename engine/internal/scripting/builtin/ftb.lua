-- FTB modpack provider: installs a Feed The Beast modpack as a self-contained
-- server. It is hidden from the create-server UI — the "ftb" shop drives it,
-- passing an opaque "<packId>/<versionId>" as the version. install() fetches the
-- pack's version manifest, downloads its server files (resolving CurseForge-hosted
-- ones with the configured key), installs the pinned mod loader via the shared
-- mplib helpers, and persists a normalized manifest so export/update work without
-- re-fetching the FTB API.

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

-- normalize_files maps the FTB manifest's files into mplib's normalized entries.
-- Client-only files are kept but flagged so export can carry them while install
-- and update skip them.
local function normalize_files(manifest)
  local out = {}
  for _, f in ipairs(manifest.files or {}) do
    if type(f) == "table" and (f.name or "") ~= "" then
      local entry = { dest = mplib.join_path(f.path, f.name), sha1 = f.sha1 }
      if (f.url or "") ~= "" then
        entry.url = f.url
      elseif type(f.curseforge) == "table" and f.curseforge.project and f.curseforge.file then
        entry.project_id = f.curseforge.project
        entry.file_id = f.curseforge.file
      end
      if f.clientonly then
        entry.client_only = true
      end
      out[#out + 1] = entry
    end
  end
  return out
end

-- persist writes the normalized manifest consumed by export and update.
local function persist(manifest, ver, files, mc, loader_name, loader_version)
  local version_name = (manifest.name and manifest.name ~= "") and manifest.name or ver
  mplib.write_manifest({
    format = 1,
    name = manifest.name or "",
    version_name = version_name,
    mc_version = mc,
    loader = loader_name,
    loader_version = loader_version,
    files = files,
  })
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

  local files = normalize_files(manifest)
  mplib.download_files(ctx, files, curseforge_key(ctx))

  local java_major = jhmc.java_major_for(mc)
  local args = mplib.install_loader(ctx, loader_name, loader_version, mc, java_major)

  persist(manifest, ver, files, mc, loader_name, loader_version)

  ctx.step("install.progress.done", 1)
  return {
    java_major = java_major,
    args = args,
    mc_version = mc,
    loader = loader_name,
    pack_version = tostring(ctx.version),
  }
end

-- update moves an installed pack to another version of the same pack: files
-- only the old version listed are deleted, new/changed ones are downloaded, and
-- the loader is reinstalled only when its pinned target changed. Files the pack
-- never listed (the world, user-added configs) are untouched; a pack file the
-- user edited is overwritten by the new pack version — pack files belong to the
-- pack.
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

  local mc, loader_name, loader_version = read_targets(new_manifest)
  if not mc or mc == "" then
    error("ftb: modpack has no Minecraft version target")
  end
  if not loader_name or loader_name == "" then
    error("ftb: modpack has no modloader target")
  end

  local old_files = normalize_files(old_manifest)
  local new_files = normalize_files(new_manifest)
  local key = curseforge_key(ctx)
  mplib.diff_apply(ctx, mplib.server_index(old_files), mplib.server_index(new_files), {
    to_item = function(f) return mplib.to_download_item(f, key) end,
  })

  local omc, oloader_name, oloader_version = read_targets(old_manifest)
  local java_major = jhmc.java_major_for(mc)
  local args
  if loader_name ~= oloader_name or loader_version ~= oloader_version or mc ~= omc then
    args = mplib.install_loader(ctx, loader_name, loader_version, mc, java_major)
  end

  persist(new_manifest, ver, new_files, mc, loader_name, loader_version)

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
