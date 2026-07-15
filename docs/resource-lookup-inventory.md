# Resource lookup inventory

Finalized 2026-07-15 against the exact Task 1 baseline committed at
`c0e947b`. One occurrence means one syntactic call to `ILocalizer.Get`, including
nested and multiline calls; direct construction and dependency pass-through are
tracked separately. The current source audit found 127 calls containing
`localizer.Get(` on one line plus the split fluent call in
`Models/ServerItem.cs:172`, for **128** final lookups.

The design's earlier graph estimate is not the baseline. The auditable baseline
is the 186-row inventory in `c0e947b:docs/resource-lookup-inventory.md`.

| Summary metric | Count |
|---|---:|
| Exact Task 1 baseline lookups | 186 |
| Final dynamic lookups (`DynamicState` + `BackendKey` + `RuntimeFormat`) | 127 |
| Removed static/XAML-ownable lookups | 58 |
| Imperative exceptions within the final total | 1 |
| **Final lookup total** | **128** |

The removed total consists of all 56 baseline `StaticXaml` rows plus two
baseline `RuntimeFormat` rows (`Server_PortAutoValue` and
`Server_ValueUnknown`) that final review found were static and moved to XAML.

| Final classification | Count |
|---|---:|
| DynamicState | 99 |
| BackendKey | 5 |
| RuntimeFormat | 23 |
| ImperativeException | 1 |
| **Total lookups** | **128** |

## Retained lookup occurrences

