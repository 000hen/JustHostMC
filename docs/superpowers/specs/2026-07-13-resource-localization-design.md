# Resource and Localization Refactor Design

## Objective

Refactor the WinUI 3 application so static user-facing text is resolved by XAML,
runtime localization uses native MRT Core resource identifiers, equivalent common
copy is shared deliberately, and the complete resource system is documented and
enforced by automated checks.

## Confirmed Requirements

- Static UI content must be declared in XAML and localized with `x:Uid`; C# must
  not fetch or assign text that XAML can own.
- Runtime lookup is reserved for genuinely dynamic content: backend-provided
  localization keys, values containing runtime data, runtime-selected status or
  error messages, and imperative UI that cannot be represented in XAML.
- Formatting in C# is prohibited when the same result can be expressed through
  XAML binding or another native XAML mechanism.
- Attached properties are localized through their full property identifier, for
  example `RefreshButton.ToolTipService.ToolTip`.
- Semantically interchangeable text such as Save and Cancel uses a shared common
  resource. Coincidentally identical English text may remain separate when its
  meaning or translation context differs.
- WinUI property context is significant. A shared identifier may be reused when
  the target property is compatible; property-specific common variants are
  permitted when native `x:Uid` requires different properties such as `.Content`
  and `.CloseButtonText`.
- Segmented resource identifiers use dots in `.resw` and forward slashes in
  `ResourceLoader.GetString`, such as resource `Menu.File.Open` and lookup
  `Menu/File/Open`. Underscore normalization is removed.
- Resource documentation must cover strings and non-string resources, and
  `AGENTS.md` must link to it.

## Microsoft Platform Conventions

The app will retain the Windows App SDK layout documented by Microsoft:

```text
app/JustHostMC.App/
  Strings/
    en-US/Resources.resw
    zh-TW/Resources.resw
  Assets/
```

`en-US` remains the default and fallback language. `zh-TW` remains the alternate
locale declared in `Package.appxmanifest`. `x:Uid` property identifiers use the
form `Identifier.Property`; attached properties use their complete property path.
Manifest resources remain simple identifiers referenced by `ms-resource:` URIs.

Primary references:

- Microsoft Learn, "Localize strings in your UI and app package manifest"
  (`windows/apps/windows-app-sdk/mrtcore/localize-strings`).
- Microsoft Learn, "ResourceDictionary and XAML resource references"
  (`windows/apps/develop/platform/xaml/xaml-resource-dictionary`).
- Microsoft Learn, "Make your app localizable"
  (`windows/apps/design/globalizing/prepare-your-app-for-localization`).

## Lookup Classification

Every current `LocalizationService` or `ILocalizer.Get` call will be classified
before migration:

1. **Static XAML property**: move to `x:Uid` and remove the C# lookup. Examples
   include headings, labels, button content, dialog button copy represented by a
   XAML-owned dialog, titles exposed by XAML controls, and tooltips.
2. **XAML-bindable runtime value**: expose only the raw runtime data/state from
   the ViewModel and express presentation in XAML when WinUI supports it.
3. **Dynamic localized content**: retain programmatic lookup when the resource is
   selected by runtime state, contains runtime substitutions, comes from a
   backend `LocalizedMessage`, or belongs to imperative UI that cannot be moved
   safely into XAML.
4. **Diagnostic/non-display text**: do not localize. Raw gRPC diagnostic details
   must not replace the localized user-facing message.

The initial graph-backed inventory found 141 lookup occurrences across 75 C#
members and 21 direct `LocalizationService` construction sites. The implementation
will create an exact machine-readable inventory so none are silently skipped.

## Resource Naming and Deduplication

- XAML-owned identifiers describe the element or reusable semantic role and end
  in the native property name in `.resw`, for example
  `RefreshButton.ToolTipService.ToolTip`.
- Common actions use a `Common` namespace/identifier family. They are reused
  across compatible property contexts.
- Dynamic identifiers use semantic dotted names. C# passes the corresponding
  slash-separated path directly to `ResourceLoader.GetString`.
- Backend keys remain dotted at the protocol boundary and are converted to slash
  paths only at the MRT Core lookup boundary.
- Each locale has exactly the same resource-name set.
- Duplicate names are invalid. Duplicate values are flagged for review and are
  allowed only when documented semantic or property-context differences require
  them.
- Translator comments explain placeholders, context, and intentional duplicates.
- Placeholder sets must match between locales.

## XAML Migration

Static content will be moved into the owning `.xaml` file. Existing imperative
dialogs will be assessed individually: reusable or structurally significant
dialogs become XAML `ContentDialog` components; truly runtime-created dialogs may
retain programmatic lookup for their non-XAML-owned properties. This exception
must be explicit in the inventory.

Tooltips, accessibility names, descriptions, headers, placeholders, and similar
attached or secondary properties will be localized on the element itself. No
code-behind helper may fetch a static string merely to feed an `x:Bind` method.

The migration must respect the project's runtime XAML hazards, especially never
reusing an `x:Uid` whose property identifiers are invalid for another control
type.

## Runtime Localization API

`LocalizationService` remains only as the dynamic localization boundary used by
ViewModels, models with dynamic display state, backend localization messages, and
rare imperative UI. It will:

- accept dotted semantic keys from application/backend code;
- translate dots to `/` only for MRT Core lookup;
- perform named placeholder substitution for dynamic messages;
- preserve a safe missing-resource fallback without hiding malformed resource
  identifiers from validation tests.

Static views and controls must not construct `LocalizationService`.

## Validation and Tests

Automated resource-policy tests will parse C#, XAML, and both `.resw` files and
verify:

- locale key parity;
- valid XML and unique resource names;
- matching placeholder sets;
- every XAML `x:Uid` has compatible property resources;
- prohibited underscore normalization is absent;
- segmented dynamic keys resolve through slash syntax;
- known common actions do not proliferate duplicate semantic resources;
- static-only views and controls do not construct or call
  `LocalizationService`;
- resource references in the manifest and XAML resolve.

Verification includes the policy tests, the C# test suite, an x64 WinUI build,
and page/dialog smoke testing where available. Build success alone does not prove
that XAML localization is runtime-safe.

## Documentation Deliverable

Create `docs/resources.md` as the maintained resource guide. It will document:

- locale directory layout, fallback behavior, and manifest declarations;
- string identifier naming, `x:Uid`, attached properties, common resources,
  dynamic lookup syntax, placeholders, and translator comments;
- images and other file resources, URI conventions, language/scale qualifiers,
  accessibility, and the rule against embedding localizable text in images;
- adding a locale or resource, validation commands, packaging considerations,
  and troubleshooting PRI lookup failures;
- the static-versus-dynamic decision table and approved exceptions.

`AGENTS.md` will link to `docs/resources.md` from its cross-cutting resource and
localization guidance.

## Non-Goals

- Changing user-visible product behavior unrelated to localization.
- Rewriting backend localization contracts.
- Introducing a custom XAML markup extension or third-party localization system.
- Translating into additional languages during this refactor.

## Success Criteria

- All C# localization lookups are inventoried and justified; static lookups are
  removed.
- Static UI copy is owned by XAML resources.
- Dynamic lookup uses dotted resource names and slash-separated MRT paths without
  underscore normalization.
- Common semantic copy is shared subject to WinUI property compatibility.
- Locale resources are structurally consistent and validation is automated.
- `docs/resources.md` is comprehensive and referenced from `AGENTS.md`.
- The test suite and x64 WinUI build pass, with no new XAML/MVVM analyzer issues.
