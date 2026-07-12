# Resource and Localization Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move all static WinUI copy out of C# and into native XAML resources, retain only justified dynamic localization lookups, adopt MRT slash lookup syntax, deduplicate common copy, and document and validate the complete resource system.

**Architecture:** Keep `Strings/<language>/Resources.resw` as the single MRT Core string source. XAML owns static properties through `x:Uid`; `LocalizationService` remains the narrow dynamic boundary and converts dotted semantic keys to slash-separated MRT paths. Source-policy tests parse C#, XAML, the manifest, and both `.resw` files without loading WinUI.

**Tech Stack:** .NET 9, C# preview, WinUI 3 / Windows App SDK 2.2, MRT Core `ResourceLoader`, XAML `x:Uid`, xUnit, LINQ to XML.

## Global Constraints

- Static UI content must be declared in XAML and localized with `x:Uid`; C# must not fetch or assign text that XAML can own.
- Runtime lookup is reserved for backend-provided localization keys, values containing runtime data, runtime-selected status or error messages, and imperative UI that cannot be represented in XAML.
- Formatting in C# is prohibited when the same result can be expressed through XAML binding or another native XAML mechanism.
- Attached properties use their full identifier, for example `RefreshButton.ToolTipService.ToolTip`.
- Semantically interchangeable copy such as Save and Cancel uses shared `Common` identifiers; property-specific variants are permitted only when native WinUI targets incompatible properties.
- Coincidentally identical English text remains separate when its meaning or translation context differs.
- `.resw` identifiers use dots; `ResourceLoader.GetString` receives forward-slash paths. No underscore normalization is permitted.
- `en-US` is the default/fallback locale and `zh-TW` is the alternate locale.
- Do not introduce a custom XAML markup extension or a third-party localization system.
- Preserve the project's WinUI runtime-safety rules, especially never sharing an `x:Uid` across control types with incompatible properties.
- Prefix every shell command, and every command-chain segment, with `rtk`.

---

### Task 1: Add the resource-policy safety net and lookup inventory

**Files:**
- Create: `app/JustHostMC.Core.Tests/ResourcePolicyTests.cs`
- Create: `docs/resource-lookup-inventory.md`
- Read: `app/JustHostMC.App/Strings/en-US/Resources.resw`
- Read: `app/JustHostMC.App/Strings/zh-TW/Resources.resw`
- Read: every `app/JustHostMC.App/**/*.xaml` and `app/JustHostMC.App/**/*.cs` file

**Interfaces:**
- Consumes: repository source files located by walking upward from `AppContext.BaseDirectory` until `JustHostMC.sln` is found.
- Produces: `ResourcePolicyTests` and a checked-in classification of every `ILocalizer.Get`/`LocalizationService` use as `StaticXaml`, `DynamicState`, `BackendKey`, `RuntimeFormat`, or `ImperativeException`.

- [ ] **Step 1: Write the failing locale and naming tests**

Create `ResourcePolicyTests.cs` with these helpers and initial tests:

