# Mod/Plugin Metadata Parser (importable Lua parsers)

## Context

The per-server **Plugins/Mods** panel currently lists jar files as bare
`name + size` (`ModService.List` in `engine/internal/grpc/modservice.go` →
`ModsViewModel`/`ModFileItem` → `ServerPage.xaml`). Users can't tell what a jar
actually *is* without opening it.

We want a **mod/plugin parser** that reads each jar's embedded metadata and shows
**icon, name, author(s), version, description, website** in the UI. Mods and
plugins describe themselves in three different serialization formats — **JSON**
(`fabric.mod.json`, `quilt.mod.json`, `mcmod.info`), **TOML**
(`META-INF/mods.toml`, `META-INF/neoforge.mods.toml`), and **YAML**
(`plugin.yml`, `paper-plugin.yml`) — but the extracted result must be uniform.

Per the requirement and the cited references (`jamiemansfield/go-modmeta`,
`Rearth/Modpack-Inspector`): the **format-specific extraction logic lives in Lua,
not Go**. Go only exposes low-level *parse primitives* (TOML/YAML decode, zip
entry reads) as dynamic host APIs, reusing existing Go packages. Parsers are
**user-importable** Lua scripts (like providers/automation scripts), sandboxed and
permission-gated, with a permission-consent UI. Built-ins cover the comprehensive
format set. Parsed metadata (icons included) is delivered **inline in
`ModService.List`, cached** per jar.

This mirrors the two existing script subsystems almost exactly:
- Providers: `scripting.Registry` + `ProviderService` (`providerservice.go`),
  persisted under `<data>/providers/<id>/`, grants in `grants.json`.
- Automation: `scripting.Manager` + `ScriptService` (`scriptservice.go`),
  persisted under `<data>/scripts/`, grants in `script-grants.json`.

Parsers become a third parallel subsystem: `scripting.ParserSet` +
`ParserService`, persisted under `<data>/parsers/`, grants in
`parser-grants.json`.

## Approach

### 1. Proto contract — `proto/mcmanager/v1/mcmanager.proto`
(single source of truth; regen both sides after editing)

- Add `ModMetadata` and reference it from `ModFile`:
  ```proto
  message ModMetadata {
    bool parsed = 1;              // a parser matched this jar
    string parser_id = 2;         // which parser produced it
    string loader = 3;            // "fabric"|"quilt"|"forge"|"neoforge"|"forge-legacy"|"bukkit"|"paper"
    string mod_id = 4;
    string name = 5;
    string version = 6;
    repeated string authors = 7;
    string description = 8;
    string website = 9;
    bytes icon = 10;              // raw image bytes (png/jpg), optional
  }
  message ModFile { string name = 1; int64 size_bytes = 2; ModMetadata metadata = 3; }
  ```
- Add the parser management surface (copy the `ProviderService` shape; reuse
  `ProviderRef` + `SetPermissionsRequest`):
  ```proto
  message ParserInfo { /* id,name,website,description,version,author,builtin,
                          repeated Permission permissions, repeated PermissionKind granted,
                          repeated string formats */ }
  message ParserList { repeated ParserInfo parsers = 1; }
  message ImportParserRequest { string lua_source = 1; }
  service ParserService {
    rpc List(Empty) returns (ParserList);
    rpc Import(ImportParserRequest) returns (ParserInfo);
    rpc Remove(ProviderRef) returns (Empty);
    rpc SetPermissions(SetPermissionsRequest) returns (ParserInfo);
  }
  ```
