-- NeoForge mod metadata parser: reads META-INF/neoforge.mods.toml.
meta = {
  id = "parser-neoforge",
  name = "NeoForge Mod Parser",
  description = "Reads META-INF/neoforge.mods.toml metadata from NeoForge mod jars.",
  version = "1.0.0",
  author = "JustHostMC",
  formats = { "META-INF/neoforge.mods.toml" },
  permissions = {
    { kind = "fs_server", reason = "Read mod jars to extract their metadata" },
  },
}

function parse(ctx)
  local raw = jhmc.zip_read(ctx.jar, "META-INF/neoforge.mods.toml")
  if raw == nil then return nil end
  local ok, t = pcall(jhmc.toml_decode, raw)
  if not ok or type(t) ~= "table" then return nil end
  if type(t.mods) ~= "table" or type(t.mods[1]) ~= "table" then return nil end
  local mod = t.mods[1]

  local authors = {}
  local author_str = mod.authors or t.authors
  if type(author_str) == "string" and author_str ~= "" then
    for name in author_str:gmatch("[^,]+") do
      name = name:match("^%s*(.-)%s*$")
      if name ~= "" then authors[#authors + 1] = name end
    end
  end

  local icon
  local logo = mod.logoFile or t.logoFile
  if type(logo) == "string" and logo ~= "" then
    icon = jhmc.zip_read(ctx.jar, (logo:gsub("^/", "")))
  end

  local version = mod.version
  if type(version) == "string" and version:find("${", 1, true) then version = nil end

  return {
    loader = "neoforge",
    mod_id = mod.modId,
    name = mod.displayName or mod.modId,
    version = version,
    authors = authors,
    description = mod.description,
    website = mod.displayURL or t.displayURL,
    icon = icon,
  }
end
