using System.Runtime.CompilerServices;
using Xunit;

namespace JustHostMC.Core.Tests;

public class MainWindowLifecycleTests {
    [Fact]
    public void OnClosed_CleansUpWindowBeforeAwaitingDisposal() {
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
        var asynchronousDisposal = method.IndexOf(
            "await Shell.Main.DisposeAsync();", StringComparison.Ordinal);

        Assert.True(lastWindowCleanup >= 0,
                    "The final window cleanup operation was not found.");
        Assert.True(
            asynchronousDisposal > lastWindowCleanup,
            "Window cleanup must complete before asynchronous disposal.");
    }

    private static string MainWindowSourcePath(
        [CallerFilePath] string testSourcePath = "") =>
        Path.GetFullPath(Path.Combine(Path.GetDirectoryName(testSourcePath)!,
                                      "..", "JustHostMC.App",
                                      "MainWindow.xaml.cs"));
}
