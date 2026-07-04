# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

JustHostMC is a Windows desktop app to create, run, and manage multiple isolated
Minecraft servers, distributed via the Microsoft Store. It needs zero
pre-installed Java/Docker/WSL — the backend downloads runtime dependencies (JRE,
server jars) on demand and caches them.

It is a **polyglot monorepo with one contract**: a C#/WinUI 3 frontend talks to a
Go backend daemon over gRPC on a Windows Named Pipe. The `.proto` is the single
source of truth that both sides generate from.

```
WinUI 3 (C#)  ──gRPC over Named Pipe──▶  engine (Go)  ──IsolationBackend──▶  per-server java processes
```

| Dir | Role |
|-----|------|
| `proto/` | `.proto` contract (source of truth) + buf config |
| `engine/` | Go backend daemon (`cmd/engine` entry, `internal/*`, generated stubs in `gen/`) |
| `app/` | C# WinUI 3 frontend: `JustHostMC.Core` (lib), `JustHostMC.Core.Tests` (xUnit), `JustHostMC.App` (UI) |
| `build/` | Output dir for the bundled `engine.exe` (gitignored) |

The active solution is `JustHostMC.sln`. The product is C#/WinUI 3 + Go only — the
legacy C++/WinRT scaffold that used to sit in the repo root (`JustHostMC.vcxproj`,
`JustHostMC/`, root `Assets/`) has been removed.

## Commands

One-time dev setup (verifies Go, .NET 9 SDK, buf, protoc-gen-go,
protoc-gen-go-grpc; offers `go install` for the buf/protoc tools):

```powershell
.\setup.ps1
```

Full build pipeline (buf generate → go build+test → dotnet restore/build/test):

```powershell
.\build.ps1                         # Debug | x64 (default)
.\build.ps1 -Configuration Release -Platform ARM64
.\build.ps1 -SkipTests -SkipEngine -SkipProto   # reuse prior outputs
```

From Visual Studio, **F5 just works**: `app/Engine.targets` auto-runs
`buf generate` (when Go stubs are missing) and `go build` before the C# build.

### Protobuf codegen — run after ANY edit to `proto/mcmanager/v1/mcmanager.proto`

- **Go stubs are gitignored** (`engine/gen/`) and MUST be regenerated:
  `cd proto; buf generate`. Output goes to `engine/gen/mcmanager/v1`.
- **C# stubs regenerate automatically** on every `dotnet build` of
  `JustHostMC.Core` (Grpc.Tools `<Protobuf>` item, `GrpcServices=Client`).

### Engine (Go) — run from `engine/`

```bash
go build ./...                       # compile everything
go test ./...                        # unit tests (fast, no network)
go test ./internal/provider/ -run TestVanilla   # single package / single test
# The bundled binary the app launches:
CGO_ENABLED=0 go build -trimpath -buildvcs=false -mod=readonly \
  -ldflags="-s -w -buildid=" -o ../build/engine.exe ./cmd/engine
```

Gated end-to-end tests really download (~100 MB) and boot real servers; they skip
unless `JHMC_INTEGRATION=1`:

```powershell
$env:JHMC_INTEGRATION='1'; go test ./internal/e2e/   # (Bash: JHMC_INTEGRATION=1 go test ./internal/e2e/)
```

### App (C#) — WinUI requires an explicit platform (AnyCPU is unsupported and errors)

```powershell
dotnet build app/JustHostMC.App/JustHostMC.App.csproj -p:Platform=x64
dotnet test  app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj
dotnet test  app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj --filter "FullyQualifiedName~Health"
```

- `JustHostMC.Core.Tests` are **integration tests that launch the real
  `engine.exe`** (`EngineFixture`), so a built `build/engine.exe` must exist.
- Fast inner loop (skip the Go rebuild when the engine is unchanged): add
  `-p:SkipEngineBuild=true` (and `-p:SkipProtobufGenerate=true`).
- Store/MSIX build (the app defaults to unpackaged so plain `dotnet build`/`test`
  and F5 work unsigned): override at package time —
  `msbuild app/JustHostMC.App/JustHostMC.App.csproj /t:Publish /p:Platform=x64 /p:WindowsPackageType=MSIX /p:GenerateAppxPackageOnBuild=true`.
- Running `makepri.exe` from Git Bash needs `MSYS_NO_PATHCONV=1` or the `/if`
  `/of` switches get mangled into Windows paths.

## Architecture

### The contract drives everything

`proto/mcmanager/v1/mcmanager.proto` defines all messages and **12 services**
(`EngineService`, `ServerService`, `ConsoleService`, `BackupService`,
`SettingsService`, `PlayerService`, `MetricsService`, `ModService`,
`ConfigService`, `ProviderService`, `ScriptService`, `ParserService`). Adding or changing an RPC is a proto edit first, then regen on
both sides, then implement the Go `*Service` and call it from a C# ViewModel.
`csharp_namespace = McManager.Grpc`; Go import path
`github.com/000hen/justhostmc/engine`.

