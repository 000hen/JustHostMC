# MSIX Packaging

JustHostMC ships as an MSIX package for the Microsoft Store, but the app project
defaults `WindowsPackageType` to `None` so that `dotnet build`/`test` and F5 run as
a fast, unsigned, **unpackaged** app. It switches to `MSIX` automatically whenever a
package is actually produced — any build that sets `GenerateAppxPackageOnBuild=true`
or `PublishAppxPackage=true`, which the Visual Studio **Create App Packages** wizard
and `/t:Publish` both do. (Forcing `MSIX` on while a package is built — or leaving it
`None` while one is — is a hard MSBuild error, which is why these two have to track
each other.) `Package.appxmanifest` supplies the package identity, visual assets,
supported languages, and `runFullTrust` capability.

## Visual Studio

Open `JustHostMC.sln`, select the `JustHostMC.App` project, and double-click
`Package.appxmanifest` in Solution Explorer. Visual Studio should open its
manifest designer. Press F5 to build, deploy, and launch the packaged app.

The packaged debug profile lives in
`app/JustHostMC.App/Properties/launchSettings.json`. Its command is
`MsixPackage`, which tells Visual Studio to deploy the package before attaching
the debugger. Local network loopback is enabled because the app communicates
with its bundled engine over `127.0.0.1`.

The designer depends on MSBuild classifying the file as an `AppxManifest` item
with the `Designer` subtype. Setting `WindowsPackageType` to `None` makes the
file a generic project item and disables the manifest UI. Do not set that value
in the project unless the app is intentionally being converted to unpackaged.
For source-only editing, use **Open With > XML (Text) Editor**.

## Build an installable package

Build an unsigned package for local validation:

```powershell
dotnet build app/JustHostMC.App/JustHostMC.App.csproj `
  --configuration Release `
  -p:Platform=x64 `
  -p:GenerateAppxPackageOnBuild=true `
  -p:AppxPackageSigningEnabled=false
```

Package output is written below `app/JustHostMC.App/AppPackages`. An unsigned
package cannot be installed normally; use it for manifest and packaging
validation only.

### Store upload package (`.msixupload`)

The Microsoft Store accepts a `.msixupload` bundle. From Visual Studio, use
**Project > Package and Publish > Create App Packages…**, pick the sideloading or
Microsoft Store option, choose the architectures to bundle, and the wizard writes
`JustHostMC.App_<version>_<arch>_bundle.msixupload` under `AppPackages`. The
equivalent command line (what the wizard runs) is:

```powershell
msbuild app/JustHostMC.App/JustHostMC.App.csproj /restore /t:Build `
  /p:Configuration=Release /p:Platform=x64 `
  /p:AppxBundle=Always /p:AppxBundlePlatforms="x64|arm64" `
  /p:UapAppxPackageBuildMode=StoreUpload `
  /p:GenerateAppxPackageOnBuild=true
```

The `.msixupload` is intentionally **unsigned**: the Store re-signs it on
submission, so no certificate is needed to produce or upload it. Do not turn on
`GenerateTemporaryStoreCertificate` while signing is disabled — on a bundle build it
makes the SDK's disposable-certificate cleanup fail (`MSB4044`, empty
`CertificateThumbprint`).

### Signed sideload package

To produce a `.msix`/`.msixbundle` that installs locally, sign it with a certificate
whose subject matches the manifest's `Identity Publisher` (pass
`-p:AppxPackageSigningEnabled=true -p:PackageCertificateKeyFile=… -p:PackageCertificateThumbprint=…`,
or use the `winapp` flow). Never commit private signing keys or their passwords.

## Troubleshooting

- The manifest designer only loads when `WindowsPackageType` evaluates to `MSIX`.
  The project defaults to `None` (unpackaged dev), so the designer is normally
  unavailable — edit `Package.appxmanifest` with **Open With > XML (Text) Editor**.
- If F5 reports that a packaged launch profile is missing, confirm that
  `Properties/launchSettings.json` exists and contains a profile whose
  `commandName` is `MsixPackage`, then reload the project.
