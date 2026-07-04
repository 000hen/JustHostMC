-- Legacy Forge (1.12 and earlier) mod metadata parser: reads mcmod.info.
meta = {
  id = "parser-forge-legacy",
  name = "Legacy Forge Mod Parser",
  description = "Reads mcmod.info metadata from legacy Forge mod jars.",
  version = "1.0.0",
  author = "JustHostMC",
  formats = { "mcmod.info" },
  permissions = {
    { kind = "fs_server", reason = "Read mod jars to extract their metadata" },
  },
}

function parse(ctx)
  local raw = jhmc.zip_read(ctx.jar, "mcmod.info")
  if raw == nil then return nil end
  local ok, t = pcall(jhmc.json_decode, raw)
  if not ok or type(t) ~= "table" then return nil end

  -- mcmod.info is either a plain array of mods or {"modList": [...]}.
  local mod
  if type(t[1]) == "table" then
    mod = t[1]
  elseif type(t.modList) == "table" and type(t.modList[1]) == "table" then
    mod = t.modList[1]
  else
    return nil
  end

  local authors = {}
  if type(mod.authorList) == "table" then
    for _, a in ipairs(mod.authorList) do
      if type(a) == "string" then authors[#authors + 1] = a end
    end
  end

  local icon
  if type(mod.logoFile) == "string" and mod.logoFile ~= "" then
    icon = jhmc.zip_read(ctx.jar, (mod.logoFile:gsub("^/", "")))
  end

  return {
    loader = "forge-legacy",
    mod_id = mod.modid,
    name = mod.name or mod.modid,
    version = mod.version,
    authors = authors,
    description = mod.description,
    website = mod.url,
    icon = icon,
  }
end
