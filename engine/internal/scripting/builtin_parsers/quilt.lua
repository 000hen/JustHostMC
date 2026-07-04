-- Quilt mod metadata parser: reads quilt.mod.json.
-- https://github.com/QuiltMC/rfcs/blob/main/specification/0002-quilt.mod.json.md
meta = {
  id = "parser-quilt",
  name = "Quilt Mod Parser",
  description = "Reads quilt.mod.json metadata from Quilt mod jars.",
  version = "1.0.0",
  author = "JustHostMC",
  formats = { "quilt.mod.json" },
  permissions = {
    { kind = "fs_server", reason = "Read mod jars to extract their metadata" },
  },
}

function parse(ctx)
  local raw = jhmc.zip_read(ctx.jar, "quilt.mod.json")
  if raw == nil then return nil end
  raw = raw:gsub("^\239\187\191", "")
  local ok, m = pcall(jhmc.json_decode, raw)
  if not ok or type(m) ~= "table" or type(m.quilt_loader) ~= "table" then return nil end

  local ql = m.quilt_loader
  local md = type(ql.metadata) == "table" and ql.metadata or {}

  -- contributors is a map of name -> role.
  local authors = {}
  if type(md.contributors) == "table" then
    for name in pairs(md.contributors) do
      if type(name) == "string" then authors[#authors + 1] = name end
    end
    table.sort(authors)
  end

  local website
  if type(md.contact) == "table" then
    if type(md.contact.homepage) == "string" then
      website = md.contact.homepage
    elseif type(md.contact.sources) == "string" then
      website = md.contact.sources
    elseif type(md.contact.issues) == "string" then
      website = md.contact.issues
    end
  end

  -- Like Fabric, Quilt accepts either one icon path or a resolution map.
  local icon_path
  if type(md.icon) == "string" then
    icon_path = md.icon
  elseif type(md.icon) == "table" then
    local best_n
    for k, v in pairs(md.icon) do
      local n = tonumber(k)
      if n and type(v) == "string" and (best_n == nil or n > best_n) then
        best_n, icon_path = n, v
      end
    end
  end
  local icon
  if type(icon_path) == "string" and icon_path ~= "" then
    icon = jhmc.zip_read(ctx.jar, (icon_path:gsub("^/", "")))
  end

  return {
    loader = "quilt",
    mod_id = ql.id,
    name = md.name or ql.id,
    version = ql.version,
    authors = authors,
    description = md.description,
    website = website,
    icon = icon,
  }
end