- Regen: `cd proto; buf generate` (Go stubs → `engine/gen/...`; C# regens on build).

### 2. Go dependencies — `engine/go.mod`
- `go get github.com/BurntSushi/toml` (TOML; the lib `go-modmeta` itself uses)
- `go get gopkg.in/yaml.v3` (YAML; v3 decodes maps with **string** keys)
- `go mod tidy`. NB the bundled-binary build uses `-mod=readonly`, so tidy first.

### 3. New Lua host parse primitives — `engine/internal/scripting/hostfuncs.go`
Register in `newJHMC` alongside the existing `json_decode`:
- `jhmc.toml_decode(s) -> table` — `toml.Decode` into `map[string]any` → `goToLua`.
- `jhmc.yaml_decode(s) -> table` — `yaml.Unmarshal` into `any` → `goToLua`.
- `jhmc.zip_read(zipRel, name) -> string|nil` — read one entry from a jar (bytes
  as a Lua string; `nil` if absent; size-capped, e.g. 16 MiB). Gated by
  `fs_server`; reuse `resolvePath` for the zip path.
- `jhmc.zip_entries(zipRel) -> {names}` — list entry names. Gated by `fs_server`.

Reuse `archive/zip` (already imported), `resolvePath` (path-escape guard),
`inv.require(PERMISSION_FS_SERVER)`, `goToLua`.

**Generalize `goToLua`** — `engine/internal/scripting/convert.go`
Current `goToLua` only handles `float64`, `string`, `bool`, `[]any`,
`map[string]any`. TOML/YAML decoders emit `int64`/`int`, and TOML emits concrete
`[]map[string]any` for array-of-tables. Rewrite `goToLua` **reflection-based**
(handle `reflect.Slice/Array` → list table, `reflect.Map` → keyed table, all int/
uint/float kinds → `LNumber`, bool/string, `time.Time`→string, nil→`LNil`) so any
decoder output converts. Keep behavior identical for the JSON path (covered by
existing tests).

### 4. Parser subsystem — new files in `engine/internal/scripting/`
Model on `luaprovider.go` + `registry.go` (do **not** reuse the provider
`Registry`; add a focused parallel set, consistent with Manager precedent):

- `ModMeta` struct: `Loader, ModID, Name, Version string; Authors []string;
  Description, Website string; Icon []byte`.
- `parser.go` — `LuaParser` (analog of `LuaProvider`): `meta, source, host,
  builtin, grantsFn`. `Parse(ctx, serverDir, jarRel) (ModMeta, bool, error)`
  drives a new `invocation.parseMod(src, dir, jarRel)` (analog of `install`):
  builds `ctx.jar = jarRel`, calls global `parse(ctx)` under `PCall`. Script
  returns `nil` → `(_, false, nil)` (no match); returns a table → read fields
  (`icon` is a Lua string → `[]byte`).
- `parserregistry.go` — `ParserSet` (analog of `Registry`): `AddSource`,
  `AddParserFile`, `Get`, `Remove`, `List`, `EffectiveGrants` (identical
  built-in-trusted grant resolution via `effectiveGrants`). Plus
  `ParseJar(ctx, serverDir, jarRel) (ModMeta, string /*parserID*/, bool)`:
  iterate parsers in registration order, first `Matched` wins; **swallow a single
  parser's error** (log, continue) so one broken user parser can't break listing.
- `parserbuiltin.go` — `//go:embed builtin_parsers/*.lua` + `LoadBuiltinParsers`
  (analog of `builtin.go`; separate embed dir so the provider loader doesn't pick
  these up). `userparsers.go` — `LoadUserParsers(set, dir)` loading
  `parsers/<id>.lua` (loose files, like automation scripts — parsers need no
  bundled assets).
- `meta.go` — add optional `Formats []string` to `Meta` (parsed from a
  `formats` string-table when present; harmless for providers/scripts).

**Built-in parser scripts** — `engine/internal/scripting/builtin_parsers/`
(each returns `nil` when its signature file is absent → next parser tries):
- `fabric.lua` — `fabric.mod.json` (JSON): `id`, `name`, `version`,
  `authors[]` (string or `{name=}`), `description`, `contact.homepage`,
  `icon` (string or size→path map) → read icon bytes via `zip_read`.
- `quilt.lua` — `quilt.mod.json` (JSON): `quilt_loader.{id,version,metadata.*}`.
- `forge.lua` — `META-INF/mods.toml` (TOML): top-level + `[[mods]]` →
  `modId,displayName,version,description,authors,displayURL,logoFile`.
- `neoforge.lua` — `META-INF/neoforge.mods.toml` (same shape as forge).
- `forge_legacy.lua` — `mcmod.info` (JSON array/`modList`):
  `modid,name,version,description,authorList,url,logoFile`.
- `bukkit.lua` — `plugin.yml` **or** `paper-plugin.yml` (YAML):
  `name,version,author|authors,description,website`; loader `bukkit`/`paper`.

### 5. Enrich `ModService.List` + cache — `engine/internal/grpc/modservice.go`
- `NewModService(store, paths, parser)` gains a `parser` param, an interface:
  `ParseJar(ctx, serverDir, jarRel string) (scripting.ModMeta, string, bool)`
  (satisfied by `*scripting.ParserSet`).
- In `List`, for each jar compute `ModFile.Metadata` (map `scripting.ModMeta` →
  proto `ModMetadata`, incl. icon bytes). **Cache** keyed by
  `serverID|name|size|mtimeUnixNano` (mutex-guarded map on the service) so
  repeated tab visits don't re-parse unchanged jars; re-uploads change mtime →
  re-parse. No parser match → `Metadata{Parsed:false}` (UI shows filename only).
  `Upload`/`Remove` unchanged (size/mtime key self-invalidates).

### 6. `ParserService` + wiring
- `engine/internal/grpc/parserservice.go` — near-verbatim copy of
  `providerservice.go` (List/Import/Remove/SetPermissions, clamp granted to
  declared, block removing built-ins, `Forget` grant on remove), over
  `*scripting.ParserSet` + a parser `GrantStore`, persisting `parsers/<id>.lua`.
  `info()` also fills `ParserInfo.formats` from `meta.Formats`.
- `engine/internal/grpc/server.go` — add `ParserService` to `Config` +
  `RegisterParserServiceServer` guard (mirror the `ProviderService` block).
- `engine/cmd/engine/main.go` — near the provider wiring (lines ~67–74):
  ```go
  parserGrants := scripting.NewGrantStore(filepath.Join(paths.Base, "parser-grants.json"))
  parsers := scripting.NewParserSet(host, parserGrants)
  scripting.LoadBuiltinParsers(parsers)                  // log.Fatalf on error
  parsersDir := filepath.Join(paths.Base, "parsers")
  scripting.LoadUserParsers(parsers, parsersDir)         // log.Printf on error
  ```
  Change `ModService: grpcsvc.NewModService(registry, paths)` →
  `NewModService(registry, paths, parsers)` and add
  `ParserService: grpcsvc.NewParserService(parsers, parserGrants, parsersDir)`.

### 7. C# app
- `app/JustHostMC.Core/DaemonClient.cs` — add
  `ParserService.ParserServiceClient Parsers`.
- **Display** (the core ask):
  - `Models/ModFileItem.cs` — add `ModId, DisplayName, Version, Description,
    Website, Loader, Authors` (joined string), `HasMetadata/HasIcon/HasWebsite/
    HasDescription`, `Title` (`DisplayName` ?? filename), and
    `ImageSource? Icon`.
  - `ViewModels/ModsViewModel.cs` — in `RefreshCoreAsync`, map
    `file.Metadata`; decode `metadata.Icon` (ByteString) →
    `BitmapImage` via `InMemoryRandomAccessStream` on the UI thread; pass into
    `ModFileItem`.
  - `Views/ServerPage.xaml` — expand the mods `DataTemplate` (~line 778): icon
    `Image` with a `FontIcon` fallback (respect the WinUI DataTemplate binding
    rules — keep `{Binding}`, not `ElementName`), `Title`, version, authors,
    wrapping/truncated description, website `HyperlinkButton`. Keep the remove
    flyout.
- **Parser management UI** (makes "importable" real end-to-end) — model on
  `Views/ScriptsPage.xaml(.cs)` + `ViewModels/ScriptsViewModel.cs`, reusing
  `Views/PermissionConsentDialog.xaml`: a `ParsersPage`/`ParsersViewModel`
  (List/Import-from-file/Remove/SetPermissions). Register it in
  `ViewModels/NavShellViewModel.cs` next to Scripts.
- **i18n** — add keys to both `Strings/en-US/Resources.resw` and
  `Strings/zh-Hant/Resources.resw` (en-US is base/fallback): `Mods_By`,
  `Mods_NoMetadata`, `Mods_Website`, `Mods_Version`, and Parsers page/consent
  strings.

### 8. Docs
- `docs/scripting.md` — document `jhmc.toml_decode`/`yaml_decode`/`zip_read`/
  `zip_entries` and a new "Parser scripts" section (`meta.formats`, `parse(ctx)`
  contract, `ctx.jar`, return `nil` for no-match, unified return shape).
- `CLAUDE.md` — note the new `ParserService` (12 services) + parser scripts +
  the mod-metadata enrichment.

## Testing / Verification

**Go (TDD the core):**
- `engine/internal/scripting/hostfuncs_test.go` (or new) — `toml_decode`,
  `yaml_decode`, `zip_read`/`zip_entries` against fixtures; assert int/array-of-
  tables convert correctly (the `goToLua` generalization).
- `engine/internal/scripting/parser_test.go` — build tiny in-memory jars
  (`archive/zip` into a temp server dir) for **each** format
  (fabric/quilt/mods.toml/neoforge/mcmod.info/plugin.yml/paper-plugin.yml) with an
  icon entry; assert unified `ModMeta` + icon bytes; assert non-match returns
  `false`; assert a broken parser is skipped.
- `engine/internal/grpc/modservice_test.go` — extend: `List` populates
  `Metadata` for a known jar; cache hit avoids re-parse; unsupported layout and
  parse-miss degrade to `Parsed:false`.
- `engine/internal/grpc/parserservice_test.go` — mirror
  `providerservice`/`scriptservice` tests (import/list/remove/permissions,
  built-in-not-removable, grant clamping).
- Run: `cd engine; go build ./... && go test ./...`.

**C# / build pipeline:**
- `.\build.ps1` (buf generate → go build+test → dotnet build+test).
- `dotnet build app/JustHostMC.App/JustHostMC.App.csproj -p:Platform=x64`.
- Manual smoke (WinUI XAML errors are runtime, not build): F5, open a
  Paper/Fabric server → Plugins/Mods tab, upload a real jar, confirm icon + name +
  author + version + description + website render; smoke the new Parsers page
  (import a `.lua`, grant permissions, remove). Verify a jar with no recognizable
  metadata still lists by filename.

## Notes / decisions
- Parser detection = "first registered parser whose signature file is present
  wins"; built-ins registered before user parsers.
- Parsers declare `fs_server` (read the jar via `zip_read`); a network-enriching
  parser would also declare `network`. Built-ins are trusted (granted by default);
  user parsers start ungranted until consent — identical to providers.
- Icons travel as raw bytes inline in `List` (capped); C# `BitmapImage`
  auto-detects PNG/JPG. Fine for realistic folder sizes given the cache.
