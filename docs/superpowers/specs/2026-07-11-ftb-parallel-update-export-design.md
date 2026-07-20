# FTB modpacks: parallel downloads, update, and export

Date: 2026-07-11 · Branch: `feature/modpack-support` · Status: approved

## Problem

1. FTB modpack installs are slow: `ftb.lua` downloads the manifest's files one at
   a time, and every CurseForge-hosted file adds a blocking API round-trip to
   resolve its download URL.
2. Once installed, a modpack server is frozen: the pack identity
   (`packId/versionId`) is discarded after install (`ServerService.Create`
   overwrites `rec.McVersion` with the resolved MC version), so there is no way
   to move a server to a newer pack version.
3. Players joining a modded server need the matching client pack with the
   server's configs. There is no way to export one.

## Decisions (user-approved)

- Update = **manifest diff** (like the official FTB App): delete files the new
  version dropped, download new/changed ones; world and non-pack files
  untouched.
- Export = **CurseForge pack zip** (`manifest.json` + `overrides/`), importable
  by the CurseForge app, Prism, ATLauncher.
- UI placement: **server actions flyout** (伺服器動作), items visible only for
  modpack servers.
- All four pieces in one round.

## 1. Parallel downloads — `jhmc.download_all`

New host function in `engine/internal/scripting/hostfuncs.go`:

```lua
jhmc.download_all(items, { concurrency = 6 })
-- item: { dest = "mods/x.jar", sha1 = "…", url = "https://…" }
-- or:   { dest = "…", sha1 = "…", resolve = { url = "https://…", headers = {…} } }
```

- `resolve` means: GET the JSON endpoint, take the `data` string field as the
  real download URL (the CurseForge `download-url` convention), then download.
  Resolution happens inside the worker, so CF API round-trips parallelize too.
- Go side: worker pool (errgroup + semaphore), default concurrency 6, capped
  (say 16). First error cancels the batch via context; the failing file's name
  is in the error. Requires `network` permission; every `dest` goes through
  `resolvePath` (server-dir confinement, unchanged).