### IPC & security model

- The engine serves gRPC over a **Windows Named Pipe** (`\\.\pipe\JustHostMC-<guid>`).
  Named pipes are inherently local-only and use the OS security model for access
  control — no TCP port is opened and no session token is needed.
- The app launches `engine.exe` as a child process, generating a unique pipe name
  (`JustHostMC-<guid>`) and passing it via env var `MCMANAGER_PIPE`. The engine
  creates the named pipe listener using `go-winio` and serves gRPC on it.
- **Ready handshake**: the engine prints `MCMANAGER_READY` as the first stdout
  line; everything else (logs) goes to **stderr** — never write engine logs to
  stdout, it corrupts the handshake.
- **Lifecycle**: closing the engine's stdin trips its watchdog → graceful
  shutdown; kill-the-tree is the fallback (`EngineHost.DisposeAsync`).
- **Data dir**: packaged (MSIX) runs pass `MCMANAGER_DATA_DIR` =
  `ApplicationData.Current.LocalFolder` so uninstall wipes everything cleanly;
  unpackaged dev runs leave it unset and the engine uses its `%LOCALAPPDATA%`
  default.

### Engine (Go) layout

- `cmd/engine/main.go` — entry point + all dependency wiring (builds the provider
  map, opens the SQLite store, creates the console hub + log sink, selects the
  isolation backend, registers all services, runs startup reconcile + log janitor).
- `internal/grpc/` — gRPC service implementations (one file per service) plus
  `server.go` (named pipe listener + server builder), `errors.go` (error
  mapping). Services depend on the interfaces below, not concrete types.
- `internal/provider/` — the `Provider` interface (`Versions` + `Install` →
  `LaunchSpec{JavaMajor, Args}`) plus shared helpers: `javamajor.go` maps an MC
  version to the required Java major; sentinel errors in `errors.go`
  (`ErrVersionNotFound`, `ErrChecksumMismatch`). **Concrete server types are no
  longer Go adapters here — they are Lua scripts (see below).**
- `internal/scripting/` — the Lua core: `host.go` runs sandboxed gopher-lua
  (`base/table/string/math` only; no `os`/`io`/`require`; fs confined to the
  server dir) and exposes the permission-gated `jhmc.*` host API (`hostfuncs.go`:
  http/json/toml/yaml/zip/fs/store/...). Three script subsystems build on it:
  **providers** (`Registry`, `LuaProvider`, `builtin/*.lua` — vanilla, paper,
  spigot, fabric), **automation** (the `automation/` subpackage: `Manager` +
  the `server.*`/`on_*`/`schedule` runtime, via the exported surface in
  `export.go`), and **mod-metadata parsers** (`ParserSet`, `LuaParser`,
  `builtin_parsers/*.lua` — fabric/quilt/forge/neoforge/forge-legacy/bukkit; they
  enrich `ModService.List` with icon/name/authors/version, cached per jar).
  `GrantStore` (`grants.go`) persists per-script permission decisions
  (`grants.json`, `script-grants.json`, `parser-grants.json`).
  `ProviderService`/`ScriptService`/`ParserService` are wired in
  `internal/grpc/`. **Adding a server type is "drop a `.lua` in `builtin/` or
  import one at runtime" — not a Go provider; same for parsers in
  `builtin_parsers/`.** See [`docs/scripting.md`](docs/scripting.md) for the
  script-authoring guide.
- `internal/scriptlog/` — automation log ring buffer; `internal/scriptdata/` —
  per-script `jhmc.store` KV files; `internal/players/` also hosts the
  `EventBus` (roster state-diff join/leave events feeding `on_join`/`on_leave`
  and `server.players`).
- `internal/isolation/` — `IsolationBackend` interface with two impls:
  `jobobject_windows.go` (default; Windows Job Objects = memory cap +
  kill-on-close) and `docker.go` (opt-in, never auto-installed). `SelectMode`
  picks Docker only when the user opted in AND a daemon is detected.
- `internal/store/` — `Store` interface + `sqlite.go` (modernc.org/sqlite, no CGo).
- `internal/console/` — `hub.go`: per-server console ring buffer + fan-out
  (pub/sub) feeding ConsoleService, the log sink, and the player tracker.
- Other packages: `backup/` (safe-online zip snapshot/restore with zip-slip
  guard), `logging/` (daily-rotating sink + age/size purge), `settings/` (JSON
  prefs), `players/` (roster parsed from the console stream), `jre/` (on-demand
  Adoptium JRE/JDK, cached per major), `dl/`, `appdata/`.

### App (C#/WinUI 3) layout

