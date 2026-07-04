# Expand Automation Scripting APIs — v2

The current automation API is functional but minimal — scripts can only react to console lines, send commands, and start/stop servers. This plan adds the high-impact APIs that make automation scripts actually useful for real-world workflows: full HTTP client, player management, server queries, persistent key-value storage, and a clean architecture refactor.

---

## Architecture Analysis: Should This Be Independent?

The current `scripting` package (22 files, ~1300 LOC) carries **three distinct responsibilities** in a single Go package:

```
scripting/
├── Provider runtime ─── host.go, hostfuncs.go, luaprovider.go, registry.go, builtin.go, userproviders.go
├── Automation runtime ─ automation.go (Manager, runner, autoAPI)
├── Shared infra ─────── permissions.go, grants.go, enabledstore.go, meta.go, convert.go,
│                         errors.go, host.go (sandbox, invocation), logbuffer.go
└── builtin/ ─────────── vanilla.lua, paper.lua, ...
```

**Problem**: Providers and automation are architecturally separate systems that happen to share the sandbox and host functions. As we add server queries, player management, KV storage, and an event bus, the automation surface grows substantially — but providers need none of it. The `Manager` already depends on `Console`, `ServerControl`, and `LogBuffer`; adding `ServerQuery`, `PlayerManager`, `KVStore`, and `EventBus` would make the package unfocused and the dependency graph messy.

### Proposed package split

```
engine/internal/
├── scripting/               ← CORE: sandbox, host functions, permissions, meta, Lua↔Go, grants
│   ├── automation/          ← NEW subpackage: Manager, runner, autoAPI, event wiring
│   └── builtin/             ← unchanged: embedded provider .lua files
├── scriptlog/               ← NEW: LogBuffer + LogLine (extracted from scripting)
├── scriptdata/              ← NEW: per-script KV store
└── players/                 ← EXTENDED: add EventBus for structured join/leave events
```

| Package | Responsibility | Depends on |
|---------|---------------|------------|
| `scripting` | Sandbox, `jhmc.*` host functions, permissions, meta, registry, providers | `provider`, proto |
| `scripting/automation` | `Manager`, `runner`, `autoAPI`, event hook dispatch | `scripting` (parent), `scriptlog`, `scriptdata`, `players`, proto |
| `scriptlog` | `LogBuffer`, `LogLine`, ring buffer + fan-out | (none) |
| `scriptdata` | Per-script JSON-backed KV store | (none) |
| `players` | `Roster`, **`EventBus`** (structured join/leave events) | (none) |

**Why this split works:**
- `scripting` stays focused: sandbox + host API + providers. No automation imports.
- `automation` can import its parent (`scripting`) for `Host`, `invocation`, `GrantSet`, etc., but `scripting` never imports `automation` — clean one-way dependency.
- `scriptlog` and `scriptdata` are standalone utilities — zero coupling.
- The `players` package already owns roster parsing; adding an `EventBus` there is natural.

> [!IMPORTANT]
> **This is a code-organization refactor, not a rewrite.** The actual logic in `automation.go` moves mostly unchanged into `scripting/automation/`. Imports and constructor signatures adjust, but behavior is identical. The split can be done as a single atomic commit.

---

## Resolved Open Questions

**HTTP response size limit**: Configurable per-request via `opts.max_body` (default 64 MiB, matches current behavior). Scripts that need more (e.g., downloading a large manifest) can raise it.

**KV storage scope**: Strictly isolated. Each script gets its own store file (`script-data/<scriptID>.json`). No cross-script access.

**`on_join`/`on_leave` robustness**: These will NOT parse console lines with regex inside the automation hook. Instead, they are powered by a **new `players.EventBus`** — an event-sourced system built on top of the existing `Roster`. The `EventBus` wraps a `Roster` and emits structured `PlayerEvent{Type, ServerID, Name}` whenever the roster's state changes (join detected, leave detected). The console hub feeds lines into the EventBus (just like it already feeds the line observer for log persistence), and automation subscribes to the EventBus — not to raw console output. This means:
- Parsing logic lives in exactly one place (`Roster.Apply()`)
- Events are derived from **state changes**, not pattern matching
- If `Roster`'s regex improves, all subscribers benefit automatically
- The automation system never touches raw console text for player events

