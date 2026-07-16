# Resources and localization

JustHostMC uses MRT Core resources for the WinUI application. The governing
rule is XAML first: static user-facing text belongs on the XAML element that
displays it, while C# localization is reserved for values that have no
declarative UI representation.

## Directory structure and languages

- `app/JustHostMC.App/Strings/en-US/Resources.resw` is the base language and
  fallback.
- `app/JustHostMC.App/Strings/zh-TW/Resources.resw` is Traditional Chinese for
  Taiwan.
- `app/JustHostMC.App/Assets/` contains packaged images and manifest artwork.
- `app/JustHostMC.App/App.xaml` contains application-wide XAML resources,
  including converters used by `x:Bind`.

`en-US` is the project `DefaultLanguage`. Both languages are declared in
`Package.appxmanifest` and in `AppxDefaultResourceQualifiers`. Packaged apps use
the user's preferred language matching; unpackaged Windows App SDK apps use the
system display language. Resolution falls back to `en-US`.

To add a language, create `Strings/<BCP-47 tag>/Resources.resw` with exactly the
same identifiers, add the language to the manifest and
`AppxDefaultResourceQualifiers`, run the resource tests, build, and exercise the
complete smoke matrix in that language.

## Static XAML localization

Put `x:Uid` on every XAML element with static visible or accessibility text.
The resource identifier is property-qualified:

```xaml
<Button x:Uid="CommonSaveButton" />
<TextBox x:Uid="ServerNameField" />
```

```xml
<data name="CommonSaveButton.Content" xml:space="preserve">
  <value>Save</value>
</data>
<data name="ServerNameField.Header" xml:space="preserve">
  <value>Name</value>
</data>
```

This applies to `Text`, `Content`, `Header`, `Title`, `PlaceholderText`, dialog
button properties, InfoBar text, menu items, empty states, tooltips, and
automation properties. Do not add a C# property whose only purpose is to carry
a static localized string into XAML.

Attached properties use the fully qualified MRT Core form:

```xml
<data name="RefreshButton.[using:Microsoft.UI.Xaml.Controls]ToolTipService.ToolTip"
      xml:space="preserve"><value>Refresh</value></data>
<data name="SendButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name"
      xml:space="preserve"><value>Send command</value></data>
```

Never reuse one UID across control types unless every property supplied by that
UID exists on every target. In particular, a `.Content` entry cannot be shared
with a `TextBlock`, which requires `.Text` and otherwise can crash at page load.

## Dynamic UI

Prefer, in order:

1. Separate localized labels and bound semantic values in neighboring elements.
2. Two localized elements selected by visibility for finite states.
3. A reusable `UserControl`, `DataTemplate`, style, or template selector.
4. A verified Community Toolkit single-value converter.
5. Programmatic lookup only for a complete translator-controlled format,
   backend key, or framework surface that XAML cannot represent.

WinUI 3 has no `MultiBinding` or `IMultiValueConverter`. Use `x:Bind` function
binding or a small view/control instead. Do not split a translator-controlled
sentence when word order or plural grammar must remain translatable.

## ContentDialog architecture

Localized dialogs are XAML classes whose root is `ContentDialog`. Static title,
body, field labels, and button text come from the dialog root/elements' UIDs.
Code may pass runtime data, set `XamlRoot`, update validation state, show the
dialog, and handle its result. Large existing bodies may remain reusable
`UserControl` instances inside a dedicated dialog class.

Use `ContentDialogSizing.Apply` where a dialog needs the repository's wide
sizing behavior, and base any custom style on `DefaultContentDialogStyle`.
Never return to a factory that accepts already-localized title/body/button
strings.

## Tooltips and imperative controls

Static tooltips use an attached-property UID in XAML. When a WinUI limitation
requires a control to be created in C# (notably the Mod Shop `SelectorBar`), C#
may choose a XAML-defined style or template containing the localized tooltip;
it must not load and inject the string. Preserve the documented imperative
SelectorBar construction because `ItemsSource` plus an `x:Bind` template for
plain managed types can crash at startup.

## Programmatic lookup

