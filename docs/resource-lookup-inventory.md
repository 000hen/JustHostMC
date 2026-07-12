# Resource lookup inventory

Baseline captured 2026-07-13. The baseline is **186** `ILocalizer.Get` occurrences across **32** files. Direct construction is tracked separately so construction-only owners such as `SettingsPage.xaml.cs` are not lost.

| Classification | Count |
|---|---:|
| StaticXaml | 56 |
| DynamicState | 99 |
| BackendKey | 5 |
| RuntimeFormat | 25 |
| ImperativeException | 1 |
| **Total lookups** | **186** |

## Lookup occurrences

| File:line | Resource expression | Classification | XAML owner / justification | Final action |
|---|---|---|---|---|
| `Controls/ScriptEntryCard.xaml.cs:32` | `"Scripts_RemoveConfirmBody"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/ScriptEntryCard.xaml.cs:35` | `"Scripts_RemoveConfirmPrimary"` | StaticXaml | `ScriptEntryCard.xaml` → `RemoveConfirmButton` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerConfigPanel.xaml.cs:26` | `"ServerSectionConfig/Text"` | StaticXaml | `ServerConfigPanel.xaml` → `ServerSectionConfig` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerConfigPanel.xaml.cs:29` | `"ServerSectionConfigHint/Text"` | StaticXaml | `ServerConfigPanel.xaml` → `ConfigActiveHint` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerConfigPanel.xaml.cs:30` | `"ConfigStoppedHint/Text"` | StaticXaml | `ServerConfigPanel.xaml` → `ConfigStoppedHint` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerHeaderPanel.xaml.cs:138` | `"Server_PortAutoValue"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerHeaderPanel.xaml.cs:142` | `Server_MemoryValue(memory)` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerHeaderPanel.xaml.cs:145` | `"Server_ValueUnknown"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerModsPanel.xaml.cs:38` | `"ServerSectionModsHint/Text"` | StaticXaml | `ServerModsPanel.xaml` → `ModsActiveHint` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerModsPanel.xaml.cs:39` | `"ModsStoppedHint/Text"` | StaticXaml | `ServerModsPanel.xaml` → `ModsStoppedHint` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerPerformancePanel.xaml.cs:27` | `"ServerSectionPerformance/Text"` | StaticXaml | `ServerPerformancePanel.xaml` → `ServerSectionPerformance` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerPerformancePanel.xaml.cs:30` | `"ServerSectionPerformanceHint/Text"` | StaticXaml | `ServerPerformancePanel.xaml` → `ServerSectionPerformance` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:56` | `"Players_Header"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:59` | `"PlayersEmptyHint/Text"` | StaticXaml | `ServerPlayersPanel.xaml` → `PlayersEmptyHint` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:60` | `"ServerSectionPlayersHint/Text"` | StaticXaml | `ServerPlayersPanel.xaml` → `PlayersPopulatedHint` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:87` | `"PlayerDataDialog_ActionName"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:99` | `"PlayerInventoryDialog_ActionName"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:113` | `PlayerDialogBase_TitleFormat(actionName, player.Name)` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:117` | `"PlayerDialogBase_CloseButtonText"` | StaticXaml | `ServerPlayersPanel.xaml` → `PlayerDialogHost` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:135` | `"BanListDialog_Title"` | StaticXaml | `ServerPlayersPanel.xaml` → `BanListHostDialog` | Move property to `x:Uid`; remove lookup |
| `Controls/Server/ServerPlayersPanel.xaml.cs:137` | `"BanListDialog_CloseButtonText"` | StaticXaml | `ServerPlayersPanel.xaml` → `BanListHostDialog` | Move property to `x:Uid`; remove lookup |
| `MainWindow.xaml.cs:96` | `"AppTitle"` | StaticXaml | `MainWindow.xaml` → root `MainWindow` | Move property to `x:Uid`; remove lookup |
| `MainWindow.xaml.cs:312` | `"ServerTeachingTip_StartAction"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `MainWindow.xaml.cs:320` | `ServerTeachingTip_InstalledTitle / StartedTitle / StoppedTitle / CrashedTitle; {server}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `MainWindow.xaml.cs:331` | `ServerTeachingTip_InstalledMessage / StartedMessage / StoppedMessage / CrashedMessage` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `MainWindow.xaml.cs:431` | `"CreateServerDialog_Title"` | StaticXaml | `MainWindow.xaml` → `CreateServerDialog` | Move property to `x:Uid`; remove lookup |
| `MainWindow.xaml.cs:434` | `"CreateServerDialog_PrimaryButtonText"` | StaticXaml | `MainWindow.xaml` → `CreateServerDialog` | Move property to `x:Uid`; remove lookup |
| `MainWindow.xaml.cs:436` | `"CreateServerDialog_CloseButtonText"` | StaticXaml | `MainWindow.xaml` → `CreateServerDialog` | Move property to `x:Uid`; remove lookup |
| `Models/BanEntryItem.cs:19` | `BanList_TypeIp / BanList_TypePlayer` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/BanEntryItem.cs:24` | `"BanList_NoReason"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ConfigEntryItem.cs:80` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ModFileItem.cs:19` | `"Mods_ParseFailed"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ModFileItem.cs:34` | `Mods_TypeAndVersionMismatch; {loader}, {version}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ModFileItem.cs:38` | `"Mods_TypeMismatch"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ModFileItem.cs:39` | `Mods_VersionMismatch; {version}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ParserItem.cs:28` | `Parsers_Formats; {formats}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/PermissionLabels.cs:25` | `LabelKey(kind)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:114` | `ServerStatus_Installing / Starting / Running / Stopping / Stopped / Crashed / Unknown` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:136` | `ServerNav_StateChangedAutomationName / ServerNav_AutomationName; {name}, {status}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ServerItem.cs:148` | `"ServerType_Vanilla"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:149` | `"ServerType_Paper"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:150` | `"ServerType_Spigot"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:151` | `"ServerType_Forge"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:152` | `"ServerType_NeoForge"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:153` | `"ServerType_Fabric"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:154` | `"ServerType_Unknown"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:172` | `"Server_PortLabel"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Models/ServerItem.cs:174` | `"Server_PortAutoValue"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Models/ServerItem.cs:175` | `ServerState_Stop / ServerState_Starting / ServerState_Stopping / ServerState_Installing / ServerState_Start` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:67` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:75` | `"Backups_Creating"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:84` | `"Backups_Created"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:86` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:98` | `"Backups_Restoring"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:106` | `"Backups_Restored"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:108` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:120` | `"Backups_Deleting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:127` | `"Backups_Deleted"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/BackupsViewModel.cs:129` | `MapBackupError(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:91` | `"EngineStatus_Connecting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:97` | `"EngineStatus_Connecting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:103` | `"EngineStatus_Connected"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:113` | `"EngineStatus_Failed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:144` | `"CreateServer_DefaultName"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:184` | `"install_progress_preparing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:191` | `"install_progress_preparing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:203` | `step.Key` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/MainViewModel.cs:220` | `"install_progress_done"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:222` | `"install_ready_to_run"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:231` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:232` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:307` | `"ServerState_Starting"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:330` | `"ServerState_Stopping"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/MainViewModel.cs:359` | `step.Key` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/MainViewModel.cs:394` | `ServerState_* selected by proto.Status` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:178` | `Mods_KindMod / Mods_KindPlugin` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:321` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:337` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:393` | `"Mods_ExportDone"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:395` | `"Mods_ExportFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:406` | `Mods_OperationFailedDetail; {summary}, {code}, {detail}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `ViewModels/ModsViewModel.cs:408` | `MapErrorKey(ex)` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ModsViewModel.cs:414` | `Mods_OperationFailedDetail; {summary}, {code}, {detail}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `ViewModels/ModsViewModel.cs:415` | `"Mods_OperationFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
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
| `ViewModels/ScriptsViewModel.cs:322` | `"Scripts_SystemLogName"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:331` | `"Scripts_LogEntryFallbackTitle"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ScriptsViewModel.cs:366` | `"Scripts_CurrentSessionTitle"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:133` | `"Config_LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:155` | `"Config_Saved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:161` | `"Config_SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:184` | `"Config_Saved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ServerConfigViewModel.cs:190` | `"Config_SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:108` | `"Settings_LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:128` | `status` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:146` | `"Settings_Saved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:151` | `"Settings_SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:161` | `Backend_Mode_Docker / Backend_Mode_OnMachine` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:166` | `Backend_DockerAvailable(version)` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `ViewModels/SettingsViewModel.cs:168` | `"Backend_DockerUnavailable"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:185` | `"Backend_DockerPrefSaved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:188` | `"Settings_SaveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:228` | `Settings_PurgeResult; {count}, {size}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `ViewModels/SettingsViewModel.cs:234` | `"Settings_PurgeFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:244` | `"Settings_RemovingData"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:251` | `"Settings_DataRemoved"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/SettingsViewModel.cs:254` | `"Settings_RemoveDataFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:123` | `"Shop_LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:156` | `"Shop_LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:215` | `"Shop_InstallDone"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:220` | `key` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopDetailViewModel.cs:223` | `"Shop_InstallFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopViewModel.cs:106` | `$"shop.category.{id}"` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/ShopViewModel.cs:150` | `"Shop_LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopViewModel.cs:181` | `s.Title.Key` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/ShopViewModel.cs:196` | `"Shop_LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `ViewModels/ShopViewModel.cs:208` | `descriptionKey` | BackendKey | Identifier arrives from a backend/provider payload; static XAML cannot select it | Retain dynamic lookup |
| `ViewModels/ShopViewModel.cs:252` | `"Shop_LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BackupsDialog.xaml.cs:46` | `"Backups_ExportSourceMissing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BackupsDialog.xaml.cs:66` | `"Backups_Exported"` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/BackupsDialog.xaml.cs:69` | `"Backups_ExportFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BackupsDialog.xaml.cs:81` | `"error.server_running"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BackupsDialog.xaml.cs:105` | `"Backups_FolderMissing"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:25` | `"BanListStoppedNotice_Title"` | StaticXaml | `BanListDialog.xaml` → `BanListStoppedNotice` | Move property to `x:Uid`; remove lookup |
| `Views/BanListDialog.xaml.cs:26` | `"BanListStoppedNotice_Message"` | StaticXaml | `BanListDialog.xaml` → `BanListStoppedNotice` | Move property to `x:Uid`; remove lookup |
| `Views/BanListDialog.xaml.cs:44` | `"BanList_LoadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:55` | `"BanList_TargetRequired"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:76` | `"BanList_AddFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:84` | `"BanList_StoppedRequired"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/BanListDialog.xaml.cs:103` | `"BanList_RemoveFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/EngineStdioWindow.xaml.cs:42` | `"EngineMonitor_Title"` | StaticXaml | `EngineStdioWindow.xaml` → root `EngineStdioWindow` and `MonitorTitleBar` | Move property to `x:Uid`; remove lookup |
| `Views/EngineStdioWindow.xaml.cs:215` | `EngineMonitor_StatusPaused / EngineMonitor_Status; {pid}, {visible}, {total}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/HomePage.xaml.cs:101` | `"ServerDelete_Title"` | StaticXaml | `HomePage.xaml` → `DeleteServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:102` | `"ServerDelete_Body"` | StaticXaml | `HomePage.xaml` → `DeleteServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:103` | `"ServerDelete_Confirm"` | StaticXaml | `HomePage.xaml` → `DeleteServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:104` | `"Common_Cancel"` | StaticXaml | `HomePage.xaml` → `DeleteServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:126` | `"CreateServerDialog_Title"` | StaticXaml | `HomePage.xaml` → `CreateServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:129` | `"CreateServerDialog_PrimaryButtonText"` | StaticXaml | `HomePage.xaml` → `CreateServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:131` | `"CreateServerDialog_CloseButtonText"` | StaticXaml | `HomePage.xaml` → `CreateServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:154` | `"EditServerDialog_Title"` | StaticXaml | `HomePage.xaml` → `EditServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:157` | `"EditServerDialog_PrimaryButtonText"` | StaticXaml | `HomePage.xaml` → `EditServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:159` | `"EditServerDialog_CloseButtonText"` | StaticXaml | `HomePage.xaml` → `EditServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:174` | `"EditServerName_Header"` | StaticXaml | `HomePage.xaml` → `RenameServerNameBox` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:180` | `"RenameServerDialog_Title"` | StaticXaml | `HomePage.xaml` → `RenameServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:182` | `"Common_Save"` | StaticXaml | `HomePage.xaml` → `RenameServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/HomePage.xaml.cs:183` | `"Common_Cancel"` | StaticXaml | `HomePage.xaml` → `RenameServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ScriptLogsWindow.xaml.cs:23` | `"ScriptLogsWindow_Title"` | StaticXaml | `ScriptLogsWindow.xaml` → root `ScriptLogsWindow` and `LogsTitleBar` | Move property to `x:Uid`; remove lookup |
| `Views/ScriptsPage.xaml.cs:43` | `"Scripts_ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:58` | `"Scripts_ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:80` | `"Scripts_ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:101` | `"Scripts_ReadFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:145` | `"Scripts_OpenFolderFailed"` | DynamicState | Live model, command, validation, or RPC state selects the displayed resource | Retain state-dependent lookup |
| `Views/ScriptsPage.xaml.cs:171` | `PermissionConsentTitleNamed; {name}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/ScriptsPage.xaml.cs:175` | `"PermissionConsentDialog_PrimaryButtonText"` | StaticXaml | `ScriptsPage.xaml` → `PermissionConsentHostDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ScriptsPage.xaml.cs:177` | `"PermissionConsentDialog_CloseButtonText"` | StaticXaml | `ScriptsPage.xaml` → `PermissionConsentHostDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerDialog.xaml.cs:126` | `CreateServer_ProviderAuthor; {author}` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/ServerPage.xaml.cs:157` | `"BackupsDialog_CloseButtonText"` | StaticXaml | `ServerPage.xaml` → `BackupsHostDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:177` | `"EditServerDialog_Title"` | StaticXaml | `ServerPage.xaml` → `EditServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:180` | `"EditServerDialog_PrimaryButtonText"` | StaticXaml | `ServerPage.xaml` → `EditServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:182` | `"EditServerDialog_CloseButtonText"` | StaticXaml | `ServerPage.xaml` → `EditServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:197` | `"EditServerName_Header"` | StaticXaml | `ServerPage.xaml` → `RenameServerNameBox` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:203` | `"RenameServerDialog_Title"` | StaticXaml | `ServerPage.xaml` → `RenameServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:205` | `"Common_Save"` | StaticXaml | `ServerPage.xaml` → `RenameServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:206` | `"Common_Cancel"` | StaticXaml | `ServerPage.xaml` → `RenameServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:219` | `"ServerFolder_NotFoundTitle"` | StaticXaml | `ServerPage.xaml` → `ServerFolderMissingDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:220` | `"ServerFolder_NotFoundBody"` | StaticXaml | `ServerPage.xaml` → `ServerFolderMissingDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:275` | `"ServerDelete_Title"` | StaticXaml | `ServerPage.xaml` → `DeleteServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:276` | `"ServerDelete_Body"` | StaticXaml | `ServerPage.xaml` → `DeleteServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:277` | `"ServerDelete_Confirm"` | StaticXaml | `ServerPage.xaml` → `DeleteServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ServerPage.xaml.cs:278` | `"Common_Cancel"` | StaticXaml | `ServerPage.xaml` → `DeleteServerDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ShopDetailPage.xaml.cs:106` | `"Shop_DependencyPromptBody"` | StaticXaml | `ShopDetailPage.xaml` → `DependencyPromptBody` | Move property to `x:Uid`; remove lookup |
| `Views/ShopDetailPage.xaml.cs:113` | `"Shop_DependencyPromptTitle"` | StaticXaml | `ShopDetailPage.xaml` → `DependencyPromptDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ShopDetailPage.xaml.cs:115` | `"Shop_InstallConfirm"` | StaticXaml | `ShopDetailPage.xaml` → `DependencyPromptDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ShopDetailPage.xaml.cs:116` | `"Common_Cancel"` | StaticXaml | `ShopDetailPage.xaml` → `DependencyPromptDialog` | Move property to `x:Uid`; remove lookup |
| `Views/ShopSearchPage.xaml.cs:113` | `Shop_ResultSummary; total, query` | RuntimeFormat | Runtime values or runtime-selected arguments must be inserted after lookup | Retain formatted lookup |
| `Views/ShopWindow.xaml.cs:81` | `"ShopWindow_Title"` | StaticXaml | `ShopWindow.xaml` → root `ShopWindow` and `ShopTitleBar` | Move property to `x:Uid`; remove lookup |
| `Views/ShopWindow.xaml.cs:144` | `"Shop_KeyMissingTooltip"` | ImperativeException | `ShopSelector` owns imperatively created `SelectorBarItem` tooltips; a plain managed `ItemsSource` with an `{x:Bind}` template crashes at startup | Retain imperative lookup |

## Direct `LocalizationService` construction inventory

There are **20** direct constructions across **19** files. The chained construction at `ScriptLogsWindow.xaml.cs:23` is also represented above.

| File:line | Classification | Ownership / justification | Final action |
|---|---|---|---|
| `Controls/ScriptEntryCard.xaml.cs:15` | RuntimeFormat | Retained remove-confirmation format lookup | Retain/inject localizer for runtime ownership |
| `Controls/Server/ServerConfigPanel.xaml.cs:9` | StaticXaml | All copy moves to `ServerConfigPanel.xaml` | Remove construction |
| `Controls/Server/ServerHeaderPanel.xaml.cs:11` | RuntimeFormat | Runtime port and memory values | Retain/inject localizer for runtime ownership |
| `Controls/Server/ServerModsPanel.xaml.cs:12` | StaticXaml | Descriptions move to `ServerModsPanel.xaml` | Remove construction |
| `Controls/Server/ServerPerformancePanel.xaml.cs:9` | StaticXaml | Title and description move to `ServerPerformancePanel.xaml` | Remove construction |
| `Controls/Server/ServerPlayersPanel.xaml.cs:12` | RuntimeFormat | Player counts, names, and dialog-title formatting | Retain/inject localizer for runtime ownership |
| `MainWindow.xaml.cs:84` | DynamicState | Shared by `MainViewModel` and runtime teaching tips | Retain/inject localizer for runtime ownership |
| `MainWindow.xaml.cs:425` | StaticXaml | Duplicate used only by `CreateServerDialog` | Remove construction |
| `Views/BackupsDialog.xaml.cs:17` | DynamicState | Picker results and operation state | Retain/inject localizer for runtime ownership |
| `Views/BanListDialog.xaml.cs:15` | DynamicState | Validation and RPC result state | Retain/inject localizer for runtime ownership |
| `Views/EngineStdioWindow.xaml.cs:19` | RuntimeFormat | Live monitor status formatting | Retain/inject localizer for runtime ownership |
| `Views/HomePage.xaml.cs:18` | StaticXaml | Current uses are XAML-ownable dialog chrome | Remove construction |
| `Views/ScriptLogsWindow.xaml.cs:23` | StaticXaml | Chained construction supplies only window title | Remove construction |
| `Views/ScriptsPage.xaml.cs:21` | DynamicState | Script errors and named consent title | Retain/inject localizer for runtime ownership |
| `Views/ServerDialog.xaml.cs:20` | RuntimeFormat | Provider author text | Retain/inject localizer for runtime ownership |
| `Views/ServerPage.xaml.cs:19` | StaticXaml | Current uses are XAML-ownable dialog chrome | Remove construction |
| `Views/SettingsPage.xaml.cs:17` | DynamicState | Injected into `SettingsViewModel` for live state | Retain/inject localizer for runtime ownership |
| `Views/ShopDetailPage.xaml.cs:15` | StaticXaml | Current uses are dependency-dialog static copy | Remove construction |
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
| `Services/ILocalizer.cs:8` | Defines the programmatic lookup contract | All 186 classified `.Get` occurrences |
| `Services/LocalizationService.cs:6,10` | Implements the MRT lookup boundary | All 186 `.Get` occurrences; slash-path policy is covered by `ResourcePolicyTests` |
| `ViewModels/NavShellViewModel.cs:38` | Passes `ILocalizer` into `ServerViewModelCache` | `ServerViewModelCache.cs:25` → server-scoped view models |
| `Views/PermissionConsentDialog.xaml.cs:13,38` | Passes `ILocalizer` through `ConsentRow` | `PermissionLabels.cs:25` (`DynamicState`) |

## Classification rules

- **StaticXaml**: fixed copy owned by a named XAML element/property; migrate with `x:Uid`.
- **DynamicState**: runtime state, validation, or an operation result selects the resource.
- **BackendKey**: the semantic identifier arrives from the engine/provider payload.
- **RuntimeFormat**: runtime values must be inserted into localized copy.
- **ImperativeException**: static copy that must remain imperative for a documented lifecycle constraint. The baseline contains one: the `ShopSelector` tooltip, because the XAML `ItemsSource`/`x:Bind` alternative crashes at startup.
