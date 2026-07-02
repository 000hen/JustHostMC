-- NeoForge: installed via its installer's --installServer step. NeoForge version
-- strings encode the Minecraft version:
--   * legacy 3-part "A.B.<build>" -> MC "1.A.B" (B==0 -> "1.A", e.g. 21.0.x -> 1.21)
--   * current "A.B.C.<build>[-qualifier]" -> MC "A.B.C"
--     (C==0 -> "A.B", e.g. 26.2.0.x -> 26.2)
-- The current 4-part scheme is the MC-2026 versioning; preserve it exactly.

meta = {
  id = "neoforge",
  name = "NeoForge",
  website = "https://neoforged.net",
  description = "A community-driven Forge fork, installed via the official NeoForge installer.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "mods", -- drops NeoForge mods into a mods/ folder
  permissions = {
    { kind = "network", reason = "Download NeoForge's maven metadata and the installer." },
    { kind = "install", reason = "Resolve a JRE and run the NeoForge installer's --installServer." },
    { kind = "fs_server", reason = "Inspect the installed libraries to detect the launch command." },
  },
}

local NEOFORGE_MAVEN = "https://maven.neoforged.net/releases/net/neoforged/neoforge"
-- NeoForge's 1.20.1 releases predate the neoforge artifact and remain published
-- under net.neoforged:forge. Both feeds are needed for the complete version list.
local LEGACY_MAVEN = "https://maven.neoforged.net/releases/net/neoforged/forge"

