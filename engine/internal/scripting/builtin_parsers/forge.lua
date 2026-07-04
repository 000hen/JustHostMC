-- Forge mod metadata parser: reads META-INF/mods.toml (modern Forge).
meta = {
  id = "parser-forge",
  name = "Forge Mod Parser",
  description = "Reads META-INF/mods.toml metadata from Forge mod jars.",
  version = "1.0.0",
  author = "JustHostMC",
  formats = { "META-INF/mods.toml" },
  permissions = {
    { kind = "fs_server", reason = "Read mod jars to extract their metadata" },
  },
}

function parse(ctx)
  local raw = jhmc.zip_read(ctx.jar, "META-INF/mods.toml")
  if raw == nil then return nil end
  local ok, t = pcall(jhmc.toml_decode, raw)
  if not ok or type(t) ~= "table" then return nil end
  if type(t.mods) ~= "table" or type(t.mods[1]) ~= "table" then return nil end
  local mod = t.mods[1]

  -- authors/logoFile/displayURL may sit at the top level or on the mod entry.
  local authors = {}
  local author_str = mod.authors or t.authors
  if type(author_str) == "string" and author_str ~= "" then
    for name in author_str:gmatch("[^,]+") do
      name = name:match("^%s*(.-)%s*$")
      if name ~= "" then authors[#authors + 1] = name end
    end
  elseif type(author_str) == "table" then
    for _, name in ipairs(author_str) do
      if type(name) == "string" and name ~= "" then authors[#authors + 1] = name end
    end
  end

  local icon
  local logo = mod.logoFile or t.logoFile
  if type(logo) == "string" and logo ~= "" then
    icon = jhmc.zip_read(ctx.jar, (logo:gsub("^/", "")))
  end

  local version = mod.version
  -- Forge commonly leaves this placeholder in mods.toml and supplies the real
  -- value in META-INF/MANIFEST.MF.
  if version == "${file.jarVersion}" then
    local manifest = jhmc.zip_read(ctx.jar, "META-INF/MANIFEST.MF")
    if type(manifest) == "string" then
      version = manifest:match("[\r\n]?Implementation%-Version:%s*([^\r\n]+)")
      if version then version = version:match("^%s*(.-)%s*$") end
    else
      version = nil
    end
  elseif type(version) == "string" and version:find("${", 1, true) then
    version = nil
  end

  return {
    loader = "forge",
    mod_id = mod.modId,
    name = mod.displayName or mod.modId,
    version = version,
    authors = authors,
    description = mod.description,
    website = mod.displayURL or t.displayURL or t.issueTrackerURL,
    icon = icon,
  }
end
