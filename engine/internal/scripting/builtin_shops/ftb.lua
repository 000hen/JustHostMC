-- Feed The Beast modpack shop. Talks to the FTB v1 API
-- (https://api.feed-the-beast.com/v1/modpacks/public):
--   GET /modpack/featured/{n}          -> { packs = [id...], total }
--   GET /modpack/popular/installs/{n}  -> { packs = [id...], total }
--   GET /modpack/search/{n}?term=...   -> { packs = [id...], total }
--   GET /modpack/{id}                  -> pack detail (name, synopsis, description,
--                                         art, authors, versions[], installs, ...)
-- Read access is keyless. Browsing lists return pack ids only, so cards are
-- hydrated from /modpack/{id} (capped, and ETag-cached). Modpacks are not
-- distributable as a single file: they install as a whole new server via the
-- "ftb" provider, so resolve_file is intentionally an error.

meta = {
  id = "ftb",
  name = "FTB Modpacks",
  website = "https://www.feed-the-beast.com",
  description = "Browse and install Feed The Beast modpacks as new servers.",
  version = "1.0.0",
  author = "JustHostMC",
  kinds = { "modpack" },
  needs_key = false,
  permissions = {
    { kind = "network", reason = "Query the FTB API to browse and install modpacks" },
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
-- Non-2xx responses raise, mapping 404 onto the engine's "not found" bridge.
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

function home(ctx)
  local featured = hydrate_ids(get(BASE .. "/modpack/featured/20", TTL_BROWSE), HYDRATE_CAP)
  local popular = hydrate_ids(get(BASE .. "/modpack/popular/installs/20", TTL_BROWSE), HYDRATE_CAP)
  return { sections = {
    { title_key = "shop.home.featured", projects = featured },
    { title_key = "shop.home.popular", projects = popular },
  } }
end

function search(ctx)
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

function detail(ctx)
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

function versions(ctx)
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

function resolve_file(ctx)
  -- Required by the shop contract but never reached by the modpack flow: a pack
  -- is installed as a whole server by the "ftb" provider, not downloaded as a file.
  error("not distributable: FTB modpacks install as a new server")
end
