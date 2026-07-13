-- CurseForge modpack provider: installs a CurseForge modpack as a self-contained
-- server from its client pack (manifest.json + overrides/). Hidden from the
-- create-server UI — the "curseforge_modpacks" shop drives it, passing an opaque
-- "<projectId>/<fileId>" as the version. install() downloads the pack zip,
-- resolves its listed mods through the CurseForge API, installs the pinned loader
-- (shared mplib helpers), and persists a normalized manifest so export/update work
-- without re-fetching. The CurseForge API key is reused from the CurseForge shop.

meta = {
  id = "curseforge_modpacks",
  name = "CurseForge",
  website = "https://www.curseforge.com",
  description = "Installs a CurseForge modpack as a self-contained server.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "mods",
  hidden = true,
  permissions = {
    { kind = "network", reason = "Download the modpack, resolve its files via the CurseForge API, and fetch the loader installer." },
    { kind = "install", reason = "Resolve a JRE and run the Forge/NeoForge installer's --installServer." },
    { kind = "fs_server", reason = "Write the pack's mods/configs and detect the launch command." },
  },
  config = {
    { key = "curseforge_api_key", type = "secret", name = "CurseForge API key",
      description = "Required to resolve and download CurseForge pack files", required = false },
  },
}

local API = "https://api.curseforge.com"

function versions()
  return {}
end

local function key(ctx)
  local cfg = ctx.config or {}
  return cfg.curseforge_api_key or ""
end

-- pack_zip_url resolves the direct download URL for a CurseForge pack file,
-- falling back to the dedicated download-url endpoint when downloadUrl is null.
local function pack_zip_url(project, file, k)
  local res = jhmc.http({
    url = API .. "/v1/mods/" .. project .. "/files/" .. file,
    headers = { ["x-api-key"] = k },
  })
  if res.status < 200 or res.status >= 300 then
    error("curseforge: HTTP " .. res.status .. " resolving pack file " .. project .. "/" .. file)
  end
  local f = jhmc.json_decode(res.body).data or {}
  local url = f.downloadUrl
  if not url or url == "" then
    local r2 = jhmc.http({
      url = API .. "/v1/mods/" .. project .. "/files/" .. file .. "/download-url",
      headers = { ["x-api-key"] = k },
    })
    if r2.status >= 200 and r2.status < 300 then
      url = jhmc.json_decode(r2.body).data
    end
  end
  if not url or url == "" then
    error("curseforge: modpack file not distributable (author disabled third-party downloads)")
  end
  return url
end

-- fetch_pack downloads the pack zip for "<project>/<file>" to .jhmc/pack.zip.
local function fetch_pack(ctx, project, file, k)
  local url = pack_zip_url(project, file, k)
  ctx.step("install.progress.downloading_server", 0)
  ctx.log("modpack.zip")
  jhmc.fs.mkdir(".jhmc")
  jhmc.download(url, { dest = ".jhmc/pack.zip" })
end

local function parse_version(v)
  local project, file = tostring(v):match("^([^/]+)/([^/]+)$")
  if not project or not file then
    error("invalid modpack version id: " .. tostring(v))
  end
  return project, file
end

function install(ctx)
  local project, file = parse_version(ctx.version)
  local k = key(ctx)
  if k == "" then
    error("this modpack is hosted on CurseForge and needs a CurseForge API key (set it in the provider settings)")
  end

  ctx.step("install.progress.resolving_version", -1)
  fetch_pack(ctx, project, file, k)

  local spec = mplib.install_cf_pack(ctx, ".jhmc/pack.zip", k)
  jhmc.fs.remove(".jhmc/pack.zip")

  ctx.step("install.progress.done", 1)
  spec.pack_version = tostring(ctx.version)
  return spec
end

function update(ctx)
  local project, file = parse_version(ctx.version)
  local oproject = parse_version(ctx.old_version)
  if project ~= oproject then
    error("update must stay within the same modpack (" .. oproject .. " -> " .. project .. ")")
  end
  local k = key(ctx)
  if k == "" then
    error("this modpack is hosted on CurseForge and needs a CurseForge API key (set it in the provider settings)")
  end
  local old = mplib.read_manifest()
  if not old then
    error("cannot update: this server has no saved modpack manifest")
  end

  ctx.step("install.progress.resolving_version", -1)
  fetch_pack(ctx, project, file, k)
  local m = mplib.cf_pack_meta(".jhmc/pack.zip", k)

  mplib.diff_apply(ctx, mplib.server_index(old.files or {}), mplib.server_index(m.files), {
    changed = function(a, b) return tostring(a.file_id or "") ~= tostring(b.file_id or "") end,
    to_item = function(x) return mplib.to_download_item(x, k) end,
  })
  jhmc.unzip(".jhmc/pack.zip", ".", { prefix = "overrides/" })

  local java_major = jhmc.java_major_for(m.mc)
  local args
  if m.loader_name ~= old.loader or m.loader_version ~= old.loader_version or m.mc ~= old.mc_version then
    args = mplib.install_loader(ctx, m.loader_name, m.loader_version, m.mc, java_major)
  end

  mplib.write_manifest({
    format = 1,
    name = m.name,
    version_name = m.version,
    mc_version = m.mc,
    loader = m.loader_name,
    loader_version = m.loader_version,
    files = m.files,
  })
  jhmc.fs.remove(".jhmc/pack.zip")

  ctx.step("install.progress.done", 1)
  return {
    java_major = java_major,
    args = args,
    mc_version = m.mc,
    loader = m.loader_name,
    pack_version = tostring(ctx.version),
  }
end