| File:line | Resource expression | Classification | XAML owner / justification | Final status |
|---|---|---|---|---|
| `Controls/ScriptEntryCard.xaml.cs:32` | `"Scripts.RemoveConfirmBody"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Controls/Server/ServerHeaderPanel.xaml.cs:153` | `Server.MemoryValue(memory)` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Controls/Server/ServerPlayersPanel.xaml.cs:56` | `"Players.Header"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Controls/Server/ServerPlayersPanel.xaml.cs:87` | `"PlayerDataDialog.ActionName"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Controls/Server/ServerPlayersPanel.xaml.cs:99` | `"PlayerInventoryDialog.ActionName"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Controls/Server/ServerPlayersPanel.xaml.cs:113` | `PlayerDialogBase.TitleFormat(actionName, player.Name)` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `MainWindow.xaml.cs:311` | `"ServerTeachingTip.StartAction"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `MainWindow.xaml.cs:319` | `ServerTeachingTip.InstalledTitle / StartedTitle / StoppedTitle / CrashedTitle; {server}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `MainWindow.xaml.cs:330` | `ServerTeachingTip.InstalledMessage / StartedMessage / StoppedMessage / CrashedMessage` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/BanEntryItem.cs:19` | `BanList.TypeIp / BanList.TypePlayer` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/BanEntryItem.cs:24` | `"BanList.NoReason"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ConfigEntryItem.cs:80` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ModFileItem.cs:19` | `"Mods.ParseFailed"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Models/ModFileItem.cs:34` | `Mods.TypeAndVersionMismatch; {loader}, {version}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Models/ModFileItem.cs:38` | `"Mods.TypeMismatch"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Models/ModFileItem.cs:39` | `Mods.VersionMismatch; {version}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Models/ParserItem.cs:28` | `Parsers.Formats; {formats}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Models/PermissionLabels.cs:25` | `LabelKey(kind)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:114` | `ServerStatus.Installing / Starting / Running / Stopping / Stopped / Crashed / Unknown` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:136` | `ServerNav.StateChangedAutomationName / ServerNav.AutomationName; {name}, {status}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Models/ServerItem.cs:148` | `"ServerType.Vanilla"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:149` | `"ServerType.Paper"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:150` | `"ServerType.Spigot"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:151` | `"ServerType.Forge"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:152` | `"ServerType.NeoForge"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:153` | `"ServerType.Fabric"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:154` | `"ServerType.Unknown"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:172` | `"Server.PortLabel"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Models/ServerItem.cs:174` | `"Server.PortAutoValue"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Models/ServerItem.cs:175` | `ServerState.Stop / ServerState.Starting / ServerState.Stopping / ServerState.Installing / ServerState.Start` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:67` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:75` | `"Backups.Creating"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:84` | `"Backups.Created"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:86` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:98` | `"Backups.Restoring"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:106` | `"Backups.Restored"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:108` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:120` | `"Backups.Deleting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:127` | `"Backups.Deleted"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/BackupsViewModel.cs:129` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:91` | `"EngineStatus.Connecting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:97` | `"EngineStatus.Connecting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:103` | `"EngineStatus.Connected"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:113` | `"EngineStatus.Failed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:144` | `"CreateServer.DefaultName"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:184` | `"install.progress.preparing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:191` | `"install.progress.preparing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:203` | `step.Key` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retained — backend or provider supplies the resource key |
| `ViewModels/MainViewModel.cs:220` | `"install.progress.done"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:222` | `"install.ready.to.run"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:231` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:232` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:307` | `"ServerState.Starting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:330` | `"ServerState.Stopping"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/MainViewModel.cs:359` | `step.Key` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retained — backend or provider supplies the resource key |
| `ViewModels/MainViewModel.cs:394` | `ServerState.Installing / ServerState.Starting / ServerState.Stopping` selected by `proto.Status` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ModsViewModel.cs:185` | `Mods.KindMods / Mods.KindPlugins` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ModsViewModel.cs:328` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ModsViewModel.cs:344` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ModsViewModel.cs:400` | `"Mods.ExportDone"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ModsViewModel.cs:402` | `"Mods.ExportFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ModsViewModel.cs:413` | `Mods.OperationFailedDetail; {summary}, {code}, {detail}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `ViewModels/ModsViewModel.cs:415` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ModsViewModel.cs:421` | `Mods.OperationFailedDetail; {summary}, {code}, {detail}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `ViewModels/ModsViewModel.cs:422` | `"Mods.OperationFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:101` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:120` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:134` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:162` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:183` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:204` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:220` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:233` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:243` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:253` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:322` | `"Scripts.SystemLogName"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:331` | `"Scripts.LogEntryFallbackTitle"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ScriptsViewModel.cs:366` | `"Scripts.CurrentSessionTitle"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ServerConfigViewModel.cs:133` | `"Config.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ServerConfigViewModel.cs:155` | `"Config.Saved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ServerConfigViewModel.cs:161` | `"Config.SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ServerConfigViewModel.cs:184` | `"Config.Saved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ServerConfigViewModel.cs:190` | `"Config.SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:108` | `"Settings.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:128` | `status` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:146` | `"Settings.Saved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:151` | `"Settings.SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:161` | `Backend.Mode.Docker / Backend.Mode.OnMachine` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:166` | `Backend.DockerAvailable(version)` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `ViewModels/SettingsViewModel.cs:168` | `"Backend.DockerUnavailable"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:185` | `"Backend.DockerPrefSaved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:188` | `"Settings.SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:228` | `Settings.PurgeResult; {count}, {size}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `ViewModels/SettingsViewModel.cs:234` | `"Settings.PurgeFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:244` | `"Settings.RemovingData"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:251` | `"Settings.DataRemoved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/SettingsViewModel.cs:254` | `"Settings.RemoveDataFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ShopDetailViewModel.cs:123` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ShopDetailViewModel.cs:156` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ShopDetailViewModel.cs:215` | `"Shop.InstallDone"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ShopDetailViewModel.cs:220` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ShopDetailViewModel.cs:223` | `"Shop.InstallFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ShopViewModel.cs:106` | `$"shop.category.{id}"` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retained — backend or provider supplies the resource key |
| `ViewModels/ShopViewModel.cs:150` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ShopViewModel.cs:181` | `s.Title.Key` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retained — backend or provider supplies the resource key |
| `ViewModels/ShopViewModel.cs:196` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `ViewModels/ShopViewModel.cs:208` | `descriptionKey` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retained — backend or provider supplies the resource key |
| `ViewModels/ShopViewModel.cs:252` | `"Shop.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/BackupsDialog.xaml.cs:46` | `"Backups.ExportSourceMissing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/BackupsDialog.xaml.cs:66` | `"Backups.Exported"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Views/BackupsDialog.xaml.cs:69` | `"Backups.ExportFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/BackupsDialog.xaml.cs:81` | `"error.server_running"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/BackupsDialog.xaml.cs:105` | `"Backups.FolderMissing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/BanListDialog.xaml.cs:39` | `"BanList.LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/BanListDialog.xaml.cs:50` | `"BanList.TargetRequired"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/BanListDialog.xaml.cs:71` | `"BanList.AddFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/BanListDialog.xaml.cs:79` | `"BanList.StoppedRequired"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/BanListDialog.xaml.cs:98` | `"BanList.RemoveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/EngineStdioWindow.xaml.cs:212` | `EngineMonitor.StatusPaused / EngineMonitor.Status; {pid}, {visible}, {total}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Views/ScriptsPage.xaml.cs:43` | `"Scripts.ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/ScriptsPage.xaml.cs:58` | `"Scripts.ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/ScriptsPage.xaml.cs:80` | `"Scripts.ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/ScriptsPage.xaml.cs:101` | `"Scripts.ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/ScriptsPage.xaml.cs:145` | `"Scripts.OpenFolderFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retained — runtime state selects the resource |
| `Views/ScriptsPage.xaml.cs:169` | `PermissionConsentTitleNamed; {name}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Views/ServerDialog.xaml.cs:126` | `CreateServer.ProviderAuthor; {author}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Views/ShopSearchPage.xaml.cs:113` | `Shop.ResultSummary; total, query` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retained — runtime values require localized formatting |
| `Views/ShopWindow.xaml.cs:141` | `"Shop.KeyMissingTooltip"` | ImperativeException | `ShopSelector` owns imperatively created `SelectorBarItem` tooltips; a plain managed `ItemsSource` with an `{x:Bind}` template crashes at startup | Retained — documented imperative XAML-crash exception |