```csharp
using System.Text.RegularExpressions;
using System.Xml.Linq;
using Xunit;

namespace JustHostMC.Core.Tests;

public sealed class ResourcePolicyTests {
    private static readonly string Root = FindRepositoryRoot();
    private static readonly string AppRoot =
        Path.Combine(Root, "app", "JustHostMC.App");

    [Fact]
    public void LocalesExposeTheSameResourceNames() {
        var english = LoadResources("en-US").Select(ResourceName).ToHashSet();
        var chinese = LoadResources("zh-TW").Select(ResourceName).ToHashSet();
        Assert.Empty(english.Except(chinese));
        Assert.Empty(chinese.Except(english));
    }

    [Theory]
    [InlineData("en-US")]
    [InlineData("zh-TW")]
    public void ResourceNamesAreUnique(string language) {
        var duplicates = LoadResources(language)
            .GroupBy(ResourceName, StringComparer.OrdinalIgnoreCase)
            .Where(group => group.Count() > 1)
            .Select(group => group.Key);
        Assert.Empty(duplicates);
    }

    [Fact]
    public void LocalePlaceholdersMatch() {
        var english = LoadResourceMap("en-US");
        var chinese = LoadResourceMap("zh-TW");
        foreach (var key in english.Keys) {
            Assert.Equal(Placeholders(english[key]), Placeholders(chinese[key]));
        }
    }

    [Fact]
    public void DynamicLookupUsesMrtSlashPaths() {
        var source = File.ReadAllText(Path.Combine(
            AppRoot, "Services", "LocalizationService.cs"));
        Assert.Contains("key.Replace('.', '/')", source, StringComparison.Ordinal);
        Assert.DoesNotContain("key.Replace('.', '_')", source, StringComparison.Ordinal);
    }

    private static IReadOnlyList<XElement> LoadResources(string language) =>
        XDocument.Load(Path.Combine(AppRoot, "Strings", language, "Resources.resw"))
            .Root!.Elements("data").ToArray();

    private static Dictionary<string, string> LoadResourceMap(string language) =>
        LoadResources(language).ToDictionary(
            ResourceName,
            element => element.Element("value")?.Value ?? string.Empty,
            StringComparer.OrdinalIgnoreCase);

    private static string ResourceName(XElement element) =>
        element.Attribute("name")!.Value;

    private static string[] Placeholders(string value) =>
        Regex.Matches(value, @"\{[A-Za-z][A-Za-z0-9_]*\}")
            .Select(match => match.Value)
            .Distinct(StringComparer.Ordinal)
            .Order(StringComparer.Ordinal)
            .ToArray();

    private static string FindRepositoryRoot() {
        for (var directory = new DirectoryInfo(AppContext.BaseDirectory);
             directory is not null;
             directory = directory.Parent) {
            if (File.Exists(Path.Combine(directory.FullName, "JustHostMC.sln")))
                return directory.FullName;
        }
        throw new DirectoryNotFoundException("Could not locate JustHostMC.sln");
    }
}
```

- [ ] **Step 2: Run the new tests and verify the expected red state**

Run:

```powershell
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj --filter FullyQualifiedName~ResourcePolicyTests -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: `DynamicLookupUsesMrtSlashPaths` fails because `LocalizationService` still replaces `.` with `_`; any pre-existing locale or placeholder mismatches are also reported rather than suppressed.

- [ ] **Step 3: Generate and check the exact lookup inventory**

Populate `docs/resource-lookup-inventory.md` with one row per lookup occurrence and these columns:

```markdown
| File:line | Resource expression | Classification | XAML owner / justification | Final action |
|---|---|---|---|---|
| `Controls/Server/ServerPerformancePanel.xaml.cs:27` | `ServerSectionPerformance/Text` | StaticXaml | `ServerPerformancePanel.xaml`, `ServerSectionPerformance.Title` | Remove lookup |
| `ViewModels/MainViewModel.cs:203` | `step.Key` | BackendKey | Backend progress stream | Retain dynamic lookup |
| `Views/EngineStdioWindow.xaml.cs:215` | paused/status format | RuntimeFormat | PID and visible count are runtime data | Retain formatted lookup |
```

The inventory must cover all files returned by these searches, including Models,
ViewModels, Controls, Views, `MainWindow.xaml.cs`, `ScriptLogsWindow.xaml.cs`, and
`SettingsPage.xaml.cs`:

```powershell
rtk grep "LocalizationService|ILocalizer|\.Get\(" app/JustHostMC.App --glob "*.cs"
rtk grep "x:Uid=" app/JustHostMC.App --glob "*.xaml"
```

Record the final counts at the top. Every `StaticXaml` row names its destination
XAML element; every retained row states why runtime data or imperative ownership
makes XAML insufficient.

- [ ] **Step 4: Commit the safety net and inventory**

```powershell
rtk git add app/JustHostMC.Core.Tests/ResourcePolicyTests.cs docs/resource-lookup-inventory.md
rtk git commit -m "test: inventory and validate localization resources"
```

---

### Task 2: Replace underscore normalization with native MRT paths

**Files:**
- Modify: `app/JustHostMC.App/Services/LocalizationService.cs`
- Modify: `app/JustHostMC.App/Services/ILocalizer.cs`
- Modify: `app/JustHostMC.App/Strings/en-US/Resources.resw`
- Modify: `app/JustHostMC.App/Strings/zh-TW/Resources.resw`
- Modify: dynamic callers listed in `docs/resource-lookup-inventory.md`

**Interfaces:**
- Consumes: application/backend semantic keys such as `install.progress.preparing`.
- Produces: `ILocalizer.Get(string key)` where dotted segments become MRT `/` paths only at the `ResourceLoader` boundary.

- [ ] **Step 1: Strengthen the failing key-policy test**

Add:

```csharp
[Fact]
public void ProgrammaticKeysDoNotUseUnderscoreSeparators() {
    var offenders = Directory.EnumerateFiles(AppRoot, "*.cs", SearchOption.AllDirectories)
        .SelectMany(path => File.ReadLines(path)
            .Select((line, index) => (path, line, number: index + 1)))
        .Where(item => Regex.IsMatch(
            item.line,
            @"\.Get\(\s*""[A-Za-z0-9]+_[A-Za-z0-9_]"))
        .Select(item => $"{Path.GetRelativePath(Root, item.path)}:{item.number}");
    Assert.Empty(offenders);
}
```

Run the filtered test and confirm it fails on current keys such as
`EngineStatus_Connecting`, `Backups_Creating`, and `Shop_LoadFailed`.

- [ ] **Step 2: Implement slash normalization**

Replace `NormalizeKey` with:

```csharp
// MRT Core represents segmented resource identifiers as slash-separated paths.
private static string ToLookupPath(string key) => key.Replace('.', '/');
```

Both `Get` overloads call `_loader.GetString(ToLookupPath(key))`. Update
`ILocalizer` XML comments to say callers use dotted semantic identifiers and the
implementation performs MRT path conversion.

- [ ] **Step 3: Rename programmatic resource identifiers consistently**

For every retained lookup in the inventory, change underscore-separated semantic
keys to dotted identifiers in C# and both locale files. Examples are exact:

```text
EngineStatus_Connecting     -> EngineStatus.Connecting
Backups_Creating            -> Backups.Creating
Mods_OperationFailedDetail  -> Mods.OperationFailedDetail
Settings_SaveFailed         -> Settings.SaveFailed
Shop_LoadFailed             -> Shop.LoadFailed
ServerState_Starting        -> ServerState.Starting
```

Backend keys already containing dots remain unchanged. Property identifiers used
only by `x:Uid` remain `Element.Property`; delete any obsolete flat alias instead
of renaming it.

- [ ] **Step 4: Run the focused policy tests**

```powershell
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj --filter FullyQualifiedName~ResourcePolicyTests -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: slash-path and programmatic-key tests pass. Locale parity and placeholder tests also pass after paired renames.

