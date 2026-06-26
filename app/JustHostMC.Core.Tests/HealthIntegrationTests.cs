using Grpc.Core;
using JustHostMC.Core;
using McManager.Grpc;
using Xunit;

namespace JustHostMC.Core.Tests;

/// <summary>
/// End-to-end M0 acceptance: the C# app launches the real Go engine, completes
/// the port handshake, and calls Health over the token-authenticated channel.
/// </summary>
public class HealthIntegrationTests
{
    private static Empty Empty => new();

    [Fact]
    public async Task StartAsync_ReportsLoopbackPort()
    {
        await using var host = EngineFixture.NewHost();

        var connection = await host.StartAsync();

        Assert.InRange(connection.Port, 1, 65535);
        Assert.False(string.IsNullOrWhiteSpace(connection.Token));
    }

    [Fact]
    public async Task Health_Succeeds_WithValidToken()
    {
        await using var host = EngineFixture.NewHost();
        var connection = await host.StartAsync();
        await using var client = new DaemonClient(connection);

        var response = await client.Engine.HealthAsync(
            Empty, deadline: DateTime.UtcNow.AddSeconds(10));

        Assert.NotNull(response);
    }

    [Fact]
    public async Task Health_Rejected_WithWrongToken()
    {
        await using var host = EngineFixture.NewHost();
        var connection = await host.StartAsync();
        await using var client = new DaemonClient(connection.Port, "wrong-token");

        var ex = await Assert.ThrowsAsync<RpcException>(async () =>
            await client.Engine.HealthAsync(Empty, deadline: DateTime.UtcNow.AddSeconds(10)));

        Assert.Equal(StatusCode.Unauthenticated, ex.StatusCode);
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