---

## Proposed Changes

### Overview of New Lua APIs

| API | Permission | Description |
|-----|-----------|-------------|
| `jhmc.http(opts)` | `network` | Full HTTP client (GET/POST/PUT/DELETE, headers, body, timeout, configurable size limit) |
| `server.list()` | `server_query` | List all registered servers |
| `server.info(id)` | `server_query` | Get server details (name, version, port, status, provider) |
| `server.players(id)` | `player_manage` | List online players for a server |
| `server.kick(id, name, reason?)` | `player_manage` + `console_write` | Kick a player (sends console command) |
| `server.ban(id, target, reason?)` | `player_manage` | Ban a player (writes `banned-players.json`) |
| `server.unban(id, target)` | `player_manage` | Unban a player |
| `server.bans(id)` | `player_manage` | List banned players/IPs |
| `on_join(id, handler)` | `player_manage` | Fire when a player joins (via EventBus, not regex) |
| `on_leave(id, handler)` | `player_manage` | Fire when a player leaves (via EventBus, not regex) |
| `jhmc.store.get(key)` | *(none)* | Read from per-script persistent KV store |
| `jhmc.store.set(key, value)` | *(none)* | Write to per-script persistent KV store |
| `jhmc.store.delete(key)` | *(none)* | Delete a key |
| `jhmc.store.keys()` | *(none)* | List all keys |
| `jhmc.time()` | *(none)* | Current UTC Unix timestamp (seconds, float) |
| `sleep(seconds)` | `schedule` | Cooperative sleep (blocks Lua, not the job pump) |

---

### Component 1: Proto — New Permission Kinds

#### [MODIFY] [mcmanager.proto](file:///d:/devs/VisualStudio/JustHostMC/proto/mcmanager/v1/mcmanager.proto)

Add two new `PermissionKind` values after `PERMISSION_SCHEDULE = 7`:
```diff
   PERMISSION_SCHEDULE = 7;       // run actions on a timer
+  PERMISSION_SERVER_QUERY = 8;   // list/query server metadata from scripts
+  PERMISSION_PLAYER_MANAGE = 9;  // query/kick/ban players from scripts
 }
```

Regenerate Go and C# stubs afterward.

---

### Component 2: Robust Player Event System

#### [MODIFY] [tracker.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/players/tracker.go)

Keep `Roster` unchanged. It already returns `bool` from `Apply()` indicating state change, and has `Names()`.

#### [NEW] `engine/internal/players/eventbus.go`

A structured event system built on top of Roster:

```go
package players

// EventType distinguishes join from leave.
type EventType int

const (
    EventJoin  EventType = iota
    EventLeave
)

// PlayerEvent is a structured event emitted when a player joins or leaves.
type PlayerEvent struct {
    Type     EventType
    ServerID string
    Name     string
}

// EventBus wraps per-server Rosters and emits structured PlayerEvents.
// The console hub calls Feed() for each console line; subscribers receive
// events derived from Roster state diffs — never from raw regex matching.
type EventBus struct {
    mu      sync.Mutex
    rosters map[string]*Roster           // server id → roster
    subs    map[chan PlayerEvent]struct{} // live subscribers
}

func NewEventBus() *EventBus

// Feed parses a console line for the given server, updates the roster, and
// emits join/leave events if the set of online players changed.
// It compares Names() before and after Apply() to determine exactly who
// joined or left.
func (eb *EventBus) Feed(serverID, line string)

// Subscribe returns a live channel of player events. Call cancel to
// unsubscribe.
func (eb *EventBus) Subscribe() (live <-chan PlayerEvent, cancel func())

// OnlinePlayers returns the current roster for a server.
func (eb *EventBus) OnlinePlayers(serverID string) []string

// Reset clears the roster for a server (called when server stops).
func (eb *EventBus) Reset(serverID string)
```

