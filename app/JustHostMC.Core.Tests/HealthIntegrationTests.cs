using JustHostMC.Core;
using McManager.Grpc;
using Xunit;

namespace JustHostMC.Core.Tests;

/// <summary>
/// End-to-end M0 acceptance: the C# app launches the real Go engine, completes
/// the named-pipe handshake, and calls Health over the channel.
/// </summary>
public class HealthIntegrationTests
{
    private static Empty Empty => new();

    [Fact]
    public async Task StartAsync_ReportsPipeName()
    {
        await using var host = EngineFixture.NewHost();

        var connection = await host.StartAsync();

        Assert.False(string.IsNullOrWhiteSpace(connection.PipeName));
    }

    [Fact]
    public async Task StartAsync_CapturesReadyLineInStdioHistory()
    {
        await using var host = EngineFixture.NewHost();

        await host.StartAsync();

        Assert.Contains(host.GetStdioSnapshot(), entry =>
            entry.Stream == EngineStdioStream.StdOut
            && entry.Message == "MCMANAGER_READY");
        Assert.NotNull(host.ProcessId);
    }

    [Fact]
    public async Task Health_Succeeds()
    {
        await using var host = EngineFixture.NewHost();
        var connection = await host.StartAsync();
        await using var client = new DaemonClient(connection);

        var response = await client.Engine.HealthAsync(
            Empty, deadline: DateTime.UtcNow.AddSeconds(10));

        Assert.NotNull(response);
    }

    [Fact]
    public async Task StartAsync_Throws_WhenEngineMissing()
    {
        await using var host = new EngineHost(new EngineHostOptions
        {
            EnginePath = Path.Combine(AppContext.BaseDirectory, "no-such-engine.exe"),
        });

        await Assert.ThrowsAsync<FileNotFoundException>(() => host.StartAsync());
    }
}
