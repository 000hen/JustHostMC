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
    { kind = "network", reason = "Download Mojang's version manifest and SpigotMC's BuildTools." },
    { kind = "install", reason = "Resolve a JDK and run BuildTools to compile the server jar." },
  },
}

-- BuildTools.jar from SpigotMC's Jenkins CI (latest successful build). It
-- supports any release Minecraft version via its --rev flag.
local BUILDTOOLS = "https://hub.spigotmc.org/jenkins/job/BuildTools/lastSuccessfulBuild/artifact/target/BuildTools.jar"
-- Spigot (via BuildTools) supports all release versions, so its installable
-- version list is just Mojang's release manifest.
local MANIFEST = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

-- versions returns every release version id, newest first (Mojang's order).
function versions()
  local m = jhmc.http_json(MANIFEST)
  local out = {}
  for _, e in ipairs(m.versions) do
    if e.type == "release" then
      out[#out + 1] = e.id
    end
  end
  return out
end

-- install downloads BuildTools.jar into ctx.dir, runs it with --rev ctx.version
-- (resolving a full JDK to compile with), and returns the launch spec for the
-- spigot-<version>.jar BuildTools produces.
function install(ctx)
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
