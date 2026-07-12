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
            Assert.DoesNotContain("LocalizationService", source,
                                  StringComparison.Ordinal);
            Assert.DoesNotContain("_localizer.Get", source,
                                  StringComparison.Ordinal);
        }
    }

    [Fact]
    public void EveryXamlUidHasAResourceProperty() {
        var keys = LoadResourceMap("en-US").Keys.ToArray();
        var missing = XamlUids()
            .Where(uid => !keys.Any(key => key.StartsWith(
                uid + ".", StringComparison.OrdinalIgnoreCase)))
            .Distinct(StringComparer.OrdinalIgnoreCase);
        Assert.Empty(missing);
    }

    [Fact]
    public void AUidIsNotSharedAcrossDifferentElementTypes() {
        var conflicts = XamlUidElements()
            .GroupBy(item => item.Uid, StringComparer.OrdinalIgnoreCase)
            .Where(group => group.Select(item => item.Element).Distinct().Count() > 1)
            .Select(group => group.Key);
        Assert.Empty(conflicts);
    }

    [Fact]
    public void XamlUidResourcePropertiesMatchElementTypes() {
        var keys = LoadResourceMap("en-US").Keys.ToArray();
        var incompatible = XamlUidElements()
            .Where(item => XamlDisplayProperties.ContainsKey(item.Element))
            .SelectMany(item => keys
                .Where(key => key.StartsWith(
                    item.Uid + ".", StringComparison.OrdinalIgnoreCase))
                .Select(key => (
                    item.Uid,
                    item.Element,
                    Property: key[(item.Uid.Length + 1)..])))
            .Where(item => !IsAttachedResourceProperty(item.Property) &&
                           !XamlDisplayProperties[item.Element]
                               .Contains(item.Property))
            .Select(item => $"{item.Uid}.{item.Property} ({item.Element})");
        Assert.Empty(incompatible);
    }

    [Fact]
    public void StaticViewChromeIsOwnedByXaml() {
        string[] resourceKeys = [
            "AppTitle",
            "BackupsDialog.CloseButtonText",
            "BanListStoppedNotice.Title",
            "BanListStoppedNotice.Message",
            "Common.Cancel",
            "Common.Save",
            "CreateServerDialog.Title",
            "CreateServerDialog.PrimaryButtonText",
            "CreateServerDialog.CloseButtonText",
            "EditServerDialog.Title",
            "EditServerDialog.PrimaryButtonText",
            "EditServerDialog.CloseButtonText",
            "EditServerName.Header",
            "EngineMonitor.Title",
            "PermissionConsentDialog.PrimaryButtonText",
            "PermissionConsentDialog.CloseButtonText",
            "RenameServerDialog.Title",
            "ScriptLogsWindow.Title",
            "ServerDelete.Title",
            "ServerDelete.Body",
            "ServerDelete.Confirm",
            "ServerFolder.NotFoundTitle",
            "ServerFolder.NotFoundBody",
            "Shop.DependencyPromptBody",
            "Shop.DependencyPromptTitle",
            "Shop.InstallConfirm",
            "ShopWindow.Title",
        ];
        var offenders = Directory.EnumerateFiles(
                AppRoot, "*.xaml.cs", SearchOption.AllDirectories)
            .SelectMany(path => resourceKeys
                .Where(key => File.ReadAllText(path).Contains(
                    $"Get(\"{key}\")", StringComparison.Ordinal))
                .Select(key => $"{Path.GetRelativePath(Root, path)} ({key})"));
        Assert.Empty(offenders);
    }

    [Fact]
    public void ContentDialogSizingAttachesHandlersOnlyOnce() {
        var source = File.ReadAllText(Path.Combine(
            AppRoot, "Controls", "ContentDialogSizing.cs"));
        Assert.Contains(
            "if (dialog.Resources.ContainsKey(SizingAttachedKey))",
            source, StringComparison.Ordinal);
    }

    [Fact]
    public void ServerModsDescriptionsUseLifecycleStateIndependentOfModificationCapability() {
        var viewModelSource = File.ReadAllText(Path.Combine(
            AppRoot, "ViewModels", "ModsViewModel.cs"));
        Assert.Contains("public bool IsServerStopped", viewModelSource,
                        StringComparison.Ordinal);
        Assert.Contains("OnPropertyChanged(nameof(IsServerStopped))", viewModelSource,
                        StringComparison.Ordinal);

        var panel = XDocument.Load(Path.Combine(
            AppRoot, "Controls", "Server", "ServerModsPanel.xaml"));
        XNamespace x = "http://schemas.microsoft.com/winfx/2006/xaml";
        string VisibilityFor(string uid) => panel.Descendants()
            .Single(element => (string?)element.Attribute(x + "Uid") == uid)
            .Attribute("Visibility")!.Value;

        var stoppedVisibility = VisibilityFor("ServerSectionModsHint");
        var runningVisibility = VisibilityFor("ModsStoppedHint");
        Assert.Contains("Mods.IsServerStopped", stoppedVisibility,
                        StringComparison.Ordinal);
        Assert.DoesNotContain("ConverterParameter=invert", stoppedVisibility,
                              StringComparison.Ordinal);
        Assert.Contains("Mods.IsServerStopped", runningVisibility,
                        StringComparison.Ordinal);
        Assert.Contains("ConverterParameter=invert", runningVisibility,
                        StringComparison.Ordinal);
        Assert.DoesNotContain("Mods.CanModify", stoppedVisibility,
                              StringComparison.Ordinal);
        Assert.DoesNotContain("Mods.CanModify", runningVisibility,
                              StringComparison.Ordinal);
    }

    [Fact]
    public void ServerPanelLayoutUidsDoNotReuseNavigationItemUids() {
        XNamespace x = "http://schemas.microsoft.com/winfx/2006/xaml";
        var navigationUids = XDocument.Load(Path.Combine(
                AppRoot, "Views", "ServerPage.xaml"))
            .Descendants()
            .Where(element => element.Name.LocalName == "SelectorBarItem")
            .Select(element => (string?)element.Attribute(x + "Uid"))
            .Where(uid => uid is not null)
            .Cast<string>()
            .ToHashSet(StringComparer.Ordinal);
        var resources = LoadResourceMap("en-US");
        var panels = new[] {
            (Path: "Controls/Server/ServerConfigPanel.xaml",
             Uid: "ServerSectionConfigPanel",
             Properties: new[] { "Title" }),
            (Path: "Controls/Server/ServerModsPanel.xaml",
             Uid: "ServerSectionModsPanel",
             Properties: new[] { "Title" }),
            (Path: "Controls/Server/ServerPerformancePanel.xaml",
             Uid: "ServerSectionPerformancePanel",
             Properties: new[] { "Title", "Description" }),
        };

        foreach (var panel in panels) {
            var source = File.ReadAllText(Path.Combine(
                AppRoot, panel.Path.Replace('/', Path.DirectorySeparatorChar)));
            Assert.DoesNotContain(panel.Uid, navigationUids);
            Assert.Contains($"x:Uid=\"{panel.Uid}\"", source,
                            StringComparison.Ordinal);
            foreach (var property in panel.Properties)
                Assert.Contains($"{panel.Uid}.{property}", resources.Keys);
        }
    }

    [Fact]
    public void ProgrammaticKeysDoNotUseUnderscoreSeparators() {
        var resourceNames = LoadResources("en-US")
            .Select(ResourceName)
            .ToHashSet(StringComparer.Ordinal);
        var offenders = Directory.EnumerateFiles(AppRoot, "*.cs", SearchOption.AllDirectories)
            .SelectMany(path => ResourceKeyLiterals(File.ReadAllText(path), resourceNames)
                .Select(item => $"{Path.GetRelativePath(Root, path)}:{item.Line} ({item.Key})"));
        Assert.Empty(offenders);
    }

    [Fact]
    public void ProgrammaticKeyPolicyCoversMappedAndPrefixedLiterals() {
        const string source = """
            localizer.Get(state switch {
                State.Running => "ServerStatus_Running",
                _ => ResourceKey("ServerStatus_", state),
            });
            """;
        var resources = new HashSet<string>(StringComparer.Ordinal) {
            "ServerStatus.Running",
            "ServerStatus.Stopped",
        };

        Assert.Equal(
            ["ServerStatus_Running", "ServerStatus_"],
            ResourceKeyLiterals(source, resources).Select(item => item.Key));
    }

    [Theory]
    [InlineData("PermissionConsentDialog_Title")]
    [InlineData("Scripts_RemoveConfirmTitle")]
    [InlineData("Scripts_RemoveConfirmCancel")]
    public void ObsoleteResourceAliasesAreAbsent(string alias) {
        Assert.DoesNotContain(alias, LoadResourceMap("en-US").Keys);
        Assert.DoesNotContain(alias, LoadResourceMap("zh-TW").Keys);
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

    private static IEnumerable<(string Key, int Line)> ResourceKeyLiterals(
        string source,
        IReadOnlySet<string> resourceNames) =>
        Regex.Matches(source, @"""(?:\\.|[^""\\])*""")
            .Select(match => (
                Key: match.Value[1..^1],
                Line: source.AsSpan(0, match.Index).Count('\n') + 1))
            .Where(item => IsUnderscoreSeparatedResource(item.Key, resourceNames));

    private static bool IsUnderscoreSeparatedResource(
        string candidate,
        IReadOnlySet<string> resourceNames) {
        if (!candidate.Contains('_', StringComparison.Ordinal))
            return false;
        if (resourceNames.Contains(candidate))
            return !candidate.Contains('.', StringComparison.Ordinal) ||
                Regex.IsMatch(candidate, @"_[A-Z]");
        return resourceNames.Any(resource =>
            candidate.Length <= resource.Length &&
            candidate.Select((character, index) => (character, index)).All(item =>
                item.character == resource[item.index] ||
                item.character == '_' && resource[item.index] == '.') &&
            candidate.Select((character, index) =>
                character == '_' && resource[index] == '.').Any(matches => matches));
    }

    private static string[] Placeholders(string value) =>
        Regex.Matches(value, @"\{[A-Za-z][A-Za-z0-9_]*\}")
            .Select(match => match.Value)
            .Distinct(StringComparer.Ordinal)
            .Order(StringComparer.Ordinal)
            .ToArray();

    private static readonly IReadOnlyDictionary<string, IReadOnlySet<string>>
        XamlDisplayProperties =
            new Dictionary<string, IReadOnlySet<string>>(StringComparer.Ordinal) {
                ["AppBarButton"] = Set("Label"),
                ["AutoSuggestBox"] = Set("PlaceholderText"),
                ["Button"] = Set("Content"),
                ["CheckBox"] = Set("Content"),
                ["ComboBox"] = Set("Header", "PlaceholderText"),
                ["ContentDialog"] = Set(
                    "Title", "Content", "PrimaryButtonText",
                    "SecondaryButtonText", "CloseButtonText"),
                ["Expander"] = Set("Header"),
                ["HyperlinkButton"] = Set("Content"),
                ["InfoBar"] = Set("Title", "Message"),
                ["MenuFlyoutItem"] = Set("Text"),
                ["NavigationViewItem"] = Set("Content"),
                ["NumberBox"] = Set("Header", "PlaceholderText"),
                ["PasswordBox"] = Set("Header", "PlaceholderText"),
                ["RadioButton"] = Set("Content"),
                ["SelectorBarItem"] = Set("Text"),
                ["TeachingTip"] = Set(
                    "Title", "Subtitle", "ActionButtonContent",
                    "CloseButtonContent"),
                ["TextBlock"] = Set("Text"),
                ["TextBox"] = Set("Header", "PlaceholderText", "Text"),
                ["TitleBar"] = Set("Title"),
                ["ToggleSwitch"] = Set("Header", "OnContent", "OffContent"),
                ["Window"] = Set("Title"),
            };

    private static IReadOnlySet<string> Set(params string[] properties) =>
        properties.ToHashSet(StringComparer.Ordinal);

    private static bool IsAttachedResourceProperty(string property) =>
        property.StartsWith("[", StringComparison.Ordinal) ||
        property.StartsWith("ToolTipService.", StringComparison.Ordinal);

    private static IEnumerable<string> XamlUids() =>
        XamlUidElements().Select(item => item.Uid);

    private static IEnumerable<(string Uid, string Element)> XamlUidElements() {
        XNamespace x = "http://schemas.microsoft.com/winfx/2006/xaml";
        foreach (var path in Directory.EnumerateFiles(
                     AppRoot, "*.xaml", SearchOption.AllDirectories)) {
            foreach (var element in XDocument.Load(path).Root!.DescendantsAndSelf()) {
                if (element.Attribute(x + "Uid") is { Value: var uid })
                    yield return (uid, element.Name.LocalName);
            }
        }
    }

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
