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
  if not ok or type(t) ~= "table" then
    error("invalid META-INF/neoforge.mods.toml: " .. tostring(t))
  end
  if type(t.mods) ~= "table" or type(t.mods[1]) ~= "table" then
    error("invalid META-INF/neoforge.mods.toml: missing [[mods]] entry")
  end
  local mod = t.mods[1]

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

  local game_version
  if type(t.dependencies) == "table" then
    local dependencies = t.dependencies[mod.modId]
    if type(dependencies) == "table" then
      for _, dependency in ipairs(dependencies) do
        if type(dependency) == "table" and dependency.modId == "minecraft" then
          game_version = dependency.versionRange
          break
        end
      end
    end
  end

  return {
    loader = "neoforge",
    game_version = game_version,
    mod_id = mod.modId,
    name = mod.displayName or mod.modId,
    version = version,
    authors = authors,
    description = mod.description,
    website = mod.displayURL or t.displayURL or t.issueTrackerURL,
    icon = icon,
  }
end
