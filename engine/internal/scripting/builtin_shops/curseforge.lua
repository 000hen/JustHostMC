-- CurseForge mod shop. Talks to the CurseForge for Studios REST API
-- (https://docs.curseforge.com/rest-api/), which requires an x-api-key header
-- (meta.needs_key): the engine injects the key via ctx.config.api_key.
--   GET  /v1/mods/search                        gameId=432, classId 6=mods 5=bukkit plugins
--   GET  /v1/mods/{id}                          detail card
--   GET  /v1/mods/{id}/description              full description (HTML)
--   GET  /v1/mods/{id}/files                    gameVersion + modLoaderType filters
--   GET  /v1/mods/{id}/files/{fileId}           one file
--   GET  /v1/mods/{id}/files/{fileId}/download-url  fallback when downloadUrl is null
--   POST /v1/mods                               batch cards (dependency titles)
--   POST /v1/mods/featured                      featured/popular/recentlyUpdated
-- Page size caps at 50 and index+pageSize <= 10000.

meta = {
  id = "curseforge",
  name = "CurseForge",
  website = "https://www.curseforge.com",
  description = "Browse and install mods and plugins from CurseForge.",
  version = "1.0.0",
  author = "JustHostMC",
  needs_key = true,
  permissions = {
    { kind = "network", reason = "Query the CurseForge API to browse, search and download mods" },
  },
}

local API = "https://api.curseforge.com"
local GAME_MINECRAFT = 432
local CLASS_MODS = 6
local CLASS_BUKKIT_PLUGINS = 5

local TTL_BROWSE = 120
local TTL_DETAIL = 600

-- ModLoaderType enum (docs #tocS_ModLoaderType).
local LOADERS = { forge = 1, liteloader = 3, fabric = 4, quilt = 5, neoforge = 6 }
-- ModsSearchSortField enum (docs #tocS_ModsSearchSortField).
local SORTS = { relevance = 2, downloads = 6, follows = 2, newest = 11, updated = 3 }

local function urlencode(s)
  return (string.gsub(s, "[^%w%-%._~]", function(c)
    return string.format("%%%02X", string.byte(c))
  end))
end

local api_key -- set per call from ctx.config

local function check(res, url)
  if res.status == 404 then error("curseforge: not found: " .. url) end
  if res.status == 401 or res.status == 403 then
    error("curseforge: HTTP " .. res.status .. " (invalid or missing API key)")
  end
  if res.status < 200 or res.status >= 300 then
    error("curseforge: HTTP " .. res.status .. " for " .. url)
  end
  return jhmc.json_decode(res.body)
end

-- get fetches through the ETag disk cache with the API key header.
local function get(url, ttl)
  return check(jhmc.http_cache({ url = url, ttl = ttl, headers = { ["x-api-key"] = api_key } }), url)
end

-- post issues an uncached POST (used for featured + batch lookups).
local function post(url, body_tbl)
  return check(jhmc.http({
    url = url,
    method = "POST",
    body = jhmc.json_encode(body_tbl),
    headers = { ["x-api-key"] = api_key, ["Content-Type"] = "application/json" },
  }), url)
end

local function class_id(kind)
  if kind == "plugin" then return CLASS_BUKKIT_PLUGINS end
  return CLASS_MODS
end

local function project_card(m, kind)
  local cats = {}
  for _, c in ipairs(m.categories or {}) do cats[#cats + 1] = c.name end
  return {
    project_id = tostring(m.id),
    slug = m.slug,
    title = m.name,
    summary = m.summary,
    icon_url = m.logo and m.logo.thumbnailUrl or "",
    author = m.authors and m.authors[1] and m.authors[1].name or "",
    downloads = m.downloadCount,
    follows = m.thumbsUpCount,
    categories = cats,
    project_type = kind or (m.classId == CLASS_BUKKIT_PLUGINS and "plugin" or "mod"),
  }
end

-- search_url builds /v1/mods/search. modLoaderType must be paired with a
-- gameVersion, and bukkit plugins have no loader dimension at all.
local function search_url(ctx, sort_field, offset, limit)
  local url = API .. "/v1/mods/search?gameId=" .. GAME_MINECRAFT
    .. "&classId=" .. class_id(ctx.kind)
    .. "&sortField=" .. sort_field .. "&sortOrder=desc"
    .. "&index=" .. offset .. "&pageSize=" .. limit
  if (ctx.query or "") ~= "" then
    url = url .. "&searchFilter=" .. urlencode(ctx.query)
  end
  if (ctx.mc_version or "") ~= "" then
    url = url .. "&gameVersion=" .. urlencode(ctx.mc_version)
    local lt = ctx.kind ~= "plugin" and LOADERS[ctx.loader or ""] or nil
    if lt then url = url .. "&modLoaderType=" .. lt end
  end
  return url
end

function search(ctx)
  api_key = ctx.config.api_key
  local sort_field = SORTS[ctx.sort] or SORTS.relevance
  local body = get(search_url(ctx, sort_field, ctx.offset or 0, ctx.limit or 20), TTL_BROWSE)
  local projects = {}
  for _, m in ipairs(body.data or {}) do
    projects[#projects + 1] = project_card(m, ctx.kind)
  end
  local total = body.pagination and body.pagination.totalCount or 0
  return { projects = projects, total = total }
end

function home(ctx)
  api_key = ctx.config.api_key
  -- Featured is a POST (uncached); fall back to plain popular search if the
  -- endpoint misbehaves for this game/class combination.
  local ok, body = pcall(post, API .. "/v1/mods/featured",
    { gameId = GAME_MINECRAFT, excludedModIds = {} })
  local sections = {}
  if ok and body.data then
    local function section(title_key, mods)
      local projects = {}
      for _, m in ipairs(mods or {}) do
        if m.classId == class_id(ctx.kind) then
          projects[#projects + 1] = project_card(m, ctx.kind)
        end
        if #projects >= 12 then break end
      end
      if #projects > 0 then
        sections[#sections + 1] = { title_key = title_key, projects = projects }
      end
    end
    section("shop.home.featured", body.data.featured)
    section("shop.home.popular", body.data.popular)
    section("shop.home.updated", body.data.recentlyUpdated)
  end
  if #sections == 0 then
    local popular = get(search_url(ctx, SORTS.downloads, 0, 12), TTL_BROWSE)
    local projects = {}
    for _, m in ipairs(popular.data or {}) do
      projects[#projects + 1] = project_card(m, ctx.kind)
    end
    sections[#sections + 1] = { title_key = "shop.home.popular", projects = projects }
  end
  return { sections = sections }
end

function detail(ctx)
  api_key = ctx.config.api_key
  local m = get(API .. "/v1/mods/" .. urlencode(ctx.project_id), TTL_DETAIL).data
  local desc = get(API .. "/v1/mods/" .. urlencode(ctx.project_id) .. "/description", TTL_DETAIL).data
  local gallery = {}
  for _, sshot in ipairs(m.screenshots or {}) do
    gallery[#gallery + 1] = { url = sshot.url, title = sshot.title, description = sshot.description }
  end
  local gvs, seen = {}, {}
  for _, idx in ipairs(m.latestFilesIndexes or {}) do
    if idx.gameVersion and not seen[idx.gameVersion] then
      seen[idx.gameVersion] = true
      gvs[#gvs + 1] = idx.gameVersion
    end
  end
  return {
    project = project_card(m),
    body = desc,
    body_format = "html",
    gallery = gallery,
    links = {
      website = m.links and m.links.websiteUrl or "",
      source = m.links and m.links.sourceUrl or "",
      issues = m.links and m.links.issuesUrl or "",
      wiki = m.links and m.links.wikiUrl or "",
    },
    game_versions = gvs,
    license = "",
    updated = m.dateModified,
  }
end

local CHANNELS = { [1] = "release", [2] = "beta", [3] = "alpha" }

-- files_url builds /v1/mods/{id}/files with the compatibility filters.
local function files_url(ctx)
  local url = API .. "/v1/mods/" .. urlencode(ctx.project_id) .. "/files?pageSize=50"
  if (ctx.mc_version or "") ~= "" then
    url = url .. "&gameVersion=" .. urlencode(ctx.mc_version)
  end
  local lt = ctx.kind ~= "plugin" and LOADERS[ctx.loader or ""] or nil
  if lt then url = url .. "&modLoaderType=" .. lt end
  return url
end

local function sha1_of(f)
  for _, h in ipairs(f.hashes or {}) do
    if h.algo == 1 then return h.value end -- HashAlgo 1 = Sha1
  end
  return nil
end

-- dep_titles resolves mod names for dependency ids in one batch POST.
local function dep_titles(ids)
  if #ids == 0 then return {} end
  local body = post(API .. "/v1/mods", { modIds = ids })
  local titles = {}
  for _, m in ipairs(body.data or {}) do
    titles[m.id] = m.name
  end
  return titles
end

function versions(ctx)
  api_key = ctx.config.api_key
  local body = get(files_url(ctx), TTL_BROWSE)

  local dep_ids, seen = {}, {}
  for _, f in ipairs(body.data or {}) do
    for _, d in ipairs(f.dependencies or {}) do
      if d.relationType == 3 and d.modId and not seen[d.modId] then -- 3 = RequiredDependency
        seen[d.modId] = true
        dep_ids[#dep_ids + 1] = d.modId
      end
    end
  end
  local titles = {}
  if #dep_ids > 0 and #dep_ids <= 30 then
    local ok, result = pcall(dep_titles, dep_ids)
    if ok then titles = result end
  end

  local out = {}
  for _, f in ipairs(body.data or {}) do
    local deps = {}
    for _, d in ipairs(f.dependencies or {}) do
      if d.relationType == 3 and d.modId then
        deps[#deps + 1] = {
          project_id = tostring(d.modId),
          title = titles[d.modId] or "",
          required = true,
        }
      end
    end
    local gvs, loaders = {}, {}
    for _, sgv in ipairs(f.sortableGameVersions or {}) do
      -- Loader rows have no version type; game versions carry gameVersion.
      if sgv.gameVersion and sgv.gameVersion ~= "" then
        gvs[#gvs + 1] = sgv.gameVersion
      elseif sgv.gameVersionName then
        loaders[#loaders + 1] = string.lower(sgv.gameVersionName)
      end
    end
    out[#out + 1] = {
      id = tostring(f.id),
      name = f.displayName,
      version_number = f.fileName,
      channel = CHANNELS[f.releaseType] or "",
      game_versions = gvs,
      loaders = loaders,
      date = f.fileDate,
      downloads = f.downloadCount,
      filename = f.fileName,
      size = f.fileLength,
      dependencies = deps,
    }
  end
  return { versions = out }
end

function resolve_file(ctx)
  api_key = ctx.config.api_key
  local f
  if (ctx.version_id or "") ~= "" then
    f = get(API .. "/v1/mods/" .. urlencode(ctx.project_id)
      .. "/files/" .. urlencode(ctx.version_id), TTL_DETAIL).data
  else
    -- Latest compatible file (files come newest-first).
    local body = get(files_url(ctx), TTL_BROWSE)
    f = body.data and body.data[1]
    if not f then error("curseforge: no compatible file found") end
  end
  local url = f.downloadUrl
  if not url or url == "" then
    -- Some files omit downloadUrl; the dedicated endpoint may still serve it.
    local ok, body = pcall(get, API .. "/v1/mods/" .. urlencode(ctx.project_id)
      .. "/files/" .. tostring(f.id) .. "/download-url", TTL_DETAIL)
    if ok then url = body.data end
    if not url or url == "" then
      error("curseforge: file not distributable (author disabled third-party downloads)")
    end
  end
  return {
    url = url,
    filename = f.fileName,
    size = f.fileLength,
    sha1 = sha1_of(f),
  }
end
