# Resource lookup inventory

Updated 2026-07-15 after migrating static view, window, dialog, tooltip, and accessibility copy to XAML and normalizing common action resources. The inventory contains **128** `ILocalizer.Get` occurrences. Task 5 renamed 15 XAML-owned resource keys into compatible `Common*` families and removed 66 unreferenced aliases; neither change affects the runtime lookup count. Direct construction is tracked separately so construction-only owners such as `SettingsPage.xaml.cs` are not lost.

| Classification | Count |
|---|---:|
| StaticXaml | 0 |
| DynamicState | 99 |
| BackendKey | 5 |
| RuntimeFormat | 23 |
| ImperativeException | 1 |
| **Total lookups** | **128** |

## Lookup occurrences

| File:line | Resource expression | Classification | XAML owner / justification | Final action |
|---|---|---|---|---|
| `Controls/ScriptEntryCard.xaml.cs:32` | `"Scripts.RemoveConfirmBody"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerHeaderPanel.xaml.cs:153` | `Server.MemoryValue(memory)` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:56` | `"Players.Header"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:87` | `"PlayerDataDialog.ActionName"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:99` | `"PlayerInventoryDialog.ActionName"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:113` | `PlayerDialogBase.TitleFormat(actionName, player.Name)` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `MainWindow.xaml.cs:311` | `"ServerTeachingTip.StartAction"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `MainWindow.xaml.cs:319` | `ServerTeachingTip.InstalledTitle / StartedTitle / StoppedTitle / CrashedTitle; {server}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `MainWindow.xaml.cs:330` | `ServerTeachingTip.InstalledMessage / StartedMessage / StoppedMessage / CrashedMessage` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/BanEntryItem.cs:19` | `BanList.TypeIp / BanList.TypePlayer` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/BanEntryItem.cs:24` | `"BanList.NoReason"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ConfigEntryItem.cs:80` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ModFileItem.cs:19` | `"Mods.ParseFailed"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ModFileItem.cs:34` | `Mods.TypeAndVersionMismatch; {loader}, {version}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ModFileItem.cs:38` | `"Mods.TypeMismatch"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ModFileItem.cs:39` | `Mods.VersionMismatch; {version}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ParserItem.cs:28` | `Parsers.Formats; {formats}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/PermissionLabels.cs:25` | `LabelKey(kind)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:114` | `ServerStatus.Installing / Starting / Running / Stopping / Stopped / Crashed / Unknown` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:136` | `ServerNav.StateChangedAutomationName / ServerNav.AutomationName; {name}, {status}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ServerItem.cs:148` | `"ServerType.Vanilla"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:149` | `"ServerType.Paper"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:150` | `"ServerType.Spigot"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:151` | `"ServerType.Forge"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:152` | `"ServerType.NeoForge"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:153` | `"ServerType.Fabric"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:154` | `"ServerType.Unknown"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:172` | `"Server.PortLabel"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ServerItem.cs:174` | `"Server.PortAutoValue"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:175` | `ServerState.Stop / ServerState.Starting / ServerState.Stopping / ServerState.Installing / ServerState.Start` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:67` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:75` | `"Backups.Creating"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:84` | `"Backups.Created"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:86` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:98` | `"Backups.Restoring"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:106` | `"Backups.Restored"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:108` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:120` | `"Backups.Deleting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:127` | `"Backups.Deleted"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:129` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:91` | `"EngineStatus.Connecting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:97` | `"EngineStatus.Connecting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:103` | `"EngineStatus.Connected"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:113` | `"EngineStatus.Failed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:144` | `"CreateServer.DefaultName"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:184` | `"install.progress.preparing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:191` | `"install.progress.preparing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:203` | `step.Key` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/MainViewModel.cs:220` | `"install.progress.done"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:222` | `"install.ready.to.run"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:231` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:232` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:307` | `"ServerState.Starting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:330` | `"ServerState.Stopping"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:359` | `step.Key` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/MainViewModel.cs:394` | `ServerState.Installing / ServerState.Starting / ServerState.Stopping` selected by `proto.Status` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:178` | `Mods.KindMods / Mods.KindPlugins` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:321` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:337` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:393` | `"Mods.ExportDone"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:395` | `"Mods.ExportFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:406` | `Mods.OperationFailedDetail; {summary}, {code}, {detail}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `ViewModels/ModsViewModel.cs:408` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:414` | `Mods.OperationFailedDetail; {summary}, {code}, {detail}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `ViewModels/ModsViewModel.cs:415` | `"Mods.OperationFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:101` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:120` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:134` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:162` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:183` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:204` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:220` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:233` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:243` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:253` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:322` | `"Scripts.SystemLogName"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:331` | `"Scripts.LogEntryFallbackTitle"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:366` | `"Scripts.CurrentSessionTitle"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:133` | `"Config.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:155` | `"Config.Saved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:161` | `"Config.SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:184` | `"Config.Saved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:190` | `"Config.SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:108` | `"Settings.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:128` | `status` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:146` | `"Settings.Saved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:151` | `"Settings.SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:161` | `Backend.Mode.Docker / Backend.Mode.OnMachine` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:166` | `Backend.DockerAvailable(version)` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `ViewModels/SettingsViewModel.cs:168` | `"Backend.DockerUnavailable"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:185` | `"Backend.DockerPrefSaved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:188` | `"Settings.SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:228` | `Settings.PurgeResult; {count}, {size}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `ViewModels/SettingsViewModel.cs:234` | `"Settings.PurgeFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:244` | `"Settings.RemovingData"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:251` | `"Settings.DataRemoved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:254` | `"Settings.RemoveDataFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:123` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:156` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:215` | `"Shop.InstallDone"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:220` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:223` | `"Shop.InstallFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopViewModel.cs:106` | `$"shop.category.{id}"` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/ShopViewModel.cs:150` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopViewModel.cs:181` | `s.Title.Key` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/ShopViewModel.cs:196` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopViewModel.cs:208` | `descriptionKey` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/ShopViewModel.cs:252` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BackupsDialog.xaml.cs:46` | `"Backups.ExportSourceMissing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BackupsDialog.xaml.cs:66` | `"Backups.Exported"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/BackupsDialog.xaml.cs:69` | `"Backups.ExportFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BackupsDialog.xaml.cs:81` | `"error.server_running"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BackupsDialog.xaml.cs:105` | `"Backups.FolderMissing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:39` | `"BanList.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:50` | `"BanList.TargetRequired"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:71` | `"BanList.AddFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:79` | `"BanList.StoppedRequired"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:98` | `"BanList.RemoveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/EngineStdioWindow.xaml.cs:212` | `EngineMonitor.StatusPaused / EngineMonitor.Status; {pid}, {visible}, {total}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/ScriptsPage.xaml.cs:43` | `"Scripts.ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:58` | `"Scripts.ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:80` | `"Scripts.ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:101` | `"Scripts.ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:145` | `"Scripts.OpenFolderFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:169` | `PermissionConsentTitleNamed; {name}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/ServerDialog.xaml.cs:126` | `CreateServer.ProviderAuthor; {author}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/ShopSearchPage.xaml.cs:113` | `Shop.ResultSummary; total, query` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/ShopWindow.xaml.cs:141` | `"Shop.KeyMissingTooltip"` | ImperativeException | `ShopSelector` owns imperatively created `SelectorBarItem` tooltips; a plain managed `ItemsSource` with an `{x:Bind}` template crashes at startup | Retain imperative lookup |

