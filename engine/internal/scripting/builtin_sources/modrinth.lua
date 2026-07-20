-- Modrinth source: one script, two roles. The `shop` role browses/searches the
-- keyless Modrinth v2 API (https://docs.modrinth.com/api/) for mods, plugins and
-- modpacks; the hidden `provider` role installs a Modrinth modpack (.mrpack) as a
-- self-contained server. Both roles share one meta.id ("modrinth"). The old split
-- id "modrinth_modpacks" resolves here via meta.aliases so existing modpack
-- servers keep updating. Read access is keyless; the host sets the identifying
-- User-Agent Modrinth requires.
--   GET /search                 query, facets (JSON array-of-arrays), index, offset, limit<=100
--   GET /project/{id}           detail: body (Markdown), gallery, links, stats
--   GET /project/{id}/version   loaders=[...] game_versions=[...] filters
--   GET /version/{id}           one version
--   GET /projects?ids=[...]     batch cards (dependency titles)

meta = {
  id = "modrinth",
  name = "Modrinth",
  website = "https://modrinth.com",
  description = "Browse and install mods, plugins and modpacks from Modrinth.",
  version = "1.0.0",
  author = "JustHostMC",
  aliases = { "modrinth_modpacks" },
  permissions = {
    { kind = "network", reason = "Query the Modrinth API to browse, search and download mods and modpacks" },
    { kind = "install", reason = "Resolve a JRE and run the Forge/NeoForge installer's --installServer for a modpack." },
    { kind = "fs_server", reason = "Write an installed modpack's mods/configs and detect the launch command." },
  },
}

local API = "https://api.modrinth.com/v2"

-- Cache TTLs (seconds): browse pages refresh often; detail pages can lean on
-- ETag revalidation for longer.
local TTL_BROWSE = 120
local TTL_DETAIL = 600

local function urlencode(s)
  return (string.gsub(s, "[^%w%-%._~]", function(c)
    return string.format("%%%02X", string.byte(c))
  end))
end

-- get fetches url through the ETag disk cache and decodes the JSON body.
-- Non-2xx responses raise, mapping 404 onto the engine's "not found" bridge.
local function get(url, ttl)
  local res = jhmc.http_cache({ url = url, ttl = ttl })
  if res.status == 404 or res.status == 410 then
    error("modrinth: not found: " .. url)
  end
  if res.status < 200 or res.status >= 300 then
    error("modrinth: HTTP " .. res.status .. " for " .. url)
  end
  return jhmc.json_decode(res.body)
end

-- loader_names returns the Modrinth loader names for one of our loader ids.
-- Paper servers also run Spigot/Bukkit plugins, Spigot runs Bukkit plugins.
local function loader_names(loader, kind)
  if loader == "" then return {} end
  if kind == "plugin" then
    if loader == "paper" then return { "paper", "spigot", "bukkit" } end
    if loader == "spigot" then return { "spigot", "bukkit" } end
    return { loader }
  end
  return { loader }
end

-- facets builds the JSON facet expression: inner array = OR, outer = AND. A
-- modpack browse pins only project_type and any chosen categories (packs carry
-- their own MC version and loader), while mods/plugins add loader + version.
local function facets(ctx)
  local groups
  if ctx.kind == "modpack" then
    groups = { '["project_type:modpack"]' }
  else
    groups = { '["project_type:mod"]' }
    local loaders = loader_names(ctx.loader or "", ctx.kind or "mod")
    if #loaders > 0 then
      local parts = {}
      for _, l in ipairs(loaders) do parts[#parts + 1] = '"categories:' .. l .. '"' end
      groups[#groups + 1] = "[" .. table.concat(parts, ",") .. "]"
    end
    if (ctx.mc_version or "") ~= "" then
      groups[#groups + 1] = '["versions:' .. ctx.mc_version .. '"]'
    end
  end
  if #(ctx.categories or {}) > 0 then
    local parts = {}
    for _, category in ipairs(ctx.categories) do
      parts[#parts + 1] = '"categories:' .. category .. '"'
    end
    groups[#groups + 1] = "[" .. table.concat(parts, ",") .. "]"
  end
  return "[" .. table.concat(groups, ",") .. "]"
end

local function project_card(hit, kind)
  return {
    project_id = hit.project_id or hit.id,
    slug = hit.slug,
    title = hit.title,
    summary = hit.description,
    icon_url = hit.icon_url,
    author = hit.author,
    downloads = hit.downloads,
    follows = hit.follows or hit.followers,
    categories = hit.display_categories or hit.categories,
    project_type = kind == "modpack" and "modpack" or hit.project_type,
  }
end

local function run_search(ctx, index, offset, limit)
  local url = API .. "/search?query=" .. urlencode(ctx.query or "")
    .. "&facets=" .. urlencode(facets(ctx))
    .. "&index=" .. index
    .. "&offset=" .. offset
    .. "&limit=" .. limit
  local body = get(url, TTL_BROWSE)
  local projects = {}
  for _, hit in ipairs(body.hits or {}) do
    projects[#projects + 1] = project_card(hit, ctx.kind)
  end
  return projects, body.total_hits or 0
end

local function do_home(ctx)
  local popular = run_search(ctx, "downloads", 0, 12)
  local recommended = run_search(ctx, "follows", 0, 12)
  local updated = run_search(ctx, "updated", 0, 12)
  return { sections = {
    { title_key = "shop.home.popular", projects = popular },
    { title_key = "shop.home.recommended", projects = recommended },
    { title_key = "shop.home.updated", projects = updated },
  } }
end

-- sort ids map 1:1 onto Modrinth search indexes.
local SORTS = { relevance = "relevance", downloads = "downloads", follows = "follows",
  newest = "newest", updated = "updated" }

local function do_search(ctx)
  local projects, total = run_search(ctx, SORTS[ctx.sort] or "relevance",
    ctx.offset or 0, ctx.limit or 20)
  return { projects = projects, total = total }
end

local function do_detail(ctx)
  local p = get(API .. "/project/" .. urlencode(ctx.project_id), TTL_DETAIL)
  local gallery = {}
  for _, g in ipairs(p.gallery or {}) do
    gallery[#gallery + 1] = { url = g.url, title = g.title, description = g.description }
  end
  local kindpath = ctx.kind == "modpack" and "modpack" or "mod"
  return {
    project = {
      project_id = p.id,
      slug = p.slug,
      title = p.title,
      summary = p.description,
      icon_url = p.icon_url,
      downloads = p.downloads,
      follows = p.followers,
      categories = p.categories,
      project_type = ctx.kind == "modpack" and "modpack" or p.project_type,
    },
    body = p.body,
    body_format = "markdown",
    gallery = gallery,
    links = {
      website = "https://modrinth.com/" .. kindpath .. "/" .. (p.slug or p.id),
      source = p.source_url,
      issues = p.issues_url,
      wiki = p.wiki_url,
      discord = p.discord_url,
    },
    game_versions = p.game_versions,
    loaders = p.loaders,
    license = p.license and p.license.id or "",
    updated = p.updated,
  }
end

-- versions_url builds /project/{id}/version with loader/game_version filters. A
-- modpack ctx carries no loader/mc_version, so it degrades to all pack versions.
local function versions_url(ctx)
  local url = API .. "/project/" .. urlencode(ctx.project_id) .. "/version"
  local sep = "?"
  local loaders = loader_names(ctx.loader or "", ctx.kind or "mod")
  if #loaders > 0 then
    local parts = {}
    for _, l in ipairs(loaders) do parts[#parts + 1] = '"' .. l .. '"' end
    url = url .. sep .. "loaders=" .. urlencode("[" .. table.concat(parts, ",") .. "]")
    sep = "&"
  end
  if (ctx.mc_version or "") ~= "" then
    url = url .. sep .. "game_versions=" .. urlencode('["' .. ctx.mc_version .. '"]')
  end
  return url
end

-- primary_file picks the artifact to install: the primary flag when set,
-- otherwise the first file.
local function primary_file(v)
  local f = v.files and v.files[1]
  for _, cand in ipairs(v.files or {}) do
    if cand.primary then f = cand break end
  end
  return f
end

-- dep_titles resolves display names for dependency project ids in one batch
-- call (GET /projects?ids=[...]).
local function dep_titles(ids)
  if #ids == 0 then return {} end
  local parts = {}
  for _, id in ipairs(ids) do parts[#parts + 1] = '"' .. id .. '"' end
  local url = API .. "/projects?ids=" .. urlencode("[" .. table.concat(parts, ",") .. "]")
  local titles = {}
  for _, p in ipairs(get(url, TTL_DETAIL)) do
    titles[p.id] = p.title
  end
  return titles
end

local function do_versions(ctx)
  local list = get(versions_url(ctx), TTL_BROWSE)

  -- Collect unique required dependency ids for one batch title lookup.
  local dep_ids, seen = {}, {}
  for _, v in ipairs(list) do
    for _, d in ipairs(v.dependencies or {}) do
      if d.project_id and not seen[d.project_id] then
        seen[d.project_id] = true
        dep_ids[#dep_ids + 1] = d.project_id
      end
    end
  end
  local titles = {}
  if #dep_ids > 0 and #dep_ids <= 30 then
    local ok, result = pcall(dep_titles, dep_ids)
    if ok then titles = result end
  end

  local out = {}
  for _, v in ipairs(list) do
    local f = primary_file(v)
    if f then
      local deps = {}
      for _, d in ipairs(v.dependencies or {}) do
        if d.project_id then
          deps[#deps + 1] = {
            project_id = d.project_id,
            title = titles[d.project_id] or "",
            required = d.dependency_type == "required",
          }
        end
      end
      out[#out + 1] = {
        id = v.id,
        name = v.name,
        version_number = v.version_number,
        channel = v.version_type,
        game_versions = v.game_versions,
        loaders = v.loaders,
        date = v.date_published,
        downloads = v.downloads,
        filename = f.filename,
        size = f.size,
        dependencies = deps,
      }
    end
  end
  return { versions = out }
end

local function do_resolve_file(ctx)
  local v
  if (ctx.version_id or "") ~= "" then
    v = get(API .. "/version/" .. urlencode(ctx.version_id), TTL_DETAIL)
  else
    -- Latest version compatible with the server (list is newest-first).
    local list = get(versions_url(ctx), TTL_BROWSE)
    v = list[1]
    if not v then error("modrinth: no compatible version found") end
  end
  local f = primary_file(v)
  if not f then error("modrinth: version has no files") end
  return {
    url = f.url,
    filename = f.filename,
    size = f.size,
    sha1 = f.hashes and f.hashes.sha1,
    sha512 = f.hashes and f.hashes.sha512,
  }
end

shop = {
  kinds = { "mod", "plugin", "modpack" },
  needs_key = false,
  home = do_home,
  search = do_search,
  detail = do_detail,
  versions = do_versions,
  resolve_file = do_resolve_file,
}

-- Provider role: installs a Modrinth modpack (.mrpack) as a self-contained
-- server. Driven by the shop with an opaque "<projectId>/<versionId>" version.
-- Modrinth read access is keyless.
local function provider_parse_version(v)
  local project, version_id = tostring(v):match("^([^/]+)/([^/]+)$")
  if not project or not version_id then
    error("invalid modpack version id: " .. tostring(v))
  end
  return project, version_id
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

local function provider_install(ctx)
  local _, version_id = provider_parse_version(ctx.version)

  ctx.step("install.progress.resolving_version", -1)
  fetch_pack(ctx, version_id)

  local spec = mplib.install_mrpack(ctx, ".jhmc/pack.mrpack")
  jhmc.fs.remove(".jhmc/pack.mrpack")

  ctx.step("install.progress.done", 1)
  spec.pack_version = tostring(ctx.version)
  return spec
end

local function provider_update(ctx)
  local project, version_id = provider_parse_version(ctx.version)
  local oproject = provider_parse_version(ctx.old_version)
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

provider = {
  hidden = true,
  mod_layout = "mods",
  versions = function() return {} end,
  install = provider_install,
  update = provider_update,
}
