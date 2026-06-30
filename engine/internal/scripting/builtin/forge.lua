-- Forge: Minecraft Forge installed by running the official installer's
-- --installServer step. Forge builds are resolved from the promotions feed.
--
-- Modern installers (1.17+) generate a libraries/.../win_args.txt arg file that
-- the server is launched with via `java @argfile nogui`; older installers leave
-- a runnable forge-*.jar instead.

meta = {
  id = "forge",
  name = "Forge",
  website = "https://files.minecraftforge.net",
  description = "The original Minecraft mod loader, installed via the official Forge installer.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "mods", -- drops Forge mods into a mods/ folder
  permissions = {
    { kind = "network", reason = "Download Forge's promotions feed and the installer." },
    { kind = "install", reason = "Resolve a JRE and run the Forge installer's --installServer." },
    { kind = "fs_server", reason = "Inspect the installed libraries to detect the launch command." },
  },
}

local PROMOTIONS = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"
local MAVEN = "https://maven.minecraftforge.net/net/minecraftforge/forge"

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

-- versions returns every Minecraft version that has a Forge build, newest-first.
-- The promotions feed is a JSON object, so its key order is undefined; we sort.
function versions()
  local promos = jhmc.http_json(PROMOTIONS).promos
  local seen = {}
  local out = {}
  for key in pairs(promos) do
    -- keys look like "1.20.1-recommended" / "1.20.1-latest"
    local mc = key:match("^(.*)%-[^%-]+$")
    if mc and not seen[mc] then
      seen[mc] = true
      out[#out + 1] = mc
    end
  end
  table.sort(out, less_desc)
  return out
end

-- resolve_build picks the recommended build for an MC version, falling back to
-- the latest.
local function resolve_build(promos, mc)
  return promos[mc .. "-recommended"] or promos[mc .. "-latest"]
end

-- find_args_file walks libraries/ for a generated win_args.txt. jhmc.fs.glob is
-- backed by Go's filepath.Glob, which has no recursive "**" wildcard, so we
-- probe a few nesting depths (the installer puts it at
-- libraries/net/minecraftforge/forge/<ver>/win_args.txt, i.e. depth 4).
local function find_args_file()
  local prefix = "libraries"
  for _ = 1, 8 do
    prefix = prefix .. "/*"
    local hits = jhmc.fs.glob(prefix .. "/win_args.txt")
    if hits[1] then
      return hits[1]
    end
  end
  return nil
end

-- detect_launch reproduces the Go detectServerLaunch: prefer a generated
-- win_args.txt arg file under libraries/, else a runnable forge-*.jar.
local function detect_launch()
  local args_file = find_args_file()
  if args_file then
    return { "@" .. args_file, "nogui" }
  end
  for _, pat in ipairs({ "forge*.jar", "server.jar" }) do
    for _, jar in ipairs(jhmc.fs.glob(pat)) do
      if not jar:lower():find("installer") then
        return { "-jar", jar, "nogui" }
      end
    end
  end
  error("no win_args.txt or server jar after install")
end

-- install downloads the Forge installer for ctx.version, runs --installServer,
-- and returns the detected launch spec.
function install(ctx)
  ctx.step("install.progress.resolving_version", -1)
  local promos = jhmc.http_json(PROMOTIONS).promos
  local build = resolve_build(promos, ctx.version)
  if not build then
    error("version not found: " .. ctx.version)
  end
  local major = jhmc.java_major_for(ctx.version)
  local full = ctx.version .. "-" .. build
  local installer_url = MAVEN .. "/" .. full .. "/forge-" .. full .. "-installer.jar"

  ctx.step("install.progress.downloading_installer", 0)
  ctx.log("forge-" .. full .. "-installer.jar")
  jhmc.download(installer_url, { dest = "installer.jar" })

  ctx.step("install.progress.running_installer", -1)
  ctx.log("java -jar installer.jar --installServer")
  jhmc.run_jar({
    java_major = major,
    args = { "-jar", "installer.jar", "--installServer" },
    dir = ".",
  })

  local args = detect_launch()
  ctx.step("install.progress.done", 1)
  return { java_major = major, args = args }
end