## Removed or moved lookup occurrences

These are the 58 baseline occurrences that no longer exist in C#. Baseline file
and line references intentionally point to `c0e947b`; every row has a final
status and an owning XAML resource.

| Baseline file:line | Resource expression | Baseline classification | Final status |
|---|---|---|---|
| `Controls/ScriptEntryCard.xaml.cs:35` | `Scripts_RemoveConfirmPrimary` | StaticXaml | Moved to XAML — `CommonRemoveButton.Content` |
| `Controls/Server/ServerConfigPanel.xaml.cs:26` | `ServerSectionConfig/Text` | StaticXaml | Moved to XAML — `ServerSectionConfigPanel.Title` |
| `Controls/Server/ServerConfigPanel.xaml.cs:29` | `ServerSectionConfigHint/Text` | StaticXaml | Moved to XAML — `ServerSectionConfigHint.Text` |
| `Controls/Server/ServerConfigPanel.xaml.cs:30` | `ConfigStoppedHint/Text` | StaticXaml | Moved to XAML — `ConfigStoppedHint.Text` |
| `Controls/Server/ServerHeaderPanel.xaml.cs:138` | `Server_PortAutoValue` | RuntimeFormat | Moved to XAML — `ServerMetaPortAutoValue.Text` |
| `Controls/Server/ServerHeaderPanel.xaml.cs:145` | `Server_ValueUnknown` | RuntimeFormat | Moved to XAML — `ServerMetaMemoryUnknownValue.Text` |
| `Controls/Server/ServerModsPanel.xaml.cs:38` | `ServerSectionModsHint/Text` | StaticXaml | Moved to XAML — `ServerSectionModsHint.Text` |
| `Controls/Server/ServerModsPanel.xaml.cs:39` | `ModsStoppedHint/Text` | StaticXaml | Moved to XAML — `ModsStoppedHint.Text` |
| `Controls/Server/ServerPerformancePanel.xaml.cs:27` | `ServerSectionPerformance/Text` | StaticXaml | Moved to XAML — `ServerSectionPerformancePanel.Title` |
| `Controls/Server/ServerPerformancePanel.xaml.cs:30` | `ServerSectionPerformanceHint/Text` | StaticXaml | Moved to XAML — `ServerSectionPerformancePanel.Description` |
| `Controls/Server/ServerPlayersPanel.xaml.cs:59` | `PlayersEmptyHint/Text` | StaticXaml | Moved to XAML — `PlayersEmptyHint.Text` |
| `Controls/Server/ServerPlayersPanel.xaml.cs:60` | `ServerSectionPlayersHint/Text` | StaticXaml | Moved to XAML — `ServerSectionPlayersHint.Text` |
| `Controls/Server/ServerPlayersPanel.xaml.cs:117` | `PlayerDialogBase_CloseButtonText` | StaticXaml | Moved to XAML — `PlayerDialogHost.CloseButtonText` |
| `Controls/Server/ServerPlayersPanel.xaml.cs:135` | `BanListDialog_Title` | StaticXaml | Moved to XAML — `BanListHostDialog.Title` |
| `Controls/Server/ServerPlayersPanel.xaml.cs:137` | `BanListDialog_CloseButtonText` | StaticXaml | Moved to XAML — `BanListHostDialog.CloseButtonText` |
| `MainWindow.xaml.cs:96` | `AppTitle` | StaticXaml | Removed from native Window — `MainWindow.Title` x:Uid assignment crashes `MainWindow.InitializeComponent`; the custom `MainWindowTitleBar.Title` owns the visible title |
| `MainWindow.xaml.cs:431` | `CreateServerDialog_Title` | StaticXaml | Moved to XAML — `CreateServerDialog.Title` |
| `MainWindow.xaml.cs:434` | `CreateServerDialog_PrimaryButtonText` | StaticXaml | Moved to XAML — `CreateServerDialog.PrimaryButtonText` |
| `MainWindow.xaml.cs:436` | `CreateServerDialog_CloseButtonText` | StaticXaml | Moved to XAML — `CreateServerDialog.CloseButtonText` |
| `Views/BanListDialog.xaml.cs:25` | `BanListStoppedNotice_Title` | StaticXaml | Moved to XAML — `BanListStoppedNotice.Title` |
| `Views/BanListDialog.xaml.cs:26` | `BanListStoppedNotice_Message` | StaticXaml | Moved to XAML — `BanListStoppedNotice.Message` |
| `Views/EngineStdioWindow.xaml.cs:42` | `EngineMonitor_Title` | StaticXaml | Removed from native Window — `EngineStdioWindow.Title` x:Uid assignment crashes `InitializeComponent`; the custom `EngineMonitorTitleBar.Title` owns the visible title |
| `Views/HomePage.xaml.cs:101` | `ServerDelete_Title` | StaticXaml | Moved to XAML — `DeleteServerDialog.Title` |
| `Views/HomePage.xaml.cs:102` | `ServerDelete_Body` | StaticXaml | Moved to XAML — `DeleteServerDialog.Content` |
| `Views/HomePage.xaml.cs:103` | `ServerDelete_Confirm` | StaticXaml | Moved to XAML — `DeleteServerDialog.PrimaryButtonText` |
| `Views/HomePage.xaml.cs:104` | `Common_Cancel` | StaticXaml | Moved to XAML — `DeleteServerDialog.CloseButtonText` |
| `Views/HomePage.xaml.cs:126` | `CreateServerDialog_Title` | StaticXaml | Moved to XAML — `CreateServerDialog.Title` |
| `Views/HomePage.xaml.cs:129` | `CreateServerDialog_PrimaryButtonText` | StaticXaml | Moved to XAML — `CreateServerDialog.PrimaryButtonText` |
| `Views/HomePage.xaml.cs:131` | `CreateServerDialog_CloseButtonText` | StaticXaml | Moved to XAML — `CreateServerDialog.CloseButtonText` |
| `Views/HomePage.xaml.cs:154` | `EditServerDialog_Title` | StaticXaml | Moved to XAML — `EditServerDialog.Title` |
| `Views/HomePage.xaml.cs:157` | `EditServerDialog_PrimaryButtonText` | StaticXaml | Moved to XAML — `EditServerDialog.PrimaryButtonText` |
| `Views/HomePage.xaml.cs:159` | `EditServerDialog_CloseButtonText` | StaticXaml | Moved to XAML — `EditServerDialog.CloseButtonText` |
| `Views/HomePage.xaml.cs:174` | `EditServerName_Header` | StaticXaml | Moved to XAML — `RenameServerNameBox.Header` |
| `Views/HomePage.xaml.cs:180` | `RenameServerDialog_Title` | StaticXaml | Moved to XAML — `RenameServerDialog.Title` |
| `Views/HomePage.xaml.cs:182` | `Common_Save` | StaticXaml | Moved to XAML — `RenameServerDialog.PrimaryButtonText` |
| `Views/HomePage.xaml.cs:183` | `Common_Cancel` | StaticXaml | Moved to XAML — `RenameServerDialog.CloseButtonText` |
| `Views/ScriptLogsWindow.xaml.cs:23` | `ScriptLogsWindow_Title` | StaticXaml | Removed from native Window — `ScriptLogsWindow.Title` x:Uid assignment crashes `InitializeComponent`; the custom `ScriptLogsTitleBar.Title` owns the visible title |
| `Views/ScriptsPage.xaml.cs:175` | `PermissionConsentDialog_PrimaryButtonText` | StaticXaml | Moved to XAML — `PermissionConsentDialog.PrimaryButtonText` |
| `Views/ScriptsPage.xaml.cs:177` | `PermissionConsentDialog_CloseButtonText` | StaticXaml | Moved to XAML — `PermissionConsentDialog.CloseButtonText` |
| `Views/ServerPage.xaml.cs:157` | `BackupsDialog_CloseButtonText` | StaticXaml | Moved to XAML — `CommonCloseDialog.CloseButtonText` |
| `Views/ServerPage.xaml.cs:177` | `EditServerDialog_Title` | StaticXaml | Moved to XAML — `EditServerDialog.Title` |
| `Views/ServerPage.xaml.cs:180` | `EditServerDialog_PrimaryButtonText` | StaticXaml | Moved to XAML — `EditServerDialog.PrimaryButtonText` |
| `Views/ServerPage.xaml.cs:182` | `EditServerDialog_CloseButtonText` | StaticXaml | Moved to XAML — `EditServerDialog.CloseButtonText` |
| `Views/ServerPage.xaml.cs:197` | `EditServerName_Header` | StaticXaml | Moved to XAML — `RenameServerNameBox.Header` |
| `Views/ServerPage.xaml.cs:203` | `RenameServerDialog_Title` | StaticXaml | Moved to XAML — `RenameServerDialog.Title` |
| `Views/ServerPage.xaml.cs:205` | `Common_Save` | StaticXaml | Moved to XAML — `RenameServerDialog.PrimaryButtonText` |
| `Views/ServerPage.xaml.cs:206` | `Common_Cancel` | StaticXaml | Moved to XAML — `RenameServerDialog.CloseButtonText` |
| `Views/ServerPage.xaml.cs:219` | `ServerFolder_NotFoundTitle` | StaticXaml | Moved to XAML — `ServerFolderMissingDialog.Title` |
| `Views/ServerPage.xaml.cs:220` | `ServerFolder_NotFoundBody` | StaticXaml | Moved to XAML — `ServerFolderMissingDialog.Content` |
| `Views/ServerPage.xaml.cs:275` | `ServerDelete_Title` | StaticXaml | Moved to XAML — `DeleteServerDialog.Title` |
| `Views/ServerPage.xaml.cs:276` | `ServerDelete_Body` | StaticXaml | Moved to XAML — `DeleteServerDialog.Content` |
| `Views/ServerPage.xaml.cs:277` | `ServerDelete_Confirm` | StaticXaml | Moved to XAML — `DeleteServerDialog.PrimaryButtonText` |
| `Views/ServerPage.xaml.cs:278` | `Common_Cancel` | StaticXaml | Moved to XAML — `DeleteServerDialog.CloseButtonText` |
| `Views/ShopDetailPage.xaml.cs:106` | `Shop_DependencyPromptBody` | StaticXaml | Moved to XAML — `DependencyPromptBody.Text` |
| `Views/ShopDetailPage.xaml.cs:113` | `Shop_DependencyPromptTitle` | StaticXaml | Moved to XAML — `DependencyPromptDialog.Title` |
| `Views/ShopDetailPage.xaml.cs:115` | `Shop_InstallConfirm` | StaticXaml | Moved to XAML — `DependencyPromptDialog.PrimaryButtonText` |
| `Views/ShopDetailPage.xaml.cs:116` | `Common_Cancel` | StaticXaml | Moved to XAML — `DependencyPromptDialog.CloseButtonText` |
| `Views/ShopWindow.xaml.cs:81` | `ShopWindow_Title` | StaticXaml | Removed from native Window — `ShopWindow.Title` x:Uid assignment crashes `InitializeComponent`; the custom `ShopWindowTitleBar.Title` owns the visible title |

