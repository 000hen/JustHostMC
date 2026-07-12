using Grpc.Core;
using McManager.Grpc;
using Xunit;

namespace JustHostMC.Core.Tests;

public class ServerChangesIntegrationTests {
    [Fact]
    public async Task WatchChanges_StartsReadyAndDoesNotReplayInitialList() {
        await using var host = EngineFixture.NewHost();
        var connection = await host.StartAsync();
        await using var client = new DaemonClient(connection);
        using var call = client.Servers.WatchChanges(new Empty());
        using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(5));

        Assert.True(await call.ResponseStream.MoveNext(timeout.Token));
        var ready = call.ResponseStream.Current;
        Assert.Equal(ServerChangeEvent.ChangeOneofCase.Ready,
                     ready.ChangeCase);

        var initial = await client.Servers.ListAsync(
            new Empty(), deadline: DateTime.UtcNow.AddSeconds(5),
            cancellationToken: timeout.Token);
        Assert.NotNull(initial);

        using var noReplay =
            new CancellationTokenSource(TimeSpan.FromMilliseconds(250));
        await Assert.ThrowsAnyAsync<Exception>(async () =>
                                                   await call.ResponseStream
                                                       .MoveNext(noReplay.Token));
    }
}