`LocalizationService` is for runtime-only localization. Resource identifiers
may be hierarchical, for example `install.progress.downloading_server`. MRT
Core exposes such identifiers to `ResourceLoader.GetString` with slash paths,
so the service maps dots to slashes:

```text
install.progress.downloading_server
    -> install/progress/downloading_server
```

Do not change property dots in XAML identifiers such as `SaveButton.Content`;
those are resolved directly by XAML. Missing programmatic resources return the
key for dynamic fallback scenarios and emit a debugger diagnostic rather than
being silently hidden.

Backend `LocalizedMessage` values deliberately use dotted keys plus runtime
arguments. Their RESW identifiers remain dotted and their placeholders remain
translator-controlled. Raw gRPC status details, exception messages, and Go
diagnostics are never normal user-facing fallbacks; map typed error codes to
localized resources.

## Community Toolkit converters

Converters come from `CommunityToolkit.WinUI.Converters`, aligned with the
other Windows Community Toolkit packages. App-wide converters live in
`Application.Resources`, because an `x:Bind` converter declared only in an
inner element's resources can fail during XAML loading. `BoolToVisibility` uses
`ConverterParameter=True` for inversion. Collection, string, and object values
must use their matching converters rather than being passed to a bool converter.

RESW identifiers are not XAML `StaticResource` keys. Do not pass a RESW key as a
converter's `TrueValue` or `FalseValue`; use localized elements/templates.

## Semantic deduplication

Resource identity includes meaning, target property, and grammatical context,
not just the English spelling. Reuse a physical property entry only when those
are compatible. When XAML requires duplicate property entries for different
target types or responsive copies, identify the canonical semantic entry in a
translator comment using `Duplicate of <identifier>`. Identical words with
different meanings remain separate and receive context comments. The resource
tests reject duplicate identifiers and undocumented duplicate visible values.

## Approved C# exceptions

Every remaining programmatic lookup must fit this table and be covered by the
resource audit. Convenience is not an exception.

| Area | Allowed reason |
| --- | --- |
| Backend progress and shop messages | Runtime `LocalizedMessage` key and arguments supplied by gRPC/Lua |
| Error mapping | Typed gRPC/error code selected at runtime; diagnostic detail is not displayed |
| Translator-controlled formats | Complete sentence needs runtime placeholders or accessibility ordering |
| `TrayIconService` | Win32 notification-area tooltip and menu objects cannot consume XAML |
| Runtime accessibility announcements | The complete announcement is created for a runtime-only event |
| Upstream metadata | Raw upstream names/descriptions are data, not application UI literals |

Static view text, finite-state labels, dialog chrome, and tooltips are not on
this list.

### Audited programmatic exception inventory

The July 2026 refactor reduced programmatic localization from 200 lookups in
34 files to 59 lookups in 20 files. This is the exhaustive remaining inventory;
adding another source file requires documenting why XAML cannot own the text.

