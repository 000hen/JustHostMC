-- LiteLoader mod metadata parser: reads litemod.json from .litemod or .jar files.
meta = {
  id = "parser-liteloader",
  name = "LiteLoader Mod Parser",
  description = "Reads litemod.json metadata from LiteLoader mods.",
  version = "1.0.0",
  author = "JustHostMC",
  formats = { "litemod.json" },
  permissions = {
    { kind = "fs_server", reason = "Read mod jars to extract their metadata" },
  },
}

function parse(ctx)
  local raw = jhmc.zip_read(ctx.jar, "litemod.json")
  if raw == nil then return nil end
  raw = raw:gsub("^\239\187\191", "")
  local ok, m = pcall(jhmc.json_decode, raw)
  if not ok or type(m) ~= "table" or type(m.name) ~= "string" then
    error("invalid litemod.json: " .. tostring(m))
  end

  local authors = {}
  if type(m.author) == "string" and m.author ~= "" then authors[1] = m.author end

  local version = m.version
  if version == nil or version == "" then
    local revision = tonumber(m.revision) or 0
    if type(m.mcversion) == "string" and m.mcversion ~= "" then
      version = m.mcversion .. ":" .. tostring(revision)
    end
  end

  return {
    loader = "liteloader",
    game_version = m.mcversion,
    mod_id = m.name,
    name = m.name,
    version = version,
    authors = authors,
    description = m.description,
    website = m.url,
  }
end