- [ ] **Step 5: Commit native key lookup**

```powershell
rtk git add app/JustHostMC.App/Services app/JustHostMC.App/Strings app/JustHostMC.App/Models app/JustHostMC.App/ViewModels app/JustHostMC.App/Views app/JustHostMC.App/Controls app/JustHostMC.App/MainWindow.xaml.cs app/JustHostMC.Core.Tests/ResourcePolicyTests.cs docs/resource-lookup-inventory.md
rtk git commit -m "refactor: use native MRT resource paths"
```

---

### Task 3: Move static control copy from C# into XAML

**Files:**
- Modify: `app/JustHostMC.App/Controls/Server/ServerConfigPanel.xaml`
- Modify: `app/JustHostMC.App/Controls/Server/ServerConfigPanel.xaml.cs`
- Modify: `app/JustHostMC.App/Controls/Server/ServerModsPanel.xaml`
- Modify: `app/JustHostMC.App/Controls/Server/ServerModsPanel.xaml.cs`
- Modify: `app/JustHostMC.App/Controls/Server/ServerPerformancePanel.xaml`
- Modify: `app/JustHostMC.App/Controls/Server/ServerPerformancePanel.xaml.cs`
- Modify: `app/JustHostMC.App/Controls/Server/ServerPlayersPanel.xaml`
- Modify: `app/JustHostMC.App/Controls/Server/ServerPlayersPanel.xaml.cs`
- Modify: `app/JustHostMC.App/Controls/Server/ServerHeaderPanel.xaml`
- Modify: `app/JustHostMC.App/Controls/Server/ServerHeaderPanel.xaml.cs`
- Modify: `app/JustHostMC.App/Controls/ScriptEntryCard.xaml`
- Modify: `app/JustHostMC.App/Controls/ScriptEntryCard.xaml.cs`
- Modify: both `Resources.resw` files
- Modify: `app/JustHostMC.Core.Tests/ResourcePolicyTests.cs`

