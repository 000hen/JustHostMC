-- Modrinth modpack provider: installs a Modrinth modpack (.mrpack) as a
-- self-contained server. Hidden from the create-server UI — the
-- "modrinth_modpacks" shop drives it, passing an opaque "<projectId>/<versionId>"
-- as the version. install() downloads the .mrpack, installs its listed files and
-- the pinned loader (shared mplib helpers), and persists a normalized manifest so
-- export/update work without re-fetching. Modrinth read access is keyless.

meta = {
  id = "modrinth_modpacks",
  name = "Modrinth",
  website = "https://modrinth.com",
  description = "Installs a Modrinth modpack as a self-contained server.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "mods",
  hidden = true,
  permissions = {
    { kind = "network", reason = "Download the modpack, its files, and the loader installer." },
    { kind = "install", reason = "Resolve a JRE and run the Forge/NeoForge installer's --installServer." },
    { kind = "fs_server", reason = "Write the pack's mods/configs and detect the launch command." },
  },
}

local API = "https://api.modrinth.com/v2"

function versions()
  return {}
end

local function parse_version(v)
  local project, version_id = tostring(v):match("^([^/]+)/([^/]+)$")
  if not project or not version_id then
    error("invalid modpack version id: " .. tostring(v))
  end
  return project, version_id
end

-- primary_file picks the .mrpack artifact: the primary flag when set, else the
-- first file.
local function primary_file(v)
  local f = v.files and v.files[1]
  for _, cand in ipairs(v.files or {}) do
    if cand.primary then f = cand break end
  end
  return f
end

-- fetch_pack downloads the .mrpack for version_id to .jhmc/pack.mrpack.
local function fetch_pack(ctx, version_id)
  local res = jhmc.http({ url = API .. "/version/" .. version_id })
  if res.status < 200 or res.status >= 300 then
    error("modrinth: HTTP " .. res.status .. " resolving version " .. version_id)
  end
  local f = primary_file(jhmc.json_decode(res.body))
  if not f or not f.url or f.url == "" then
    error("modrinth: version has no downloadable file")
  end
  ctx.step("install.progress.downloading_server", 0)
  ctx.log(f.filename or "modpack.mrpack")
  jhmc.fs.mkdir(".jhmc")
  jhmc.download(f.url, { dest = ".jhmc/pack.mrpack", sha1 = f.hashes and f.hashes.sha1 })
end

function install(ctx)
  local _, version_id = parse_version(ctx.version)

  ctx.step("install.progress.resolving_version", -1)
  fetch_pack(ctx, version_id)

  local spec = mplib.install_mrpack(ctx, ".jhmc/pack.mrpack")
  jhmc.fs.remove(".jhmc/pack.mrpack")

  ctx.step("install.progress.done", 1)
  spec.pack_version = tostring(ctx.version)
  return spec
end

function update(ctx)
  local project, version_id = parse_version(ctx.version)
  local oproject = parse_version(ctx.old_version)
  if project ~= oproject then
    error("update must stay within the same modpack (" .. oproject .. " -> " .. project .. ")")
  end
  local old = mplib.read_manifest()
  if not old then
    error("cannot update: this server has no saved modpack manifest")
  end

  ctx.step("install.progress.resolving_version", -1)
  fetch_pack(ctx, version_id)
  local m = mplib.mrpack_meta(".jhmc/pack.mrpack")

  mplib.diff_apply(ctx, mplib.server_index(old.files or {}), mplib.server_index(m.files), {
    to_item = function(x) return mplib.to_download_item(x, nil) end,
  })
  mplib.unzip_overrides(".jhmc/pack.mrpack")

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
  jhmc.fs.remove(".jhmc/pack.mrpack")

  ctx.step("install.progress.done", 1)
  return {
    java_major = java_major,
    args = args,
    mc_version = m.mc,
    loader = m.loader_name,
    pack_version = tostring(ctx.version),
  }
end
