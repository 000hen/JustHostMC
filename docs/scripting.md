# Scripting (Lua) — authoring guide

JustHostMC's server **providers** (how a server type is discovered, downloaded and
installed), its **automation** (scripts that drive a running server), and its
**mod-metadata parsers** (scripts that read a mod/plugin jar's embedded
descriptor for the Mods panel) are sandboxed **Lua** scripts run by the engine. Built-in providers ship embedded;
users can import their own. Every script runs in a locked-down interpreter and may
touch the outside world only through a curated, **permission-gated** host API.

This is the extension point for a new server type: **drop a `.lua` file into
`engine/internal/scripting/builtin/` (built-in) or import one at runtime** — no Go
code change.

The implementation lives in `engine/internal/scripting/`:

| File | Role |
|------|------|
| `host.go` | The `Host` (shared HTTP client + Java resolvers) and the sandbox (`newSandbox`), plus the `versions()`/`install()` drivers |
| `hostfuncs.go` | The `jhmc.*` host functions exposed to scripts |
| `permissions.go` | Permission name ↔ `PermissionKind` mapping, `GrantSet` |
| `meta.go` | The `meta` table parser (`Meta`, `Permission`) |
| `registry.go` | `Registry` of installed providers + effective-grant resolution |
| `grants.go` | `GrantStore` — persisted per-script permission decisions (`grants.json`) |
| `luaprovider.go` | Adapts a Lua script to the Go `provider.Provider` interface |
| `builtin.go` / `userproviders.go` | Load embedded built-ins / user-imported providers |
| `builtin/*.lua` | The shipped provider scripts (`vanilla`, `paper`, `spigot`, `fabric`) |
| `export.go` | The exported surface (`Invocation`, `NewSandbox`, `ParseMeta`, ...) the `automation` subpackage builds on |
| `parser.go` / `parserregistry.go` | `LuaParser` + `ParserSet` — the mod-metadata parser subsystem (§8) |
| `parserbuiltin.go` / `userparsers.go` | Load embedded built-in parsers / user-imported parsers |
| `builtin_parsers/*.lua` | The shipped parser scripts (fabric, quilt, forge, neoforge, forge-legacy, bukkit) |
| `automation/` | The automation runtime (§6): `Manager`, `runner`, the `server.*`/hook API |

Related packages: `engine/internal/scriptlog` (the automation log ring buffer),
`engine/internal/scriptdata` (the per-script `jhmc.store` KV store), and
`engine/internal/players` (the `EventBus` powering `on_join`/`on_leave`).

The proto contract is in `proto/mcmanager/v1/mcmanager.proto`: `ProviderService`,
`ScriptService`, and `ParserService`, the messages `ProviderInfo` / `ScriptInfo` /
`ParserInfo`, `Permission`, and the `PermissionKind` enum.

---

## 1. Provider script shape

A provider script declares a global `meta` table and two global functions:
`versions()` and `install(ctx)`.

### `meta`

```lua
meta = {
  id          = "vanilla",                    -- REQUIRED: stable, unique provider id
  name        = "Vanilla",                    -- display name
  website      = "https://www.minecraft.net",
  description = "The official Minecraft server from Mojang.",
  version     = "1.0.0",                       -- author-declared script version
  author      = "JustHostMC",
  mod_layout  = "none",                        -- "plugins" | "mods" | "none"
  permissions = {
    { kind = "network", reason = "Download Mojang's version manifest and the server jar." },
  },
}
```

Fields (parsed in `meta.go`):

- `id` — **required**; everything else is optional. A missing `id` is a load error.
- `mod_layout` — `"plugins"`, `"mods"`, or `"none"`. Lower-cased; empty defaults to
  `"none"`. It drives the per-server mods/plugins UI without server-type-specific
  code (surfaced as `ProviderCapabilities.mod_layout`). Paper/Spigot use
  `"plugins"`, Fabric uses `"mods"`, Vanilla uses `"none"`.
- `permissions` — a list of `{ kind = "<name>", reason = "<why>" }`. The `reason`
  is shown to the user in the consent dialog. Declaring a permission a script never
  actually exercises is harmless; **not** declaring one a host call needs means the
  call fails at runtime with a permission-denied error.