| Source | Calls | Approved runtime reason |
| --- | ---: | --- |
| `app/JustHostMC.App/Controls/ScriptEntryCard.xaml.cs` | 1 | Translator-controlled confirmation format containing the selected script name |
| `app/JustHostMC.App/Controls/Server/ServerHeaderPanel.xaml.cs` | 1 | Translator-controlled memory value format |
| `app/JustHostMC.App/Controls/Server/ServerPlayersPanel.xaml.cs` | 1 | Translator-controlled live player-count header |
| `app/JustHostMC.App/MainWindow.xaml.cs` | 1 | Runtime teaching-tip event kind selects the announcement title |
| `app/JustHostMC.App/Models/ConfigEntryItem.cs` | 1 | Dynamic configuration identifier lookup with a humanized fallback |
| `app/JustHostMC.App/Models/ModFileItem.cs` | 4 | Runtime parser result and translator-controlled compatibility formats |
| `app/JustHostMC.App/Models/ParserItem.cs` | 1 | Translator-controlled format list description |
| `app/JustHostMC.App/Models/ServerItem.cs` | 3 | Runtime navigation accessibility, status, and port formats |
| `app/JustHostMC.App/Services/TrayIconService.cs` | 3 | Win32 notification-area tooltip and menu objects cannot consume XAML |
| `app/JustHostMC.App/ViewModels/BackupsViewModel.cs` | 1 | Typed backup error-code mapping |
| `app/JustHostMC.App/ViewModels/MainViewModel.cs` | 10 | Backend progress keys, runtime tracker states, and generated default name |
| `app/JustHostMC.App/ViewModels/ModsViewModel.cs` | 4 | Typed mod-operation error-code mapping |
| `app/JustHostMC.App/ViewModels/ScriptsViewModel.cs` | 10 | Typed script-operation error-code mapping |
| `app/JustHostMC.App/ViewModels/SettingsViewModel.cs` | 4 | Translator-controlled count, version, and size result formats |
| `app/JustHostMC.App/ViewModels/ShopDetailViewModel.cs` | 2 | Runtime source-name format and backend error key |
| `app/JustHostMC.App/ViewModels/ShopViewModel.cs` | 4 | Backend/Lua category and home-section keys |
| `app/JustHostMC.App/Views/EngineStdioWindow.xaml.cs` | 1 | Translator-controlled live PID and entry-count status |
| `app/JustHostMC.App/Views/PlayerInventoryDialog.xaml.cs` | 5 | Runtime inventory-slot tooltip and accessibility labels, including translator-controlled numeric formats |
| `app/JustHostMC.App/Views/ServerDialog.xaml.cs` | 1 | Translator-controlled provider-author format |
| `app/JustHostMC.App/Views/ShopSearchPage.xaml.cs` | 1 | Translator-controlled result-count and query summary |

## Images, manifest resources, and themes

Package logos, tile/splash artwork, and images loaded by URI belong under
`Assets/`. XAML uses a relative package path or `ms-appx:///Assets/...`; code
must use an absolute `ms-appx:///` URI. Provide scale variants with names such
as `logo.scale-100.png`, `logo.scale-200.png`, and `logo.scale-400.png` when an
image must remain sharp at different DPI settings. Use `theme-dark`/
`theme-light` or high-contrast qualifiers only when the bitmap itself changes;
brushes, dimensions, styles, and control templates belong in a XAML resource
dictionary.

Manifest `DisplayName` and `Description` use `ms-resource:` identifiers. Icon-
only controls still need localized automation names even when the image itself
does not change by language. Do not duplicate or move assets without a concrete
packaging, scale, theme, or localization need.

## Validation and runtime testing

Run:

```powershell
rtk dotnet test app\JustHostMC.App.ResourceTests\JustHostMC.App.ResourceTests.csproj
rtk dotnet build app\JustHostMC.App\JustHostMC.App.csproj -p:Platform=x64
rtk dotnet test app\JustHostMC.Core.Tests\JustHostMC.Core.Tests.csproj
```

The resource tests validate XML, unique/non-empty identifiers, locale parity,
XAML UID coverage, dotted backend identifiers, converter architecture, dialog
architecture, tooltip ownership, and raw-diagnostic safety.

A build is necessary but insufficient. In both `en-US` and `zh-TW`, instantiate
MainWindow; Home, Scripts, Settings, every Server section, Mod Shop
home/search/detail, engine/script-log windows, every dialog, menus, flyouts,
templates, and localized tooltips. Repeat for unpackaged output and the MSIX
path when packaging tooling is available. Watch the debugger/XAML binding output
for missing resources and attached-property failures.

## Migration summary and WinUI pitfalls

The 2026 resource refactor moved dotted backend aliases to MRT slash lookup,
replaced the custom visibility converter with Community Toolkit converters,
moved localized tooltip selection and dialog chrome into XAML, synchronized
`en-US`/`zh-TW`, added automated validation, and documented runtime-only C#
exceptions.

Keep these runtime constraints in mind:

- marshal stream/background changes through `DispatcherQueue`;
- keep app-wide `x:Bind` converters in `Application.Resources` or the XAML root;
- do not use `ElementName` binding from inside a list-item template;
- do not bind `NavigationView.MenuItemsSource` to plain managed items with an
  `x:Bind` template;
- never share a UID property across incompatible control types;
- compile success does not prove that an attached-property UID works at runtime.
