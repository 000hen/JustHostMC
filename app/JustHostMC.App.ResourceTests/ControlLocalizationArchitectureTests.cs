using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class ControlLocalizationArchitectureTests {
    [Fact]
    public void AssignedControlsDoNotFetchStaticOrFiniteStateStringsFromCode() {
        var forbiddenKeys = new[] {
            "LogViewer_JumpToLatest",
            "Scripts_RemoveConfirmPrimary",
            "ServerSectionConfig/Text",
            "ServerSectionConfigHint/Text",
            "ConfigStoppedHint/Text",
            "Server_PortAutoValue",
            "Server_ValueUnknown",
            "ServerSectionModsHint/Text",
            "ModsStoppedHint/Text",
            "ServerSectionPerformance/Text",
            "ServerSectionPerformanceHint/Text",
            "PlayersEmptyHint/Text",
            "ServerSectionPlayersHint/Text",
        };
        var sourceFiles = new[] {
            RepositoryLayout.AppPath("Controls", "LogViewer.xaml.cs"),
            RepositoryLayout.AppPath("Controls", "ScriptEntryCard.xaml.cs"),
            RepositoryLayout.AppPath("Controls", "Server", "ServerConfigPanel.xaml.cs"),
            RepositoryLayout.AppPath("Controls", "Server", "ServerHeaderPanel.xaml.cs"),
            RepositoryLayout.AppPath("Controls", "Server", "ServerModsPanel.xaml.cs"),
            RepositoryLayout.AppPath("Controls", "Server", "ServerPerformancePanel.xaml.cs"),
            RepositoryLayout.AppPath("Controls", "Server", "ServerPlayersPanel.xaml.cs"),
            RepositoryLayout.AppPath("Views", "SettingsPage.xaml.cs"),
        };
        var source = string.Join('\n', sourceFiles.Select(File.ReadAllText));

        foreach (var key in forbiddenKeys)
            Assert.DoesNotContain($"\"{key}\"", source, StringComparison.Ordinal);

        var settingsXaml = File.ReadAllText(RepositoryLayout.AppPath(
            "Views", "SettingsPage.xaml"));
        Assert.DoesNotContain("PlaceholderText=\"API Key\"", settingsXaml,
                              StringComparison.Ordinal);
    }
}
