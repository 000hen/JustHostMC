using JustHostMC.Core;
using Xunit;

namespace JustHostMC.Core.Tests;

public class EngineStdioEntryTests {
    [Theory]
    [InlineData("[DEBUG] grpc unary started", EngineDiagnosticLevel.Debug)]
    [InlineData("[INFO] grpc unary completed",
                EngineDiagnosticLevel.Information)]
    [InlineData("[WARN] grpc unary failed", EngineDiagnosticLevel.Warning)]
    [InlineData("[ERROR] grpc unary failed", EngineDiagnosticLevel.Error)]
    [InlineData("[FATAL] serve: broken pipe", EngineDiagnosticLevel.Error)]
    [InlineData("legacy untagged engine output",
                EngineDiagnosticLevel.Information)]
    public void Level_ParsesEngineSeverity(string message,
                                           EngineDiagnosticLevel expected) {
        var entry = new EngineStdioEntry(1, DateTimeOffset.UtcNow,
                                         EngineStdioStream.StdErr, message);

        Assert.Equal(expected, entry.Level);
    }

    [Fact]
    public void Level_TreatsParentLifecycleMarkerAsDebug() {
        var entry = new EngineStdioEntry(
            1, DateTimeOffset.UtcNow, EngineStdioStream.StdIn,
            "[open] Parent lifecycle watchdog connected.");

        Assert.Equal(EngineDiagnosticLevel.Debug, entry.Level);
    }

    [Theory]
    [InlineData("2026/07/07 14:25:37.123456 [INFO] grpc unary completed",
                "grpc unary completed")]
    [InlineData("2026/07/07 14:25:37.123456 [ERROR] registry write failed",
                "registry write failed")]
    [InlineData("MCMANAGER_READY", "MCMANAGER_READY")]
    public void DisplayMessage_RemovesDuplicateGoLogPrefix(string raw,
                                                           string expected) {
        var entry = new EngineStdioEntry(1, DateTimeOffset.UtcNow,
                                         EngineStdioStream.StdErr, raw);

        Assert.Equal(expected, entry.DisplayMessage);
    }
}
