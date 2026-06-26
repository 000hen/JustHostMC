# JustHostMC — Minecraft Server Manager for Windows

> [!NOTE]
> This is an experimental project, collaborating with AIs. Please use at your own
> risk. I cannot promise anything about the quality, security, or stability of this
> project. If you want to use it in production, please review the code and test it
> thoroughly first. I am not responsible for any damage or loss caused by using
> this project.

A Windows desktop app to create, run, and manage multiple isolated Minecraft
servers (vanilla, Paper/Spigot, Forge/NeoForge, Fabric), distributed via the
Microsoft Store. **Zero config**: no pre-installed Java/Docker/WSL required —
the app downloads runtime dependencies (per-version OpenJDK, server software) on
demand and caches them. **i18n-ready** with English as the base/fallback
language and additional translations (e.g. `zh-Hant`).

## Architecture

```
WinUI 3 (C#)  <-- gRPC (loopback) -->  engine (Go)  <-- IsolationBackend -->  per-server processes
```

- **Frontend** — C# / WinUI 3 (Windows App SDK), MVVM (`CommunityToolkit.Mvvm`),
  packaged as a full-trust MSIX desktop app.
- **Backend** — Go daemon (`engine`) that provisions and supervises servers.
- **IPC** — gRPC over loopback. The app launches the bundled `engine` as a child
  process, which binds `127.0.0.1` on a random port and reports it on stdout. A
  per-launch random session token authenticates every gRPC call.
- The `.proto` contract is the **single source of truth**; both C# and Go stubs
  are generated from it.

## Repository layout

```
/proto        .proto contract (source of truth) + buf config
/engine       Go backend (cmd/engine, internal/*, generated stubs in gen/)
/app          C# WinUI 3 frontend
/build        MSIX packaging / signing / scripts
/legacy-cpp   archived default C++/WinRT scaffold (safe to delete)
```

> OpenJDK and other runtime dependencies are **not** committed or packaged; the
> engine downloads them on demand and caches them under app data.

## Prerequisites (development)

| Tool | Version | Purpose |
|------|---------|---------|
| .NET SDK | 8+ | C# WinUI app |
| Windows App SDK | 1.x | WinUI 3 runtime |
| Go | 1.22+ | engine |
| protoc **or** buf | recent | codegen |
| `protoc-gen-go`, `protoc-gen-go-grpc` | recent | Go gRPC stubs (`go install`) |

## Build & run

```powershell
# 1. Generate Go gRPC stubs (C# stubs are generated at build time via Grpc.Tools).
cd proto ; buf generate

# 2. Build & test the engine, then produce the bundled binary the app launches.
cd ../engine ; go build ./... ; go test ./... ; go build -o ../build/engine.exe ./cmd/engine

# 3. Build the app (WinUI requires an explicit platform; AnyCPU is unsupported).
cd .. ; dotnet build app/JustHostMC.App/JustHostMC.App.csproj -p:Platform=x64

# 4. Run the cross-language end-to-end tests (app launches the real engine).
dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj
```

> `JustHostMC.sln` ties the three C# projects together for Visual Studio. Run the
> app from VS (F5); headless `dotnet run` of a WinUI app needs extra packaging
> setup that arrives in M6.

### M0 verification (all green)

- `go test ./...` — engine auth interceptors + Health, in-process gRPC e2e.
- `dotnet test` — **6/6**, including a real C# → Go `Health` call over the
  token-authed loopback channel, and rejection of calls with a wrong/missing token.
- The app's compiled PRI indexes both `en-US` (default/fallback) and `zh-Hant`
  for every UI string, proving the localization pipeline end-to-end.

### M1 verification (all green)

- Unit tests: Vanilla provider (Mojang manifest/version parsing via httptest),
  JRE manager (Adoptium asset resolution, extract, checksum, cache-hit), the
  download helper, job-object lifecycle (real child process), store, app-data
  paths, and heap/launch-arg math.
- `go test ./internal/isolation/` drives a real Job Object: start, stdin command,
  graceful stop, force stop, list/attach.
- **End-to-end acceptance** (`JHMC_INTEGRATION=1 go test ./internal/e2e/`): on a
  path with no pre-installed Java, the engine downloaded the vanilla server jar
  **and** the matching JRE, started the server (`Done (4.541s)!`), then stopped
  it and tore down the process tree — **PASS**.
- The WinUI create flow streams localized step + progress bar + a ring-buffered
  raw-output detail box (DispatcherQueue-marshaled), with a first-run "runs on
  this PC" notice.

### M4 verification (all green)