- Progress: on each completed file emit `Progress{Step: "shop.install.downloading",
  Fraction: completed/total, LogLine: dest-basename}`. Per-byte fractions from
  individual downloads are NOT emitted (they'd interleave across workers).
- HTTP status/error semantics per item match today's `download_file` behavior,
  including the CF 403 "author disallows third-party downloads" message.

`ftb.lua` `install_files` builds the full item list (direct-URL files, and
CF-hosted ones as `resolve` items with the configured key header) and makes one
`download_all` call. The missing-CF-key check stays in Lua, up front, before
any download starts.

## 2. Persist pack identity

- `ftb.lua` `install()` (and `update()`) return `pack_version = ctx.version`
  (the opaque `"packId/versionId"`) in the launch spec.
- Engine: new field on the provider spec struct, stored on the server registry
  record (`ProviderVersion`), returned in the proto `Server` message as
  `string provider_version` (new field, next free tag).
- Empty for every non-modpack provider. The app treats "non-empty
  provider_version" as "this is a modpack server".

## 3. Update modpack

**Proto:** `rpc UpdateModpack(UpdateModpackRequest) returns (stream InstallProgress)`
on `ServerService`; request = `{ string id; string version; }` where version is
the new `"packId/versionId"`.

**Engine (`serverservice.go`):**
- Server must be `STOPPED` (same guard style as delete/update); status flips to
  `INSTALLING` during the run, back to `STOPPED` on success, and on failure the
  server dir is NOT wiped (unlike create) — status returns to `STOPPED` and the
  error is returned; the old install keeps working.
- Looks up the provider from `rec.ProviderID`, requires it to implement the new
  optional Lua entry point `update(ctx)`; `ctx.version` = new id,
  `ctx.old_version` = stored `ProviderVersion`. Progress streams through the
  same `newProgressSender` + install log as create.
- On success: store new `ProviderVersion`, update `McVersion`/`JavaMajor`/launch
  args from the returned spec (same post-install bookkeeping as create).

**`ftb.lua` `update(ctx)`:**
- Fetch old + new version manifests. Diff server-side files by `path+name`:
  - present only in old → delete (via `jhmc.fs` remove).
  - present in both with different `sha1` → re-download.
  - present only in new → download.
  - user files (configs they edited that the pack doesn't own, world, etc.) are
    untouched because only manifest-listed paths are considered. A pack file
    the user edited (sha1 differs from BOTH old and new manifest) is
    overwritten with the new pack version — pack files belong to the pack.
- Downloads go through `download_all`.
- If the modloader target (name or version) changed: re-run the loader install
  (existing `install_loader`); else keep existing launch args.
- Returns the same spec shape as `install` (including `pack_version`).

**App:**
- `ServerActionsFlyout` (wherever 伺服器動作 menu is built): add
  "更新模組包…" / "Update modpack…", visible iff `Server.ProviderVersion != ""`,
  enabled iff server stopped.
- Dialog lists versions from the existing `Shop.GetVersions(shopId, projectId)`
  (shopId = `Server.ProviderId`, projectId = pack half of `ProviderVersion`),
  newest first, current one marked, older ones allowed (downgrade) but the
  current one disabled.
- Confirm → hand off to a new `MainViewModel.UpdateModpackAsync(server, version)`
  that streams into the global `ProgressService` tracker exactly like
  `InstallServerAsync` (step text, log lines, error detail, `RefreshAsync`).

## 4. Export modpack

**Proto:** `rpc ExportModpack(ExportModpackRequest) returns (stream InstallProgress)`;
request = `{ string id; string dest_path; }` (absolute zip path picked by the
user, following the existing `ExportAll` dest_path pattern).

**Engine:** Go-side (not Lua — needs no provider polymorphism yet; FTB-only,
gated on `rec.ProviderID == "ftb"` + stored `ProviderVersion`):
- Refetch the pack version manifest for `ProviderVersion`.
- Build CurseForge pack zip:
  - `manifest.json`: `manifestType: "minecraftModpack"`, `manifestVersion: 1`,
    pack name (server name), `version` (pack version name), `minecraft`
    `{ version, modLoaders: [{ id: "neoforge-<ver>"|"forge-<ver>"|"fabric-<ver>", primary: true }] }`,
    `files[]` = every manifest file with `curseforge` project/file ids —
    **including `clientonly` ones** — as `{ projectID, fileID, required: true }`,
    `overrides: "overrides"`.
  - `overrides/`: the server's live config-ish dirs (`config/`, `defaultconfigs/`,
    `kubejs/`, `scripts/`, `resourcepacks/`, `shaderpacks/` — those that exist),
    plus server-local mod jars NOT covered by a CF manifest entry. Excluded
    always: `world*/`, `logs/`, `backups/`, `libraries/`, `crash-reports/`,
    `.jhmc*`, `server.properties`, `eula.txt`, `*.jar` at root (loader/installer).
  - `clientonly` files without CF ids (direct-URL) are downloaded at export time
    into `overrides/` at their manifest path — hence the progress stream
    (`shop.install.downloading` steps reuse existing localization).
- Progress: manifest fetch → client-only downloads (fraction) → zipping → done.
- No CurseForge API key needed (only CF-id references, no CF downloads).

**App:** "匯出模組包…" / "Export modpack…" in the same flyout (visible iff
`ProviderVersion != ""`; works while running — read-only). `FileSavePicker`
(`.zip`, suggested name `<server>-<packversion>.zip`) → stream into the global
tracker with a distinct step key (`shop.export.*`).

## Localization

New keys (en-US + zh-TW): flyout items (`ServerAction_UpdateModpack`,
`ServerAction_ExportModpack`), update-dialog title/body/confirm, version-list
"current" badge, export step keys (`shop.export.preparing`, `shop.export.zipping`,
`shop.export.done`), tracker labels for update/export runs.

## Testing

- Go: unit tests for `download_all` (worker pool, resolve chain, error cancels
  batch, checksum mismatch), manifest diff (delete/changed/new/user-file cases),
  CF manifest.json builder (clientonly included, overrides exclusions).
- Live verify: install a pack (should be visibly faster), update it to an
  adjacent version, export it and inspect the zip's manifest.json + overrides.

## Out of scope

- Update/export for non-FTB modpack sources (design leaves room: `update()` is
  a generic provider entry point; export is FTB-gated in the engine for now).
- `.mrpack` export, server-pack export, config-only export.
- Parallelizing non-modpack providers' downloads (they're single large files).
