-- Vanilla: official Mojang Minecraft server jars.
--
-- This is the reference provider script. A provider declares a `meta` table and
-- two functions: versions() -> {string,...} and install(ctx) -> launch spec.
-- It may use only the permission-gated jhmc.* host API.

meta = {
  id = "vanilla",
  name = "Vanilla",
  website = "https://www.minecraft.net",
  description = "The official Minecraft server from Mojang.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "none", -- no plugins/mods folder
  permissions = {
    { kind = "network", reason = "Download Mojang's version manifest and the server jar." },
  },
}

local MANIFEST = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
local DEFAULT_JAVA = 8 -- pre-1.17 versions omit javaVersion and run on Java 8

-- versions returns every installable version id, newest first (Mojang's order).
function versions()
  local m = jhmc.http_json(MANIFEST)
  local out = {}
  for _, e in ipairs(m.versions or {}) do
    out[#out + 1] = e.id
  end
  return out
end

local function find_version(list, id)
  for _, e in ipairs(list) do
    if e.id == id then return e end
  end
  return nil
end

-- install downloads server.jar for ctx.version into ctx.dir and returns the
-- launch spec (the Java major Mojang declares for that version).
function install(ctx)
  ctx.step("install.progress.resolving_version", -1)
  local m = jhmc.http_json(MANIFEST)
  local entry = find_version(m.versions or {}, ctx.version)
  if not entry then
    error("version not found: " .. ctx.version)
  end

  local detail = jhmc.http_json(entry.url)
  local server = detail.downloads.server

  ctx.step("install.progress.downloading_server", 0)
  ctx.log("server.jar")
  jhmc.download(server.url, { dest = "server.jar", sha1 = server.sha1 })

  local major = DEFAULT_JAVA
  if detail.javaVersion and detail.javaVersion.majorVersion then
    major = detail.javaVersion.majorVersion
  end

  ctx.step("install.progress.done", 1)
  return { java_major = major, args = { "-jar", "server.jar", "nogui" } }
end
