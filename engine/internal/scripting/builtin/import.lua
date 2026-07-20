-- Local modpack import provider: installs a server from a modpack file the user
-- picked on their machine (a CurseForge client pack zip or a Modrinth .mrpack).
-- Hidden from the create-server UI — the ImportModpack RPC stages the file into
-- the new server dir at .jhmc/import.zip and drives this provider. It detects the
-- format and reuses the shared mplib install routines, so an imported server
-- carries the same normalized manifest as a shop-installed one (Export works).
-- There is no update() — a local file has no upstream version to diff against.

meta = {
  id = "import",
  name = "Imported Modpack",
  website = "",
  description = "Installs a server from a local CurseForge or Modrinth modpack file.",
  version = "1.0.0",
  author = "JustHostMC",
  mod_layout = "mods",
  hidden = true,
  permissions = {
    { kind = "network", reason = "Download the pack's mods and the loader installer." },
    { kind = "install", reason = "Resolve a JRE and run the Forge/NeoForge installer's --installServer." },
    { kind = "fs_server", reason = "Unpack the modpack and detect the launch command." },
  },
  config = {
    { key = "curseforge_api_key", type = "secret", name = "CurseForge API key",
      description = "Required only when importing a CurseForge pack with CurseForge-hosted mods", required = false },
  },
}

local STAGED = ".jhmc/import.zip"

function versions()
  return {}
end

local function key(ctx)
  local cfg = ctx.config or {}
  return cfg.curseforge_api_key or ""
end

function install(ctx)
  local entries = jhmc.zip_entries(STAGED)
  local has = {}
  for _, e in ipairs(entries) do has[e] = true end

  ctx.step("install.progress.resolving_version", -1)
  local spec
  if has["manifest.json"] then
    local k = key(ctx)
    if k == "" then
      error("this CurseForge modpack needs a CurseForge API key to resolve its mods (set it in the provider settings)")
    end
    spec = mplib.install_cf_pack(ctx, STAGED, k)
  elseif has["modrinth.index.json"] then
    spec = mplib.install_mrpack(ctx, STAGED)
  else
    error("not a recognized modpack file: expected a CurseForge manifest.json or a Modrinth modrinth.index.json")
  end

  jhmc.fs.remove(STAGED)

  ctx.step("install.progress.done", 1)
  local version = (spec.version_name and spec.version_name ~= "") and spec.version_name or "local"
  spec.version_name = nil
  -- A non-empty pack_version enables the Export action; the "import" provider id
  -- is how the app keeps the Update action hidden (no upstream to diff against).
  spec.pack_version = "import/" .. version
  return spec
end