-- fetch_versions reads every NeoForge version from maven-metadata.xml. There is
-- no XML host API, so we extract <version>...</version> entries with a pattern;
-- maven lists them oldest-first, which we preserve.
local function fetch_versions(maven)
  local xml = jhmc.http_get(maven .. "/maven-metadata.xml")
  local out = {}
  for v in xml:gmatch("<version>(.-)</version>") do
    out[#out + 1] = v
  end
  return out
end

-- num_parts returns the dot-separated parts of v.
local function num_parts(v)
  local parts = {}
  for p in v:gmatch("[^.]+") do
    parts[#parts + 1] = p
  end
  return parts
end

-- mc_for maps a NeoForge version to its Minecraft version (mirrors mcForNeoForge).
local function mc_for(v)
  local parts = num_parts(v)
  if #parts == 3 then -- legacy A.B.<build> -> MC 1.A.B
    local a, b = tonumber(parts[1]), tonumber(parts[2])
    if not a or not b then return nil end
    if b == 0 then return "1." .. a end
    return "1." .. a .. "." .. b
  elseif #parts >= 4 then -- current A.B.C.<build>[-qualifier] -> MC A.B.C
    local a, b, c = tonumber(parts[1]), tonumber(parts[2]), tonumber(parts[3])
    local build = parts[4]:match("^%d+")
    if not a or not b or not c or not build then return nil end
    if c == 0 then return a .. "." .. b end
    return a .. "." .. b .. "." .. c
  end
  return nil
end

-- legacy_mc_for extracts the Minecraft version from a legacy NeoForge
-- coordinate such as "1.20.1-47.1.106".
local function legacy_mc_for(v)
  return v:match("^(%d+%.%d+%.%d+)%-")
end

-- parse_mc splits an MC version into major/minor/patch (mirrors parseMC; ignores
-- non-numeric suffixes on a part).
local function parse_mc(mc)
  local parts = num_parts(mc)
  if #parts < 2 then return nil end
  local major = tonumber(parts[1])
  local minor = tonumber(parts[2]:match("^%d+") or "")
  if not major or not minor then return nil end
  local patch = tonumber((parts[3] or ""):match("^%d+") or "") or 0
  return major, minor, patch
end

-- prefix_for maps an MC version to the NeoForge version prefix to match
-- (mirrors neoForgePrefix). Legacy MC 1.x -> "minor.patch."; current MC scheme
-- (no leading "1.") -> "major.minor.patch.".
local function prefix_for(mc)
  local major, minor, patch = parse_mc(mc)
  if not major then return nil end
  if major == 1 then
    return minor .. "." .. patch .. "."
  end
  return major .. "." .. minor .. "." .. patch .. "."
end

-- neo_patch returns the build component of a NeoForge version. It is the third
-- segment for legacy coordinates and the fourth for current coordinates; a
-- qualifier may itself contain dots, so the final segment is not reliable.
local function neo_patch(v)
  local parts = num_parts(v)
  local build_index = #parts >= 4 and 4 or 3
  return tonumber(parts[build_index]:match("^%d+") or "") or 0
end

-- build_greater compares dot-separated numeric build identifiers. Legacy
-- coordinates currently use 47.1.x, but comparing every component avoids
-- assuming that the major/minor build line will never change.
local function build_greater(a, b)
  local ap, bp = num_parts(a), num_parts(b)
  local count = math.max(#ap, #bp)
  for i = 1, count do
    local av = tonumber((ap[i] or ""):match("^%d+") or "") or -1
    local bv = tonumber((bp[i] or ""):match("^%d+") or "") or -1
    if av ~= bv then return av > bv end
  end
  return a > b
end

-- less_desc orders MC versions newest-first by numeric (major, minor, patch),
-- with the raw string as a stable tiebreak (mirrors provider.sortMCDesc).
local function less_desc(a, b)
  local amaj, amin, apat = parse_mc(a)
  local bmaj, bmin, bpat = parse_mc(b)
  amaj, amin, apat = amaj or 0, amin or 0, apat or 0
  bmaj, bmin, bpat = bmaj or 0, bmin or 0, bpat or 0
  if amaj ~= bmaj then return amaj > bmaj end
  if amin ~= bmin then return amin > bmin end
  if apat ~= bpat then return apat > bpat end
  return a > b
end

-- versions returns the Minecraft versions NeoForge supports, newest-first (maven
-- lists oldest-first, so we sort to match the Go provider's order).
function versions()
  local seen = {}
  local out = {}
  for _, v in ipairs(fetch_versions(NEOFORGE_MAVEN)) do
    local mc = mc_for(v)
    if mc and not seen[mc] then
      seen[mc] = true
      out[#out + 1] = mc
    end
  end
  for _, v in ipairs(fetch_versions(LEGACY_MAVEN)) do
    local mc = legacy_mc_for(v)
    if mc and not seen[mc] then
      seen[mc] = true
      out[#out + 1] = mc
    end
  end
  table.sort(out, less_desc)
  return out
end

-- resolve_modern_version picks the highest build from the current artifact.
local function resolve_modern_version(mc)
  local prefix = prefix_for(mc)
  if not prefix then return nil end
  local best, best_patch = nil, -1
  for _, v in ipairs(fetch_versions(NEOFORGE_MAVEN)) do
    if v:sub(1, #prefix) == prefix then
      local p = neo_patch(v)
      if p > best_patch then
        best, best_patch = v, p
      end
    end
  end
  return best
end

-- resolve_legacy_version picks the highest net.neoforged:forge build for an MC
-- version. The feed contains one malformed coordinate without an MC prefix;
-- legacy_mc_for deliberately ignores it.
local function resolve_legacy_version(mc)
  local best, best_build = nil, nil
  for _, v in ipairs(fetch_versions(LEGACY_MAVEN)) do
    local candidate_mc, build = v:match("^(%d+%.%d+%.%d+)%-(.+)$")
    if candidate_mc == mc and (not best_build or build_greater(build, best_build)) then
      best, best_build = v, build
    end
  end
  return best
end

-- resolve_version returns the Maven base, artifact id, and highest matching
-- version. Modern coordinates take precedence if the feeds ever overlap.
local function resolve_version(mc)
  local modern = resolve_modern_version(mc)
  if modern then return NEOFORGE_MAVEN, "neoforge", modern end
  local legacy = resolve_legacy_version(mc)
  if legacy then return LEGACY_MAVEN, "forge", legacy end
  return nil, nil, nil
end

-- find_args_file walks libraries/ for a generated win_args.txt. jhmc.fs.glob is
-- backed by Go's filepath.Glob (no recursive "**"), so we probe a few nesting
-- depths (the installer puts it at
-- libraries/net/neoforged/neoforge/<ver>/win_args.txt, i.e. depth 4).
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
-- win_args.txt arg file under libraries/, else a runnable neoforge-*.jar.
local function detect_launch()
  local args_file = find_args_file()
  if args_file then
    return { "@" .. args_file, "nogui" }
  end
  for _, pat in ipairs({ "neoforge*.jar", "forge*.jar", "server.jar" }) do
    for _, jar in ipairs(jhmc.fs.glob(pat)) do
      if not jar:lower():find("installer") then
        return { "-jar", jar, "nogui" }
      end
    end
  end
  error("no win_args.txt or server jar after install")
end

-- install resolves the NeoForge build for ctx.version, downloads its installer,
-- runs --installServer, and returns the detected launch spec.
function install(ctx)
  ctx.step("install.progress.resolving_version", -1)
  local maven, artifact, nf = resolve_version(ctx.version)
  if not nf then
    error("version not found: " .. ctx.version)
  end
  local major = jhmc.java_major_for(ctx.version)
  local installer_url = maven .. "/" .. nf .. "/" .. artifact .. "-" .. nf .. "-installer.jar"

  ctx.step("install.progress.downloading_installer", 0)
  ctx.log(artifact .. "-" .. nf .. "-installer.jar")
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