The `Feed()` method works as follows:
1. Get (or create) the `Roster` for the server
2. Snapshot `roster.Names()` **before**
3. Call `roster.Apply(line)`
4. If `Apply` returns `true` (state changed), snapshot `Names()` **after**
5. Diff the two snapshots: new names → `EventJoin`, missing names → `EventLeave`
6. Fan out the events to all subscribers

This gives us **state-diff-based events** — maximally robust. No duplicate regex parsing.

#### [MODIFY] [hub.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/console/hub.go)

Wire the EventBus as a second line observer. The Hub already has `SetLineObserver(fn func(id, line string))` for log persistence. We extend this pattern:

```diff
 type Hub struct {
     mu       sync.Mutex
     consoles map[string]*serverConsole
     ringSize int
     observer func(id, line string)
+    observers []func(id, line string) // additional observers (EventBus, etc.)
 }

+// AddLineObserver registers an additional callback invoked for every console
+// line (alongside the primary observer). It must not block.
+func (h *Hub) AddLineObserver(fn func(id, line string)) {
+    h.mu.Lock()
+    h.observers = append(h.observers, fn)
+    h.mu.Unlock()
+}
```

In the `Register()` goroutine, call all observers for each line.

---

### Component 3: Package Refactor — Extract LogBuffer

#### [NEW] `engine/internal/scriptlog/logbuffer.go`

Move `LogBuffer`, `LogLine`, `NewLogBuffer`, `Append`, `Subscribe` verbatim from [logbuffer.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/scripting/logbuffer.go) into the new `scriptlog` package. Change only the `package` declaration.

#### [DELETE] [logbuffer.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/scripting/logbuffer.go)

Remove after extraction.

