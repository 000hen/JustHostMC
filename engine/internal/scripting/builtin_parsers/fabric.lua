-- Fabric mod metadata parser: reads fabric.mod.json.
-- https://wiki.fabricmc.net/documentation:fabric_mod_json
meta = {
  id = "parser-fabric",
  name = "Fabric Mod Parser",
  description = "Reads fabric.mod.json metadata from Fabric mod jars.",
  version = "1.0.0",
  author = "JustHostMC",
  formats = { "fabric.mod.json" },
  permissions = {
    { kind = "fs_server", reason = "Read mod jars to extract their metadata" },
  },
}

function parse(ctx)
  local raw = jhmc.zip_read(ctx.jar, "fabric.mod.json")
  if raw == nil then return nil end
  -- JSON descriptors produced by some Windows tooling start with a UTF-8 BOM.
  raw = raw:gsub("^\239\187\191", "")
  local ok, m = pcall(jhmc.json_decode, raw)
  if not ok or type(m) ~= "table" then return nil end

  local authors = {}
  local function append_people(people)
    if type(people) ~= "table" then return end
    for _, a in ipairs(people) do
      if type(a) == "string" then
        authors[#authors + 1] = a
      elseif type(a) == "table" and type(a.name) == "string" then
        authors[#authors + 1] = a.name
      end
    end
  end
  append_people(m.authors)
  append_people(m.contributors)

  local website
  if type(m.contact) == "table" then
    if type(m.contact.homepage) == "string" then
      website = m.contact.homepage
    elseif type(m.contact.sources) == "string" then
      website = m.contact.sources
    elseif type(m.contact.issues) == "string" then
      website = m.contact.issues
    end
  end

  -- icon is a path string or a map of size -> path; pick the largest size.
  local icon_path
  if type(m.icon) == "string" then
    icon_path = m.icon
  elseif type(m.icon) == "table" then
    local best_n
    for k, v in pairs(m.icon) do
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
    loader = "fabric",
    mod_id = m.id,
    name = m.name or m.id,
    version = m.version,
    authors = authors,
    description = m.description,
    website = website,
    icon = icon,
  }
end