- Unit tests cover the non-vanilla providers: Paper (PaperMC build API),
  Forge (promotions → recommended/latest installer), NeoForge (maven-metadata
  parse + MC-version mapping), the shared installer runner (stdout/stderr → the
  install detail box), launch detection (`win_args.txt` arg-file or jar), and the
  `JavaMajorForMC` heuristic for non-vanilla versions.
- The create dialog offers Vanilla / Paper / Forge / NeoForge and reloads the
  version list per type.
- **End-to-end acceptance** (`JHMC_INTEGRATION=1 go test ./internal/e2e/`): Paper
  (`Done (...)`) and Forge (runs the installer — *"The server installed
  successfully"* — then boots the generated launcher) both download, install, and
  boot real servers on a clean path with no pre-installed Java — **PASS**.

### M5 verification (all green)

- Unit tests: portable archive round-trip + zip-slip guard + `session.lock`
  exclusion; the backup `Manager` (create/list newest-first/restore/delete, unsafe
  id rejection); the `BackupService` (safe-online save-off → flush → save-on dance,
  resume-on-error, flush-timeout fallback, restore-requires-stopped, typed error
  codes); log retention `Purge` (by age, by total-size cap oldest-first, age-then-
  size); the daily-rotating console-log `Sink`; persisted `Settings`; the
  `SettingsService` (get/set/purge-now); and install-log persistence that survives
  a failed install (so the cause is findable).
- **End-to-end acceptance** (`JHMC_INTEGRATION=1 go test ./internal/e2e/`): on a
  **running** real vanilla server, a safe-online backup produced a ~100 MB archive
  **with the server still running** (zero downtime), then — after a stop — the
  snapshot **restored** world-consistently (`world/level.dat` present) — **PASS**.

### M6 verification (all green)

- Unit test: the app can redirect the engine's data dir (`MCMANAGER_DATA_DIR`) —
  verified end-to-end by launching the real engine and confirming it writes its
  registry under the configured directory. When packaged, the app points this at
  `ApplicationData.Current.LocalFolder`, so **uninstall removes all data**.
- Unit test: `RemoveAllData` stops every server and wipes servers/backups/logs/JRE
  (releasing log handles first) while keeping the engine runnable — surfaced as
  **Settings → Remove all data** (flyout-confirmed).
- **Packaging proven**: `msbuild /t:Build /p:WindowsPackageType=MSIX
  /p:GenerateAppxPackageOnBuild=true /p:AppxPackageSigningEnabled=false` produces a
  valid **`.msix`** (86 entries) containing `AppxManifest.xml` (declares only
  `runFullTrust`, languages `en-US`+`zh-Hant`), the bundled **`engine/engine.exe`**,
  the visual assets, and `resources.pri`. Only signing is deferred to submission
  (needs a cert / Store association). See [docs/WACK-checklist.md](docs/WACK-checklist.md),
  the live [privacy policy](https://muisnowdevs.one/privacy), and [LICENSE](LICENSE).
  The default `dotnet build` stays unpackaged for dev.

### M7 verification (all green)

- The engine is backend-agnostic: `DockerBackend` implements the same
  `IsolationBackend` contract as the on-machine Job Object backend.
- Unit tests: `DetectDocker` (available / daemon-down / no-version), consent-gated
  `SelectMode` (Docker **only** with opt-in **and** availability; otherwise falls
  back), container naming + `docker run`/`stop`/`ps` arg construction, and the
  `SettingsService` backend RPCs (`GetBackendInfo`, `SetUseDocker` persists).
- Consent & transparency (PROMPT §8, §10.7): Docker is **opt-in** and **never
  auto-installed**; on this dev machine the Docker CLI is present but the daemon is
  off, so detection reports *unavailable* and the engine **falls back to
  on-machine** — exactly the intended behavior. **Settings → Where servers run**
  shows the active mode + Docker availability and offers the opt-in (effective next
  launch). The live container lifecycle requires Docker Desktop running.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.

## Milestones

| ID | Scope | Status |
|----|-------|--------|
| M0 | Skeleton: proto + dual codegen, Health RPC over token-authed loopback gRPC, i18n base | ✅ done |
| M1 | Vanilla lifecycle + on-demand JRE (Job Objects) | ✅ done |
| M2 | Streaming console | ✅ done |
| M3 | SQLite persistence + re-adopt running servers + crash detection | ✅ done |
| M4 | Paper / Forge / NeoForge providers | ✅ done |
| M5 | Safe online backups + log retention | ✅ done |
| M6 | MSIX packaging, WACK, clean uninstall, privacy policy | ✅ done |
| M7 | Docker backend (detect + consent) | ✅ done |

See `CLAUDE_CODE_PROMPT.md` for the full specification.
