using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class BackupConfigFiniteStateTests {
    [Fact]
    public void BackupWorkflowUsesSemanticStateAndXamlResources() {
        var viewModel = File.ReadAllText(RepositoryLayout.AppPath(
            "ViewModels", "BackupsViewModel.cs"));
        var xaml = File.ReadAllText(RepositoryLayout.AppPath(
            "Views", "BackupsDialog.xaml"));

        Assert.Contains("BackupStatus", viewModel, StringComparison.Ordinal);
        Assert.DoesNotContain("_localizer.Get(\"Backups_", viewModel,
                              StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"BackupsStatusCreating\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"BackupsStatusCreated\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"BackupsStatusRestoring\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"BackupsStatusRestored\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"BackupsStatusDeleting\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"BackupsStatusDeleted\"", xaml,
                        StringComparison.Ordinal);
        Assert.DoesNotContain("_localizer.Get(",
                              File.ReadAllText(RepositoryLayout.AppPath(
                                  "Views", "BackupsDialog.xaml.cs")),
                              StringComparison.Ordinal);
    }

    [Fact]
    public void ConfigWorkflowUsesSemanticStateAndXamlResources() {
        var viewModel = File.ReadAllText(RepositoryLayout.AppPath(
            "ViewModels", "ServerConfigViewModel.cs"));
        var xaml = File.ReadAllText(RepositoryLayout.AppPath(
            "Controls", "Server", "ServerConfigPanel.xaml"));

        Assert.Contains("ConfigStatus", viewModel, StringComparison.Ordinal);
        Assert.DoesNotContain("_localizer.Get(\"Config_", viewModel,
                              StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ConfigStatusSaved\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ConfigStatusLoadFailed\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ConfigStatusSaveFailed\"", xaml,
                        StringComparison.Ordinal);
    }

    [Fact]
    public void ConfigValueTypesUseSemanticFlagsAndXamlResources() {
        var model = File.ReadAllText(RepositoryLayout.AppPath(
            "Models", "ConfigEntryItem.cs"));
        var xaml = File.ReadAllText(RepositoryLayout.AppPath(
            "Controls", "ConfigEntryEditor.xaml"));

        Assert.DoesNotContain("TypeText", model, StringComparison.Ordinal);
        Assert.DoesNotContain("ConfigType_", model, StringComparison.Ordinal);
        Assert.Contains("IsBoolean", model, StringComparison.Ordinal);
        Assert.Contains("IsInteger", model, StringComparison.Ordinal);
        Assert.Contains("IsChoice", model, StringComparison.Ordinal);
        Assert.Contains("IsText", model, StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ConfigTypeBooleanText\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ConfigTypeIntegerText\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ConfigTypeChoiceText\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ConfigTypeTextText\"", xaml,
                        StringComparison.Ordinal);
    }
}
