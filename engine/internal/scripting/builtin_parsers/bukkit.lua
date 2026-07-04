-- Bukkit/Spigot/Paper plugin metadata parser: reads plugin.yml or
-- paper-plugin.yml.
meta = {
  id = "parser-bukkit",
  name = "Bukkit Plugin Parser",
  description = "Reads plugin.yml / paper-plugin.yml metadata from Bukkit-family plugin jars.",
  version = "1.0.0",
  author = "JustHostMC",
  formats = { "plugin.yml", "paper-plugin.yml" },
  permissions = {
    { kind = "fs_server", reason = "Read plugin jars to extract their metadata" },
  },
}

function parse(ctx)
  local loader = "bukkit"
  local raw = jhmc.zip_read(ctx.jar, "plugin.yml")
  if raw == nil then
    raw = jhmc.zip_read(ctx.jar, "paper-plugin.yml")
    if raw == nil then return nil end
    loader = "paper"
  end
  local ok, m = pcall(jhmc.yaml_decode, raw)
  if not ok or type(m) ~= "table" or m.name == nil then return nil end

  local authors = {}
  if type(m.authors) == "table" then
    for _, a in ipairs(m.authors) do
      if type(a) == "string" then authors[#authors + 1] = a end
    end
  elseif type(m.author) == "string" and m.author ~= "" then
    authors[1] = m.author
  end

  return {
    loader = loader,
    mod_id = m.name,
    name = m.name,
    version = tostring(m.version or ""),
    authors = authors,
    description = m.description,
    website = m.website,
  }
end
