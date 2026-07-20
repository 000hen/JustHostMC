using System.Runtime.CompilerServices;
using Xunit;

namespace JustHostMC.Core.Tests;

public class SparklineLifecycleTests {
    [Fact]
    public void CollectionSubscription_IsScopedToLoadedLifetime() {
        var source = File.ReadAllText(SparklineSourcePath());

        Assert.Contains("Loaded += OnLoaded;", source,
                        StringComparison.Ordinal);
        Assert.Contains("Unloaded += OnUnloaded;", source,
                        StringComparison.Ordinal);
        Assert.Contains("private void AttachSeries()", source,
                        StringComparison.Ordinal);
        Assert.Contains("private void DetachSeries()", source,
                        StringComparison.Ordinal);

        var unloaded = MethodBody(source, "private void OnUnloaded");
        Assert.Contains("_isLoaded = false;", unloaded,
                        StringComparison.Ordinal);
        Assert.Contains("DetachSeries();", unloaded, StringComparison.Ordinal);
    }

    [Fact]
    public void Redraw_ReturnsBeforeTouchingVisualWhenUnloaded() {
        var source = File.ReadAllText(SparklineSourcePath());
        var redraw = MethodBody(source, "private void Redraw");

        var loadedGuard =
            redraw.IndexOf("if (!_isLoaded)", StringComparison.Ordinal);
        var visualAccess =
            redraw.IndexOf("Line.Points", StringComparison.Ordinal);

        Assert.True(loadedGuard >= 0,
                    "Redraw must guard against an unloaded control.");
        Assert.True(
            visualAccess > loadedGuard,
            "The loaded-state guard must run before native visual access.");
    }

    private static string MethodBody(string source, string signature) {
        var methodStart = source.IndexOf(signature, StringComparison.Ordinal);
        var methodEnd   = source.IndexOf("\n    private ", methodStart + 1,
                                         StringComparison.Ordinal);
        if (methodEnd < 0)
            methodEnd = source.LastIndexOf("\n}", StringComparison.Ordinal);

        Assert.True(methodStart >= 0, $"{signature} was not found.");
        Assert.True(methodEnd > methodStart,
                    $"The end of {signature} was not found.");
        return source[methodStart..methodEnd];
    }

    private static string SparklineSourcePath(
        [CallerFilePath] string testSourcePath = "") =>
        Path.GetFullPath(Path.Combine(Path.GetDirectoryName(testSourcePath)!,
                                      "..", "JustHostMC.App", "Controls",
                                      "Sparkline.xaml.cs"));
}