## Completed static XAML ownership

The 43 former `StaticXaml` lookups were removed. Root `Window` and custom
`TitleBar` elements use distinct UIDs; reusable dialogs live in the owning
XAML resource dictionary and C# now supplies only runtime content, names,
dependency choices, and enablement. This covers `MainWindow`, `HomePage`,
`ServerPage`, `ScriptsPage`, `ShopDetailPage`, `BanListDialog`,
`EngineStdioWindow`, `ScriptLogsWindow`, and `ShopWindow`.

Dynamic endpoint/install-step tooltips remain bindings. The only static
imperative exception remains `ShopWindow`'s selector tooltip for the documented
WinUI `ItemsSource`/`x:Bind` startup-crash constraint.

## Direct `LocalizationService` construction inventory

There are **14** direct constructions across **14** files.

| File:line | Classification | Ownership / justification | Final action |
|---|---|---|---|
| `Controls/ScriptEntryCard.xaml.cs:15` | RuntimeFormat | Retained remove-confirmation format lookup | Retain/inject localizer for runtime ownership |
| `Controls/Server/ServerHeaderPanel.xaml.cs:11` | RuntimeFormat | Runtime memory formatting | Retain/inject localizer for runtime ownership |
| `Controls/Server/ServerPlayersPanel.xaml.cs:12` | RuntimeFormat | Player counts, names, and dialog-title formatting | Retain/inject localizer for runtime ownership |
| `MainWindow.xaml.cs:84` | DynamicState | Shared by `MainViewModel` and runtime teaching tips | Retain/inject localizer for runtime ownership |
| `Views/BackupsDialog.xaml.cs:17` | DynamicState | Picker results and operation state | Retain/inject localizer for runtime ownership |
| `Views/BanListDialog.xaml.cs:15` | DynamicState | Validation and RPC result state | Retain/inject localizer for runtime ownership |
| `Views/EngineStdioWindow.xaml.cs:19` | RuntimeFormat | Live monitor status formatting | Retain/inject localizer for runtime ownership |
| `Views/ScriptsPage.xaml.cs:21` | DynamicState | Script errors and named consent title | Retain/inject localizer for runtime ownership |
| `Views/ServerDialog.xaml.cs:20` | RuntimeFormat | Provider author text | Retain/inject localizer for runtime ownership |
| `Views/ServerPage.xaml.cs:19` | DynamicState | Passed into server-scoped view models for live state and RPC errors | Retain/inject localizer for runtime ownership |
| `Views/SettingsPage.xaml.cs:17` | DynamicState | Injected into `SettingsViewModel` for live state | Retain/inject localizer for runtime ownership |
| `Views/ShopDetailPage.xaml.cs:15` | DynamicState | Passed into `ShopDetailViewModel` for live load/install state | Retain/inject localizer for runtime ownership |
| `Views/ShopSearchPage.xaml.cs:22` | RuntimeFormat | Runtime result count/query summary | Retain/inject localizer for runtime ownership |
| `Views/ShopWindow.xaml.cs:67` | ImperativeException | Shared with `ShopViewModel`; also required by the imperative `ShopSelector` tooltip whose XAML `ItemsSource`/`x:Bind` alternative crashes at startup | Retain/inject localizer for runtime ownership |

