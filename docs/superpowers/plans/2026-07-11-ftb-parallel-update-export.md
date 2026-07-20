# FTB Parallel Downloads + Modpack Update/Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Speed up FTB modpack installs with a parallel batch-download host function, and add "update modpack" (manifest diff) and "export modpack" (CurseForge client zip) for modpack servers.

**Architecture:** A new Go host function `jhmc.download_all` gives single-threaded Lua a worker pool; the pack identity (`packId/versionId`) is persisted on the server record so two new streaming RPCs (`UpdateModpack`, `ExportModpack`) can act on it. The app adds two flyout items on the server header, streaming both operations through the existing global install-progress tracker.

**Tech Stack:** Go (engine, gopher-lua host), Lua (ftb.lua provider), protobuf/buf, WinUI 3 C# (app).

**Spec:** `docs/superpowers/specs/2026-07-11-ftb-parallel-update-export-design.md`

## Global Constraints

- Go tests: `go test ./...` from `engine/`. App build: `dotnet build app\JustHostMC.App\JustHostMC.App.csproj -p:Platform=x64`.
- After editing `proto/mcmanager/v1/mcmanager.proto`: run `buf generate` from `proto/`. If Go later reports `undefined: mcmanagerv1.X`, a background VS build clobbered the stubs — just re-run `buf generate`.
- All edited `.xaml` files MUST keep their UTF-8 BOM.
- No `[ObservableProperty]` partial properties in C# (breaks the XAML compiler); use plain properties/`SetProperty`.
- Localized strings go in BOTH `app\JustHostMC.App\Strings\en-US\Resources.resw` and `app\JustHostMC.App\Strings\zh-TW\Resources.resw` (folder is `zh-TW`, not zh-Hant).
- gRPC stream sends are NOT concurrency-safe: any concurrent progress emission must be serialized with a mutex.
- Commit after each task; message prefix `feat:`/`test:` as appropriate, ending with the Claude Co-Authored-By trailer.

---

### Task 1: Proto — provider_version field + UpdateModpack/ExportModpack RPCs

**Files:**
- Modify: `proto/mcmanager/v1/mcmanager.proto`

**Interfaces (produces):**
- `Server.provider_version = 11` (string)
- `message UpdateModpackRequest { string id = 1; string version = 2; }`
- `message ExportModpackRequest { string id = 1; string dest_path = 2; }`
- `rpc UpdateModpack(UpdateModpackRequest) returns (stream InstallProgress);`
- `rpc ExportModpack(ExportModpackRequest) returns (stream InstallProgress);`

- [ ] **Step 1: Edit the proto.** In `message Server` (line ~36) add after `loader = 10`:
```proto
  // Opaque provider pack version ("packId/versionId") for modpack servers;
  // empty for normal servers. Non-empty means update/export are available.
  string provider_version = 11;
```
After `UpdateServerRequest` (line ~66) add:
```proto
message UpdateModpackRequest {
  string id = 1;
  string version = 2; // new opaque "packId/versionId"
}
message ExportModpackRequest {
  string id = 1;
  string dest_path = 2; // absolute .zip path chosen by the user
}
```
In `service ServerService` (line ~748) add after `rpc Update`:
```proto
  rpc UpdateModpack(UpdateModpackRequest) returns (stream InstallProgress); // manifest-diff move to another pack version
  rpc ExportModpack(ExportModpackRequest) returns (stream InstallProgress); // CurseForge client pack zip
```
- [ ] **Step 2: Regenerate stubs.** Run from `proto/`: `buf generate`. Expected: exit 0, `engine/gen/...` updated.
- [ ] **Step 3: Engine still compiles.** Run from `engine/`: `go build ./...`. Expected: exit 0 (new RPCs unimplemented is fine — the generated server uses `mustEmbedUnimplemented`).
- [ ] **Step 4: Commit** `feat(proto): provider_version + UpdateModpack/ExportModpack RPCs`.

---

### Task 2: Persist pack identity end-to-end

