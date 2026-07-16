using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class MainSurfaceFiniteStateTests {
    [Fact]
    public void EngineConnectionStateIsSemanticAndLocalizedInHomeXaml() {
        var viewModel = RepositoryLayout.ReadAppFile(
            "ViewModels", "MainViewModel.cs");
        var xaml = RepositoryLayout.ReadAppFile("Views", "HomePage.xaml");

        Assert.Contains("EngineConnectionState", viewModel,
                        StringComparison.Ordinal);
        Assert.DoesNotContain("_localizer.Get(\"EngineStatus_", viewModel,
                              StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"EngineStatusConnectingText\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"EngineStatusConnectedText\"", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"EngineStatusFailedText\"", xaml,
                        StringComparison.Ordinal);
    }

    [Fact]
    public void InstallErrorsDoNotExposeGrpcDiagnosticDetails() {
        var viewModel = RepositoryLayout.ReadAppFile(
            "ViewModels", "MainViewModel.cs");

        Assert.DoesNotContain("ex.Status.Detail", viewModel,
                              StringComparison.Ordinal);
    }

    [Fact]
    public void ServerCardAndHeaderFiniteLabelsAreLocalizedInXaml() {
        var model = RepositoryLayout.ReadAppFile("Models", "ServerItem.cs");
        var home = RepositoryLayout.ReadAppFile("Views", "HomePage.xaml");
        var header = RepositoryLayout.ReadAppFile(
            "Controls", "Server", "ServerHeaderPanel.xaml");
        var xaml = home + '\n' + header;

        foreach (var property in new[] {
                     "public string StatusText", "public string StateActionText",
                     "public string DeleteActionText",
                 })
            Assert.DoesNotContain(property, model, StringComparison.Ordinal);

        foreach (var keyPrefix in new[] {
                     "ServerState_", "ServerDelete_Action",
                     "ServerInstallRemove_Action", "ServerType_",
                 })
            Assert.DoesNotContain($"\"{keyPrefix}", model,
                                  StringComparison.Ordinal);

        foreach (var uid in new[] {
                     "ServerStatusRunningText", "ServerStatusStoppedText",
                     "ServerStatusInstallingText", "ServerStatusStartingText",
                     "ServerStatusStoppingText", "ServerStatusCrashedText",
                     "ServerStatusUnknownText", "ServerStateStartText",
                     "ServerStateStopText", "ServerStateStartingText",
                     "ServerStateStoppingText", "ServerStateInstallingText",
                     "ServerTypeVanillaText", "ServerTypePaperText",
                     "ServerTypeSpigotText", "ServerTypeForgeText",
                     "ServerTypeNeoForgeText", "ServerTypeFabricText",
                     "ServerTypeUnknownText",
                 })
            Assert.Contains($"x:Uid=\"{uid}\"", xaml,
                            StringComparison.Ordinal);

        Assert.Contains("x:Uid=\"HomeCardDeleteMenuItem\"", home,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"HomeCardRemoveIncompleteMenuItem\"", home,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ServerDeleteMenuItem\"", header,
                        StringComparison.Ordinal);
        Assert.Contains("x:Uid=\"ServerRemoveIncompleteMenuItem\"", header,
                        StringComparison.Ordinal);
    }
}
