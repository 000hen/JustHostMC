-- Modrinth modpack shop. Talks to the same keyless Modrinth v2 API as the mod
-- shop, filtered to project_type:modpack, and browses packs to install as new
-- servers. A modpack is not a single installable file — it installs as a whole
-- server through the "modrinth_modpacks" provider, so resolve_file is an error.

meta = {
  id = "modrinth_modpacks",
  name = "Modrinth",
  website = "https://modrinth.com",
  description = "Browse and install Modrinth modpacks as new servers.",
  version = "1.0.0",
  author = "JustHostMC",
  kinds = { "modpack" },
  needs_key = false,
  permissions = {
    { kind = "network", reason = "Query the Modrinth API to browse and install modpacks" },
  },
}

local API = "https://api.modrinth.com/v2"

local TTL_BROWSE = 120
local TTL_DETAIL = 600

local function urlencode(s)
  return (string.gsub(s, "[^%w%-%._~]", function(c)
    return string.format("%%%02X", string.byte(c))
  end))
end

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

-- facets filters to modpacks; packs pin their own MC version and loader, so only
-- project_type and any chosen categories are applied.
local function facets(ctx)
  local groups = { '["project_type:modpack"]' }
  if #(ctx.categories or {}) > 0 then
    local parts = {}
    for _, category in ipairs(ctx.categories) do
      parts[#parts + 1] = '"categories:' .. category .. '"'
    end
    groups[#groups + 1] = "[" .. table.concat(parts, ",") .. "]"
  end
  return "[" .. table.concat(groups, ",") .. "]"
end

local function project_card(hit)
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
    project_type = "modpack",
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
    projects[#projects + 1] = project_card(hit)
  end
  return projects, body.total_hits or 0
end

function home(ctx)
  local popular = run_search(ctx, "downloads", 0, 12)
  local recommended = run_search(ctx, "follows", 0, 12)
  local updated = run_search(ctx, "updated", 0, 12)
  return { sections = {
    { title_key = "shop.home.popular", projects = popular },
    { title_key = "shop.home.recommended", projects = recommended },
    { title_key = "shop.home.updated", projects = updated },
  } }
end

local SORTS = { relevance = "relevance", downloads = "downloads", follows = "follows",
  newest = "newest", updated = "updated" }

function search(ctx)
  local projects, total = run_search(ctx, SORTS[ctx.sort] or "relevance",
    ctx.offset or 0, ctx.limit or 20)
  return { projects = projects, total = total }
end

function detail(ctx)
  local p = get(API .. "/project/" .. urlencode(ctx.project_id), TTL_DETAIL)
  local gallery = {}
  for _, g in ipairs(p.gallery or {}) do
    gallery[#gallery + 1] = { url = g.url, title = g.title, description = g.description }
  end
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
      project_type = "modpack",
    },
    body = p.body,
    body_format = "markdown",
    gallery = gallery,
    links = {
      website = "https://modrinth.com/modpack/" .. (p.slug or p.id),
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

local function primary_file(v)
  local f = v.files and v.files[1]
  for _, cand in ipairs(v.files or {}) do
    if cand.primary then f = cand break end
  end
  return f
end

function versions(ctx)
  -- All pack versions, newest-first; a modpack version is not loader-filtered.
  local list = get(API .. "/project/" .. urlencode(ctx.project_id) .. "/version", TTL_BROWSE)
  local out = {}
  for _, v in ipairs(list) do
    local f = primary_file(v)
    out[#out + 1] = {
      id = v.id,
      name = v.name,
      version_number = v.version_number,
      channel = v.version_type,
      game_versions = v.game_versions,
      date = v.date_published,
      downloads = v.downloads,
      filename = f and f.filename or "",
      size = f and f.size or 0,
    }
  end
  return { versions = out }
end

function resolve_file(ctx)
  -- Required by the shop contract but never reached by the modpack flow: a pack
  -- installs as a whole server by the "modrinth_modpacks" provider.
  error("not distributable: Modrinth modpacks install as a new server")
end
