-- Feed The Beast source: one script, two roles. The `shop` role browses/searches
-- the keyless FTB v1 API (https://api.feed-the-beast.com/v1/modpacks/public) for
-- modpacks; the hidden `provider` role installs an FTB modpack as a
-- self-contained server, resolving CurseForge-hosted files with a CurseForge API
-- key. Both roles share meta.id ("ftb"). The provider's key is CurseForge's, kept
-- under its own config key "curseforge_api_key" (distinct from the CurseForge
-- source's api_key) so existing stored values and the fallback wiring survive.
--   GET /modpack/featured/{n}          -> { packs = [id...], total }
--   GET /modpack/popular/installs/{n}  -> { packs = [id...], total }
--   GET /modpack/search/{n}?term=...   -> { packs = [id...], total }
--   GET /modpack/{id}                  -> pack detail
--   GET /modpack/{id}/{versionId}      -> version manifest (provider install)

meta = {
  id = "ftb",
  name = "FTB Modpacks",
  website = "https://www.feed-the-beast.com",
  description = "Browse and install Feed The Beast modpacks as new servers.",
  version = "1.0.0",
  author = "JustHostMC",
  config = {
    { key = "curseforge_api_key", type = "secret", name = "CurseForge API key",
      description = "Used for pack files hosted on CurseForge", required = false },
  },
  permissions = {
    { kind = "network", reason = "Query the FTB API to browse and install modpacks, and download their files and loader installer." },
    { kind = "install", reason = "Resolve a JRE and run the Forge/NeoForge installer's --installServer." },
    { kind = "fs_server", reason = "Write the pack's mods/configs and detect the launch command." },
  },
}

local BASE = "https://api.feed-the-beast.com/v1/modpacks/public"

-- Cache TTLs (seconds): browse lists refresh often; pack detail leans on ETag
-- revalidation for longer.
local TTL_BROWSE = 120
local TTL_DETAIL = 600

-- Per-section hydration cap: browsing endpoints return up to 20 ids, but each
-- card costs one detail fetch, so bound it.
local HYDRATE_CAP = 10

local function urlencode(s)
  return (string.gsub(s, "[^%w%-%._~]", function(c)
    return string.format("%%%02X", string.byte(c))
  end))
end

-- get fetches url through the ETag disk cache and decodes the JSON body.
local function get(url, ttl)
  local res = jhmc.http_cache({ url = url, ttl = ttl })
  if res.status == 404 or res.status == 410 then
    error("ftb: not found: " .. url)
  end
  if res.status < 200 or res.status >= 300 then
    error("ftb: HTTP " .. res.status .. " for " .. url)
  end
  return jhmc.json_decode(res.body)
end

-- square_art returns the pack's square artwork url, falling back to the first
-- artwork of any kind.
local function square_art(p)
  local first = ""
  for _, a in ipairs(p.art or {}) do
    if type(a) == "table" and a.url and a.url ~= "" then
      if first == "" then first = a.url end
      if a.type == "square" then return a.url end
    end
  end
  return first
end

-- authors_str joins the pack's author display names.
local function authors_str(p)
  local names = {}
  for _, a in ipairs(p.authors or {}) do
    if type(a) == "table" and a.name and a.name ~= "" then
      names[#names + 1] = a.name
    end
  end
  return table.concat(names, ", ")
end

-- card builds a project card from a hydrated pack detail.
local function card(p)
  return {
    project_id = tostring(p.id),
    title = p.name,
    summary = p.synopsis,
    icon_url = square_art(p),
    author = authors_str(p),
    downloads = p.installs,
    project_type = "modpack",
  }
end

-- channel_of maps an FTB version type ("Release"/"Beta"/"Alpha"/…) onto the
-- shop's release-channel vocabulary.
local function channel_of(t)
  t = string.lower(t or "")
  if t == "beta" then return "beta" end
  if t == "alpha" then return "alpha" end
  return "release"
end

-- hydrate_ids fetches pack detail for up to cap ids from a { packs = [...] }
-- browse response and returns their cards. Individual failures are skipped so
-- one bad pack does not blank a whole section.
local function hydrate_ids(resp, cap)
  local projects = {}
  local ids = (type(resp) == "table" and resp.packs) or {}
  for i, id in ipairs(ids) do
    if i > cap then break end
    local ok, p = pcall(get, BASE .. "/modpack/" .. urlencode(tostring(id)), TTL_DETAIL)
    if ok and type(p) == "table" and p.name then
      projects[#projects + 1] = card(p)
    end
  end
  return projects
end

local function do_home(ctx)
  local featured = hydrate_ids(get(BASE .. "/modpack/featured/20", TTL_BROWSE), HYDRATE_CAP)
  local popular = hydrate_ids(get(BASE .. "/modpack/popular/installs/20", TTL_BROWSE), HYDRATE_CAP)
  return { sections = {
    { title_key = "shop.home.featured", projects = featured },
    { title_key = "shop.home.popular", projects = popular },
  } }
end

local function do_search(ctx)
  -- The FTB search endpoint returns only a single top-N page, so pagination
  -- beyond the first request ends here rather than repeating results.
  if (ctx.offset or 0) > 0 then
    return { projects = {}, total = 0 }
  end
  local limit = ctx.limit or 20
  local resp = get(BASE .. "/modpack/search/" .. limit .. "?term=" .. urlencode(ctx.query or ""), TTL_BROWSE)
  local projects = hydrate_ids(resp, limit)
  local total = (type(resp) == "table" and resp.total) or #projects
  return { projects = projects, total = total }
end

local function do_detail(ctx)
  local p = get(BASE .. "/modpack/" .. urlencode(ctx.project_id), TTL_DETAIL)
  return {
    project = card(p),
    body = p.description or "",
    body_format = "markdown",
    gallery = {},
    links = {
      website = "https://www.feed-the-beast.com/modpacks/" .. tostring(p.id),
    },
    game_versions = {},
    loaders = {},
    -- The version list omits per-version targets; the create flow resolves the
    -- concrete MC version + loader at install time.
    updated = "",
  }
end

local function do_versions(ctx)
  local p = get(BASE .. "/modpack/" .. urlencode(ctx.project_id), TTL_DETAIL)
  local list = p.versions or {}
  local out = {}
  -- Present newest-first (the API lists versions oldest-first).
  for i = #list, 1, -1 do
    local v = list[i]
    if type(v) == "table" and v.id ~= nil then
      out[#out + 1] = {
        id = tostring(v.id),
        name = v.name,
        version_number = v.name,
        channel = channel_of(v.type),
        date = "",
      }
    end
  end
  return { versions = out }
end

local function do_resolve_file(ctx)
  -- Required by the shop contract but never reached by the modpack flow: a pack
  -- is installed as a whole server by this script's provider role.
  error("not distributable: FTB modpacks install as a new server")
end

shop = {
  kinds = { "modpack" },
  needs_key = false,
  home = do_home,
  search = do_search,
  detail = do_detail,
  versions = do_versions,
  resolve_file = do_resolve_file,
}

-- Provider role: installs an FTB modpack as a self-contained server from its
-- version manifest. Driven by the shop with an opaque "<packId>/<versionId>"
-- version. CurseForge-hosted files use the configured curseforge_api_key.
local FTB_API = "https://api.feed-the-beast.com/v1/modpacks/public"

-- curseforge_key returns the configured CurseForge API key ("" when unset). It is
-- seeded from the shared CurseForge shop key by cmd/engine wiring when the user
-- leaves it blank.
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

local function provider_install(ctx)
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

-- provider_update moves an installed pack to another version of the same pack:
-- files only the old version listed are deleted, new/changed ones downloaded, and
-- the loader reinstalled only when its pinned target changed.
local function provider_update(ctx)
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

provider = {
  hidden = true,
  mod_layout = "mods",
  versions = function() return {} end,
  install = provider_install,
  update = provider_update,
}