## Pass-through and localization-boundary uses

These files are returned by the mandated `LocalizationService|ILocalizer|\.Get\(` search but do not own an additional lookup occurrence. Each passes the dependency to an owner already classified above or defines the lookup boundary.

| File:line | Use | Downstream classified ownership |
|---|---|---|
| `Models/ProviderItem.cs:9` | Passes `ILocalizer` to `ScriptEntryItem` | `ScriptEntryItem.cs:14` → `PermissionLabels.cs:25` |
| `Models/ScriptEntryItem.cs:14` | Builds the granted-permission summary | `PermissionLabels.cs:25` (`DynamicState`) |
| `Models/ScriptItem.cs:9` | Passes `ILocalizer` to `ScriptEntryItem` | `ScriptEntryItem.cs:14` → `PermissionLabels.cs:25` |
| `Models/ServerViewModelCache.cs:25` | Passes `ILocalizer` into server-scoped view models | `ModsViewModel.cs` and `ServerConfigViewModel.cs` rows |
| `Services/ILocalizer.cs:8` | Defines the programmatic lookup contract | All 128 classified `.Get` occurrences |
| `Services/LocalizationService.cs:6,10` | Implements the MRT lookup boundary | All 128 `.Get` occurrences; slash-path policy is covered by `ResourcePolicyTests` |
| `ViewModels/NavShellViewModel.cs:38` | Passes `ILocalizer` into `ServerViewModelCache` | `ServerViewModelCache.cs:25` → server-scoped view models |
| `Views/PermissionConsentDialog.xaml.cs:13,38` | Passes `ILocalizer` through `ConsentRow` | `PermissionLabels.cs:25` (`DynamicState`) |

## Classification rules

- **StaticXaml**: fixed copy owned by a named XAML element/property; migrate with `x:Uid`.
- **DynamicState**: runtime state, validation, or an operation result selects the resource.
- **BackendKey**: the semantic identifier arrives from the engine/provider payload.
- **RuntimeFormat**: runtime values must be inserted into localized copy.
- **ImperativeException**: static copy that must remain imperative for a documented lifecycle constraint. The baseline contains one: the `ShopSelector` tooltip, because the XAML `ItemsSource`/`x:Bind` alternative crashes at startup.
