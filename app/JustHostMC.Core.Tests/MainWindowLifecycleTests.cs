using System.Runtime.CompilerServices;
using Xunit;

namespace JustHostMC.Core.Tests;

public class MainWindowLifecycleTests {
    [Fact]
    public void OnClosed_CleansUpWindowBeforeAwaitingShellDisposal() {
        var source      = File.ReadAllText(MainWindowSourcePath());
        var methodStart = source.IndexOf("private async void OnClosed",
                                         StringComparison.Ordinal);
        var methodEnd   = source.IndexOf("\n    private ", methodStart + 1,
                                         StringComparison.Ordinal);

        Assert.True(methodStart >= 0, "OnClosed method was not found.");
        Assert.True(methodEnd > methodStart,
                    "OnClosed method end was not found.");

        var method = source[methodStart..methodEnd];
        var lastWindowCleanup =
            method.IndexOf("UntrackServer(server);", StringComparison.Ordinal);
        var asynchronousDisposal = method.IndexOf("await Shell.DisposeAsync();",
                                                  StringComparison.Ordinal);

        Assert.True(lastWindowCleanup >= 0,
                    "The final window cleanup operation was not found.");
        Assert.True(
            asynchronousDisposal > lastWindowCleanup,
            "Window cleanup must complete before asynchronous disposal.");
    }

    [Fact]
    public void NavShell_DisposeAsync_ClearsCacheBeforeStartingAllDisposals() {
        var source = File.ReadAllText(NavShellSourcePath());
        Assert.Contains("IAsyncDisposable", source, StringComparison.Ordinal);

        var methodStart = source.IndexOf(
            "public async ValueTask DisposeAsync()", StringComparison.Ordinal);
        var methodEnd =
            source.IndexOf("\n    }\n}", methodStart, StringComparison.Ordinal);

        Assert.True(methodStart >= 0,
                    "NavShell DisposeAsync method was not found.");
        Assert.True(methodEnd > methodStart,
                    "NavShell DisposeAsync method end was not found.");

        var method   = source[methodStart..methodEnd];
        var snapshot = method.IndexOf("_serverVmCache.Values.ToArray()",
                                      StringComparison.Ordinal);
        var clear =
            method.IndexOf("_serverVmCache.Clear();", StringComparison.Ordinal);
        var cacheDisposal = method.IndexOf("cache.DisposeAsync().AsTask()",
                                           StringComparison.Ordinal);
        var mainDisposal  = method.IndexOf("Main.DisposeAsync().AsTask()",
                                           StringComparison.Ordinal);
        var awaitAll =
            method.IndexOf("await Task.WhenAll", StringComparison.Ordinal);

        Assert.True(snapshot >= 0,
                    "Cache values must be snapshotted for disposal.");
        Assert.True(
            clear > snapshot,
            "The cache must be cleared immediately after snapshotting.");
        Assert.True(
            cacheDisposal > clear,
            "Cached view models must be disposed after the cache is cleared.");
        Assert.True(
            mainDisposal > cacheDisposal,
            "MainViewModel disposal must start after cached disposal starts.");
        Assert.True(
            awaitAll > mainDisposal,
            "Every disposal must start before the method awaits completion.");
    }

    private static string MainWindowSourcePath(
        [CallerFilePath] string testSourcePath = "") =>
        Path.GetFullPath(Path.Combine(Path.GetDirectoryName(testSourcePath)!,
                                      "..", "JustHostMC.App",
                                      "MainWindow.xaml.cs"));

    private static string NavShellSourcePath(
        [CallerFilePath] string testSourcePath = "") =>
        Path.GetFullPath(Path.Combine(Path.GetDirectoryName(testSourcePath)!,
                                      "..", "JustHostMC.App", "ViewModels",
                                      "NavShellViewModel.cs"));
}
