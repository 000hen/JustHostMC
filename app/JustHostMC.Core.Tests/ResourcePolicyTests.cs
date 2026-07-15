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
            "MainWindow.Title",
            "MainWindowTitleBar.Title",
            "CreateServerDialog.Title",
            "CreateServerDialog.PrimaryButtonText",
            "CreateServerDialog.CloseButtonText",
            "BanListStoppedNotice.Title",
            "BanListStoppedNotice.Message",
            "EngineStdioWindow.Title",
            "EngineMonitorTitleBar.Title",
            "DeleteServerDialog.Title",
            "DeleteServerDialog.Content",
            "DeleteServerDialog.PrimaryButtonText",
            "DeleteServerDialog.CloseButtonText",
            "EditServerDialog.Title",
            "EditServerDialog.PrimaryButtonText",
            "EditServerDialog.CloseButtonText",
            "EditServerName.Header",
            "RenameServerDialog.Title",
            "RenameServerDialog.PrimaryButtonText",
            "RenameServerDialog.CloseButtonText",
            "RenameServerNameBox.Header",
            "ScriptLogsWindow.Title",
            "ScriptLogsTitleBar.Title",
            "PermissionConsentDialog.PrimaryButtonText",
            "PermissionConsentDialog.CloseButtonText",
            "ServerFolderMissingDialog.Title",
            "ServerFolderMissingDialog.Content",
            "ServerFolderMissingDialog.CloseButtonText",
            "DependencyPromptDialog.Title",
            "DependencyPromptDialog.PrimaryButtonText",
            "DependencyPromptDialog.CloseButtonText",
            "DependencyPromptBody.Text",
            "ShopWindow.Title",
            "ShopWindowTitleBar.Title");
    private static readonly IReadOnlyList<DuplicateResourceRule>
        DuplicateResourceRules = [
            Rule("JustHostMC", "PROPERTY-CONTEXT", "canonical AppDisplayName; the manifest value, Window.Title, and TitleBar.Title have incompatible owners", "AppDisplayName", "MainWindow.Title", "MainWindowTitleBar.Title"),
            Rule("Create server", "SEMANTIC-HOMOGRAPH", "CreateServerButtonLabel.Text is an action label while CreateServerDialog.Title is a dialog heading", "CreateServerButtonLabel.Text", "CreateServerDialog.Title"),
            Rule("Running", "SEMANTIC-HOMOGRAPH", "HomeRunningServersLabel.Text labels a server-count summary while ServerStatus.Running is one server's lifecycle state", "HomeRunningServersLabel.Text", "ServerStatus.Running"),
            Rule("Installing", "RUNTIME-CONTRACT", "canonical ServerStatus.Installing; ServerState.Installing is a state-change message key while ServerStatus.Installing is a status-enum key", "ServerState.Installing", "ServerStatus.Installing"),
            Rule("Starting", "RUNTIME-CONTRACT", "canonical ServerStatus.Starting; ServerState.Starting is a state-change message key while ServerStatus.Starting is a status-enum key", "ServerState.Starting", "ServerStatus.Starting"),
            Rule("Stopping", "RUNTIME-CONTRACT", "canonical ServerStatus.Stopping; ServerState.Stopping is a state-change message key while ServerStatus.Stopping is a status-enum key", "ServerState.Stopping", "ServerStatus.Stopping"),
            Rule("Unknown", "SEMANTIC-HOMOGRAPH", "the keys mean unavailable memory, unknown lifecycle status, and unknown server type respectively", "ServerMetaMemoryUnknownValue.Text", "ServerStatus.Unknown", "ServerType.Unknown"),
            Rule("Cancel", "PROPERTY-CONTEXT", "canonical CreateServerDialog.CloseButtonText; each ContentDialog x:Uid also owns a distinct title and actions", "CreateServerDialog.CloseButtonText", "DeleteServerDialog.CloseButtonText", "DependencyPromptDialog.CloseButtonText", "EditServerDialog.CloseButtonText", "PermissionConsentDialog.CloseButtonText", "RenameServerDialog.CloseButtonText"),
            Rule("Name", "PROPERTY-CONTEXT", "canonical CreateServerName.Header; CreateServerName also owns PlaceholderText so it cannot share RenameServerNameBox's x:Uid", "CreateServerName.Header", "RenameServerNameBox.Header"),
            Rule("Type", "PROPERTY-CONTEXT", "canonical CommonTypeComboBox.Header; TextBox.Header and TextBlock.Text target incompatible control/property variants", "CommonTypeComboBox.Header", "CommonTypeLabel.Text", "EditServerType.Header"),
            Rule("Version", "PROPERTY-CONTEXT", "canonical CommonVersionLabel.Text; CreateServerVersion.Header belongs to a ComboBox that also owns PlaceholderText", "CommonVersionLabel.Text", "CreateServerVersion.Header"),
            Rule("Port", "PROPERTY-CONTEXT", "canonical ServerMetaPortLabel.Text; CreateServerPort.Header targets NumberBox.Header rather than TextBlock.Text", "CreateServerPort.Header", "ServerMetaPortLabel.Text"),
            Rule("Dismiss", "PROPERTY-CONTEXT", "canonical CommonDismissButton.Content; the icon-only dismiss owner needs ToolTipService.ToolTip instead of Button.Content", "CommonDismissButton.Content", "CommonDismissTooltipButton.ToolTipService.ToolTip"),
            Rule("Close", "PROPERTY-CONTEXT", "canonical CommonCloseDialog.CloseButtonText; BanListHostDialog also owns context-specific dialog resources", "BanListHostDialog.CloseButtonText", "CommonCloseDialog.CloseButtonText"),
            Rule("Remove all data", "SEMANTIC-HOMOGRAPH", "SettingsDataHeader.Text names the data section while SettingsRemoveDataButton.Content is a destructive action", "SettingsDataHeader.Text", "SettingsRemoveDataButton.Content"),
            Rule("Stop", "SEMANTIC-HOMOGRAPH", "EngineMonitorStopButton.Content is a monitor command while ServerState.Stop is a server-state action label", "EngineMonitorStopButton.Content", "ServerState.Stop"),
            Rule("Home", "PROPERTY-CONTEXT", "canonical HomeTitle.Text; the page heading targets TextBlock.Text while navigation targets NavigationViewItem.Content", "HomeTitle.Text", "NavHome.Content"),
            Rule("Settings", "PROPERTY-CONTEXT", "canonical SettingsTitle.Text; the page heading targets TextBlock.Text while navigation targets NavigationViewItem.Content", "NavSettings.Content", "SettingsTitle.Text"),
            Rule("Plugins / Mods", "PROPERTY-CONTEXT", "canonical ServerSectionMods.Text; SelectorBarItem.Text and the panel Title property have incompatible owners", "ServerSectionMods.Text", "ServerSectionModsPanel.Title"),
            Rule("Configuration", "PROPERTY-CONTEXT", "canonical ServerSectionConfig.Text; SelectorBarItem.Text and the panel Title property have incompatible owners", "ServerSectionConfig.Text", "ServerSectionConfigPanel.Title"),
            Rule("Performance", "PROPERTY-CONTEXT", "canonical ServerSectionPerformance.Text; SelectorBarItem.Text and the panel Title property have incompatible owners", "ServerSectionPerformance.Text", "ServerSectionPerformancePanel.Title"),
            Rule("Auto", "RUNTIME-CONTRACT", "canonical ServerMetaPortAutoValue.Text; Server.PortAutoValue is returned by runtime metadata lookup", "Server.PortAutoValue", "ServerMetaPortAutoValue.Text"),
            Rule("Manage bans", "PROPERTY-CONTEXT", "canonical ManageBansButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; the composed button also needs a child TextBlock.Text label", "ManageBansButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name", "ManageBansButtonLabel.Text"),
            Rule("Player", "SEMANTIC-HOMOGRAPH", "BanList.TypePlayer is a ban-entry type while BanListTypePlayerItem.Content is the corresponding picker option", "BanList.TypePlayer", "BanListTypePlayerItem.Content"),
            Rule("Inventory", "SEMANTIC-HOMOGRAPH", "PlayerInventoryDialog.ActionName is a dialog-title fragment while PlayerInventoryMainHeader.Text is a section heading", "PlayerInventoryDialog.ActionName", "PlayerInventoryMainHeader.Text"),
            Rule("Equipment", "SEMANTIC-HOMOGRAPH", "PlayerInventoryEquipmentHeader.Text is a player-data section while shop.category.equipment is a mod-shop category", "PlayerInventoryEquipmentHeader.Text", "shop.category.equipment"),
            Rule("Remove", "PROPERTY-CONTEXT", "canonical CommonRemoveButton.Content; icon-only automation owners cannot use visible Button.Content", "CommonRemoveAutomationButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name", "CommonRemoveButton.Content"),
            Rule("The operation failed. Please try again.", "SEMANTIC-HOMOGRAPH", "Mods.OperationFailed reports mod operations while Scripts.OperationFailed reports automation-script operations", "Mods.OperationFailed", "Scripts.OperationFailed"),
            Rule("Delete", "PROPERTY-CONTEXT", "canonical CommonDeleteMenuItem.Text; menu text and ContentDialog.PrimaryButtonText have incompatible owners", "CommonDeleteMenuItem.Text", "DeleteServerDialog.PrimaryButtonText"),
            Rule("Copy address", "PROPERTY-CONTEXT", "canonical CommonCopyAddressButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; the same icon button also needs ToolTipService.ToolTip", "CommonCopyAddressButton.ToolTipService.ToolTip", "CommonCopyAddressButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name"),
            Rule("Rename server", "PROPERTY-CONTEXT", "canonical CommonRenameServerButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; the tooltip and dialog title target incompatible properties", "CommonRenameServerButton.ToolTipService.ToolTip", "CommonRenameServerButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name", "RenameServerDialog.Title"),
            Rule("Server actions", "PROPERTY-CONTEXT", "canonical HomeCardMoreButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; the same icon button also needs ToolTipService.ToolTip", "HomeCardMoreButton.ToolTipService.ToolTip", "HomeCardMoreButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name"),
            Rule("Move server up", "PROPERTY-CONTEXT", "canonical HomeCardMoveUpButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; the same icon button also needs ToolTipService.ToolTip", "HomeCardMoveUpButton.ToolTipService.ToolTip", "HomeCardMoveUpButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name"),
            Rule("Move server down", "PROPERTY-CONTEXT", "canonical HomeCardMoveDownButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; the same icon button also needs ToolTipService.ToolTip", "HomeCardMoveDownButton.ToolTipService.ToolTip", "HomeCardMoveDownButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name"),
            Rule("Edit server", "PROPERTY-CONTEXT", "canonical CommonEditServerButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; tooltip, menu text, and dialog title target incompatible properties", "CommonEditServerButton.ToolTipService.ToolTip", "CommonEditServerButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name", "CommonEditServerMenuItem.Text", "EditServerDialog.Title"),
            Rule("Save", "PROPERTY-CONTEXT", "canonical EditServerDialog.PrimaryButtonText; each ContentDialog x:Uid also owns a distinct title and close action", "EditServerDialog.PrimaryButtonText", "RenameServerDialog.PrimaryButtonText"),
            Rule("Scripts", "PROPERTY-CONTEXT", "canonical ScriptsPageTitle.Text; the page heading targets TextBlock.Text while navigation targets NavigationViewItem.Content", "NavScripts.Content", "ScriptsPageTitle.Text"),
            Rule("Built in", "PROPERTY-CONTEXT", "canonical ScriptsBuiltinBadge.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; the badge also needs a child TextBlock.Text label", "ScriptsBuiltinBadge.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name", "ScriptsBuiltinBadgeLabel.Text"),
            Rule("All Automation Logs", "PROPERTY-CONTEXT", "canonical ScriptLogsWindow.Title; Window.Title and TitleBar.Title have incompatible owners", "ScriptLogsTitleBar.Title", "ScriptLogsWindow.Title"),
            Rule("Previous", "PROPERTY-CONTEXT", "canonical InstallProgressPreviousButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; the same icon button also needs ToolTipService.ToolTip", "InstallProgressPreviousButton.ToolTipService.ToolTip", "InstallProgressPreviousButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name"),
            Rule("Next", "PROPERTY-CONTEXT", "canonical CommonNextButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name; the same icon button also needs ToolTipService.ToolTip", "CommonNextButton.ToolTipService.ToolTip", "CommonNextButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name"),
            Rule("Mod Shop", "PROPERTY-CONTEXT", "canonical ShopWindow.Title; Window.Title and TitleBar.Title have incompatible owners", "ShopWindow.Title", "ShopWindowTitleBar.Title"),
            Rule("Recently updated", "SEMANTIC-HOMOGRAPH", "ShopSortUpdated.Content is a sort criterion while shop.home.updated is a discovery-section heading", "ShopSortUpdated.Content", "shop.home.updated"),
            Rule("Engine debug monitor", "PROPERTY-CONTEXT", "canonical EngineStdioWindow.Title; Window.Title and TitleBar.Title have incompatible owners", "EngineMonitorTitleBar.Title", "EngineStdioWindow.Title"),
        ];

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
    public void DuplicateEnglishValuesHaveExplicitOwnership() {
        Assert.Empty(DuplicateOwnershipViolations(
            LoadResources("en-US"), LoadResources("zh-TW"),
            DuplicateResourceRules));
    }

    [Fact]
    public void GenericDuplicateCommentsDoNotApproveUnknownGroups() {
        var english = SyntheticResources(
            ("First.Text", "Same", "INTENTIONAL DUPLICATE: generic"),
            ("Second.Text", "Same", "INTENTIONAL DUPLICATE: generic"));
        Assert.Contains("unapproved", DuplicateOwnershipViolations(
            english, english, []).Single(), StringComparison.Ordinal);
    }

    [Fact]
    public void DuplicateOwnershipRejectsAllowlistedMembershipChanges() {
        var resources = SyntheticResources(
            ("First.Text", "Same", "DUPLICATE SEMANTIC-HOMOGRAPH: meanings differ"),
            ("Third.Text", "Same", "DUPLICATE SEMANTIC-HOMOGRAPH: meanings differ"));
        var rule = Rule("Same", "SEMANTIC-HOMOGRAPH", "meanings differ",
                        "First.Text", "Second.Text");
        Assert.Contains("members", DuplicateOwnershipViolations(
            resources, resources, [rule]).Single(), StringComparison.Ordinal);
    }

    [Fact]
    public void LocalePlaceholdersMatch() {
        var english = LoadResourceMap("en-US");
        var chinese = LoadResourceMap("zh-TW");
        foreach (var key in english.Keys) {
            Assert.Equal(PlaceholderOccurrences(english[key]),
                         PlaceholderOccurrences(chinese[key]));
        }
    }

    [Fact]
    public void PlaceholderParityPreservesRepeatedOccurrences() {
        Assert.NotEqual(PlaceholderOccurrences("{0}"),
                        PlaceholderOccurrences("{0} {0}"));
        Assert.NotEqual(PlaceholderOccurrences("{name} {name} {count}"),
                        PlaceholderOccurrences("{name} {count}"));
    }

    [Theory]
    [InlineData("en-US")]
    [InlineData("zh-TW")]
    public void FormattedResourcesExplainEveryPlaceholder(string language) {
        var undocumented = LoadResources(language)
            .Select(element => (
                Element: element,
                Comment: element.Element("comment")?.Value ?? string.Empty,
                Placeholders: PlaceholderNames(
                    element.Element("value")?.Value ?? string.Empty)))
            .Where(item => item.Placeholders.Any(placeholder =>
                !item.Comment.Contains(placeholder, StringComparison.Ordinal)))
            .Select(item => ResourceName(item.Element));
        Assert.Empty(undocumented);
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
            _localizer.Get("DeleteServerDialog.Title");
            _localizer.Get("HomeTitle.Text");
            _localizer.Get("ServerTeachingTip.StartAction");
            """;

        Assert.Equal(
            [
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
    [InlineData("Common.Save")]
    [InlineData("Common.Cancel")]
    [InlineData("BackupsDialog.CloseButtonText")]
    [InlineData("PlayerDialogHost.CloseButtonText")]
    [InlineData("ModsRemoveConfirmButton.Content")]
    [InlineData("ServerEditMenuItem.Text")]
    [InlineData("ScriptsRemoveButton.[using:Microsoft.UI.Xaml.Automation]AutomationProperties.Name")]
    [InlineData("Shop.DependencyPromptTitle")]
    [InlineData("Shop.DependencyPromptBody")]
    [InlineData("Shop.InstallConfirm")]
    public void ObsoleteResourceAliasesAreAbsent(string alias) {
        Assert.DoesNotContain(alias, LoadResourceMap("en-US").Keys);
        Assert.DoesNotContain(alias, LoadResourceMap("zh-TW").Keys);
    }

    private static IReadOnlyList<XElement> LoadResources(string language) =>
        XDocument.Load(Path.Combine(AppRoot, "Strings", language, "Resources.resw"))
            .Root!.Elements("data").ToArray();

    private static DuplicateResourceRule Rule(
        string value, string category, string reason, params string[] keys) =>
        new(value, category, reason,
            keys.ToHashSet(StringComparer.Ordinal));

    private static string[] DuplicateOwnershipViolations(
        IReadOnlyList<XElement> english,
        IReadOnlyList<XElement> chinese,
        IReadOnlyList<DuplicateResourceRule> rules) {
        var actual = english
            .GroupBy(element => element.Element("value")?.Value.Trim(),
                     StringComparer.Ordinal)
            .Where(group => !string.IsNullOrEmpty(group.Key) && group.Count() > 1)
            .ToDictionary(group => group.Key!, group => group.ToArray(),
                          StringComparer.Ordinal);
        var approved = rules.ToDictionary(rule => rule.Value,
                                           StringComparer.Ordinal);
        var violations = new List<string>();

        foreach (var (value, elements) in actual) {
            if (!approved.TryGetValue(value, out var rule)) {
                violations.Add($"unapproved duplicate value '{value}'");
                continue;
            }
            var keys = elements.Select(ResourceName)
                .ToHashSet(StringComparer.Ordinal);
            if (!keys.SetEquals(rule.Keys)) {
                violations.Add($"duplicate '{value}' members are " +
                    string.Join(", ", keys.Order(StringComparer.Ordinal)));
                continue;
            }
            foreach (var locale in new[] { english, chinese }) {
                foreach (var element in locale.Where(element =>
                             rule.Keys.Contains(ResourceName(element)))) {
                    var comment = element.Element("comment")?.Value;
                    if (!string.Equals(comment, rule.Comment,
                                       StringComparison.Ordinal)) {
                        violations.Add($"{ResourceName(element)} must document " +
                            $"'{rule.Comment}'");
                    }
                }
            }
        }
        foreach (var rule in rules.Where(rule => !actual.ContainsKey(rule.Value)))
            violations.Add($"allowlisted duplicate '{rule.Value}' no longer exists");
        return violations.ToArray();
    }

    private static IReadOnlyList<XElement> SyntheticResources(
        params (string Key, string Value, string Comment)[] resources) =>
        resources.Select(resource => new XElement("data",
            new XAttribute("name", resource.Key),
            new XElement("value", resource.Value),
            new XElement("comment", resource.Comment))).ToArray();

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

    private static string[] PlaceholderOccurrences(string value) =>
        Regex.Matches(value, @"\{(?:[A-Za-z][A-Za-z0-9_]*|\d+)\}")
            .Select(match => match.Value)
            .Order(StringComparer.Ordinal)
            .ToArray();

    private static string[] PlaceholderNames(string value) =>
        PlaceholderOccurrences(value)
            .Distinct(StringComparer.Ordinal)
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
                     AppRoot, "*.xaml", SearchOption.AllDirectories)
                 .Where(path => !path.Contains(
                     $"{Path.DirectorySeparatorChar}obj{Path.DirectorySeparatorChar}",
                     StringComparison.OrdinalIgnoreCase) &&
                     !path.Contains(
                     $"{Path.DirectorySeparatorChar}bin{Path.DirectorySeparatorChar}",
                     StringComparison.OrdinalIgnoreCase))) {
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

    private sealed record DuplicateResourceRule(
        string Value,
        string Category,
        string Reason,
        IReadOnlySet<string> Keys) {
        public string Comment => $"DUPLICATE {Category}: {Reason}.";
    }
}
