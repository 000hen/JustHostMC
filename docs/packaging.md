# MSIX Packaging

JustHostMC uses the packaged MSIX app model. The app project sets
`WindowsPackageType` to `MSIX`, and `Package.appxmanifest` supplies its package
identity, visual assets, supported languages, and `runFullTrust` capability.

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

For an installable or Store package, configure a certificate whose subject
matches the manifest's `Identity Publisher`, or use Visual Studio's
**Package and Publish** workflow. Never commit private signing keys or their
passwords.

## Troubleshooting

- If the manifest designer is unavailable, reload the project and confirm that
  `WindowsPackageType` still evaluates to `MSIX`.
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
