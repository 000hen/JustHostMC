using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class FeatureFiniteStateTests {
    [Fact]
    public void ModsFiniteLabelsUseSemanticStateAndXamlResources() {
        var viewModel = RepositoryLayout.ReadAppFile(
            "ViewModels", "ModsViewModel.cs");
        var xaml = RepositoryLayout.ReadAppFile(
            "Controls", "Server", "ServerModsPanel.xaml");

        Assert.Contains("ModsWorkflowStatus", viewModel,
                        StringComparison.Ordinal);
        Assert.DoesNotContain("KindLabel", viewModel,
                              StringComparison.Ordinal);
        Assert.DoesNotContain("_localizer.Get(\"Mods_Export", viewModel,
                              StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ServerModsTitleConverter\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ModsStatusExported\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ModsStatusExportFailed\"", xaml,
                        StringComparison.Ordinal);
    }

    [Fact]
    public void SettingsFiniteLabelsUseSemanticStateAndXamlResources() {
        var viewModel = RepositoryLayout.ReadAppFile(
            "ViewModels", "SettingsViewModel.cs");
        var xaml = RepositoryLayout.ReadAppFile("Views", "SettingsPage.xaml");

        Assert.Contains("SettingsWorkflowStatus", viewModel,
                        StringComparison.Ordinal);
        Assert.Contains("ShopKeyConfiguration", viewModel,
                        StringComparison.Ordinal);
        Assert.Contains("BackendMode", viewModel, StringComparison.Ordinal);
        Assert.DoesNotContain("ActiveModeText", viewModel,
                              StringComparison.Ordinal);
        Assert.DoesNotContain("CurseForgeKeyStatus", viewModel,
                              StringComparison.Ordinal);

        foreach (var uid in new[] {
                     "SettingsBackendModeDocker",
                     "SettingsBackendModeOnMachine",
                     "SettingsDockerUnavailable",
                     "SettingsShopKeyNone",
                     "SettingsShopKeyUser",
                     "SettingsShopKeyBuiltin",
                     "SettingsStatusSaved",
                     "SettingsStatusSaveFailed",
                 }) {
            Assert.Contains($"x:Uid=\"{uid}\"", xaml,
                            StringComparison.Ordinal);
        }
    }

    [Fact]
    public void ShopFiniteLabelsUseSemanticStateAndXamlResources() {
        var shop = RepositoryLayout.ReadAppFile(
            "ViewModels", "ShopViewModel.cs");
        var detail = RepositoryLayout.ReadAppFile(
            "ViewModels", "ShopDetailViewModel.cs");
        var window = RepositoryLayout.ReadAppFile("Views", "ShopWindow.xaml");
        var detailXaml = RepositoryLayout.ReadAppFile(
            "Views", "ShopDetailPage.xaml");

        Assert.Contains("HasLoadFailure", shop, StringComparison.Ordinal);
        Assert.DoesNotContain("_localizer.Get(\"Shop_LoadFailed\")", shop,
                              StringComparison.Ordinal);
        Assert.Contains("ShopDetailStatus", detail, StringComparison.Ordinal);
        Assert.DoesNotContain("_localizer.Get(\"Shop_InstallDone\")", detail,
                              StringComparison.Ordinal);
        Assert.DoesNotContain("_localizer.Get(\"Shop_InstallAction\")", detail,
                              StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ShopLoadFailedBar\"", window,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ShopInstallActionText\"", detailXaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ShopStatusInstallDone\"", detailXaml,
                        StringComparison.Ordinal);
    }

    [Fact]
    public void ScriptLogFiniteLabelsAreRenderedByXaml() {
        var viewModel = RepositoryLayout.ReadAppFile(
            "ViewModels", "ScriptsViewModel.cs");
        var xaml = RepositoryLayout.ReadAppFile(
            "Views", "ScriptLogsWindow.xaml");

        foreach (var key in new[] {
                     "Scripts_SystemLogName",
                     "Scripts_LogEntryFallbackTitle",
                     "Scripts_CurrentSessionTitle",
                     "Scripts_PreviousSessionTitle",
                 }) {
            Assert.DoesNotContain($"_localizer.Get(\"{key}\")", viewModel,
                                  StringComparison.Ordinal);
        }
        Assert.Contains("x:Uid=\"ScriptLogsSystemName\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ScriptLogsCurrentSessionTitle\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ScriptLogsPreviousSessionTitle\"", xaml,
                        StringComparison.Ordinal);
    }
}