**Interfaces:**
- Consumes: existing dependency properties and ViewModel runtime data.
- Produces: controls whose static title, description, hint, tooltip, and button copy are set by `x:Uid`; C# remains only for counts, player names, memory/port values, and runtime status/error messages.

- [ ] **Step 1: Write the failing static-control policy test**

Add:

```csharp
[Fact]
public void StaticControlsDoNotConstructLocalizationService() {
    string[] files = [
        "Controls/Server/ServerConfigPanel.xaml.cs",
        "Controls/Server/ServerModsPanel.xaml.cs",
        "Controls/Server/ServerPerformancePanel.xaml.cs",
    ];
    foreach (var relativePath in files) {
        var source = File.ReadAllText(Path.Combine(
            AppRoot, relativePath.Replace('/', Path.DirectorySeparatorChar)));
        Assert.DoesNotContain("LocalizationService", source, StringComparison.Ordinal);
        Assert.DoesNotContain("_localizer.Get", source, StringComparison.Ordinal);
    }
}
```

Run it and confirm failure in all three controls.

- [ ] **Step 2: Localize `ServerSectionLayout` properties in XAML**

Replace code-bound static values with `x:Uid` on the layout element:

```xml
<server:ServerSectionLayout
    x:Uid="ServerSectionPerformance"
    IconGlyph="&#xE9D9;">
```

Use `ServerSectionPerformance.Title` and
`ServerSectionPerformance.Description` in each `.resw`. Apply the same pattern to
config and mods. Where description changes because the server is stopped, bind
visibility between two XAML-localized `TextBlock` elements or expose the boolean
state; do not fetch either static sentence through C#.

- [ ] **Step 3: Remove obsolete control localizers and helper methods**

Delete `_localizer`, `PerformanceTitle`, `PerformanceDescription`, `ConfigTitle`,
`ConfigDescription`, and `ModsDescription`. Remove unused service `using`
directives. Retain dynamic helpers in Header and Players only where they include
runtime values; move their static empty/stopped alternatives into XAML with
visibility bindings.

- [ ] **Step 4: Move ScriptEntryCard static action copy into XAML**

Keep `Scripts.RemoveConfirmBody` dynamic because it inserts `Item.Name`. Set the
static confirmation button through `x:Uid` and remove the assignment to
`RemoveConfirmButton.Content`. Use a property-compatible `CommonRemoveButton`
resource if it has the same semantics as other Remove actions.

- [ ] **Step 5: Run policy tests and build the app**

```powershell
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj --filter FullyQualifiedName~ResourcePolicyTests -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
rtk dotnet build app/JustHostMC.App/JustHostMC.App.csproj -p:Platform=x64 -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: all policy tests pass and the WinUI build has zero errors and no new MVVM Toolkit warnings.

- [ ] **Step 6: Commit static control migration**

```powershell
rtk git add app/JustHostMC.App/Controls app/JustHostMC.App/Strings app/JustHostMC.Core.Tests/ResourcePolicyTests.cs docs/resource-lookup-inventory.md
rtk git commit -m "refactor: localize static control copy in XAML"
```

---

### Task 4: Move static view, window, dialog, and tooltip copy into XAML

**Files:**
- Modify: `app/JustHostMC.App/MainWindow.xaml` and `.xaml.cs`
- Modify: all matching pairs under `app/JustHostMC.App/Views/`
- Modify: both `Resources.resw` files
- Modify: `app/JustHostMC.Core.Tests/ResourcePolicyTests.cs`
- Modify: `docs/resource-lookup-inventory.md`

**Interfaces:**
- Consumes: the approved lookup inventory.
- Produces: XAML-owned window titles, dialog chrome where representable, labels, headers, placeholders, accessibility names, and tooltips; an explicit minimal allowlist of imperative dynamic lookups.

- [ ] **Step 1: Write failing XAML ownership tests**

Add tests that ensure every `x:Uid` resolves and no uid spans different element
types:

```csharp
[Fact]
public void EveryXamlUidHasAResourceProperty() {
    var keys = LoadResourceMap("en-US").Keys.ToArray();
    var missing = XamlUids()
        .Where(uid => !keys.Any(key => key.StartsWith(uid + ".", StringComparison.OrdinalIgnoreCase)))
        .Distinct(StringComparer.OrdinalIgnoreCase);
    Assert.Empty(missing);
}

