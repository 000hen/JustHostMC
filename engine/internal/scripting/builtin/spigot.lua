-- Spigot: a Spigot server jar built locally from source via SpigotMC's
-- BuildTools. BuildTools clones the Bukkit/CraftBukkit/Spigot repositories,
-- applies patches and compiles the jar; it needs a full JDK (not just a JRE)
-- and Git on PATH. Because it compiles, install() is slow (5-15 min).

meta = {
  id = "spigot",
  name = "Spigot",
  website = "https://www.spigotmc.org",
  description = "A high-performance Bukkit server fork, built locally from source via BuildTools.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "plugins", -- drops Bukkit/Spigot plugins into a plugins/ folder
  permissions = {
    { kind = "network", reason = "Check supported Spigot versions and download SpigotMC's BuildTools." },
    { kind = "install", reason = "Resolve a JDK and run BuildTools to compile the server jar." },
  },
}

-- BuildTools.jar from SpigotMC's Jenkins CI (latest successful build). It
-- supports versions for which SpigotMC publishes a descriptor.
local BUILDTOOLS = "https://hub.spigotmc.org/jenkins/job/BuildTools/lastSuccessfulBuild/artifact/target/BuildTools.jar"
local MANIFEST = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
local VERSION_INDEX = "https://hub.spigotmc.org/versions/"

-- supported_versions parses the descriptor filenames from SpigotMC's index.
-- Numeric build aliases (for example 4440.json) are ignored; the remaining
-- ids are intersected with Mojang releases below.
local function supported_versions()
  local html = jhmc.http_get(VERSION_INDEX)
  local supported = {}
  for id in html:gmatch("href=[\"']([^\"']+)%.json[\"']") do
    if id:match("^%d+%.%d+[%w%.%-]*$") then
      supported[id] = true
    end
  end
  return supported
end

-- versions returns only releases which have a Spigot BuildTools descriptor.
-- Iterating Mojang's manifest preserves its newest-first ordering.
function versions()
  local m = jhmc.http_json(MANIFEST)
  local supported = supported_versions()
  local out = {}
  for _, e in ipairs(m.versions or {}) do
    if e.type == "release" and supported[e.id] then
      out[#out + 1] = e.id
    end
  end
  return out
end

-- install downloads BuildTools.jar into ctx.dir, runs it with --rev ctx.version
-- (resolving a full JDK to compile with), and returns the launch spec for the
-- spigot-<version>.jar BuildTools produces.
function install(ctx)
  -- Install can also be called directly (without selecting from versions()).
  -- Reject unsupported ids before BuildTools tries a descriptor URL that does
  -- not exist, and map the failure to the provider's version-not-found error.
  if not supported_versions()[ctx.version] then
    error("version not found: " .. ctx.version)
  end

  -- BuildTools both runs on, and targets, the same Java major as the server.
  local major = jhmc.java_major_for(ctx.version)

  ctx.step("install.progress.downloading_installer", 0)
  ctx.log("BuildTools.jar")
  jhmc.download(BUILDTOOLS, { dest = "BuildTools.jar" })

  ctx.step("install.progress.running_installer", -1)
  ctx.log("java -jar BuildTools.jar --rev " .. ctx.version)
  jhmc.run_jar({
    java_major = major,
    jdk = true, -- BuildTools compiles from source: a JRE is not enough
    args = { "-jar", "BuildTools.jar", "--rev", ctx.version },
  })

  -- BuildTools writes spigot-<version>.jar into the working (server) dir.
  local jar = "spigot-" .. ctx.version .. ".jar"

  ctx.step("install.progress.done", 1)
  return { java_major = major, args = { "-jar", jar, "nogui" } }
end
