# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

JustHostMC is a Windows desktop app to create, run, and manage multiple isolated
Minecraft servers, distributed via the Microsoft Store. It needs zero
pre-installed Java/Docker/WSL — the backend downloads runtime dependencies (JRE,
server jars) on demand and caches them.

It is a **polyglot monorepo with one contract**: a C#/WinUI 3 frontend talks to a
Go backend daemon over gRPC on loopback. The `.proto` is the single source of
truth that both sides generate from.

```
WinUI 3 (C#)  ──gRPC over 127.0.0.1──▶  engine (Go)  ──IsolationBackend──▶  per-server java processes
```

| Dir | Role |
|-----|------|
| `proto/` | `.proto` contract (source of truth) + buf config |
| `engine/` | Go backend daemon (`cmd/engine` entry, `internal/*`, generated stubs in `gen/`) |
| `app/` | C# WinUI 3 frontend: `JustHostMC.Core` (lib), `JustHostMC.Core.Tests` (xUnit), `JustHostMC.App` (UI) |
| `build/` | Output dir for the bundled `engine.exe` (gitignored) |

The active solution is `JustHostMC.sln`. **The repo root also still contains a
legacy C++/WinRT scaffold** (`JustHostMC.vcxproj`, `JustHostMC/`, root `Assets/`)
from before the C#/WinUI rewrite — it is not part of the product; do not edit it
when working on app features.

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

`proto/mcmanager/v1/mcmanager.proto` defines all messages and **9 services**
(`EngineService`, `ServerService`, `ConsoleService`, `BackupService`,
`SettingsService`, `PlayerService`, `MetricsService`, `ModService`,
`ConfigService`). Adding or changing an RPC is a proto edit first, then regen on
both sides, then implement the Go `*Service` and call it from a C# ViewModel.
`csharp_namespace = McManager.Grpc`; Go import path
`github.com/000hen/justhostmc/engine`.

### IPC & security model

- The engine binds **127.0.0.1 with an OS-assigned random port** (`grpc.Listen`),
  so it is never reachable off-machine. HTTP/2 cleartext (no TLS).
- The app launches `engine.exe` as a child process, passing a **per-launch 32-byte
  random session token** via env var `MCMANAGER_TOKEN`. The engine validates it on
  every call via auth interceptors; the C# `TokenInterceptor` attaches it as gRPC
  metadata header `x-mcmanager-token`.
- **Port handshake**: the engine prints `MCMANAGER_PORT=<n>` as the first stdout
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
  `auth.go` (interceptors), `server.go` (server/listener), `errors.go` (error
  mapping). Services depend on the interfaces below, not concrete types.
- `internal/provider/` — one adapter per server type implementing the `Provider`
  interface (`Versions` + `Install` → `LaunchSpec{JavaMajor, Args}`): vanilla,
  paper, spigot, forge, neoforge, fabric. Shared `installer.go` runner;
  `javamajor.go` maps an MC version to the required Java major; sentinel errors in
  `errors.go` (`ErrVersionNotFound`, `ErrChecksumMismatch`). **This is the
  extension point for a new server type.**
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
  `EngineHost` (child-process lifecycle + port handshake), `EngineConnection`
  (record: port + token), `TokenInterceptor`, and `DaemonClient` — the facade that
  owns the `GrpcChannel` and exposes the typed per-service clients.
- **`JustHostMC.App`** (`net9.0-windows10.0.19041.0`, WinUI 3, MVVM via
  CommunityToolkit.Mvvm): `App.xaml.cs` owns the engine lifecycle and exposes
  `App.Current.DaemonReady` (a `Task<DaemonClient>`). ViewModels get the client
  with `await App.Current.DaemonReady`. `Views/` (pages + ContentDialogs),
  `ViewModels/`, `Models/`, `Services/`, `Controls/`, `Converters/`.
  `NavShellViewModel` owns the single live server list (`MainViewModel`) and caches
  per-server ViewModels so gRPC streams survive page navigations.

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

## WinUI XAML-compiler pitfalls in this project (hard-won — do not relearn)

These cause cryptic startup/page-load crashes, not build errors, so `dotnet build`
will not catch them.

1. **Do not use `[ObservableProperty]`.** The source-generated partial properties
   break the WinUI XAML compiler in this project. Write manual properties with
   `SetProperty(ref _field, value)` (see `ConsoleViewModel`). `[RelayCommand]` is fine.
2. **Marshal all stream/background updates to the UI thread** via
   `DispatcherQueue.TryEnqueue` (the `RunOnUI` pattern); ViewModels driving gRPC
   streams take a `DispatcherQueue`.
3. **`x:Bind` `{StaticResource}` converters** must be declared in `App.Resources`
   or the XAML root — not an inner element's `.Resources` — else `0xC000027B` at
   startup.
4. **`{Binding ElementName=...}` is dead inside a ListView item `DataTemplate`** —
   route item commands via `Click`+`Tag` or `x:Bind`.
5. **`NavigationView.MenuItemsSource` of plain managed types + an `{x:Bind}`
   template crashes at startup** — build the items imperatively or use `{Binding}`.
6. **Never share one `x:Uid` across different control types** when its `.resw`
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
