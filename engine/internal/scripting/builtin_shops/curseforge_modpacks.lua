-- CurseForge modpack shop. Talks to the same CurseForge REST API as the mod
-- shop, filtered to classId 4471 (modpacks), and browses packs to install as new
-- servers. Needs an x-api-key (meta.needs_key); the engine injects it via
-- ctx.config.api_key, reusing the CurseForge shop key. A modpack is not a single
-- downloadable file — it installs as a whole server through the
-- "curseforge_modpacks" provider, so resolve_file is intentionally an error.

meta = {
  id = "curseforge_modpacks",
  name = "CurseForge",
  website = "https://www.curseforge.com",
  description = "Browse and install CurseForge modpacks as new servers.",
  version = "1.0.0",
  author = "JustHostMC",
  kinds = { "modpack" },
  needs_key = true,
  permissions = {
    { kind = "network", reason = "Query the CurseForge API to browse and install modpacks" },
  },
}

local API = "https://api.curseforge.com"
local GAME_MINECRAFT = 432
local CLASS_MODPACKS = 4471

local TTL_BROWSE = 120
local TTL_DETAIL = 600

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

local function get(url, ttl)
  return check(jhmc.http_cache({ url = url, ttl = ttl, headers = { ["x-api-key"] = api_key } }), url)
end

local function post(url, body_tbl)
  return check(jhmc.http({
    url = url,
    method = "POST",
    body = jhmc.json_encode(body_tbl),
    headers = { ["x-api-key"] = api_key, ["Content-Type"] = "application/json" },
  }), url)
end

local function project_card(m)
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
    project_type = "modpack",
  }
end

-- search_url builds /v1/mods/search for modpacks. Packs pin their own MC version
-- and loader, so no loader dimension is applied.
local function search_url(ctx, sort_field, offset, limit)
  local url = API .. "/v1/mods/search?gameId=" .. GAME_MINECRAFT
    .. "&classId=" .. CLASS_MODPACKS
    .. "&sortField=" .. sort_field .. "&sortOrder=desc"
    .. "&index=" .. offset .. "&pageSize=" .. limit
  if (ctx.query or "") ~= "" then
    url = url .. "&searchFilter=" .. urlencode(ctx.query)
  end
  return url
end

function search(ctx)
  api_key = ctx.config.api_key
  local sort_field = SORTS[ctx.sort] or SORTS.relevance
  local body = get(search_url(ctx, sort_field, ctx.offset or 0, ctx.limit or 20), TTL_BROWSE)
  local projects = {}
  for _, m in ipairs(body.data or {}) do
    projects[#projects + 1] = project_card(m)
  end
  local total = body.pagination and body.pagination.totalCount or 0
  return { projects = projects, total = total }
end

function home(ctx)
  api_key = ctx.config.api_key
  local function popular_section()
    local popular = get(search_url({}, SORTS.downloads, 0, 12), TTL_BROWSE)
    local projects = {}
    for _, m in ipairs(popular.data or {}) do
      projects[#projects + 1] = project_card(m)
    end
    return { title_key = "shop.home.popular", projects = projects }
  end
  local function updated_section()
    local recent = get(search_url({}, SORTS.updated, 0, 12), TTL_BROWSE)
    local projects = {}
    for _, m in ipairs(recent.data or {}) do
      projects[#projects + 1] = project_card(m)
    end
    return { title_key = "shop.home.updated", projects = projects }
  end
  return { sections = { popular_section(), updated_section() } }
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

local function sha1_of(f)
  for _, h in ipairs(f.hashes or {}) do
    if h.algo == 1 then return h.value end -- HashAlgo 1 = Sha1
  end
  return nil
end

function versions(ctx)
  api_key = ctx.config.api_key
  -- All pack files, newest-first; a modpack file is not loader-filtered.
  local body = get(API .. "/v1/mods/" .. urlencode(ctx.project_id) .. "/files?pageSize=50", TTL_BROWSE)
  local out = {}
  for _, f in ipairs(body.data or {}) do
    local gvs = {}
    for _, sgv in ipairs(f.sortableGameVersions or {}) do
      if sgv.gameVersion and sgv.gameVersion ~= "" then
        gvs[#gvs + 1] = sgv.gameVersion
      end
    end
    out[#out + 1] = {
      id = tostring(f.id),
      name = f.displayName,
      version_number = f.fileName,
      channel = CHANNELS[f.releaseType] or "",
      game_versions = gvs,
      date = f.fileDate,
      downloads = f.downloadCount,
      filename = f.fileName,
      size = f.fileLength,
    }
  end
  return { versions = out }
end

function resolve_file(ctx)
  -- Required by the shop contract but never reached by the modpack flow: a pack
  -- installs as a whole server by the "curseforge_modpacks" provider.
  error("not distributable: CurseForge modpacks install as a new server")
end
