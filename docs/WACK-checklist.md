# Windows App Certification Kit (WACK) — readiness checklist

This is the pre-submission checklist for packaging JustHostMC as a Store-ready
MSIX. Items marked **[done]** are satisfied in the repo; **[manual]** items
require interactive tooling (signing cert, the WACK GUI, or a Partner Center
account) and are performed at packaging/submission time.

## Packaging

- [done] Single-project MSIX tooling enabled (`EnableMsixTooling=true`).
- [done] `Package.appxmanifest` present with a valid `Identity`, `Properties`,
  and `Dependencies` (Windows.Desktop, MinVersion 10.0.17763.0).
- [done] Bundled `engine.exe` ships inside the package
  (`Content … Link="engine\engine.exe"`), launched from `AppContext.BaseDirectory`.
- [manual] Produce the MSIX: in Visual Studio, **Project → Package and Publish →
  Create App Packages…**, or build with
  `msbuild /t:Publish /p:Configuration=Release /p:Platform=x64 /p:GenerateAppxPackageOnBuild=true`
  and sign with your code-signing certificate (or associate with the Store, which
  supplies the identity/cert).
- [manual] Test on x64 and ARM64 (the project targets `x86;x64;ARM64`).

## Capabilities & privacy

- [done] Declares only `runFullTrust` (required: the app launches the native
  engine and manages on-machine server processes via Job Objects). No broad or
  unjustified capabilities.
- [done] Privacy policy published (`PRIVACY.md`); link it in the Store listing.
- [done] No secrets in source; the engine's IPC is loopback-only with a
  per-launch session token.
- [done] HTTPS-only downloads (Mojang / PaperMC / Forge / NeoForge / Adoptium).
- [done] The app never installs Docker/WSL/Hyper-V or changes Windows settings
  without consent; on-machine mode shows a first-run "runs on this PC" notice.

## Languages & assets

- [done] Manifest declares `en-US` (default/fallback) and `zh-Hant`; all
  user-visible strings come from `Strings/<lang>/Resources.resw`.
- [done] Visual assets present under `Assets\` (Square44x44, Square150x150,
  Wide310x150, SplashScreen, StoreLogo, plus Square71x71/Square310x310).
- [manual] Replace the placeholder logos with final brand artwork before
  submission.

## Lifecycle / clean uninstall

- [done] All engine data lives under the package's local store when packaged
  (the app sets `MCMANAGER_DATA_DIR` to `ApplicationData.Current.LocalFolder`),
  so uninstall removes servers, backups, logs, and the JRE cache.
- [done] In-app **Settings → Remove all data** stops all servers and wipes the
  data folder (verified by `RemoveAllData` unit test + engine RPC).
- [done] The engine shuts down cleanly when the app exits (stdin-close watchdog
  + Job Object kill-on-close), leaving no orphaned processes.

## Run the kit

- [manual] Run **Windows App Cert Kit** against the produced package (or via
  `appcert.exe test`), review the report, and resolve any failures before
  submitting to Partner Center.