- **`JustHostMC.Core`** (plain `net9.0`, no UI → fully unit/integration-testable):
  `EngineHost` (child-process lifecycle + named pipe handshake), `EngineConnection`
  (record: pipe name), and `DaemonClient` — the facade that owns the `GrpcChannel`
  (backed by a `NamedPipeClientStream` via `SocketsHttpHandler.ConnectCallback`)
  and exposes the typed per-service clients.
- **`JustHostMC.App`** (`net9.0-windows10.0.19041.0`, WinUI 3, MVVM via
  CommunityToolkit.Mvvm): `App.xaml.cs` owns the engine lifecycle and exposes
  `App.Current.DaemonReady` (a `Task<DaemonClient>`). ViewModels get the client
  with `await App.Current.DaemonReady`. `Views/` (pages + ContentDialogs),
  `ViewModels/`, `Models/`, `Services/`, `Controls/`, `Converters/`.
  `NavShellViewModel` owns the single live server list (`MainViewModel`) and caches
  per-server ViewModels so gRPC streams survive page navigations. Follow
  `docs/mvvm.md` for the frontend's source-generation conventions.

### Cross-cutting conventions

- **Errors (Go → C#)**: never surface raw strings to users. The engine returns a
  gRPC `status` with an `ErrorDetail{code, metadata}` packed into the details
  (`errorStatus` helper); the `ErrorCode` enum is the programmatic discriminator
  and the frontend maps it to a localized string. The status message is diagnostic
  only.
- **i18n**: dynamic display messages travel as a localization **key**
  `"namespace.method.type"` (+ args) in a `LocalizedMessage`; the frontend resolves
  it against `.resw`. Resources live in
  `app/JustHostMC.App/Strings/{en-US,zh-Hant}/Resources.resw`; **en-US is the base
  and fallback**. Backend keys use `.` separators; `LocalizationService` rewrites
  `.` → `_` to match `.resw` keys. Adding a language is a new `.resw` folder, no
  code change.

### MVVM Toolkit source generation

- Observable UI state uses `[ObservableProperty]` on public partial properties
  in a `partial` type. Do not hand-write backing fields with `SetProperty`.
- Use `[NotifyPropertyChangedFor]` for computed-property dependencies and the
  generated `On<Property>Changed` partial hooks for change side effects.
- Commands use `[RelayCommand]`; use `CanExecute` and
  `[NotifyCanExecuteChangedFor]` when availability depends on observable state.
  Do not manually allocate relay commands unless runtime composition is required.
- Use partial properties, not attributed fields: field targets produce
  `MVVMTK0045` because they are not WinRT AOT-safe. The app intentionally uses
  C# preview with .NET 9 for CommunityToolkit.Mvvm 8.4 partial-property support.
- Keep a clean build free of MVVM Toolkit analyzer warnings. See `docs/mvvm.md`
  for examples and the complete policy.

## WinUI XAML-compiler pitfalls in this project (hard-won — do not relearn)

These cause cryptic startup/page-load crashes, not build errors, so `dotnet build`
will not catch them.

1. **Marshal all stream/background updates to the UI thread** via
   `DispatcherQueue.TryEnqueue` (the `RunOnUI` pattern); ViewModels driving gRPC
   streams take a `DispatcherQueue`.
2. **`x:Bind` `{StaticResource}` converters** must be declared in `App.Resources`
   or the XAML root — not an inner element's `.Resources` — else `0xC000027B` at
   startup.
3. **`{Binding ElementName=...}` is dead inside a ListView item `DataTemplate`** —
   route item commands via `Click`+`Tag` or `x:Bind`.
4. **`NavigationView.MenuItemsSource` of plain managed types + an `{x:Bind}`
   template crashes at startup** — build the items imperatively or use `{Binding}`.
5. **Never share one `x:Uid` across different control types** when its `.resw`
   entry sets `.Content`: applying a `.Content` uid to a `TextBlock` (which has
   `.Text`) throws `XamlParseException 0x802B000A` the moment that page loads. Give
   each control type its own uid with the matching property suffix, and smoke-test
   **every** navigable page/dialog, not just Home.

## Spec & notes

- `CLAUDE_CODE_PROMPT.md` is the original full build spec (the milestone plan M0–M7
  and the `§`-numbered requirements that code comments reference).
- Minecraft's 2026 versioning changed (1.x → 26.x); Paper's v2 API is frozen (use
  v3 `Fill`); NeoForge now uses 4-part versions. Keep this in mind when touching
  providers/version parsing.
- `engine/gen/` and `build/engine.exe` are gitignored build artifacts — never
  commit them; they are reproduced by `buf generate` and `go build`.

# RTK (Rust Token Killer) - Token-Optimized Commands

## Golden Rule

**Always prefix commands with `rtk`**. If RTK has a dedicated filter, it uses it. If not, it passes through unchanged. This means RTK is always safe to use.

**Important**: Even in command chains with `&&`, use `rtk`:
```bash
# ❌ Wrong
git add . && git commit -m "msg" && git push

# ✅ Correct
rtk git add . && rtk git commit -m "msg" && rtk git push
```

## RTK Commands by Workflow

### Build & Compile (80-90% savings)
```bash
rtk cargo build         # Cargo build output
rtk cargo check         # Cargo check output
rtk cargo clippy        # Clippy warnings grouped by file (80%)
rtk tsc                 # TypeScript errors grouped by file/code (83%)
rtk lint                # ESLint/Biome violations grouped (84%)
rtk prettier --check    # Files needing format only (70%)
rtk next build          # Next.js build with route metrics (87%)
```

### Test (60-99% savings)
```bash
rtk cargo test          # Cargo test failures only (90%)
rtk go test             # Go test failures only (90%)
rtk jest                # Jest failures only (99.5%)
rtk vitest              # Vitest failures only (99.5%)
rtk playwright test     # Playwright failures only (94%)
rtk pytest              # Python test failures only (90%)
rtk rake test           # Ruby test failures only (90%)
rtk rspec               # RSpec test failures only (60%)
rtk test <cmd>          # Generic test wrapper - failures only
```

### Git (59-80% savings)
```bash
rtk git status          # Compact status
rtk git log             # Compact log (works with all git flags)
rtk git diff            # Compact diff (80%)
rtk git show            # Compact show (80%)
rtk git add             # Ultra-compact confirmations (59%)
rtk git commit          # Ultra-compact confirmations (59%)
rtk git push            # Ultra-compact confirmations
rtk git pull            # Ultra-compact confirmations
rtk git branch          # Compact branch list
rtk git fetch           # Compact fetch
rtk git stash           # Compact stash
rtk git worktree        # Compact worktree
```

Note: Git passthrough works for ALL subcommands, even those not explicitly listed.

### GitHub (26-87% savings)
```bash
rtk gh pr view <num>    # Compact PR view (87%)
rtk gh pr checks        # Compact PR checks (79%)
rtk gh run list         # Compact workflow runs (82%)
rtk gh issue list       # Compact issue list (80%)
rtk gh api              # Compact API responses (26%)
```

### JavaScript/TypeScript Tooling (70-90% savings)
```bash
rtk pnpm list           # Compact dependency tree (70%)
rtk pnpm outdated       # Compact outdated packages (80%)
rtk pnpm install        # Compact install output (90%)
rtk npm run <script>    # Compact npm script output
rtk npx <cmd>           # Compact npx command output
rtk prisma              # Prisma without ASCII art (88%)
```

### Files & Search (60-75% savings)
```bash
rtk ls <path>           # Tree format, compact (65%)
rtk read <file>         # Code reading with filtering (60%)
rtk grep <pattern>      # Search grouped by file (75%). Format flags (-c, -l, -L, -o, -Z) run raw.
rtk find <pattern>      # Find grouped by directory (70%)
```

### Analysis & Debug (70-90% savings)
```bash
rtk err <cmd>           # Filter errors only from any command
rtk log <file>          # Deduplicated logs with counts
rtk json <file>         # JSON structure without values
rtk deps                # Dependency overview
rtk env                 # Environment variables compact
rtk summary <cmd>       # Smart summary of command output
rtk diff                # Ultra-compact diffs
```

### Infrastructure (85% savings)
```bash
rtk docker ps           # Compact container list
rtk docker images       # Compact image list
rtk docker logs <c>     # Deduplicated logs
rtk kubectl get         # Compact resource list
rtk kubectl logs        # Deduplicated pod logs
```

### Network (65-70% savings)
```bash
rtk curl <url>          # Compact HTTP responses (70%)
rtk wget <url>          # Compact download output (65%)
```

### Meta Commands
```bash
rtk gain                # View token savings statistics
rtk gain --history      # View command history with savings
rtk discover            # Analyze Claude Code sessions for missed RTK usage
rtk proxy <cmd>         # Run command without filtering (for debugging)
rtk init                # Add RTK instructions to CLAUDE.md
rtk init --global       # Add RTK to ~/.claude/CLAUDE.md
```

## Token Savings Overview

| Category | Commands | Typical Savings |
|----------|----------|-----------------|
| Tests | vitest, playwright, cargo test | 90-99% |
| Build | next, tsc, lint, prettier | 70-87% |
| Git | status, log, diff, add, commit | 59-80% |
| GitHub | gh pr, gh run, gh issue | 26-87% |
| Package Managers | pnpm, npm, npx | 70-90% |
| Files | ls, read, grep, find | 60-75% |
| Infrastructure | docker, kubectl | 85% |
| Network | curl, wget | 65-70% |

Overall average: **60-90% token reduction** on common development operations.