#### Update imports in:
- [automation.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/scripting/automation.go): `*LogBuffer` → `*scriptlog.LogBuffer`, `LogLine` → `scriptlog.LogLine`
- [scriptservice.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/grpc/scriptservice.go): `scripting.LogLine` → `scriptlog.LogLine`
- [main.go](file:///d:/devs/VisualStudio/JustHostMC/engine/cmd/engine/main.go): `scripting.NewLogBuffer` → `scriptlog.NewLogBuffer`
- [automation_test.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/scripting/automation_test.go): same

---

### Component 4: Package Refactor — Extract Automation into Subpackage

#### [NEW] `engine/internal/scripting/automation/` (directory)

Move the following from `scripting/automation.go` into `scripting/automation/`:

| New file | Contents moved from `automation.go` |
|----------|-------------------------------------|
| `manager.go` | `Manager`, `NewManager`, `AddSource`, `Get`, `List`, `Enable`, `Disable`, `Shutdown`, `Remove`, `effectiveGrants`, `EffectiveGrants`, `Logs`, `start`, `wireEvents`, `watchServer`, `logErr` |
| `runner.go` | `runner` struct, `stop()`, `fire()` |
| `api.go` | `autoAPI` struct, `serverTable()`, `installGlobals()`, all API method implementations (`send`, `logs`, `start`, `stop`, `restart`, `onLog`, `onStart`, `onStop`, `schedule`, `log`, `joinSpace`) |
| `interfaces.go` | `Console`, `ServerControl` interfaces, plus the new `ServerQuery`, `PlayerManager` interfaces |
| `automation.go` | `Automation` struct, `newAutomation()`, `installAutoStubs()` |

The `scripting` package keeps: `Host`, `invocation`, `newSandbox`, `newJHMC`, all `hostfuncs.go` functions, `permissions.go`, `meta.go`, `convert.go`, `errors.go`, `grants.go`, `enabledstore.go`, `registry.go`, `luaprovider.go`, `builtin.go`, `userproviders.go`.

**Key exports needed**: The `automation` subpackage needs to access `scripting.Host`, `scripting.GrantSet`, `scripting.Grants`, `scripting.newSandbox`, `scripting.invocation`, and `scripting.newJHMC`. Since `automation` is a subpackage of `scripting`, we need to export these (capitalize) or use a different approach:

> [!IMPORTANT]
> **Option A**: Export `NewSandbox`, `Invocation`, `NewJHMC` in the `scripting` package so the `automation` subpackage can use them. These are internal engine types, not user-facing.
>
> **Option B**: Keep `automation.go` in the `scripting` package (no subpackage split) and only extract `LogBuffer` and `KVStore`. This is simpler but the `scripting` package stays fat.
>
> The plan assumes **Option A** — the cleaner architecture. The exported names are engine-internal (no gRPC/frontend consumers), and `internal/` prevents use outside the engine.

---

### Component 5: Key-Value Store

#### [NEW] `engine/internal/scriptdata/kvstore.go`

```go
package scriptdata

// KVStore provides per-script persistent key-value storage.
// Each script's data lives in a separate JSON file.
type KVStore struct {
    mu  sync.Mutex
    dir string // base directory, e.g. <dataDir>/script-data
}

func NewKVStore(dir string) *KVStore

// Get returns the value for key in the script's store, or ("", false).
func (kv *KVStore) Get(scriptID, key string) (string, bool)

// Set persists key=value in the script's store.
func (kv *KVStore) Set(scriptID, key, value string) error

// Delete removes a key.
func (kv *KVStore) Delete(scriptID, key string) error

// Keys returns all keys in the script's store.
func (kv *KVStore) Keys(scriptID string) []string
```

Storage format: `<dir>/<scriptID>.json` containing `{"key1":"val1","key2":"val2"}`.

The store is injected into the automation `Manager` and bound to each script's `invocation` so `jhmc.store.*` calls are automatically scoped to the calling script's ID.

---

### Component 6: Full HTTP Client (`jhmc.http`)

#### [MODIFY] [hostfuncs.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/scripting/hostfuncs.go)

Add `httpRequest` function and register it as `jhmc.http`:

```go
func (inv *invocation) httpRequest(L *lua.LState) int {
    inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_NETWORK)
    opts := L.CheckTable(1)

    url := strField(opts, "url")
    method := strings.ToUpper(strField(opts, "method"))
    if method == "" { method = "GET" }
    body := strField(opts, "body")
    timeout := luaNumberField(opts, "timeout")
    if timeout == 0 { timeout = 30 }
    maxBody := luaNumberField(opts, "max_body")
    if maxBody == 0 { maxBody = 64 << 20 } // 64 MiB default

    // Build request with optional body
    var bodyReader io.Reader
    if body != "" { bodyReader = strings.NewReader(body) }

    ctx := inv.ctx
    if timeout > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
        defer cancel()
    }

    req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
    // ... set headers from opts.headers table ...
    req.Header.Set("User-Agent", scriptUserAgent)

    resp, err := inv.host.client.Do(req)
    // ... read body with LimitReader(maxBody) ...

    // Build response table
    result := L.NewTable()
    result.RawSetString("status", lua.LNumber(resp.StatusCode))
    result.RawSetString("body", lua.LString(respBody))
    // response headers as a table
    hdrs := L.NewTable()
    for k, vs := range resp.Header {
        hdrs.RawSetString(strings.ToLower(k), lua.LString(strings.Join(vs, ", ")))
    }
    result.RawSetString("headers", hdrs)
    L.Push(result)
    return 1
}
```

Register in `newJHMC`: `reg("http", inv.httpRequest)`

Existing `http_get`, `http_json`, `download` remain unchanged for backward compatibility.

---

### Component 7: Server Query & Player Management APIs

#### [MODIFY] `automation/interfaces.go` (new file, see Component 4)

```go
// ServerQuery lets scripts list/inspect registered servers.
type ServerQuery interface {
    ListServers() []ServerInfo
    GetServer(id string) (ServerInfo, bool)
}

type ServerInfo struct {
    ID, Name, Provider, McVersion, Status string
    Port, MemoryMB                        int
}

// PlayerManager lets scripts query and manage players without direct
// access to the filesystem or console parsing.
type PlayerManager interface {
    OnlinePlayers(serverID string) []string
    ListBans(serverID string) ([]BanInfo, error)
    AddBan(serverID, target, reason string) error
    RemoveBan(serverID, target string) error
}

type BanInfo struct {
    Type, Target, Reason, Created string
}
```

#### [MODIFY] `automation/api.go` — extend `serverTable()`:

```go
func (a *autoAPI) serverTable(L *lua.LState) *lua.LTable {
    t := L.NewTable()
    // Existing
    t.RawSetString("send", L.NewFunction(a.send))
    t.RawSetString("logs", L.NewFunction(a.logs))
    t.RawSetString("start", L.NewFunction(a.start))
    t.RawSetString("stop", L.NewFunction(a.stop))
    t.RawSetString("restart", L.NewFunction(a.restart))
    // New: server queries
    t.RawSetString("list", L.NewFunction(a.serverList))
    t.RawSetString("info", L.NewFunction(a.serverInfo))
    // New: player management
    t.RawSetString("players", L.NewFunction(a.players))
    t.RawSetString("kick", L.NewFunction(a.kick))
    t.RawSetString("ban", L.NewFunction(a.ban))
    t.RawSetString("unban", L.NewFunction(a.unban))
    t.RawSetString("bans", L.NewFunction(a.listBans))
    return t
}
```

Add `on_join`/`on_leave` globals (in `installGlobals`):
```go
L.SetGlobal("on_join", L.NewFunction(a.onJoin))
L.SetGlobal("on_leave", L.NewFunction(a.onLeave))
L.SetGlobal("sleep", L.NewFunction(a.sleep))
```

#### `on_join`/`on_leave` wiring:

The `runner` struct gets two new hook maps:
```go
joinHooks  map[string][]*lua.LFunction // server id → on_join callbacks
leaveHooks map[string][]*lua.LFunction
```

When `Manager.start()` finishes hook registration, `wireEvents()` subscribes to the `players.EventBus`. A dedicated goroutine receives `PlayerEvent`s and dispatches to the appropriate hooks via the job pump:

```go
func (m *Manager) wirePlayerEvents(ctx context.Context, r *runner) {
    if m.events == nil { return }
    live, cancel := m.events.Subscribe()
    r.wg.Add(1)
    go func() {
        defer r.wg.Done()
        defer cancel()
        for {
            select {
            case <-ctx.Done():
                return
            case ev, ok := <-live:
                if !ok { return }
                switch ev.Type {
                case players.EventJoin:
                    if hooks := r.joinHooks[ev.ServerID]; len(hooks) > 0 {
                        r.fire(ctx, hooks, lua.LString(ev.Name))
                    }
                case players.EventLeave:
                    if hooks := r.leaveHooks[ev.ServerID]; len(hooks) > 0 {
                        r.fire(ctx, hooks, lua.LString(ev.Name))
                    }
                }
            }
        }
    }()
}
```

This is maximally robust: the automation system never parses console lines for player events. It consumes structured events from the `EventBus`, which derives them from `Roster` state diffs.

---

### Component 8: Utility Functions

#### [MODIFY] [hostfuncs.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/scripting/hostfuncs.go)

Add `jhmc.time()`:
```go
reg("time", func(L *lua.LState) int {
    L.Push(lua.LNumber(float64(time.Now().UnixMilli()) / 1000.0))
    return 1
})
```

Also add `jhmc.store` sub-table (wired to the `scriptdata.KVStore` via the `invocation`'s script ID).

#### [MODIFY] `automation/api.go`

Add `sleep(seconds)` (requires `schedule`):
```go
func (a *autoAPI) sleep(L *lua.LState) int {
    a.inv.require(L, mcmanagerv1.PermissionKind_PERMISSION_SCHEDULE)
    secs := float64(L.CheckNumber(1))
    if secs <= 0 { return 0 }
    select {
    case <-time.After(time.Duration(secs * float64(time.Second))):
    case <-a.inv.ctx.Done():
        a.inv.fail(L, a.inv.ctx.Err())
    }
    return 0
}
```

> [!WARNING]
> `sleep` blocks the calling Lua coroutine. Since each automation script's LState is single-threaded (serialized through the job pump), a sleeping script won't process other hooks until the sleep completes. This is the expected behavior — it's like `time.sleep()` in Python. The script's other hooks queue in the job channel and run when sleep returns.

---

### Component 9: Engine Wiring (`main.go`)

#### [MODIFY] [main.go](file:///d:/devs/VisualStudio/JustHostMC/engine/cmd/engine/main.go)

Wire the new components:

```go
// Player event bus — robust structured join/leave events from Roster state diffs.
eventBus := players.NewEventBus()
hub.AddLineObserver(eventBus.Feed)

// Per-script persistent KV store.
kvStore := scriptdata.NewKVStore(filepath.Join(paths.Base, "script-data"))

// Adapters for the automation system's dependency interfaces.
serverQuery := &serverQueryAdapter{store: registry}
playerMgr := &playerManagerAdapter{hub: hub, store: registry, paths: paths}

// Automation manager with all new dependencies.
automation := automation.NewManager(automation.ManagerConfig{
    Host:     host,
    Grants:   scriptGrants,
    Console:  hub,
    Control:  serverService,
    Logs:     scriptlog.NewLogBuffer(0),
    Query:    serverQuery,
    Players:  playerMgr,
    Events:   eventBus,
    KVStore:  kvStore,
})
```

Adapter structs (defined in `main.go` or a small `adapters.go`):

```go
type serverQueryAdapter struct{ store store.Store }
func (a *serverQueryAdapter) ListServers() []automation.ServerInfo { ... }
func (a *serverQueryAdapter) GetServer(id string) (automation.ServerInfo, bool) { ... }

type playerManagerAdapter struct {
    hub   *console.Hub
    store store.Store
    paths appdata.Paths
}
func (a *playerManagerAdapter) OnlinePlayers(serverID string) []string { ... }
func (a *playerManagerAdapter) ListBans(serverID string) ([]automation.BanInfo, error) { ... }
func (a *playerManagerAdapter) AddBan(serverID, target, reason string) error { ... }
func (a *playerManagerAdapter) RemoveBan(serverID, target string) error { ... }
```

---

### Component 10: Permission Registration & Frontend

#### [MODIFY] [permissions.go](file:///d:/devs/VisualStudio/JustHostMC/engine/internal/scripting/permissions.go)

```diff
   "schedule":       mcmanagerv1.PermissionKind_PERMISSION_SCHEDULE,
+  "server_query":   mcmanagerv1.PermissionKind_PERMISSION_SERVER_QUERY,
+  "player_manage":  mcmanagerv1.PermissionKind_PERMISSION_PLAYER_MANAGE,
```

#### [MODIFY] [PermissionLabels.cs](file:///d:/devs/VisualStudio/JustHostMC/app/JustHostMC.App/Models/PermissionLabels.cs)

```diff
   PermissionKind.PermissionSchedule => "Permission_Schedule",
+  PermissionKind.PermissionServerQuery => "Permission_ServerQuery",
+  PermissionKind.PermissionPlayerManage => "Permission_PlayerManage",
```

#### [MODIFY] [LuaPermissions.cs](file:///d:/devs/VisualStudio/JustHostMC/app/JustHostMC.App/Services/LuaPermissions.cs)

```diff
   ["schedule"] = PermissionKind.PermissionSchedule,
+  ["server_query"] = PermissionKind.PermissionServerQuery,
+  ["player_manage"] = PermissionKind.PermissionPlayerManage,
```

---

### Component 11: Documentation

#### [MODIFY] [scripting.md](file:///d:/devs/VisualStudio/JustHostMC/docs/scripting.md)

Full update:
- Add `server_query` and `player_manage` to the permission kinds table
- Document `jhmc.http(opts)` with complete opts reference and examples
- Document `jhmc.store.*` KV API
- Document `jhmc.time()`
- Document `server.list()`, `server.info(id)`
- Document `server.players()`, `server.kick()`, `server.ban()`, `server.unban()`, `server.bans()`
- Document `on_join(id, handler)`, `on_leave(id, handler)` with note about EventBus robustness
- Document `sleep(seconds)` with warning about blocking
- Update §6 from "intended surface" to shipped API with full signatures

---

## Summary of File Operations

| Op | Path |
|----|------|
| MODIFY | `proto/mcmanager/v1/mcmanager.proto` |
| MODIFY | `engine/internal/scripting/permissions.go` |
| MODIFY | `engine/internal/scripting/hostfuncs.go` |
| MODIFY | `engine/internal/scripting/host.go` (export `NewSandbox`, `Invocation`, `NewJHMC`) |
| MODIFY | `engine/internal/scripting/automation.go` → split into subpackage |
| NEW | `engine/internal/scripting/automation/manager.go` |
| NEW | `engine/internal/scripting/automation/runner.go` |
| NEW | `engine/internal/scripting/automation/api.go` |
| NEW | `engine/internal/scripting/automation/interfaces.go` |
| NEW | `engine/internal/scripting/automation/automation.go` |
| NEW | `engine/internal/scriptlog/logbuffer.go` |
| NEW | `engine/internal/scriptdata/kvstore.go` |
| NEW | `engine/internal/players/eventbus.go` |
| MODIFY | `engine/internal/console/hub.go` |
| MODIFY | `engine/internal/grpc/scriptservice.go` |
| MODIFY | `engine/cmd/engine/main.go` |
| MODIFY | `app/JustHostMC.App/Models/PermissionLabels.cs` |
| MODIFY | `app/JustHostMC.App/Services/LuaPermissions.cs` |
| MODIFY | `docs/scripting.md` |
| DELETE | `engine/internal/scripting/logbuffer.go` |
| DELETE | `engine/internal/scripting/automation.go` (content moves to subpackage) |

---

## Execution Order

The changes should be implemented in this order to keep the build green at each step:

1. **Proto + codegen** — add permission kinds, regenerate stubs
2. **`scriptlog`** — extract LogBuffer, update imports (pure refactor, no behavior change)
3. **`scriptdata`** — new KV store package (standalone, no dependents yet)
4. **`players/eventbus.go`** — new EventBus (standalone, no dependents yet)
5. **`console/hub.go`** — add `AddLineObserver` (backward compatible)
6. **`scripting/permissions.go`** — register new permission names
7. **`scripting/hostfuncs.go`** — add `jhmc.http()`, `jhmc.time()`, `jhmc.store.*`
8. **`scripting/automation/`** — extract + extend automation subpackage with all new APIs
9. **`main.go`** — wire everything together
10. **Frontend** — `PermissionLabels.cs`, `LuaPermissions.cs`
11. **Documentation** — `scripting.md`

## Verification Plan

### Automated Tests
```bash
cd engine && go build ./...
cd engine && go vet ./...
cd engine && go test ./internal/scripting/... -count=1 -v
cd engine && go test ./internal/scripting/automation/... -count=1 -v
cd engine && go test ./internal/scriptlog/... -count=1 -v
cd engine && go test ./internal/scriptdata/... -count=1 -v
cd engine && go test ./internal/players/... -count=1 -v
```

### Manual Verification
- Write a test automation script exercising each new API
- Verify proto codegen succeeds for both Go and C#
- Verify C# frontend builds with new permission labels
- Test on_join/on_leave with a real Minecraft server to confirm EventBus reliability