Dynamic endpoint/install-step tooltips remain bindings. The only static
imperative exception is `ShopWindow`'s selector tooltip for the documented
WinUI `ItemsSource`/`x:Bind` startup-crash constraint.

## Direct `LocalizationService` construction inventory

There are **14** direct constructions across **14** files.

| File:line | Classification | Ownership / justification | Final status |
|---|---|---|---|
| `Controls/ScriptEntryCard.xaml.cs:15` | RuntimeFormat | Retained remove-confirmation format lookup | Retained — supports classified runtime lookup ownership |
| `Controls/Server/ServerHeaderPanel.xaml.cs:11` | RuntimeFormat | Runtime memory formatting | Retained — supports classified runtime lookup ownership |
| `Controls/Server/ServerPlayersPanel.xaml.cs:12` | RuntimeFormat | Player counts, names, and dialog-title formatting | Retained — supports classified runtime lookup ownership |
| `MainWindow.xaml.cs:84` | DynamicState | Shared by `MainViewModel` and runtime teaching tips | Retained — supports classified runtime lookup ownership |
| `Views/BackupsDialog.xaml.cs:17` | DynamicState | Picker results and operation state | Retained — supports classified runtime lookup ownership |
| `Views/BanListDialog.xaml.cs:15` | DynamicState | Validation and RPC result state | Retained — supports classified runtime lookup ownership |
| `Views/EngineStdioWindow.xaml.cs:19` | RuntimeFormat | Live monitor status formatting | Retained — supports classified runtime lookup ownership |
| `Views/ScriptsPage.xaml.cs:21` | DynamicState | Script errors and named consent title | Retained — supports classified runtime lookup ownership |
| `Views/ServerDialog.xaml.cs:20` | RuntimeFormat | Provider author text | Retained — supports classified runtime lookup ownership |
| `Views/ServerPage.xaml.cs:19` | DynamicState | Passed into server-scoped view models for live state and RPC errors | Retained — supports classified runtime lookup ownership |
| `Views/SettingsPage.xaml.cs:17` | DynamicState | Injected into `SettingsViewModel` for live state | Retained — supports classified runtime lookup ownership |
| `Views/ShopDetailPage.xaml.cs:15` | DynamicState | Passed into `ShopDetailViewModel` for live load/install state | Retained — supports classified runtime lookup ownership |
| `Views/ShopSearchPage.xaml.cs:22` | RuntimeFormat | Runtime result count/query summary | Retained — supports classified runtime lookup ownership |
| `Views/ShopWindow.xaml.cs:67` | ImperativeException | Shared with `ShopViewModel`; also required by the imperative `ShopSelector` tooltip whose XAML `ItemsSource`/`x:Bind` alternative crashes at startup | Retained — supports dynamic ownership and the documented imperative exception |

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