- If Visual Studio finds the profile but reports that it does not know how to
  run the `MsixPackage` command, the Single-project MSIX debug provider did not
  load. Check **Extensions > Manage Extensions > Installed > All** for
  **Single-project MSIX Packaging Tools**, restart Visual Studio, and repair or
  reinstall that component through Visual Studio Installer if necessary. With
  Visual Studio closed, `devenv /UpdateConfiguration` rebuilds extension
  registration and `devenv /ResetSkipPkgs` clears package skip-loading state
  (the latter starts Visual Studio). The Visual Studio activity log can
  distinguish this provider failure from an invalid launch profile.
- If packaging reports a publisher mismatch, make the signing certificate
  subject match the `Publisher` value in `Package.appxmanifest`.
- If asset validation fails, confirm every logo path in the manifest exists
  under `app/JustHostMC.App/Assets` with the required dimensions.
- Keep only manifest namespace declarations that are needed by manifest
  elements; older Visual Studio installations may not understand newer schema
  namespaces.

## GitHub Release (Portable & MSI)

JustHostMC also publishes portable EXE and MSI installer builds via
[GitHub Releases](https://github.com/000hen/JustHostMC/releases). These are
built automatically by the `release.yml` workflow whenever a version tag
(`v*.*.*`) is pushed.

### Artifact types

| Artifact | Description |
|----------|-------------|
| `*-portable.exe` | Self-contained single-file executable. Extract-and-run, no installation needed. |
| `*.msi` | Windows Installer package. Installs to Program Files, creates Start Menu shortcut, supports upgrades and uninstall via Add/Remove Programs. |
| `SHA256SUMS.txt` | SHA-256 checksums for all release artifacts. |

Architectures: x64, x86, ARM64.

### Unsigned artifacts

GitHub release binaries are **not** code-signed. Users may encounter:

- **Microsoft Defender SmartScreen**: "Windows protected your PC" warning when
  running the portable EXE. Click **More info → Run anyway** after verifying
  the checksum.
- **Windows Installer**: "Unknown publisher" dialog when installing the MSI.

Always verify downloaded artifacts against the published `SHA256SUMS.txt`:

```powershell
# Verify a downloaded file
(Get-FileHash .\JustHostMC-1.0.0-windows-x64-portable.exe -Algorithm SHA256).Hash
# Compare with the hash in SHA256SUMS.txt
```

Do **not** permanently disable SmartScreen or other Windows security features.
If unsigned binaries are a concern, install from the
[Microsoft Store](https://apps.microsoft.com/detail/9NB5ZHPKMBDS) instead —
Store builds are signed by Microsoft.

### Adding code signing later

To sign release artifacts in the future:

1. Obtain an Authenticode code-signing certificate (EV recommended for
   SmartScreen reputation).
2. Store the certificate as a GitHub Actions encrypted secret
   (e.g. `CODE_SIGNING_CERT`).
3. Add a signing step in `release.yml` between the build and release jobs:
   use `signtool sign` to sign the EXE, DLLs, and MSI before uploading.
4. Update the MSI to display the publisher name from the certificate.

No workflow redesign is needed — signing slots naturally between the existing
build and release stages.

### Triggering a release

```bash
git tag v1.2.3
git push origin v1.2.3
```

The `release.yml` workflow validates the tag format, builds all architectures,
and publishes a GitHub Release with all artifacts attached.

### Distribution channel property

The `DistributionChannel` MSBuild property selects the build profile:

| Value | Behavior |
|-------|----------|
| `Dev` (default) | Unpackaged, self-contained, for F5 / local development |
| `GitHubPortable` | Single-file EXE, self-contained, no MSIX tooling |
| `GitHubMsi` | Folder-based publish output for MSI packaging |
| _(Store/MSIX)_ | Governed by `GenerateAppxPackageOnBuild` / `WindowsPackageType=MSIX` |

Example:

```powershell
dotnet publish app/JustHostMC.App/JustHostMC.App.csproj `
  --configuration Release --runtime win-x64 --self-contained true `
  -p:Platform=x64 -p:DistributionChannel=GitHubPortable -p:Version=1.2.3
```