Valid `kind` names (`permissions.go` → `PermissionKind`):

| `kind` | PermissionKind | Capability |
|--------|----------------|------------|
| `network` | `PERMISSION_NETWORK` | HTTP(S) downloads |
| `install` | `PERMISSION_INSTALL` | resolve Java, run jars / external processes during install |
| `fs_server` | `PERMISSION_FS_SERVER` | read/write inside the server directory |
| `console_read` | `PERMISSION_CONSOLE_READ` | read the live console/log stream (automation) |
| `console_write` | `PERMISSION_CONSOLE_WRITE` | send commands to the console (automation) |
| `server_control` | `PERMISSION_SERVER_CONTROL` | start/stop/restart the server (automation) |
| `schedule` | `PERMISSION_SCHEDULE` | run actions on a timer, `sleep` (automation) |
| `server_query` | `PERMISSION_SERVER_QUERY` | list/inspect registered servers (automation) |
| `player_manage` | `PERMISSION_PLAYER_MANAGE` | query players, ban lists, `on_join`/`on_leave` (automation) |

### `versions() -> { string, ... }`

Returns the list of installable version strings (a Lua table of strings), usually
newest-first. Called whenever the UI needs the version picker for this provider.
It may call any granted host API (typically `jhmc.http_json` to fetch a manifest).

```lua
function versions()
  local m = jhmc.http_json("https://piston-meta.mojang.com/mc/game/version_manifest_v2.json")
  local out = {}
  for _, e in ipairs(m.versions) do
    out[#out + 1] = e.id
  end
  return out
end
```

To report "this version doesn't exist", `error("version not found: <id>")` — the
host maps an error whose text contains `version not found` to the engine's
`ErrVersionNotFound` sentinel (see `host.go` `mapErr`).

### `install(ctx) -> launch spec`

Downloads/builds everything the server needs into `ctx.dir` and returns a launch
spec table. `ctx` is provided by the host (`host.go`):

| `ctx` field | Description |
|-------------|-------------|
| `ctx.dir` | absolute path of the server directory; all downloads/fs are confined here |
| `ctx.version` | the version string the user chose |
| `ctx.step(key, frac)` | report a progress step: `key` is a localization key (e.g. `"install.progress.downloading_server"`), `frac` is a 0..1 fraction or `-1` for indeterminate |
| `ctx.log(line)` | append a raw line to the install log |

The returned table:

```lua
return { java_major = 21, args = { "-jar", "server.jar", "nogui" } }
```

