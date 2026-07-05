-- Modrinth mod shop. Talks to the Modrinth v2 API (https://docs.modrinth.com/api/):
--   GET /search                      query, facets (JSON array-of-arrays), index, offset, limit<=100
--   GET /project/{id}                detail: body (Markdown), gallery, links, stats
--   GET /project/{id}/version        loaders=[...] game_versions=[...] filters
--   GET /version/{id}                one version
--   GET /projects?ids=[...]          batch cards (dependency titles)
-- Read access is keyless; the host sets the identifying User-Agent Modrinth
-- requires. Rate limit is 300 requests/minute.

meta = {
  id = "modrinth",
  name = "Modrinth",
  website = "https://modrinth.com",
  description = "Browse and install mods and plugins from Modrinth.",
  version = "1.0.0",
  author = "JustHostMC",
  permissions = {
    { kind = "network", reason = "Query the Modrinth API to browse, search and download mods" },
  },
}

local API = "https://api.modrinth.com/v2"

-- Cache TTLs (seconds): browse pages refresh often, detail pages can lean on
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

-- loader_facets returns the Modrinth loader names for one of our loader ids.
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

-- facets builds the JSON facet expression: inner array = OR, outer = AND.
local function facets(ctx)
  local groups = { '["project_type:mod"]' }
  local loaders = loader_names(ctx.loader or "", ctx.kind or "mod")
  if #loaders > 0 then
    local parts = {}
    for _, l in ipairs(loaders) do parts[#parts + 1] = '"categories:' .. l .. '"' end
    groups[#groups + 1] = "[" .. table.concat(parts, ",") .. "]"
  end
  if (ctx.mc_version or "") ~= "" then
    groups[#groups + 1] = '["versions:' .. ctx.mc_version .. '"]'
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
    project_type = hit.project_type,
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

-- sort ids map 1:1 onto Modrinth search indexes.
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
      project_type = p.project_type,
    },
    body = p.body,
    body_format = "markdown",
    gallery = gallery,
    links = {
      website = "https://modrinth.com/mod/" .. (p.slug or p.id),
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

-- versions_url builds /project/{id}/version with loader/game_version filters.
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

function versions(ctx)
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

function resolve_file(ctx)
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