[Fact]
public void AUidIsNotSharedAcrossDifferentElementTypes() {
    var conflicts = XamlUidElements()
        .GroupBy(item => item.uid, StringComparer.OrdinalIgnoreCase)
        .Where(group => group.Select(item => item.element).Distinct().Count() > 1)
        .Select(group => group.Key);
    Assert.Empty(conflicts);
}

private static IEnumerable<string> XamlUids() =>
    XamlUidElements().Select(item => item.uid);

private static IEnumerable<(string uid, string element)> XamlUidElements() {
    XNamespace x = "http://schemas.microsoft.com/winfx/2006/xaml";
    foreach (var path in Directory.EnumerateFiles(AppRoot, "*.xaml", SearchOption.AllDirectories)) {
        foreach (var element in XDocument.Load(path).Root!.DescendantsAndSelf()) {
            if (element.Attribute(x + "Uid") is { Value: var uid })
                yield return (uid, element.Name.LocalName);
        }
    }
}
```

Run the tests; record all existing conflicts before changing resources so current
runtime hazards are not hidden.

- [ ] **Step 2: Move window titles to root XAML resources**

Assign property-compatible root identifiers such as:

```xml
<Window ... x:Uid="MainWindow">
```

with `MainWindow.Title` in both locales. Do the same for
`EngineStdioWindow`, `ScriptLogsWindow`, and `ShopWindow`. Remove corresponding
constructor lookups and duplicate title assignments where a XAML title-bar
control can receive its own `x:Uid`.

- [ ] **Step 3: Move static dialog properties into XAML-owned dialogs**

For delete, rename, create/edit server, permission consent, backups, ban list,
dependency install, and folder-not-found dialogs:

- declare reusable `ContentDialog` markup in the owning XAML or convert an
  existing dialog view to a XAML `ContentDialog`;
- assign `x:Uid` for `Title`, `PrimaryButtonText`, and `CloseButtonText`;
- keep only runtime `Content`, names, dependency lists, and enablement in C#;
- remove duplicated create/edit/delete lookups from `MainWindow.xaml.cs`,
  `HomePage.xaml.cs`, and `ServerPage.xaml.cs`;
- use shared common property identifiers for Save, Cancel, Close, Delete, Install,
  and Remove when their semantics and target property match.

If a dialog cannot be XAML-owned without changing lifecycle behavior, keep it as
`ImperativeException` in the inventory with the exact reason and retain only its
chrome lookup. This is an exception, not the default.

- [ ] **Step 4: Move tooltips and accessibility properties to their elements**

Replace static C# tooltip assignments with identifiers such as:

```xml
<Button x:Uid="RefreshButton" ... />
```

and:

```xml
<data name="RefreshButton.ToolTipService.ToolTip" xml:space="preserve">
  <value>Refresh</value>
</data>
```

Dynamic tooltips bound to runtime state, such as the active install step and
server endpoint, remain bindings and are classified `DynamicState`.

- [ ] **Step 5: Remove hard-coded localized fallbacks from XAML**

Delete local `Text`, `Content`, `Header`, `PlaceholderText`, and tooltip literals
when the same property is supplied by `x:Uid`. Preserve non-display technical
literals, glyphs, URLs, file extensions, commands, and layout values.

- [ ] **Step 6: Run policy tests and the WinUI build**

```powershell
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj --filter FullyQualifiedName~ResourcePolicyTests -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
rtk dotnet build app/JustHostMC.App/JustHostMC.App.csproj -p:Platform=x64 -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: XAML uid tests pass; app build exits 0.

- [ ] **Step 7: Commit static view migration**

```powershell
rtk git add app/JustHostMC.App/MainWindow.xaml app/JustHostMC.App/MainWindow.xaml.cs app/JustHostMC.App/Views app/JustHostMC.App/Strings app/JustHostMC.Core.Tests/ResourcePolicyTests.cs docs/resource-lookup-inventory.md
rtk git commit -m "refactor: move static view copy into XAML"
```

---

### Task 5: Deduplicate common copy and normalize both locale files