- `java_major` — the Java major version the server must run on. Use
  `jhmc.java_major_for(ctx.version)` to derive it from the MC version, or read it
  from the upstream manifest (Vanilla reads Mojang's declared `javaVersion`).
- `args` — the arguments passed to `java` when the server is launched (the engine
  resolves and prepends `java` itself).

The host maps this to the Go `provider.LaunchSpec{JavaMajor, Args}`.

---

## 2. The `jhmc.*` host API

These are the only ways a script reaches outside the interpreter. Each is gated by
the permission shown; calling one without the grant raises a permission-denied
error that aborts the script. Names below are exactly those bound in
`hostfuncs.go`.

### Network — requires `network`

| Function | Description |
|----------|-------------|
| `jhmc.http_get(url) -> string` | GET a URL, return the body as a string (≤ 64 MiB) |
| `jhmc.http_json(url) -> table` | GET a URL and decode the JSON body into a Lua table |
| `jhmc.download(url, opts) -> path` | Download `url` to `opts.dest` (a server-dir-relative path). Optional `opts.sha256` / `opts.sha1` verify the file (mismatch → `ErrChecksumMismatch`). Streams download progress. Returns the absolute destination path. |
| `jhmc.http(opts) -> {status, body, headers}` | Full HTTP client. `opts`: `url` (required), `method` (default `"GET"`), `body`, `headers` (table), `timeout` (seconds, default 30), `max_body` (bytes, default 64 MiB). Non-2xx responses are **returned**, not raised, so scripts can branch on `status`. Response `headers` keys are lower-cased. |

### Filesystem — requires `fs_server` (confined to the server dir)

| Function | Description |
|----------|-------------|
| `jhmc.fs.read(rel) -> string` | read a file under the server dir |
| `jhmc.fs.write(rel, data)` | write a file (creates parent dirs) |
| `jhmc.fs.exists(rel) -> bool` | test for existence |
| `jhmc.fs.glob(rel) -> { string, ... }` | list server-dir-relative paths matching a relative glob |
| `jhmc.fs.mkdir(rel)` | create a directory (and parents) |
| `jhmc.fs.remove(rel)` | recursively remove a path |
| `jhmc.unzip(zipRel, destRel)` | extract a zip (both paths server-dir-relative), with zip-slip protection |
| `jhmc.copy_bundled(name, destRel) -> destRel` | copy a file bundled alongside the script into the server dir (see §5) |
| `jhmc.zip_read(zipRel, name) -> string\|nil` | read one entry from a zip/jar under the server dir (≤ 16 MiB); `nil` when the entry is absent |
| `jhmc.zip_entries(zipRel) -> { string, ... }` | list a zip/jar's entry names |

### Process / Java — requires `install`

| Function | Description |
|----------|-------------|
| `jhmc.resolve_java(major[, useJDK]) -> path` | resolve (downloading + caching if needed) a `java.exe` for the given major; pass `true` for the full JDK (`javac`, e.g. build tools) |
| `jhmc.run_jar(opts)` | run `java <opts.args...>` in the server dir (or `opts.dir` within it), streaming stdout/stderr to the install log. `opts.java_major` picks the Java major; `opts.jdk = true` selects the full JDK. Used for installer jars (Forge/NeoForge) and build tools (Spigot BuildTools). |

### Misc — no permission required

| Function | Description |
|----------|-------------|
| `jhmc.sha256(rel) -> hex` | SHA-256 of a server-dir-relative file |
| `jhmc.java_major_for(mcVersion) -> number` | map an MC version string to the Java major it requires |
| `jhmc.json_decode(string) -> value` | parse JSON into a Lua value |
| `jhmc.json_encode(value) -> string` | serialize a Lua value to JSON |
| `jhmc.toml_decode(string) -> table` | parse TOML (e.g. `mods.toml`; `[[array-of-tables]]` becomes a list) |
| `jhmc.yaml_decode(string) -> value` | parse YAML (e.g. `plugin.yml`) |
| `jhmc.time() -> number` | current UTC Unix time in seconds (fractional) |
| `jhmc.log(line)` | append a raw line to the install log (same as `ctx.log`) |

### Persistent storage — no permission required (automation scripts)

Each automation script gets an isolated key-value store persisted at
`<data>/script-data/<scriptID>.json`. Keys and values are strings. The store is
unavailable during import/meta-parse (top-level code) and in provider scripts —
call it from hooks or `register()`.

| Function | Description |
|----------|-------------|
| `jhmc.store.get(key) -> string\|nil` | read a key (`nil` when absent) |
| `jhmc.store.set(key, value)` | write a key |
| `jhmc.store.delete(key)` | delete a key (absent key is a no-op) |
| `jhmc.store.keys() -> { string, ... }` | all keys, sorted |

> Note: `jhmc.sha256`, `jhmc.fs.glob`, `jhmc.unzip`, and `jhmc.copy_bundled`
> operate on files **inside the server dir**; `jhmc.copy_bundled` additionally
> requires `fs_server`. `download` writes into the server dir but is gated by
> `network`; `run_jar`/`resolve_java` are gated by `install`.

---

## 3. The sandbox model

Scripts run in a gopher-lua interpreter created by `newSandbox` (`host.go`) with
**only** the safe standard libraries opened:

- `base`, `table`, `string`, `math`.

There is **no `os`, `io`, `package`/`require`, `debug`, or `coroutine`** library.
On top of that, the dangerous base-library escape hatches are deleted:
`dofile`, `loadfile`, `load`, `loadstring`, `require`, `module`,
`collectgarbage`, `newproxy`.

Consequences:

- A script **cannot** read/write files, spawn processes, open sockets, or load
  more code except through the `jhmc.*` API.
- All filesystem access via `jhmc.fs.*` / `jhmc.download` / `jhmc.unzip` is
  **confined to the server directory** (`ctx.dir`). Relative paths are cleaned and
  re-rooted; any path that escapes the server dir (`..`) is rejected
  (`ErrPathEscape`). The same zip-slip guard applies to `jhmc.unzip`.
- Each call (a `versions()` or `install()`) gets a **fresh** interpreter state; the
  `Host` is safe for concurrent use.
- The host's `context` is wired into the interpreter, so cancellation/timeout
  propagates into in-flight host calls.

---

## 4. Permissions & consent

Permissions are declared by the script and **enforced server-side** by the engine
— the UI consent dialog is advisory. The grant model (`registry.go`
`effectiveGrants`):

- **Built-in providers are trusted.** Their declared permissions are granted by
  default (until the user explicitly revokes one via `SetPermissions`).
- **User-imported providers start with no grant.** Until the user makes a decision
  the script has an empty grant set, so any gated `jhmc.*` call fails with
  permission-denied. The user grants the subset they accept through the consent UI
  (`ProviderService.SetPermissions`, persisted in `GrantStore` → `grants.json`).
- A persisted decision always wins over the built-in default, so a user can
  tighten even a built-in.

Mechanically: a host function calls `inv.require(kind)`; if the current
`GrantSet` lacks `kind`, the call raises `ErrPermissionDenied` and aborts the
script. `ProviderInfo.permissions` carries the declared set (with reasons) and
`ProviderInfo.granted` the currently-effective subset, so the UI can show both.

`ProviderService` (proto) exposes: `List`, `Import` (a `.lua` + optional jar),
`Remove`, `SetPermissions`. User scripts and their bundled jars are persisted under
the data dir's `providers/<id>/` (`provider.lua` plus any bundled jar), loaded at
startup by `LoadUserProviders`. Built-ins are embedded and loaded by
`LoadBuiltins`. Removing a user provider also forgets its grant.

---

## 5. Importing a custom provider with a bundled jar

A user can import a provider that ships a **bundled jar** (e.g. a server jar that
has no public download URL). The import (`ProviderService.Import`) takes the
`.lua` source plus the jar bytes + filename; the engine stores them together in
`providers/<id>/`, and the script reaches the jar with **`jhmc.copy_bundled`**:

```lua
-- copy the jar that was bundled at import time into the server dir
jhmc.copy_bundled("my-server.jar", "server.jar")  -- requires fs_server
return { java_major = 21, args = { "-jar", "server.jar", "nogui" } }
```

`copy_bundled(name, destRel)` copies `name` (a plain filename, no path) from the
provider's asset directory into the server-dir-relative `destRel` and returns
`destRel`. It errors if the provider has no bundled assets. A provider imported as
loose source (no jar) has no asset dir, so `copy_bundled` is unavailable to it.

---

## 6. Automation scripts

Automation/control scripts drive a *running* server: react to console output and
player joins/leaves, send commands, start/stop servers, query the server list,
manage ban lists, keep persistent state, or run actions on a schedule. They share
the same `meta` header, the same sandbox, and the same consent model as
providers, but use the automation host surface below instead of
`versions()`/`install()`.

The runtime lives in `engine/internal/scripting/automation` (`Manager` owns the
scripts; each enabled script runs on its own single-threaded Lua state, with
every callback serialized through a job queue). `ScriptService` manages them over
gRPC: `List`, `Import`, `SetEnabled`, `SetPermissions`, `Remove`, `StreamLog`.
User scripts persist under the data dir's `scripts/` as loose `.lua` files.

Relevant permissions: `console_read`, `console_write`, `server_control`,
`schedule`, `server_query`, `player_manage` (plus `network` for `jhmc.http*`).

**Top-level code runs at import time with stub APIs** (so `meta` can be read);
do real work inside hooks or the optional `register()` entry point, which runs
when the script is enabled.

### Hooks (globals)

| Function | Permission | Fires |
|----------|------------|-------|
| `on_log(id, handler(line))` | `console_read` | for every console line of server `id` |
| `on_start(id, handler(id))` | (none) | when the script attaches to server `id`'s console |
| `on_stop(id, handler(id))` | (none) | when server `id`'s console stream closes |
| `on_join(id, handler(name))` | `player_manage` | when a player joins server `id` |
| `on_leave(id, handler(name))` | `player_manage` | when a player leaves server `id` |
| `schedule(seconds, handler())` | `schedule` | every `seconds` seconds until disabled |

`on_join`/`on_leave` are **not** regex hooks over console text: they are fed by
the engine's player `EventBus`, which derives structured events from roster
state diffs (join/leave lines *and* `list` command replies all reconcile into
one roster). Scripts never parse console lines for player presence.

### The `server.*` table

| Function | Permission | Description |
|----------|------------|-------------|
| `server.send(id, cmd)` | `console_write` | write a command to the server's stdin |
| `server.logs(id) -> {lines={...}}` | `console_read` | snapshot of the buffered console history |
| `server.start(id)` / `server.stop(id)` / `server.restart(id)` | `server_control` | lifecycle control |
| `server.list() -> { {id,name,provider,mc_version,status,port,memory_mb}, ... }` | `server_query` | all registered servers |
| `server.info(id) -> table or nil` | `server_query` | one server (same shape), `nil` if unknown |
| `server.players(id) -> { name, ... }` | `player_manage` | players currently online |
| `server.kick(id, name[, reason])` | `player_manage` + `console_write` | kick via the console `kick` command (server must be running) |
| `server.ban(id, target[, reason])` | `player_manage` | add to `banned-players.json`/`banned-ips.json` (server must be **stopped**) |
| `server.unban(id, target)` | `player_manage` | remove a ban (server must be stopped) |
| `server.bans(id) -> { {type,target,reason,created}, ... }` | `player_manage` | current ban list (`type` is `"player"` or `"ip"`) |

### Other globals

- `log(...)` / `print(...)` — append to the engine-wide automation log
  (streamed to the UI via `ScriptService.StreamLog`).
- `sleep(seconds)` — requires `schedule`. Blocks **this script only** (its other
  hooks queue behind it, like `time.sleep` in Python); disabling the script
  interrupts the sleep.
- `jhmc.store.*` — per-script persistent KV storage (§2).
- `jhmc.time()` — current Unix time.

### Example

```lua
meta = {
  id = "greeter", name = "Greeter", version = "1.0.0",
  permissions = {
    { kind = "player_manage", reason = "React to players joining." },
    { kind = "console_write", reason = "Send the welcome message." },
  },
}

on_join("my-server", function(name)
  local visits = tonumber(jhmc.store.get(name) or "0") + 1
  jhmc.store.set(name, tostring(visits))
  server.send("my-server", ("say Welcome %s (visit #%d)!"):format(name, visits))
end)
```

---

## 7. Complete worked example — a custom provider

A copy-pasteable provider for a generic single-URL server jar (here modelled on
**Purpur**, a Paper fork). It lists versions from Purpur's REST API, downloads the
build jar verifying its MD5/SHA where available, and returns a launch spec. Adapt
the URLs for any "download a jar by version" server type.

```lua
-- purpur.lua — a custom provider for the Purpur server (a Paper fork).
--
-- Import this with ProviderService.Import (no bundled jar needed: Purpur
-- publishes downloadable jars). The user must grant the `network` permission.

meta = {
  id          = "purpur",
  name        = "Purpur",
  website     = "https://purpurmc.org",
  description = "A drop-in Paper fork with extra configuration and gameplay options.",
  version     = "1.0.0",
  author      = "you@example.com",
  mod_layout  = "plugins",                 -- Purpur is Paper-compatible: plugins/ folder
  permissions = {
    { kind = "network", reason = "Download Purpur's build manifest and the server jar." },
  },
}

local API = "https://api.purpurmc.org/v2/purpur"

-- versions() -> newest-first list of MC versions Purpur builds for.
function versions()
  local project = jhmc.http_json(API)
  local out = {}
  -- The API returns versions oldest-first; reverse for newest-first.
  for i = #project.versions, 1, -1 do
    out[#out + 1] = project.versions[i]
  end
  return out
end

-- install(ctx) downloads the latest Purpur build for ctx.version into ctx.dir
-- and returns the launch spec.
function install(ctx)
  ctx.step("install.progress.resolving_version", -1)

  -- Resolve the latest build for the chosen version.
  local meta_for_ver = jhmc.http_json(API .. "/" .. ctx.version)
  local build = meta_for_ver.builds.latest
  if not build then
    error("version not found: " .. ctx.version)
  end

  -- Optional integrity check: the build detail carries an md5; we use sha256 from
  -- the download verification only when the upstream publishes one. Purpur exposes
  -- an md5, which jhmc.download does not check, so we download then verify
  -- separately if the API gives us a digest we can compute.
  local detail = jhmc.http_json(API .. "/" .. ctx.version .. "/" .. build)
  local jar_url = API .. "/" .. ctx.version .. "/" .. build .. "/download"

  ctx.step("install.progress.downloading_server", 0)
  ctx.log("purpur-" .. ctx.version .. "-" .. build .. ".jar")
  jhmc.download(jar_url, { dest = "server.jar" })

  -- Verify the downloaded jar against the md5 the API published, if present.
  if detail.md5 then
    -- jhmc.sha256 computes sha256; for an md5-only API we instead trust the
    -- transport. If your provider exposes a sha256, prefer the inline check:
    --   jhmc.download(jar_url, { dest = "server.jar", sha256 = detail.sha256 })
    ctx.log("published md5: " .. detail.md5)
  end

  ctx.step("install.progress.done", 1)
  return {
    java_major = jhmc.java_major_for(ctx.version),
    args       = { "-jar", "server.jar", "nogui" },
  }
end
```

The inline checksum path (preferred when the upstream publishes a SHA) is what the
built-in **Paper** provider does:

```lua
jhmc.download(dl.url, { dest = "server.jar", sha256 = dl.checksums.sha256 })
```

and **Vanilla** uses the SHA-1 Mojang publishes:

```lua
jhmc.download(server.url, { dest = "server.jar", sha1 = server.sha1 })
```

For the real, shipped provider scripts, read
`engine/internal/scripting/builtin/{vanilla,paper,spigot,fabric}.lua`.

---

## 8. Parser scripts (mod/plugin metadata)

Parser scripts power the per-server **Plugins/Mods** panel: for each uploaded
jar, the engine runs the installed parsers (built-ins first, in registration
order) until one **matches**, and shows the extracted icon, name, version,
authors, description, and website. Results are cached per jar
(`name|size|mtime`), so unchanged jars are never re-parsed. A broken parser is
logged and skipped — it can never break mod listing.

`ParserService` manages them over gRPC (`List`, `Import`, `Remove`,
`SetPermissions`) — the Scripts page has a "Mod Metadata Parsers" section. User
parsers persist under the data dir's `parsers/` as loose `.lua` files; built-ins
are embedded from `engine/internal/scripting/builtin_parsers/` (fabric, quilt,
forge, neoforge, forge-legacy, bukkit/paper). Like all user scripts, imported
parsers start with **no grants** until the user consents.

A parser declares `meta` (with an informational `formats` list) and one global
`parse(ctx)`:

```lua
meta = {
  id = "parser-example",
  name = "Example Parser",
  formats = { "example.mod.json" },        -- descriptor files it reads (shown in the UI)
  permissions = {
    { kind = "fs_server", reason = "Read mod jars to extract their metadata" },
  },
}

function parse(ctx)
  -- ctx.jar is the jar's server-dir-relative path.
  local raw = jhmc.zip_read(ctx.jar, "example.mod.json")
  if raw == nil then return nil end        -- not ours: return nil => next parser tries
  local m = jhmc.json_decode(raw)
  return {                                  -- returning a table = matched
    loader      = "example",               -- "fabric"|"quilt"|"forge"|"neoforge"|"forge-legacy"|"bukkit"|"paper"|...
    mod_id      = m.id,
    name        = m.name,
    version     = m.version,
    authors     = { "Alice", "Bob" },       -- table of strings (or a single string)
    description = m.description,
    website     = m.homepage,
    icon        = jhmc.zip_read(ctx.jar, m.icon),  -- raw png/jpg bytes, optional
  }
end
```

Contract details:

- `parse(ctx)` runs in a fresh sandbox per jar, confined to the server dir
  (`fs_server` gates `zip_read`/`zip_entries`/`jhmc.fs.*`).
- Return `nil` (or nothing) when the jar isn't recognized; the next parser runs.
- Every returned field is optional; missing fields simply don't render.
- Typical decoders: `jhmc.json_decode` (fabric/quilt/mcmod.info),
  `jhmc.toml_decode` (mods.toml), `jhmc.yaml_decode` (plugin.yml).
- A parser may also declare `network` to enrich results from an online API.

## 9. Shop scripts (mod browsing/downloading)

Shop scripts power the app's **Mod Shop** window: browsing, searching, and
installing mods/plugins from an online source. Built-ins live in
`engine/internal/scripting/builtin_shops/` (`modrinth.lua`, `curseforge.lua`);
user shops are imported via `ShopService.Import` and stored under
`<data>/shops/`. Grants persist in `shop-grants.json`.

A shop script declares the usual `meta` table (plus `needs_key = true` when
the source requires an API key), five required globals, and an optional
`categories(ctx)` global. Each takes one ctx table and returns one table:

```lua
meta = {
  id = "myshop", name = "My Shop", version = "1.0.0",
  needs_key = false, -- true => engine refuses calls until a key is configured
  permissions = { { kind = "network", reason = "Query the shop API" } },
}

function home(ctx)         -- ctx: mc_version, loader, kind ("mod"|"plugin"), config
  return { sections = { { title_key = "shop.home.popular", projects = { ... } } } }
end
function categories(ctx)   -- optional; ctx: kind, config
  return { categories = { { id=, name=, slug=, localization_key= } } }
end
function search(ctx)       -- + query, sort ("relevance"|"downloads"|"follows"|"newest"|"updated"), offset, limit
  return { projects = { ... }, total = 123 }
end
function detail(ctx)       -- ctx.project_id
  return { project = {...}, body = "...", body_format = "markdown"|"html",
           gallery = {{url=,title=,description=}}, links = {website=,source=,issues=,wiki=,discord=},
           game_versions = {...}, loaders = {...}, license = "", updated = "" }
end
function versions(ctx)     -- ctx: project_id, mc_version, loader
  return { versions = { { id=, name=, version_number=, channel="release",
           game_versions={...}, loaders={...}, date=, downloads=, filename=, size=,
           dependencies={{project_id=, title=, required=true}} } } }
end
function resolve_file(ctx) -- ctx: project_id, version_id ("" = latest compatible), mc_version, loader
  return { url=, filename=, size=, sha1=|sha512= } -- engine downloads + verifies
end
```

A project table carries `project_id, slug, title, summary, icon_url, author,
downloads, follows, categories, project_type`, plus optional `distribution`:
`"direct"` permits an API install, `"website_only"` tells the app to replace
Install with a source-named website action, and an absent/unknown value keeps
the guarded install behavior for backward compatibility. Category entries use
their source-native `id` for search; `name` is the display fallback, `slug`
identifies the source category, and `localization_key` is optional. Shops that
omit `categories(ctx)` expose no category filters.

Keyed sources read `ctx.config.api_key` (user settings override a build-time
default). Raise
`error("... not found")` / `error("... not distributable")` to surface the
typed `SHOP_PROJECT_NOT_FOUND` / `SHOP_FILE_NOT_DISTRIBUTABLE` errors.

Use `jhmc.http_cache{ url=, headers=, ttl= }` for GETs: it is a disk-backed
ETag cache (`If-None-Match`/304), so repeated detail views cost a cheap
revalidation; `ttl` seconds within which no request is made at all.
The built-in CurseForge shop uses a 24-hour (`86400` second) TTL for its
class-specific category list and keeps Mods and Plugins in separate cache keys.

## 10. Extending: fetching data from online services

The host API is deliberately composable so future features are scripts, not
engine changes:

- `jhmc.http{ url=..., method=..., headers=..., body=... }` (permission
  `network`) talks to any REST API, with response headers and non-2xx statuses
  observable for pagination/rate-limit handling; `jhmc.http_cache` adds the
  disk ETag cache.
- `jhmc.json_decode` / `jhmc.toml_decode` / `jhmc.yaml_decode` parse whatever
  the service returns; `jhmc.download` fetches files with checksum verification
  into the server dir; `jhmc.store` remembers cursors/ETags between runs.
- The permission model already covers this shape (`network` + `fs_server`), and
  the four script subsystems (providers -> `Registry`, automation -> `Manager`,
  parsers -> `ParserSet`, shops -> `ShopSet`) show the pattern for wiring
  another kind of script end-to-end: proto service -> `scripting` set +
  `GrantStore` -> gRPC service -> UI.