**Files:**
- Modify: `engine/internal/provider/provider.go` (LaunchSpec, ~line 19)
- Modify: `engine/internal/scripting/host.go` (`invocation.install`, ~line 225)
- Modify: `engine/internal/store/store.go` (Server struct + Proto, lines 15-45)
- Modify: `engine/internal/grpc/serverservice.go` (Create bookkeeping, ~line 188)
- Modify: `engine/internal/scripting/builtin/ftb.lua` (install return, ~line 265)
- Test: `engine/internal/scripting/luaprovider_test.go` (extend the existing `TestInstallLaunchSpecReadsMcVersionAndLoader` pattern)

**Interfaces:**
- Produces: `provider.LaunchSpec.PackVersion string`; `store.Server.ProviderVersion string`; Lua spec key `pack_version`.

- [ ] **Step 1: Write the failing test** — copy `TestInstallLaunchSpecReadsMcVersionAndLoader`, have the script return `pack_version = "95/12695"`, assert `spec.PackVersion == "95/12695"`.
- [ ] **Step 2: Run it** — `go test ./internal/scripting/ -run PackVersion -v` → FAIL (unknown field).
- [ ] **Step 3: Implement.**
  - `provider.go` LaunchSpec: add `PackVersion string // opaque "packId/versionId" for modpack providers; empty otherwise`.
  - `host.go` install() return: add `PackVersion: strField(spec, "pack_version"),`.
  - `store.go` Server: add `ProviderVersion string`; Proto(): add `ProviderVersion: s.ProviderVersion,`.
  - `serverservice.go` Create, next to `rec.Loader = spec.Loader` (~line 194): `rec.ProviderVersion = spec.PackVersion`.
  - `ftb.lua` install() return table: add `pack_version = tostring(ctx.version),`.
- [ ] **Step 4: Test passes** — `go test ./internal/scripting/ ./internal/store/... ./internal/grpc/... ` → PASS.
- [ ] **Step 5: Commit** `feat(engine): persist modpack pack identity on the server record`.

---

### Task 3: `jhmc.download_all` host function (parallel worker pool)

**Files:**
- Modify: `engine/internal/scripting/hostfuncs.go` (register next to `download`, ~line 283; registration table near the `fs` table, look for `RawSetString("download", ...)`)
- Test: `engine/internal/scripting/hostfuncs_download_all_test.go` (new)

**Interfaces:**
- Produces (Lua): `jhmc.download_all(items, opts?)`;
  item = `{ dest = "mods/x.jar", sha1?, sha256?, url = "https://…" }` OR
  `{ dest, sha1?, resolve = { url = "https://…", headers = { ["x-api-key"] = key } } }`;
  opts = `{ concurrency = 6 }` (cap 16). Returns nothing; raises on first error.
- `resolve` semantics: GET the URL with headers, expect JSON `{ "data": "<real url>" }`
  (the CurseForge download-url convention); 403 maps to the author-disallows message.

