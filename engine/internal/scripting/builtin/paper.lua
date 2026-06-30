-- Paper: high-performance Spigot fork with the plugin ecosystem.
--
-- Uses PaperMC's Fill v3 API (https://fill.papermc.io/v3). Paper's older v2 API
-- is frozen, so the modern 26.x versions are only served by v3's `builds/latest`
-- endpoint, whose `server:default` download carries an sha256 checksum.

meta = {
  id = "paper",
  name = "Paper",
  website = "https://papermc.io",
  description = "High-performance Minecraft server with the Bukkit/Spigot plugin API.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "plugins", -- drops jars into a plugins/ folder
  permissions = {
    { kind = "network", reason = "Download PaperMC's build manifest and the server jar." },
  },
}

local API = "https://fill.papermc.io/v3"

-- parse_mc splits a version into a (major, minor, patch) sort key. Non-numeric
-- or short ids sort as zeros (matching the Go provider's mcVersionKey).
local function parse_mc(v)
  local major, minor, patch = string.match(v, "^(%d+)%.(%d+)%.?(%d*)")
  if not major then
    return 0, 0, 0
  end
  return tonumber(major) or 0, tonumber(minor) or 0, tonumber(patch) or 0
end

-- less_desc orders versions newest-first by numeric (major, minor, patch), with
-- the raw string as a stable tiebreak (mirrors provider.sortMCDesc).
local function less_desc(a, b)
  local amaj, amin, apat = parse_mc(a)
  local bmaj, bmin, bpat = parse_mc(b)
  if amaj ~= bmaj then return amaj > bmaj end
  if amin ~= bmin then return amin > bmin end
  if apat ~= bpat then return apat > bpat end
  return a > b
end

-- versions flattens every per-series build list from /projects/paper and sorts
-- them newest-first.
function versions()
  local m = jhmc.http_json(API .. "/projects/paper")
  local out = {}
  for _, list in pairs(m.versions or {}) do
    for _, id in ipairs(list) do
      out[#out + 1] = id
    end
  end
  table.sort(out, less_desc)
  return out
end

-- install resolves the latest build for ctx.version, downloads its server jar
-- (verifying the sha256 the API publishes) and returns the launch spec.
function install(ctx)
  ctx.step("install.progress.resolving_version", -1)
  local url = API .. "/projects/paper/versions/" .. ctx.version .. "/builds/latest"
  local build = jhmc.http_json(url)

  local dl = build.downloads and build.downloads["server:default"]
  if not (dl and dl.url) then
    error("version not found: " .. ctx.version)
  end

  local sha
  if dl.checksums then
    sha = dl.checksums.sha256
  end

  ctx.step("install.progress.downloading_server", 0)
  ctx.log(dl.name or "server.jar")
  jhmc.download(dl.url, { dest = "server.jar", sha256 = sha })

  ctx.step("install.progress.done", 1)
  return { java_major = jhmc.java_major_for(ctx.version), args = { "-jar", "server.jar", "nogui" } }
end
