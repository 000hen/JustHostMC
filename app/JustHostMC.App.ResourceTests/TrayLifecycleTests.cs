using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class TrayLifecycleTests {
    [Fact]
    public void ServerStatePathsSynchronizePersistentBackgroundActivity() {
        var source =
            RepositoryLayout.ReadAppFile("ViewModels", "MainViewModel.cs");
        var merge = MethodBetween(source, "private void MergeServers(",
                                  "private void ApplyServerChange");
        var apply = MethodBetween(source, "private void ApplyServerChange",
                                  "private void UpsertServer");

        AssertSynchronizesAfterStateUpdate(merge, "_serverState.Reconcile");
        AssertSynchronizesAfterStateUpdate(apply, "_serverState.Apply");
        Assert.Contains(
            "_backgroundTasks.SynchronizeServers(_serverState.Servers)", source,
            StringComparison.Ordinal);
    }

    [Fact]
    public void InitialServerSynchronizationKeepsCloseProtectionActive() {
        var source =
            RepositoryLayout.ReadAppFile("ViewModels", "MainViewModel.cs");
        var connect =
            MethodBetween(source, "public async Task ConnectAsync",
                          "public async Task<string[]> GetVersionsAsync");
        var merge = MethodBetween(source, "private void MergeServers(",
                                  "private void ApplyServerChange");

        AssertTracked(source, "public async Task ConnectAsync",
                      "public async Task<string[]> GetVersionsAsync",
                      "server-sync");
        Assert.Contains("await _initialServerSync.Task", connect,
                        StringComparison.Ordinal);
        var synchronize = merge.IndexOf("SynchronizeBackgroundTasks();",
                                        StringComparison.Ordinal);
        var ready       = merge.IndexOf("_initialServerSync.TrySetResult();",
                                        StringComparison.Ordinal);
        Assert.True(ready > synchronize,
                    "initial synchronization must complete only after " +
                        "persistent server activity is registered");
    }

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

    [Fact]
    public void LongRunningMutationsRegisterWithBackgroundTaskService() {
        var main =
            RepositoryLayout.ReadAppFile("ViewModels", "MainViewModel.cs");
        var backups =
            RepositoryLayout.ReadAppFile("ViewModels", "BackupsViewModel.cs");
        var settings =
            RepositoryLayout.ReadAppFile("ViewModels", "SettingsViewModel.cs");

        AssertTracked(main, "public async Task<bool> UpdateServerAsync",
                      "public Task<bool> RenameServerAsync", "server-update");
        AssertTracked(main, "private async Task DeleteServer",
                      "private void ApplyInstallProgress", "server-delete");
        AssertTracked(backups, "private async Task CreateBackup",
                      "private async Task Restore", "backup-create");
        AssertTracked(backups, "private async Task Restore",
                      "private async Task Delete", "backup-restore");
        AssertTracked(backups, "private async Task Delete",
                      "private static string MapBackupError", "backup-delete");
        AssertTracked(settings, "private async Task RemoveAllData",
                      "private async Task RemoveIncompleteInstallations",
                      "data-remove");
        AssertTracked(
            settings, "private async Task RemoveIncompleteInstallations",
            "private static string FormatSize", "incomplete-install-remove");
    }

    private static void AssertSynchronizesAfterStateUpdate(
        string method, string stateUpdateMarker) {
        var update =
            method.IndexOf(stateUpdateMarker, StringComparison.Ordinal);
        Assert.True(update >= 0, $"Missing state update: {stateUpdateMarker}");
        var synchronize = method.IndexOf("SynchronizeBackgroundTasks();",
                                         StringComparison.Ordinal);
        Assert.True(synchronize > update,
                    $"{stateUpdateMarker} must be followed by background " +
                        "server synchronization");
    }

    private static void AssertTracked(string source, string startMarker,
                                      string endMarker, string category) {
        var method = MethodBetween(source, startMarker, endMarker);
        var begin  = method.IndexOf($"BackgroundTasks.Begin(\"{category}\")",
                                    StringComparison.OrdinalIgnoreCase);
        Assert.True(begin >= 0, $"{startMarker} does not track {category}");
        var firstAwait = method.IndexOf("await ", StringComparison.Ordinal);
        Assert.True(firstAwait < 0 || begin < firstAwait,
                    $"{category} must be tracked before the first await");
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