- [ ] **Step 1: Write failing tests** (httptest server; use the package's existing test helpers for building an `invocation` — see how `hostfuncs` tests construct a Host):
  - N files download concurrently and all land at their dests with correct content.
  - A `resolve` item fetches the JSON endpoint and downloads `data`.
  - One 404 item fails the whole call, error names the dest, and the context cancels in-flight work (assert with a slow handler + elapsed-time bound).
  - `sha1` mismatch fails with `provider.ErrChecksumMismatch`.
  - Progress: emitted fractions are `completed/total` and monotonically observed count == number of files.
- [ ] **Step 2: Run** — `go test ./internal/scripting/ -run DownloadAll -v` → FAIL (no `download_all`).
- [ ] **Step 3: Implement** in `hostfuncs.go`:
```go
type batchItem struct {
	dest, full, url string          // full = resolved absolute path
	resolveURL      string
	resolveHeaders  map[string]string
	h               func() hash.Hash // sha constructor or nil
	want            string
}

// downloadAll backs jhmc.download_all: everything Lua-facing (arg parsing,
// resolvePath) happens on the Lua thread up front; workers are pure Go.
func (inv *invocation) downloadAll(L *lua.LState) int {
	inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_NETWORK)
	itemsTbl := L.CheckTable(1)
	concurrency := 6
	if opts, ok := L.Get(2).(*lua.LTable); ok {
		if n, ok := opts.RawGetString("concurrency").(lua.LNumber); ok && int(n) > 0 {
			concurrency = min(int(n), 16)
		}
	}
	var items []batchItem
	var parseErr error
	itemsTbl.ForEach(func(_, v lua.LValue) { /* build batchItem; resolvePath(L, dest);
		strField url/sha1/sha256; nested resolve table url+headers; set parseErr on bad item */ })
	if parseErr != nil { inv.fail(L, parseErr); return 0 }

	total := len(items)
	var done atomic.Int64
	var emitMu sync.Mutex
	emit := func(p provider.Progress) { emitMu.Lock(); inv.emit(p); emitMu.Unlock() }

	g, ctx := errgroup.WithContext(inv.ctx)
	g.SetLimit(concurrency)
	for _, it := range items {
		it := it
		g.Go(func() error {
			url := it.url
			if url == "" { // resolve chain
				u, err := resolveDownloadURL(ctx, &inv.host.client, it.resolveURL, it.resolveHeaders)
				if err != nil { return fmt.Errorf("%s: %w", it.dest, err) }
				url = u
			}
			var h hash.Hash
			if it.h != nil { h = it.h() }
			sum, _, err := dl.Download(ctx, &inv.host.client, url, it.full, h, nil)
			if err != nil { return fmt.Errorf("%s: %w", it.dest, err) }
			if it.want != "" && !strings.EqualFold(sum, it.want) {
				return fmt.Errorf("%s: %w (got %s want %s)", it.dest, provider.ErrChecksumMismatch, sum, it.want)
			}
			n := done.Add(1)
			emit(provider.Progress{Step: "shop.install.downloading",
				Fraction: float64(n) / float64(total), LogLine: path.Base(it.dest)})
			return nil
		})
	}
	if err := g.Wait(); err != nil { inv.fail(L, err); return 0 }
	return 0
}
```
  `resolveDownloadURL` (same file): GET with headers via `http.NewRequestWithContext`, 403 → `errors.New("CurseForge denied downloading (the author disallows third-party downloads)")`, non-2xx → status error, decode `{Data string}` and require non-empty. Register: `jhmcTbl.RawSetString("download_all", L.NewFunction(inv.downloadAll))` next to the existing `download` registration. Add `golang.org/x/sync/errgroup` to go.mod if absent (`go get golang.org/x/sync/errgroup`).
  Note: per-byte progress is intentionally NOT forwarded (pass `nil` to `dl.Download`) — per-file completion only, so fractions don't interleave across workers.
- [ ] **Step 4: Tests pass** — `go test ./internal/scripting/ -run DownloadAll -v` → PASS.
- [ ] **Step 5: Commit** `feat(engine): jhmc.download_all parallel batch download host function`.

---

### Task 4: ftb.lua uses download_all

**Files:**
- Modify: `engine/internal/scripting/builtin/ftb.lua` (`download_file`/`install_files`, lines 66-118)
- Test: `engine/internal/scripting/builtin_ftb_test.go` if a test harness for the script exists (check for existing ftb.lua tests; else covered by Task 3 unit tests + live verify)

- [ ] **Step 1: Replace `install_files`** (keep `join_path`; `download_file` is deleted):
```lua
-- install_files downloads every server-side file in the manifest through one
-- parallel batch; CurseForge-hosted files resolve their real URL inside the
-- batch workers.
local function install_files(ctx, files)
  local key = curseforge_key(ctx)
  local items = {}
  for _, f in ipairs(files) do
    if type(f) == "table" and not f.clientonly and (f.name or "") ~= "" then
      local dest = join_path(f.path, f.name)
      if (f.url or "") ~= "" then
        items[#items + 1] = { dest = dest, sha1 = f.sha1, url = f.url }
      else
        local cf = f.curseforge
        if type(cf) == "table" and cf.project and cf.file then
          if key == "" then
            error("install failed: file " .. (f.name or dest) ..
              " is hosted on CurseForge and needs a CurseForge API key (set it in the FTB provider settings)")
          end
          items[#items + 1] = { dest = dest, sha1 = f.sha1, resolve = {
            url = "https://api.curseforge.com/v1/mods/" .. cf.project ..
              "/files/" .. cf.file .. "/download-url",
            headers = { ["x-api-key"] = key },
          } }
        else
          error("install failed: file " .. (f.name or dest) .. " has no download source")
        end
      end
    end
  end
  if #items == 0 then return end
  ctx.step("shop.install.downloading", 0)
  jhmc.download_all(items)
end
```
- [ ] **Step 2: Engine tests still green** — `go test ./...` from `engine/` → PASS.
- [ ] **Step 3: Commit** `feat(engine): parallel FTB modpack file downloads`.

---

### Task 5: Lua `update()` contract + LuaProvider.Update

**Files:**
- Modify: `engine/internal/provider/provider.go` (new interface)
- Modify: `engine/internal/scripting/host.go` (new `invocation.update`, mirror `install` at line 177)
- Modify: `engine/internal/scripting/luaprovider.go` (new method)
- Test: `engine/internal/scripting/luaprovider_test.go`

**Interfaces (produces):**
```go
// provider.go
// Updater is optionally implemented by providers that can move an installed
// server to another version in place (modpack manifest diff).
type Updater interface {
	Update(ctx context.Context, dir, version, oldVersion string, progress func(Progress)) (LaunchSpec, error)
}
var ErrUpdateUnsupported = errors.New("provider does not support update")
```
- Lua side: optional global `update(ctx)`; ctx has everything install's ctx has plus `ctx.old_version`.

- [ ] **Step 1: Failing tests** — script with `update(ctx)` returning a spec echoing `ctx.old_version` in a field; assert `LuaProvider.Update` passes both versions and parses the spec (incl. `pack_version`). Script WITHOUT `update` → `ErrUpdateUnsupported`.
- [ ] **Step 2: Run** — FAIL (no Update method).
- [ ] **Step 3: Implement.** `host.go`: `func (inv *invocation) update(src, dir, version, oldVersion string) (provider.LaunchSpec, error)` — copy of `install` with: global name `"update"`, missing-function returns `provider.ErrUpdateUnsupported`, and `ictx.RawSetString("old_version", lua.LString(oldVersion))`. Extract the shared ctx-table construction and spec parsing into helpers (`installCtxTable`, `parseLaunchSpec`) used by both, so the two entry points can't drift. `luaprovider.go`:
```go
// Update implements provider.Updater.
func (p *LuaProvider) Update(ctx context.Context, dir, version, oldVersion string, progress func(provider.Progress)) (provider.LaunchSpec, error) {
	inv := &invocation{ctx: ctx, host: p.host, granted: p.grants(), report: progress, assetDir: p.assetDir, config: p.config()}
	return inv.update(p.source, dir, version, oldVersion)
}
```
- [ ] **Step 4: Tests pass; Step 5: Commit** `feat(engine): optional update() provider entry point`.

---

### Task 6: ftb.lua `update()` — manifest diff

**Files:**
- Modify: `engine/internal/scripting/builtin/ftb.lua` (append after `install`)

- [ ] **Step 1: Implement** (reuses `read_targets`, `join_path`, `install_files`, `install_loader`):
```lua
-- server_files indexes a manifest's server-side files by relative dest path.
local function server_files(manifest)
  local by_dest = {}
  for _, f in ipairs(manifest.files or {}) do
    if type(f) == "table" and not f.clientonly and (f.name or "") ~= "" then
      by_dest[join_path(f.path, f.name)] = f
    end
  end
  return by_dest
end

-- update moves an installed pack to another version: files only the old
-- version listed are deleted, new/changed ones are downloaded, and the loader
-- is reinstalled only when its pinned version changed. Files the pack never
-- listed (world, user configs) are untouched.
function update(ctx)
  local pack, ver = tostring(ctx.version):match("^([^/]+)/([^/]+)$")
  local opack, over = tostring(ctx.old_version):match("^([^/]+)/([^/]+)$")
  if not pack or not ver then error("invalid modpack version id: " .. tostring(ctx.version)) end
  if not opack or not over then error("invalid modpack version id: " .. tostring(ctx.old_version)) end
  if pack ~= opack then error("update must stay within the same pack (" .. opack .. " -> " .. pack .. ")") end

  ctx.step("install.progress.resolving_version", -1)
  local old_manifest = jhmc.http_json(FTB_API .. "/modpack/" .. opack .. "/" .. over)
  local new_manifest = jhmc.http_json(FTB_API .. "/modpack/" .. pack .. "/" .. ver)
  if type(old_manifest) ~= "table" or type(new_manifest) ~= "table" then
    error("ftb: bad manifest")
  end

  local old_files, new_files = server_files(old_manifest), server_files(new_manifest)

  -- Delete files the new version dropped.
  for dest in pairs(old_files) do
    if not new_files[dest] and jhmc.fs.exists(dest) then
      ctx.log("- " .. dest)
      jhmc.fs.remove(dest)
    end
  end

  -- Download new files and files whose pack hash changed.
  local changed = {}
  for dest, f in pairs(new_files) do
    local old = old_files[dest]
    if not old or (old.sha1 or "") ~= (f.sha1 or "") then
      changed[#changed + 1] = f
    end
  end
  install_files(ctx, changed)

  local mc, loader_name, loader_version = read_targets(new_manifest)
  if not mc or mc == "" then error("ftb: modpack has no Minecraft version target") end
  if not loader_name or loader_name == "" then error("ftb: modpack has no modloader target") end
  local omc, oloader_name, oloader_version = read_targets(old_manifest)

  local java_major = jhmc.java_major_for(mc)
  local args
  if loader_name ~= oloader_name or loader_version ~= oloader_version or mc ~= omc then
    args = install_loader(ctx, loader_name, loader_version, mc, java_major)
  end

  ctx.step("install.progress.done", 1)
  local spec = {
    java_major = java_major,
    mc_version = mc,
    loader = loader_name,
    pack_version = tostring(ctx.version),
  }
  spec.args = args -- nil = keep existing launch args (engine side)
  return spec
end
```
  Note: `install_files` sorts nothing; changed-list order doesn't matter. `jhmc.fs.remove` exists (hostfuncs.go:368).
- [ ] **Step 2: Engine tests green** — `go test ./...` → PASS. **Step 3: Commit** `feat(engine): ftb.lua manifest-diff update()`.

---

### Task 7: ServerService.UpdateModpack RPC

**Files:**
- Modify: `engine/internal/grpc/serverservice.go` (new method after `Update`, ~line 310)
- Test: `engine/internal/grpc/serverservice_update_test.go` (extend; see `TestUpdateVersionRefreshesLaunchSpec` for the harness)

**Interfaces:**
- Consumes: `provider.Updater`, `store.Server.ProviderVersion`, `newProgressSender`, `openInstallLog`, `maxJavaMajor`, `mapInstallError`.

- [ ] **Step 1: Failing tests** (fake provider implementing Updater): happy path updates `ProviderVersion`/`McVersion`/status back to STOPPED; keeps old `LaunchArgs` when spec.Args is nil, replaces when set; rejects RUNNING server (`FailedPrecondition`); rejects server with empty `ProviderVersion`; provider error → status back to STOPPED, dir NOT deleted, record intact.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement:**
```go
// UpdateModpack moves a modpack server to another pack version in place,
// streaming progress. Unlike Create, a failure leaves the existing install
// untouched (no cleanup wipe) — the old version keeps working.
func (s *ServerService) UpdateModpack(req *mcmanagerv1.UpdateModpackRequest, stream grpc.ServerStreamingServer[mcmanagerv1.InstallProgress]) error {
	ctx := stream.Context()
	rec, ok := s.cfg.Store.Get(req.Id)
	if !ok { return status.Error(codes.NotFound, "server not found") }
	if rec.Status != mcmanagerv1.ServerStatus_STOPPED {
		return status.Error(codes.FailedPrecondition, "server must be stopped")
	}
	if rec.ProviderVersion == "" {
		return status.Error(codes.FailedPrecondition, "server was not installed from a modpack")
	}
	newVersion := strings.TrimSpace(req.Version)
	if newVersion == "" { return status.Error(codes.InvalidArgument, "version is required") }
	entry, ok := s.cfg.Providers.Get(rec.ProviderID)
	if !ok { return status.Errorf(codes.Unimplemented, "provider %q not installed", rec.ProviderID) }
	up, ok := entry.Provider.(provider.Updater)
	if !ok { return status.Error(codes.Unimplemented, "provider does not support update") }

	oldVersion := rec.ProviderVersion
	rec.Status = mcmanagerv1.ServerStatus_INSTALLING
	_ = s.cfg.Store.Put(rec)
	restore := func() { rec.Status = mcmanagerv1.ServerStatus_STOPPED; _ = s.cfg.Store.Put(rec) }

	il := s.openInstallLog(rec.ID)
	defer il.Close()
	il.recordLine("[update] " + oldVersion + " -> " + newVersion)
	base := newProgressSender(stream)
	send := func(p provider.Progress) { il.record(p); base(p) }

	spec, err := up.Update(ctx, s.cfg.Paths.ServerDir(rec.ID), newVersion, oldVersion, send)
	if err != nil {
		send(provider.Progress{LogLine: "[error] " + err.Error()})
		il.recordLine("[error] update: " + err.Error())
		restore()
		return mapInstallError(err)
	}
	resolved := rec.McVersion
	if spec.McVersion != "" { resolved = spec.McVersion }
	spec.JavaMajor = maxJavaMajor(spec.JavaMajor, resolved)
	if _, err := s.cfg.JRE.EnsureJRE(ctx, spec.JavaMajor, send); err != nil {
		send(provider.Progress{LogLine: "[error] " + err.Error()})
		restore()
		return mapInstallError(err)
	}
	rec.JavaMajor = spec.JavaMajor
	if len(spec.Args) > 0 { rec.LaunchArgs = spec.Args }
	if spec.McVersion != "" { rec.McVersion = spec.McVersion }
	if spec.Loader != "" { rec.Loader = spec.Loader }
	if spec.PackVersion != "" { rec.ProviderVersion = spec.PackVersion }
	rec.Status = mcmanagerv1.ServerStatus_STOPPED
	_ = s.cfg.Store.Put(rec)
	send(provider.Progress{Step: "install.progress.done", Fraction: 1})
	return nil
}
```
- [ ] **Step 4: Tests pass. Step 5: Commit** `feat(engine): UpdateModpack RPC (manifest-diff modpack update)`.

---

### Task 8: ExportModpack — CurseForge zip builder + RPC

**Files:**
- Create: `engine/internal/modpack/export.go` + `export_test.go` (pure logic, testable without gRPC)
- Modify: `engine/internal/grpc/serverservice.go` (thin RPC wrapper)

**Interfaces (produces):**
```go
// package modpack
// Export builds a CurseForge-format client pack zip for an FTB pack install.
func Export(ctx context.Context, client *http.Client, o Options, progress func(provider.Progress)) error
type Options struct {
	ServerDir   string
	DestZip     string // absolute .zip
	PackVersion string // "packId/versionId"
	ServerName  string
	FTBAPIBase  string // default https://api.feed-the-beast.com/v1/modpacks/public; overridable in tests
}
```

- [ ] **Step 1: Failing tests** (httptest FTB API fixture + temp server dir):
  - `manifest.json` content: manifestType/manifestVersion/name/version, `minecraft.version`, `modLoaders[0].id` formatted `"<loader>-<version>"` with the mc prefix stripped (reuse the ftb.lua `strip_mc_prefix` rule in Go), `files[]` includes CF entries for BOTH server and `clientonly` files, `overrides: "overrides"`.
  - Overrides content: existing `config/`, `defaultconfigs/`, `kubejs/`, `scripts/`, `resourcepacks/`, `shaderpacks/` copied; `world/`, `logs/`, `backups/`, `libraries/`, `crash-reports/`, root `*.jar`, `server.properties`, `eula.txt`, `.jhmc*` excluded; local `mods/*.jar` NOT matched by a CF manifest entry included.
  - `clientonly` file with direct `url` and no CF ids → downloaded into `overrides/<path>/<name>`.
  - Progress emits `shop.export.preparing` → downloading fractions → `shop.export.zipping` → done.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement `export.go`:** fetch manifest JSON (struct with `Name`, `Targets []{Type,Name,Version}`, `Files []{Path,Name,URL,SHA1,Clientonly bool, Curseforge *{Project,File int64}}`); build the manifest.json struct; identify CF-covered mod filenames (set of `Name` where `Curseforge != nil`); walk allow-listed dirs + `mods/` (skipping CF-covered jars); download client-only direct-URL files to a temp staging dir; stream everything into `zip.NewWriter` (forward-slash paths, `overrides/` prefix). Serialize progress with a mutex if downloads are parallel (reuse `errgroup` pattern from Task 3, concurrency 6).
- [ ] **Step 4: Tests pass** — `go test ./internal/modpack/ -v`.
- [ ] **Step 5: RPC wrapper** in `serverservice.go`:
```go
// ExportModpack writes a CurseForge-format client pack zip for a modpack
// server. Read-only with respect to the server dir; allowed while running.
func (s *ServerService) ExportModpack(req *mcmanagerv1.ExportModpackRequest, stream grpc.ServerStreamingServer[mcmanagerv1.InstallProgress]) error {
	rec, ok := s.cfg.Store.Get(req.Id)
	if !ok { return status.Error(codes.NotFound, "server not found") }
	if rec.ProviderVersion == "" || rec.ProviderID != "ftb" {
		return status.Error(codes.FailedPrecondition, "server was not installed from an FTB modpack")
	}
	dest := req.DestPath
	if !filepath.IsAbs(dest) || !strings.EqualFold(filepath.Ext(dest), ".zip") {
		return status.Error(codes.InvalidArgument, "dest_path must be an absolute .zip path")
	}
	send := newProgressSender(stream)
	err := modpack.Export(stream.Context(), s.httpClient(), modpack.Options{
		ServerDir: s.cfg.Paths.ServerDir(rec.ID), DestZip: dest,
		PackVersion: rec.ProviderVersion, ServerName: rec.Name,
	}, send)
	if err != nil { return status.Errorf(codes.Internal, "export: %v", err) }
	send(provider.Progress{Step: "shop.export.done", Fraction: 1})
	return nil
}
```
  (`s.httpClient()`: check what ServerService already has; if nothing, add `HTTP *http.Client` to its cfg wired from `main.go` — same client the scripting host uses.)
- [ ] **Step 6: `go test ./...` green. Step 7: Commit** `feat(engine): ExportModpack RPC (CurseForge client pack zip)`.

---

### Task 9: App — ProviderVersion + flyout items + dialogs + global-tracker streaming

**Files:**
- Modify: `app/JustHostMC.App/Models/ServerItem.cs` (add `ProviderVersion` from proto — follow how `ProviderId` is mapped/applied)
- Modify: `app/JustHostMC.App/Controls/Server/ServerHeaderPanel.xaml` (+ `.xaml.cs`) — two `MenuFlyoutItem`s in the existing flyout (after `ServerOpenInstanceFolderMenuItem`, line ~111): `x:Uid="ServerUpdateModpackMenuItem"` (Glyph `&#xE777;`) and `x:Uid="ServerExportModpackMenuItem"` (Glyph `&#xEDE1;`). Visibility bound to a code-behind helper `ModpackItemVisibility(Server)` (`ProviderVersion != ""`); update item additionally disabled unless stopped. **Preserve BOM.**
- Modify: `app/JustHostMC.App/ViewModels/MainViewModel.cs` — two new methods next to `InstallServerAsync` (line 174):
  - `public async Task UpdateModpackAsync(ServerItem server, string version)` — clone of `InstallServerAsync`'s tracker/stream/error handling with `daemon.Servers.UpdateModpack(new UpdateModpackRequest { Id = server.Id, Version = version })`; tracker name = server name; on success `await RefreshAsync()`.
  - `public async Task ExportModpackAsync(ServerItem server, string destPath)` — same shape over `ExportModpack`; no `RefreshAsync`; tracker `IsReadyToRun` stays false, `CurrentStep` = localized `shop.export.done` on completion.
- Modify: `app/JustHostMC.App/Views/ServerPage.xaml.cs` — handle the two new header-panel events (follow the existing `OnBackupsClick`/BackupsDialog wiring pattern in ServerHeaderPanel → ServerPage):
  - **Update:** dialog (imperative, like `ShopDetailPage.CreateServerFlow`) that awaits `daemon.Shop.GetVersionsAsync(new ShopVersionsRequest { ShopId = server.ProviderId, ProjectId = packId })` where `packId = server.ProviderVersion.Split('/')[0]`; ComboBox of `ShopVersion.Name` newest-first; the entry whose `Id` == current `versionId` (`Split('/')[1]`) labeled with the "current" suffix and disabled; confirm → `_ = _main.UpdateModpackAsync(server, $"{packId}/{picked.Id}")`.
  - **Export:** `FileSavePicker` (init with `WinRT.Interop.InitializeWithWindow` — copy the existing ExportAll picker usage, see `ModsViewModel.ExportAllAsync` call site), suggested filename `$"{server.Name}.zip"`, then `_ = _main.ExportModpackAsync(server, file.Path)`.

- [ ] **Step 1: ServerItem.ProviderVersion** (map in ctor + `ApplyLocal`/merge path — mirror `ProviderId` handling).
- [ ] **Step 2: MainViewModel methods** (complete code mirroring lines 174-240, swapping the RPC call; keep `RunOnUI`, `MapErrorKey`, `ex.Status.Detail` handling identical).
- [ ] **Step 3: Flyout items + events + handlers** as above.
- [ ] **Step 4: Build** `dotnet build app\JustHostMC.App\JustHostMC.App.csproj -p:Platform=x64` → 0 errors.
- [ ] **Step 5: Commit** `feat(app): modpack update and export actions`.

---

### Task 10: Localization

**Files:**
- Modify: `app/JustHostMC.App/Strings/en-US/Resources.resw`, `app/JustHostMC.App/Strings/zh-TW/Resources.resw`

- [ ] **Step 1: Add keys to BOTH files:**

| Key | en-US | zh-TW |
|---|---|---|
| `ServerUpdateModpackMenuItem.Text` | Update modpack… | 更新模組包… |
| `ServerExportModpackMenuItem.Text` | Export modpack… | 匯出模組包… |
| `Server_UpdateModpackTitle` | Update modpack | 更新模組包 |
| `Server_UpdateModpackBody` | Pick a version for "{0}". The world and your own files are kept; pack files are replaced. | 為「{0}」選擇版本。世界與你自己的檔案會保留，模組包檔案將被更換。 |
| `Server_UpdateModpackConfirm` | Update | 更新 |
| `Server_UpdateModpackCurrent` | (current) | （目前版本） |
| `Server_UpdateModpackNoVersions` | No other versions available. | 沒有其他可用版本。 |
| `Server_ExportModpackPickerName` | CurseForge modpack | CurseForge 模組包 |
| `shop.export.preparing` | Preparing modpack export… | 正在準備匯出模組包… |
| `shop.export.zipping` | Packaging modpack… | 正在打包模組包… |
| `shop.export.done` | Modpack exported. | 模組包已匯出。 |

- [ ] **Step 2: Build → 0 errors. Step 3: Commit** `feat(app): modpack update/export strings`.

---

### Task 11: Full build + live verification

- [ ] **Step 1:** `.\build.ps1` (runs buf generate + go build + go test + dotnet build + dotnet test) → all green.
- [ ] **Step 2: Live verify** (invoke the repo `verify` skill — `.claude/skills/verify/SKILL.md` has the launch/UIA recipe):
  1. Install a small FTB pack from the home-page modpack shop; compare wall-clock of the `shop.install.downloading` phase against the previous serial run (log timestamps in `%LOCALAPPDATA%\JustHostMC\logs\<id>\install-*.log`) — expect a clear speedup.
  2. On the new server: actions flyout shows both items; Update lists versions with the current one disabled; pick the adjacent older version → global tracker streams; server flips back to stopped; `install-*.log` shows `[update]` line, deletions (`- path`) and downloads; world/eula untouched.
  3. Export → save zip → open it: `manifest.json` well-formed (spot-check `files[]` count vs manifest, `modLoaders[0].id`), `overrides/config/` present, no `world/`.
  4. Regression: normal (non-modpack) server shows neither flyout item; create-server and mod-shop installs still work.
- [ ] **Step 3: Commit** any fixes; final commit `feat: FTB parallel downloads, modpack update and export`.
