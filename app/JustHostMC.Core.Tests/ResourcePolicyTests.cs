using System.Text.RegularExpressions;
using System.Xml.Linq;
using JustHostMC.App.Controls;
using Xunit;

namespace JustHostMC.Core.Tests;

public sealed class ResourcePolicyTests {
    private static readonly string Root = FindRepositoryRoot();
    private static readonly string AppRoot =
        Path.Combine(Root, "app", "JustHostMC.App");
    private static readonly IReadOnlySet<string> RuntimeXamlLookupExceptions =
        new HashSet<string>(StringComparer.OrdinalIgnoreCase);
    private static readonly IReadOnlySet<string> Task4MigratedStaticResourceKeys =
        Set(
            "AppTitle",
            "MainWindow.Title",
            "MainWindowTitleBar.Title",
            "CreateServerDialog.Title",
            "CreateServerDialog.PrimaryButtonText",
            "CreateServerDialog.CloseButtonText",
            "BanListStoppedNotice.Title",
            "BanListStoppedNotice.Message",
            "EngineMonitor.Title",
            "EngineStdioWindow.Title",
            "EngineMonitorTitleBar.Title",
            "ServerDelete.Title",
            "ServerDelete.Body",
            "ServerDelete.Confirm",
            "Common.Cancel",
            "DeleteServerDialog.Title",
            "DeleteServerDialog.Content",
            "DeleteServerDialog.PrimaryButtonText",
            "DeleteServerDialog.CloseButtonText",
            "EditServerDialog.Title",
            "EditServerDialog.PrimaryButtonText",
            "EditServerDialog.CloseButtonText",
            "EditServerName.Header",
            "RenameServerDialog.Title",
            "Common.Save",
            "RenameServerDialog.PrimaryButtonText",
            "RenameServerDialog.CloseButtonText",
            "RenameServerNameBox.Header",
            "ScriptLogsWindow.Title",
            "ScriptLogsTitleBar.Title",
            "PermissionConsentDialog.PrimaryButtonText",
            "PermissionConsentDialog.CloseButtonText",
            "BackupsDialog.CloseButtonText",
            "ServerFolder.NotFoundTitle",
            "ServerFolder.NotFoundBody",
            "ServerFolderMissingDialog.Title",
            "ServerFolderMissingDialog.Content",
            "ServerFolderMissingDialog.CloseButtonText",
            "Shop.DependencyPromptTitle",
            "Shop.DependencyPromptBody",
            "Shop.InstallConfirm",
            "DependencyPromptDialog.Title",
            "DependencyPromptDialog.PrimaryButtonText",
            "DependencyPromptDialog.CloseButtonText",
            "DependencyPromptBody.Text",
            "ShopWindow.Title",
            "ShopWindowTitleBar.Title");

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
    public void StaticXamlResourcePropertiesAreNotReadImperatively() {
        var resourceKeys = StaticXamlResourceKeys()
            .Except(RuntimeXamlLookupExceptions, StringComparer.OrdinalIgnoreCase)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);
        var offenders = Directory.EnumerateFiles(
                AppRoot, "*.xaml.cs", SearchOption.AllDirectories)
            .SelectMany(path => StaticXamlResourceKeysReadImperatively(
                    File.ReadAllText(path), resourceKeys)
                .Select(key => $"{Path.GetRelativePath(Root, path)} ({key})"));
        Assert.Empty(offenders);
    }

    [Fact]
    public void Task4StaticResourceKeysAreDetectedInSyntheticSource() {
        const string source = """
            _localizer.Get("ServerDelete.Title");
            _localizer.Get("DeleteServerDialog.Title");
            _localizer.Get("HomeTitle.Text");
            _localizer.Get("ServerTeachingTip.StartAction");
            """;

        Assert.Equal(
            [
                "ServerDelete.Title",
                "DeleteServerDialog.Title",
                "HomeTitle.Text",
            ],
            StaticXamlResourceKeysReadImperatively(source));
    }

    [Fact]
    public void ReusableContentDialogsReleaseRuntimeContentAndHandlers() {
        var main = ReadAppSource("MainWindow.xaml.cs");
        var home = ReadAppSource("Views/HomePage.xaml.cs");
        var server = ReadAppSource("Views/ServerPage.xaml.cs");
        var scripts = ReadAppSource("Views/ScriptsPage.xaml.cs");
        var shop = ReadAppSource("Views/ShopDetailPage.xaml.cs");

        foreach (var source in new[] { main, home, server })
            Assert.DoesNotContain("CanSubmitChanged += (_, _)", source,
                                  StringComparison.Ordinal);

        AssertServerDialogCleanup(MethodBlock(
            main, "Task ShowCreateServerDialogAsync()"));
        AssertServerDialogCleanup(MethodBlock(
            home, "private async void OnAddCardClick("));
        AssertServerDialogCleanup(MethodBlock(
            home, "private async Task ShowEditDialogAsync("));
        AssertServerDialogCleanup(MethodBlock(
            server, "private async Task ShowEditDialogAsync()"));

        var backupsFinally = FinallyBlock(MethodBlock(
            server, "private async void OnBackupsClick("));
        Assert.Contains("dialog.Opened -= OnOpened;", backupsFinally,
                        StringComparison.Ordinal);
        Assert.Contains("dialog.Content = null;", backupsFinally,
                        StringComparison.Ordinal);
        Assert.Contains("dialog.Title = null;", backupsFinally,
                        StringComparison.Ordinal);

        var consentFinally = FinallyBlock(MethodBlock(
            scripts,
            "private async Task<IReadOnlyList<PermissionKind>?> RequestConsentAsync("));
        Assert.Contains("dialog.Content = null;", consentFinally,
                        StringComparison.Ordinal);
        Assert.Contains("dialog.Title = null;", consentFinally,
                        StringComparison.Ordinal);

        AssertDependencyDialogCleanup(MethodBlock(
            shop, "private async void OnInstallClick("));
        AssertRenameDialogPreservesXamlContent(MethodBlock(
            home, "private async Task ShowRenameDialogAsync("));
        AssertRenameDialogPreservesXamlContent(MethodBlock(
            server, "private async Task ShowRenameDialogAsync()"));
    }

    [Fact]
    public void DialogLifecycleAssertionsRejectSyntheticRegressions() {
        const string leakedDependencies = """
            try {
                await dialog.ShowAsync();
            } finally {
            }
            """;
        const string replacedRenameContent = """
            var dialog = (ContentDialog)Resources["RenameServerDialog"];
            var nameBox = (TextBox)dialog.Content;
            dialog.Content = new TextBox();
            """;

        Assert.ThrowsAny<Xunit.Sdk.XunitException>(
            () => AssertDependencyDialogCleanup(leakedDependencies));
        Assert.ThrowsAny<Xunit.Sdk.XunitException>(
            () => AssertRenameDialogPreservesXamlContent(
                replacedRenameContent));
    }

    [Fact]
    public void ContentDialogSizingUsesMutableModeWithOneTimeHandlers() {
        var source = File.ReadAllText(Path.Combine(
            AppRoot, "Controls", "ContentDialogSizing.cs"));
        Assert.Contains("ConditionalWeakTable<ContentDialog, SizingState>",
                        source, StringComparison.Ordinal);
        Assert.Contains("state.UseWideLayout = useWideLayout;", source,
                        StringComparison.Ordinal);
        Assert.Contains("state.Apply(dialog);", source,
                        StringComparison.Ordinal);
        Assert.Equal(1, CountOccurrences(source, "dialog.Loaded += state.OnLoaded;"));
        Assert.Equal(1, CountOccurrences(
            source, "dialog.SizeChanged += state.OnSizeChanged;"));
    }

    [Fact]
    public void ContentDialogSizingStateUsesTheLatestLayoutMode() {
        var state = new ContentDialogSizingState();

        Assert.Equal((720d, 560d), state.Calculate(availableWidth: 1200));
        state.UseWideLayout = true;
        Assert.Equal((960d, 720d), state.Calculate(availableWidth: 1200));
        state.UseWideLayout = false;
        Assert.Equal((720d, 560d), state.Calculate(availableWidth: 1200));
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

    private static string ReadAppSource(string relativePath) =>
        File.ReadAllText(Path.Combine(
            AppRoot, relativePath.Replace('/', Path.DirectorySeparatorChar)));

    private static void AssertServerDialogCleanup(string method) {
        Assert.Contains("content.CanSubmitChanged += OnCanSubmitChanged;", method,
                        StringComparison.Ordinal);
        var cleanup = FinallyBlock(method);
        Assert.Contains("content.CanSubmitChanged -= OnCanSubmitChanged;", cleanup,
                        StringComparison.Ordinal);
        Assert.Contains("dialog.Content = null;", cleanup,
                        StringComparison.Ordinal);
        Assert.Contains("dialog.IsPrimaryButtonEnabled = false;", cleanup,
                        StringComparison.Ordinal);
    }

    private static void AssertDependencyDialogCleanup(string method) {
        var cleanup = FinallyBlock(method);
        Assert.Contains("foreach (var pick in picks)", cleanup,
                        StringComparison.Ordinal);
        Assert.Contains("panel.Children.Remove(pick);", cleanup,
                        StringComparison.Ordinal);
    }

    private static void AssertRenameDialogPreservesXamlContent(string method) {
        Assert.Contains("var nameBox = (TextBox)dialog.Content;", method,
                        StringComparison.Ordinal);
        Assert.DoesNotMatch(@"dialog\.Content\s*=", method);
    }

    private static IReadOnlySet<string> StaticXamlResourceKeys() =>
        XamlUidElements()
            .SelectMany(item => LoadResourceMap("en-US").Keys.Where(key =>
                key.StartsWith(item.Uid + ".",
                               StringComparison.OrdinalIgnoreCase)))
            .Union(Task4MigratedStaticResourceKeys,
                   StringComparer.OrdinalIgnoreCase)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);

    private static string[] StaticXamlResourceKeysReadImperatively(
        string source,
        IReadOnlySet<string>? staticResourceKeys = null) {
        staticResourceKeys ??= StaticXamlResourceKeys();
        return Regex.Matches(source, @"\bGet\(\s*""(?<key>[^""]+)""")
            .Select(match => match.Groups["key"].Value)
            .Where(staticResourceKeys.Contains)
            .ToArray();
    }

    private static int CountOccurrences(string source, string value) =>
        source.Split(value, StringSplitOptions.None).Length - 1;

    private static string MethodBlock(string source, string signature) {
        var signatureStart = source.IndexOf(signature, StringComparison.Ordinal);
        Assert.True(signatureStart >= 0, $"Missing method {signature}");
        return BraceBlock(source, signatureStart);
    }

    private static string FinallyBlock(string method) {
        var finallyStart = method.IndexOf("finally", StringComparison.Ordinal);
        Assert.True(finallyStart >= 0, "Missing finally block");
        return BraceBlock(method, finallyStart);
    }

    private static string BraceBlock(string source, int searchStart) {
        var open = source.IndexOf('{', searchStart);
        Assert.True(open >= 0, "Missing opening brace");
        var depth = 0;
        for (var index = open; index < source.Length; index++) {
            if (source[index] == '{')
                depth++;
            else if (source[index] == '}' && --depth == 0)
                return source[open..(index + 1)];
        }
        throw new Xunit.Sdk.XunitException("Missing closing brace");
    }

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