**Files:**
- Modify: `app/JustHostMC.App/Strings/en-US/Resources.resw`
- Modify: `app/JustHostMC.App/Strings/zh-TW/Resources.resw`
- Modify: affected XAML identifiers under `app/JustHostMC.App/`
- Modify: `app/JustHostMC.Core.Tests/ResourcePolicyTests.cs`

**Interfaces:**
- Consumes: all post-migration XAML and dynamic resource references.
- Produces: locale files with identical key sets, matching placeholders, shared common semantic copy, and documented intentional duplicate values.

- [ ] **Step 1: Write the failing duplicate-value policy test**

Add:

```csharp
[Fact]
public void DuplicateEnglishValuesAreDocumented() {
    var undocumented = LoadResources("en-US")
        .GroupBy(element => element.Element("value")?.Value.Trim(), StringComparer.Ordinal)
        .Where(group => !string.IsNullOrEmpty(group.Key) && group.Count() > 1)
        .SelectMany(group => group)
        .Where(element => !(element.Element("comment")?.Value
            .StartsWith("INTENTIONAL DUPLICATE:", StringComparison.Ordinal) ?? false))
        .Select(ResourceName);
    Assert.Empty(undocumented);
}
```

Run it and confirm it reports current duplicate values.

- [ ] **Step 2: Consolidate common semantic actions**

Consolidate Save, Cancel, Close, Delete, Remove, Install, Edit, Refresh, Copy,
Open, Back, Next, and Dismiss into `Common` identifier families. Reuse one uid
for the same XAML property and control type. Where `.Content`, `.Text`,
`.PrimaryButtonText`, or `.CloseButtonText` makes one native uid impossible, keep
the minimum property-specific variant and add this comment in both locales:

```xml
<comment>INTENTIONAL DUPLICATE: WinUI x:Uid targets a different property; keep translation aligned with Common.Save.</comment>
```

Do not consolidate homographs whose translator context or meaning differs.

- [ ] **Step 3: Remove obsolete aliases and enforce locale parity**

Delete every resource no longer referenced by XAML, C#, or the manifest. Ensure
each deletion and rename is mirrored in `en-US` and `zh-TW`. Preserve manifest
keys `AppDisplayName` and `AppDescription` and any publisher resource actually
referenced by packaging.

- [ ] **Step 4: Add translator comments for formats**

Every value containing `{name}` placeholders receives a comment naming each
placeholder and its meaning. Placeholder spelling and count match across locales.

- [ ] **Step 5: Run all resource-policy tests**

```powershell
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj --filter FullyQualifiedName~ResourcePolicyTests -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: all locale, placeholder, uid, naming, static lookup, and duplicate-value tests pass.

- [ ] **Step 6: Commit normalized resources**

```powershell
rtk git add app/JustHostMC.App/Strings app/JustHostMC.App/Controls app/JustHostMC.App/Views app/JustHostMC.App/MainWindow.xaml app/JustHostMC.Core.Tests/ResourcePolicyTests.cs
rtk git commit -m "refactor: deduplicate localized resources"
```

---

### Task 6: Write the maintained resource guide and update agent instructions

**Files:**
- Create: `docs/resources.md`
- Modify: `AGENTS.md`
- Modify: `docs/resource-lookup-inventory.md`

**Interfaces:**
- Consumes: final implementation and policy tests.
- Produces: the authoritative contributor guide linked from repository instructions.

- [ ] **Step 1: Write `docs/resources.md`**

The document must contain these concrete sections and examples:

```markdown
# Resources and localization

## Directory layout
`Strings/en-US/Resources.resw`, `Strings/zh-TW/Resources.resw`, and `Assets/`.

## Static strings: XAML first
`<Button x:Uid="CommonSaveButton" />` with
`CommonSaveButton.Content` and attached-property examples such as
`RefreshButton.ToolTipService.ToolTip`.

## Dynamic strings: C# exception
Resource `Menu.File.Open`; lookup `_localizer.Get("Menu.File.Open")`; internal
MRT call `resourceLoader.GetString("Menu/File/Open")`.

## Common copy and duplicate policy
Reuse by semantics and compatible property; document unavoidable property-context
duplicates with `INTENTIONAL DUPLICATE:` comments.

## Placeholders and translator comments
Named placeholders only, identical placeholder sets in every locale.

