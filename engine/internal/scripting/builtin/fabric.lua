-- Fabric: the Fabric mod loader on top of a vanilla Minecraft server.
--
-- Fabric publishes a launchable server jar directly from its meta service, so
-- there is no local install step to run (unlike Forge/NeoForge): we resolve the
-- latest stable loader + installer and download the prebuilt server jar.

meta = {
  id = "fabric",
  name = "Fabric",
  website = "https://fabricmc.net",
  description = "The Fabric mod loader for Minecraft.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "mods", -- Fabric loads mods from a mods/ folder
  permissions = {
    { kind = "network", reason = "Download Fabric's version metadata and the server launcher jar." },
  },
}

local META_BASE = "https://meta.fabricmc.net/v2"

-- versions returns every stable game version Fabric supports, newest first
-- (the meta service already orders them that way).
function versions()
  local entries = jhmc.http_json(META_BASE .. "/versions/game")
  local out = {}
  for _, e in ipairs(entries) do
    if e.stable and e.version ~= "" then
      out[#out + 1] = e.version
    end
  end
  return out
end

-- latest_version returns the newest stable entry under /versions/<kind>,
-- falling back to the newest entry of any stability if none are stable.
local function latest_version(kind)
  local entries = jhmc.http_json(META_BASE .. "/versions/" .. kind)
  for _, e in ipairs(entries) do
    if e.stable and e.version ~= "" then
      return e.version
    end
  end
  for _, e in ipairs(entries) do
    if e.version ~= "" then
      return e.version
    end
  end
  return nil
end

-- install downloads the Fabric server launcher jar for ctx.version into
-- ctx.dir and returns the launch spec (the Java major the game version needs).
function install(ctx)
  ctx.step("install.progress.resolving_version", -1)
  local loader = latest_version("loader")
  if not loader then
    error("version not found: fabric loader")
  end
  local installer = latest_version("installer")
  if not installer then
    error("version not found: fabric installer")
  end

  local url = META_BASE
    .. "/versions/loader/" .. ctx.version
    .. "/" .. loader
    .. "/" .. installer
    .. "/server/jar"

  ctx.step("install.progress.downloading_server", 0)
  ctx.log("server.jar")
  jhmc.download(url, { dest = "server.jar" })

  ctx.step("install.progress.done", 1)
  return {
    java_major = jhmc.java_major_for(ctx.version),
    args = { "-jar", "server.jar", "nogui" },
  }
end
