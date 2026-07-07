-- Legacy Forge (1.12 and earlier) mod metadata parser: reads mcmod.info.
meta = {
  id = "parser-forge-legacy",
  name = "Legacy Forge Mod Parser",
  description = "Reads mcmod.info and legacy alternative metadata from Forge mod jars.",
  version = "1.0.0",
  author = "JustHostMC",
  formats = { "mcmod.info", "cccmod.info", "neimod.info" },
  permissions = {
    { kind = "fs_server", reason = "Read mod jars to extract their metadata" },
  },
}

function parse(ctx)
  local repair_legacy_json = false
  local raw = jhmc.zip_read(ctx.jar, "mcmod.info")
  if raw == nil then
    raw = jhmc.zip_read(ctx.jar, "cccmod.info")
    repair_legacy_json = raw ~= nil
  end
  if raw == nil then
    raw = jhmc.zip_read(ctx.jar, "neimod.info")
    repair_legacy_json = raw ~= nil
  end
  if raw == nil then return nil end
  raw = raw:gsub("^\239\187\191", "")
  -- CodeChickenCore/NEI metadata in the wild may contain literal line breaks
  -- inside JSON strings. Match Forge's historical repair behavior.
  if repair_legacy_json then
    raw = raw:gsub("\r\n", "\n"):gsub("\n\n", "\\n"):gsub("\n", "")
  end
  local ok, t = pcall(jhmc.json_decode, raw)
  if not ok or type(t) ~= "table" then
    error("invalid legacy mod metadata: " .. tostring(t))
  end

  -- mcmod.info is either a plain array of mods or {"modList": [...]}.
  local mod
  if type(t[1]) == "table" then
    mod = t[1]
  elseif type(t.modList) == "table" and type(t.modList[1]) == "table" then
    mod = t.modList[1]
  elseif type(t.modid) == "string" then
    mod = t
  else
    error("invalid legacy mod metadata: missing mod entry")
  end

  local authors = {}
  if type(mod.authorList) == "table" then
    for _, a in ipairs(mod.authorList) do
      if type(a) == "string" then authors[#authors + 1] = a end
    end
  elseif type(mod.authors) == "string" and mod.authors ~= "" then
    for name in mod.authors:gmatch("[^,]+") do
      name = name:match("^%s*(.-)%s*$")
      if name ~= "" then authors[#authors + 1] = name end
    end
  end

  local icon
  if type(mod.logoFile) == "string" and mod.logoFile ~= "" then
    icon = jhmc.zip_read(ctx.jar, (mod.logoFile:gsub("^/", "")))
  end

  local version = mod.version
  if type(version) == "string" and version:find("${", 1, true) then
    local manifest = jhmc.zip_read(ctx.jar, "META-INF/MANIFEST.MF")
    if type(manifest) == "string" then
      version = manifest:match("[\r\n]?Implementation%-Version:%s*([^\r\n]+)")
      if version then version = version:match("^%s*(.-)%s*$") end
    else
      version = nil
    end
  end

  return {
    loader = "forge-legacy",
    game_version = mod.mcversion,
    mod_id = mod.modid,
    name = mod.name or mod.modid,
    version = version,
    authors = authors,
    description = mod.description,
    website = mod.url,
    icon = icon,
  }
end
