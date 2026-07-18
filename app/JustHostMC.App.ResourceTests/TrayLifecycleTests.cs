using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class TrayLifecycleTests {
    [Fact]
    public void ModpackOperationsRegisterWithBackgroundTaskService() {
        var source =
            RepositoryLayout.ReadAppFile("ViewModels", "MainViewModel.cs");
        var update =
            MethodBetween(source, "public async Task UpdateModpackAsync",
                          "public async Task ExportModpackAsync");
        var export =
            MethodBetween(source, "public async Task ExportModpackAsync",
                          "private void ApplyTrackerProgress");

        Assert.Contains("_backgroundTasks.Begin(\"modpack-update\")", update,
                        StringComparison.Ordinal);
        Assert.Contains("_backgroundTasks.Begin(\"modpack-export\")", export,
                        StringComparison.Ordinal);
    }

    private static string MethodBetween(string source, string startMarker,
                                        string endMarker) {
        var start = source.IndexOf(startMarker, StringComparison.Ordinal);
        Assert.True(start >= 0, $"Missing method marker: {startMarker}");
        var end = source.IndexOf(endMarker, start, StringComparison.Ordinal);
        Assert.True(end > start, $"Missing method marker: {endMarker}");
        return source[start..end];
    }
}
