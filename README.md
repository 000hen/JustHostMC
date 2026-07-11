# JustHostMC — Minecraft Server Manager for Windows

> [!NOTE]
> This is an experimental project, collaborating with AIs. Please use at your own
> risk. I cannot promise anything about the quality, security, or stability of this
> project. If you want to use it in production, please review the code and test it
> thoroughly first. I am not responsible for any damage or loss caused by using
> this project.

A Windows desktop app to create, run, and manage multiple isolated Minecraft
servers, distributed via the Microsoft Store. **Zero config** — no pre-installed
Java, Docker, or WSL required. The app downloads runtime dependencies
(per-version OpenJDK, server software) on demand and caches them.

## Installation

**[Get it from the Microsoft Store](https://apps.microsoft.com/detail/9NB5ZHPKMBDS)** (Recommended)  
The Microsoft Store version is fully signed, automatically updated, and verified.

**[Download from GitHub Releases](https://github.com/000hen/JustHostMC/releases)**  
Portable EXEs and MSI installers are available for x64, x86, and ARM64.  
> [!WARNING]  
> GitHub release binaries are currently unsigned. You may see a SmartScreen or "Unknown publisher" warning. If you have security considerations, please download from the Microsoft Store instead. Verify downloads using the provided `SHA256SUMS.txt`.

## Features

- **Zero configuration** — downloads the correct JRE (Adoptium) and server jar
  automatically; nothing to pre-install.
- **Multiple servers** — run several isolated servers side-by-side, each with its
  own version, type, and settings.
- **Process isolation** — Windows Job Objects (default) or Docker containers
  (opt-in, never auto-installed).
- **Streaming console** — bidirectional real-time console via gRPC streaming.
- **Safe online backups** — `save-off → save-all → snapshot → save-on` with zero
  downtime; schedule or trigger on demand. Restore from any snapshot.
- **Crash detection** — automatically detects crashed servers with optional
  auto-restart.
- **Mod & plugin management** — browse and manage server mods/plugins from the UI.
- **Player management** — view and manage connected players.
- **Server metrics** — live resource monitoring with sparkline charts.
- **Per-server memory limits** — enforced via Job Objects and JVM `-Xmx`.
- **Port conflict detection** — checks for collisions at create/start time.
- **Log retention** — persistent server and automation logs with shared TTL and size-cap cleanup.
- **Clean uninstall** — optional "Remove all data" wipes servers, backups, logs,
  and JRE cache.
- **Internationalization** — English (base/fallback) and Traditional Chinese
  (`zh-Hant`); easily extensible via `.resw` resource files.
- **MSIX packaging** — full-trust desktop app, ready for the Microsoft Store.

## Supported Server Types

| Type | Source |
|------|--------|
| Vanilla | Mojang piston-meta API |
| Paper | PaperMC downloads API |
| Spigot / Bukkit | BuildTools (due to redistribution restrictions) |
| Forge | Forge promotions API |
| NeoForge | Maven metadata |
| Fabric | Fabric meta API |

## Architecture

```
WinUI 3 (C#)  <-- gRPC (Named Pipe) -->  engine (Go)  <-- IsolationBackend -->  per-server processes
```

| Layer | Role |
|-------|------|
| **Frontend** | C# / WinUI 3 (Windows App SDK), MVVM via `CommunityToolkit.Mvvm`; see the [MVVM conventions](docs/mvvm.md) |
| **Backend** | Go daemon (`engine/`) that provisions and supervises servers |
| **IPC** | gRPC over Windows Named Pipe (`\\.\pipe\JustHostMC-<guid>`); OS-level access control |
| **Contract** | `.proto` files under `proto/` are the single source of truth |

> OpenJDK and other runtime dependencies are **not** committed; the engine
> downloads them on demand and caches them under app data.

## Repository Layout

```
proto/          .proto contract + buf config
engine/         Go backend (cmd/engine, internal/*, generated stubs in gen/)
app/            C# WinUI 3 frontend (App, Core, Core.Tests)
build/          MSIX packaging / build output (engine.exe)
setup.ps1       Dev environment checker & installer
build.ps1       Full build pipeline script
```

## Getting Started

### Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| .NET SDK | 9+ | C# WinUI app |
| Windows App SDK | 2.x | WinUI 3 runtime |
| Go | 1.26+ | Engine |
| [buf](https://buf.build/docs/installation) | latest | Protobuf codegen |
| `protoc-gen-go`, `protoc-gen-go-grpc` | latest | Go gRPC stubs (`go install`) |

### Quick Start

```powershell
git clone https://github.com/000hen/JustHostMC.git
cd JustHostMC

# 1. Check & install prerequisites (one-time)
.\setup.ps1

# 2. Full build: protobuf codegen → Go engine → C# app → tests
.\build.ps1
```

`setup.ps1` validates your toolchain and offers to install missing Go-based
tools (`buf`, `protoc-gen-go`, `protoc-gen-go-grpc`) via `go install`.
`build.ps1` runs every step in the correct order.

### Build Script Options

```powershell
.\build.ps1                         # Debug | x64 (default)
.\build.ps1 -Configuration Release  # Release build
.\build.ps1 -Platform ARM64         # target ARM64
.\build.ps1 -SkipTests              # skip go test + dotnet test
.\build.ps1 -SkipEngine             # reuse existing build/engine.exe
.\build.ps1 -SkipProto              # reuse existing engine/gen/ stubs
```

### Visual Studio (F5)

Open `JustHostMC.sln` and press **F5**. The MSBuild `Engine.targets` file
automatically runs `buf generate` when Go gRPC stubs are missing, then compiles
the Go engine before the C# build begins — no manual steps needed on a fresh
clone.

The app uses the packaged MSIX model by default. Double-click
`Package.appxmanifest` in Solution Explorer to open Visual Studio's manifest
designer. See [MSIX packaging](docs/packaging.md) for command-line packaging,
signing, and troubleshooting guidance.

> **Tip**: `dotnet run` does not deploy a packaged WinUI app. Use Visual Studio
> F5, or build and install an MSIX package as described in the packaging guide.

<details>
<summary>Manual build steps (without scripts)</summary>

```powershell
# 1. Generate Go gRPC stubs (C# stubs are generated at build time via Grpc.Tools)
cd proto ; buf generate

# 2. Build & test the engine
cd ../engine
go build ./...
go test ./...
$env:CGO_ENABLED = '0'
go build -trimpath -buildvcs=false -mod=readonly -ldflags="-s -w -buildid=" -o ../build/engine.exe ./cmd/engine

# 3. Build the app (WinUI requires an explicit platform; AnyCPU is unsupported)
cd .. ; dotnet build app/JustHostMC.App/JustHostMC.App.csproj -p:Platform=x64

# 4. Run the cross-language end-to-end tests
dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj
```

</details>

## Technology Stack

| Layer | Technology |
|-------|------------|
| Frontend | C# / WinUI 3 / Windows App SDK / CommunityToolkit.Mvvm |
| Backend | Go 1.26+ |
| IPC | gRPC over Windows Named Pipe |
| Codegen | buf |
| Database | SQLite ([modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite), CGo-free) |
| Process isolation | Windows Job Objects (`golang.org/x/sys/windows`) |
| Container isolation | Docker (optional, opt-in) |
| Packaging | MSIX (full-trust `runFullTrust`) |
| i18n | `.resw` resource files + `x:Uid` bindings |
| Testing | `go test`, `dotnet test`, integration e2e |

## Milestones

| ID | Scope | Status |
|----|-------|--------|
| M0 | Skeleton: proto + dual codegen, Health RPC, i18n base | ✅ |
| M1 | Vanilla lifecycle + on-demand JRE (Job Objects) | ✅ |
| M2 | Streaming console | ✅ |
| M3 | SQLite persistence + re-adopt running servers + crash detection | ✅ |
| M4 | Paper / Forge / NeoForge providers | ✅ |
| M5 | Safe online backups + log retention | ✅ |
| M6 | MSIX packaging, WACK, clean uninstall, privacy policy | ✅ |
| M7 | Docker backend (detect + consent) | ✅ |

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

Before submitting, make sure:
1. `go test ./...` passes in `engine/`
2. `dotnet test` passes for the C# projects
3. Any new `.proto` changes are reflected in both Go and C# stubs
4. WinUI observable properties and commands follow the [MVVM Toolkit source-generation conventions](docs/mvvm.md)

## Links

- [Privacy Policy](https://muisnowdevs.one/privacy)

## License

This project is licensed under the **MIT License**. See [LICENSE](LICENSE) for
details.