## Images and file resources
Use `Assets/`, package-relative `ms-appx:///Assets/...` URIs, scale/language
qualifiers where needed, meaningful accessible text outside images, and no
embedded localizable text.

## Manifest resources
`ms-resource:AppDisplayName`, supported languages, default/fallback behavior.

## Adding a language or resource
Paired key creation, BCP-47 folder, manifest update for packaged builds, tests.

## Validation and troubleshooting
Exact `rtk dotnet test` and x64 build commands; PRI/MakePri guidance.
```

Include links to the three Microsoft Learn references recorded in the design.

- [ ] **Step 2: Correct and link the AGENTS localization guidance**

Replace the obsolete statement that points to `zh-Hant` and underscore
normalization with the actual `zh-TW` path and dotted/slash convention. Add:

```markdown
See [`docs/resources.md`](docs/resources.md) for the complete string, image,
manifest, qualifier, `x:Uid`, common-copy, and dynamic-lookup policy.
```

- [ ] **Step 3: Finalize the lookup inventory**

Mark every row `Removed`, `Moved to XAML`, or `Retained — <justification>`. Add a
summary table showing baseline count, final dynamic count, removed static count,
and imperative exception count. No row may remain unclassified.

- [ ] **Step 4: Validate documentation references and commit**

```powershell
rtk grep "docs/resources.md|Strings/{en-US,zh-TW}|forward slash" AGENTS.md docs/resources.md
rtk git diff --check
rtk git add AGENTS.md docs/resources.md docs/resource-lookup-inventory.md
rtk git commit -m "docs: define resource and localization policy"
```

Expected: references resolve, obsolete `zh-Hant`/underscore guidance is absent, and `git diff --check` exits 0.

---

### Task 7: Full verification and WinUI runtime smoke test

**Files:**
- Verify: all files changed by Tasks 1–6
- Update only on discovered defect: the owning XAML, C#, resource, test, inventory, or documentation file

**Interfaces:**
- Consumes: completed refactor.
- Produces: fresh evidence that source policy, compilation, tests, locale packaging, and runtime page/dialog loading are correct.

- [ ] **Step 1: Run the complete resource-policy suite**

```powershell
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj --filter FullyQualifiedName~ResourcePolicyTests -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: all `ResourcePolicyTests` pass with zero failures.

- [ ] **Step 2: Run the complete Go suite**

```powershell
rtk go -C engine test ./...
```

Expected: all non-gated Go tests pass; gated E2E tests report skipped unless `JHMC_INTEGRATION=1`.

- [ ] **Step 3: Build and test the complete solution**

```powershell
rtk dotnet build JustHostMC.sln -p:Platform=x64
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: build and tests exit 0 with no new XAML or MVVM Toolkit warnings.

- [ ] **Step 4: Smoke-test every navigable surface in both languages**

Launch the x64 app and, first under `en-US` and then `zh-TW`, open Home, create and
edit server dialogs, server Console/Players/Config/Performance/Mods sections,
Backups, Ban List, Scripts, script logs, Settings, engine monitor, Shop home,
search, detail, dependency confirmation, and all confirmation flyouts. Verify no
`XamlParseException`, raw resource identifier, missing tooltip, wrong control
property, untranslated static string, clipped common action, or underscore key is
visible.

- [ ] **Step 5: Inspect final source and resource diff**

```powershell
rtk git diff --check
rtk git status --short
rtk grep "Replace('.', '_')|LocalizationService" app/JustHostMC.App --glob "*.cs"
```

Expected: `diff --check` exits 0; every remaining localization construction or
lookup matches a `Retained` row in `docs/resource-lookup-inventory.md`; only
intentional working-tree files are listed.

- [ ] **Step 6: Commit any verification fixes**

If verification required corrections, stage only the exact owning files shown by
`rtk git status --short` plus the regression test, then commit. For example, a
`ServerPage` resource correction uses:

```powershell
rtk git add app/JustHostMC.App/Views/ServerPage.xaml app/JustHostMC.App/Strings/en-US/Resources.resw app/JustHostMC.App/Strings/zh-TW/Resources.resw app/JustHostMC.Core.Tests/ResourcePolicyTests.cs
rtk git commit -m "fix: address localization verification findings"
```

If no correction was required, do not create an empty commit.
